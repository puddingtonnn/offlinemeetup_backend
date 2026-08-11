package service

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/cache"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
	repomocks "github.com/puddingtonnn/offlinemeetup_backend/internal/repo/mocks"
	svcmocks "github.com/puddingtonnn/offlinemeetup_backend/internal/service/mocks"
)

// discardLog is the logger every AuthService test uses (also referenced from
// auth_test.go).
func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

// --- fake mailer -----------------------------------------------------------

type sentMail struct {
	to      string
	subject string
	body    string
}

// fakeMailer records what the background send goroutine produced. The channel
// (not a polled slice) is what gives the test a happens-before edge with that
// goroutine — a bare slice read would be a data race under -race.
type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
	ch   chan sentMail
}

func newFakeMailer() *fakeMailer {
	return &fakeMailer{ch: make(chan sentMail, 16)}
}

func (m *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	msg := sentMail{to: to, subject: subject, body: body}
	m.mu.Lock()
	m.sent = append(m.sent, msg)
	m.mu.Unlock()
	m.ch <- msg
	return nil
}

// wait blocks until the background goroutine has delivered one email.
func (m *fakeMailer) wait(t *testing.T) sentMail {
	t.Helper()
	select {
	case msg := <-m.ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("expected an email to be sent, none arrived")
		return sentMail{}
	}
}

// assertNoMail asserts nothing was sent within a short grace period.
func (m *fakeMailer) assertNoMail(t *testing.T) {
	t.Helper()
	select {
	case msg := <-m.ch:
		t.Fatalf("expected no email, got %q to %s", msg.subject, msg.to)
	case <-time.After(150 * time.Millisecond):
	}
}

var codeInBody = regexp.MustCompile(`\b\d{6}\b`)

func codeFrom(t *testing.T, msg sentMail) string {
	t.Helper()
	found := codeInBody.FindString(msg.body)
	require.NotEmpty(t, found, "no 6-digit code in email body: %s", msg.body)
	return found
}

// --- fixture ---------------------------------------------------------------

type authPwdFixture struct {
	mr       *miniredis.Miniredis
	svc      *AuthService
	authRepo *svcmocks.MockAuthRepository
	repo     *repomocks.MockPasswordUserRepository
	credRepo *repomocks.MockCredentialsRepository
	refresh  *svcmocks.MockRefreshTokenRepository
	store    *cache.RedisAuthStore
	mailer   *fakeMailer
	cfg      *config.Config
}

func setupAuthPasswordTest(t *testing.T) *authPwdFixture {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctrl := gomock.NewController(t)
	authRepo := svcmocks.NewMockAuthRepository(ctrl)
	pwdRepo := repomocks.NewMockPasswordUserRepository(ctrl)
	credRepo := repomocks.NewMockCredentialsRepository(ctrl)
	refreshRepo := svcmocks.NewMockRefreshTokenRepository(ctrl)

	log := discardLog()
	store := cache.NewRedisAuthStore(rdb, log)
	mailer := newFakeMailer()

	cfg := &config.Config{
		JWTSecret:             "test-secret",
		JWTAccessTTL:          15 * time.Minute,
		JWTRefreshTTL:         24 * time.Hour,
		MailSendTimeout:       5 * time.Second,
		EmailCodeTTL:          15 * time.Minute,
		EmailCodeMaxAttempts:  3,
		EmailResendCooldown:   60 * time.Second,
		EmailSendQuotaPerHour: 5,
	}

	svc := NewAuthService(authRepo, pwdRepo, credRepo, refreshRepo, store, mailer, nil, cfg, log)

	return &authPwdFixture{
		mr:       mr,
		svc:      svc,
		authRepo: authRepo,
		repo:     pwdRepo,
		credRepo: credRepo,
		refresh:  refreshRepo,
		store:    store,
		mailer:   mailer,
		cfg:      cfg,
	}
}

const (
	testEmail     = "New.User@Example.COM "
	testNormEmail = "new.user@example.com"
	testUsername  = "newuser"
	testPassword  = "correct horse battery"
)

// --- Register --------------------------------------------------------------

func TestAuthService_Register_FreeEmail(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	f.repo.EXPECT().FindIDByUsername(ctx, testUsername).Return(int64(0), repo.ErrNotFound)
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)
	// No user is created at register time (ADR-8): any Create/Attach call would
	// fail the gomock controller as an unexpected call.

	require.NoError(t, f.svc.Register(ctx, testEmail, testUsername, testPassword))

	pending, found, err := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, err)
	require.True(t, found, "pending registration must be saved under the normalized email")
	assert.Equal(t, testUsername, pending.Username)
	assert.Zero(t, pending.Attempts)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(pending.PasswordHash), []byte(testPassword)))
	cost, err := bcrypt.Cost([]byte(pending.PasswordHash))
	require.NoError(t, err)
	assert.Equal(t, 12, cost, "ADR-12: bcrypt cost 12")

	msg := f.mailer.wait(t)
	assert.Equal(t, testNormEmail, msg.to)
	assert.Contains(t, msg.subject, "Confirm your Meetuper registration")
	code := codeFrom(t, msg)
	// Only the hash is stored; the plaintext code never touches Redis.
	assert.NotContains(t, pending.CodeHash, code)
	assert.Equal(t, hashCode(code), pending.CodeHash)
}

// failingMailer always errors, to exercise sendMailAsync's mail.Metrics
// increment on a Mailer.Send failure.
type failingMailer struct{}

func (failingMailer) Send(context.Context, string, string, string) error {
	return errors.New("smtp: connection refused")
}

// countingMailMetrics is a mail.Metrics test double that signals over a
// channel (not a polled counter) so the test gets a happens-before edge with
// the background send goroutine under -race.
type countingMailMetrics struct {
	ch chan struct{}
}

func newCountingMailMetrics() *countingMailMetrics {
	return &countingMailMetrics{ch: make(chan struct{}, 16)}
}

func (m *countingMailMetrics) SendFailure() { m.ch <- struct{}{} }

func (m *countingMailMetrics) wait(t *testing.T) {
	t.Helper()
	select {
	case <-m.ch:
	case <-time.After(2 * time.Second):
		t.Fatal("expected mail send failure metric to be incremented, none arrived")
	}
}

// TestSendMailAsync_IncrementsFailureMetricOnSendError confirms the
// background mail-send goroutine (ADR-11) increments mail.Metrics.SendFailure
// when Mailer.Send returns an error — the signal ADR-11's silent-by-default
// backgrounding relies on for ops visibility (see task-7 brief).
func TestSendMailAsync_IncrementsFailureMetricOnSendError(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	metrics := newCountingMailMetrics()
	f.svc.mailer = failingMailer{}
	f.svc.mailMetrics = metrics

	f.repo.EXPECT().FindIDByUsername(ctx, testUsername).Return(int64(0), repo.ErrNotFound)
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)

	require.NoError(t, f.svc.Register(ctx, testEmail, testUsername, testPassword))

	metrics.wait(t)
}

func TestAuthService_Register_ExistingEmailIsIndistinguishable(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	f.repo.EXPECT().FindIDByUsername(ctx, testUsername).Return(int64(0), repo.ErrNotFound)
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(42), nil)

	// ADR-7: same nil error (→ same 202) as a fresh registration...
	require.NoError(t, f.svc.Register(ctx, testEmail, testUsername, testPassword))

	_, found, err := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, err)
	assert.True(t, found)

	// ...only the email template differs.
	msg := f.mailer.wait(t)
	assert.Contains(t, msg.subject, "Someone tried to register with your Meetuper email")
}

func TestAuthService_Register_UsernameTakenIsFast400(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	f.repo.EXPECT().FindIDByUsername(ctx, testUsername).Return(int64(7), nil)

	err := f.svc.Register(ctx, testEmail, testUsername, testPassword)
	assert.ErrorIs(t, err, ErrInvalidInput)

	_, found, storeErr := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, storeErr)
	assert.False(t, found, "no pending registration for a doomed username")
	f.mailer.assertNoMail(t)
}

func TestAuthService_Register_RejectsShortPassword(t *testing.T) {
	f := setupAuthPasswordTest(t)
	// 7 bytes — below the 8-byte floor (ADR-12).
	err := f.svc.Register(context.Background(), testEmail, testUsername, "short12")
	assert.ErrorIs(t, err, ErrInvalidInput)
	f.mailer.assertNoMail(t)
}

func TestAuthService_Register_RepeatOverwritesPending(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	f.repo.EXPECT().FindIDByUsername(ctx, testUsername).Return(int64(0), repo.ErrNotFound)
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)
	require.NoError(t, f.svc.Register(ctx, testEmail, testUsername, testPassword))
	first, _, err := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, err)
	f.mailer.wait(t)

	// ADR-8: a second register on the same email just overwrites the object —
	// new username, new password hash, new code, no DB cleanup involved.
	f.repo.EXPECT().FindIDByUsername(ctx, "seconduser").Return(int64(0), repo.ErrNotFound)
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)
	require.NoError(t, f.svc.Register(ctx, testEmail, "seconduser", "another good password"))
	second, found, err := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, err)
	require.True(t, found)

	assert.Equal(t, "seconduser", second.Username)
	assert.NotEqual(t, first.CodeHash, second.CodeHash, "the old code must be dead")
	assert.NotEqual(t, first.PasswordHash, second.PasswordHash)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(second.PasswordHash), []byte("another good password")))

	// The first code no longer verifies.
	f.mailer.wait(t)
}

// exhaustMailQuota burns the whole hourly send allowance for an email.
func exhaustMailQuota(t *testing.T, f *authPwdFixture, email string) {
	t.Helper()
	for i := 0; i < f.cfg.EmailSendQuotaPerHour; i++ {
		_, err := f.store.IncrementMailQuota(context.Background(), email, time.Hour)
		require.NoError(t, err)
	}
}

func TestAuthService_Register_OverQuotaSucceedsWithoutSending(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	exhaustMailQuota(t, f, testNormEmail)

	f.repo.EXPECT().FindIDByUsername(ctx, testUsername).Return(int64(0), repo.ErrNotFound)
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)

	// Over the hourly quota the send is suppressed, but the response shape must
	// not change (the API table has no 429 on register, and a different answer
	// here would be an inbox-occupancy oracle). Still a plain success → 202.
	require.NoError(t, f.svc.Register(ctx, testEmail, testUsername, testPassword))
	f.mailer.assertNoMail(t)

	// The pending object is still written — a resend (once the hour rolls over)
	// can revive the registration without starting from scratch.
	_, found, err := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, err)
	assert.True(t, found)
}

// --- VerifyEmail -----------------------------------------------------------

// seedPending puts a pending registration in Redis and returns the plaintext
// code, standing in for "the user received the email".
func seedPending(t *testing.T, f *authPwdFixture, username string) string {
	t.Helper()
	code, err := generateCode()
	require.NoError(t, err)
	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	require.NoError(t, err)
	require.NoError(t, f.store.SavePendingReg(context.Background(), testNormEmail, cache.PendingReg{
		PasswordHash: string(hash),
		CodeHash:     hashCode(code),
		Username:     username,
	}, f.cfg.EmailCodeTTL))
	return code
}

func TestAuthService_VerifyEmail_CreatesUserAndIssuesTokens(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	code := seedPending(t, f, testUsername)

	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)
	f.repo.EXPECT().
		CreateUserWithPassword(ctx, testNormEmail, testUsername, gomock.Any()).
		Return(int64(99), nil)
	f.refresh.EXPECT().Create(ctx, gomock.Any()).Return(nil)

	tokens, err := f.svc.VerifyEmail(ctx, testEmail, code, nil)
	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.NotEmpty(t, tokens.AccessToken)
	assert.NotEmpty(t, tokens.RefreshToken)

	_, found, err := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, err)
	assert.False(t, found, "a spent pending registration is deleted")
}

func TestAuthService_VerifyEmail_ExistingAccountGetsCredentials(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	code := seedPending(t, f, testUsername)

	// ADR-7: the email already belongs to user 42 (created via Google). Verify
	// must attach a password to THAT user — no second user, no second profile,
	// and the existing profile's username is left alone.
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(42), nil)
	f.repo.EXPECT().AttachPassword(ctx, int64(42), gomock.Any()).Return(nil)
	f.refresh.EXPECT().RevokeAllForUser(ctx, int64(42)).Return(nil)
	f.refresh.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, rt any) error {
		return nil
	})

	tokens, err := f.svc.VerifyEmail(ctx, testEmail, code, nil)
	require.NoError(t, err)
	require.NotNil(t, tokens)
}

// The attach branch is an upsert, so it also covers "this email already had an
// email/password account and the password is being REPLACED". Overwriting a
// password hash must kill every existing session — otherwise a refresh token
// stolen under the old password keeps working.
func TestAuthService_VerifyEmail_PasswordOverwriteRevokesAllSessions(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	code := seedPending(t, f, testUsername)

	const existingID = int64(42)

	// gomock.InOrder pins the ordering that makes this safe: the revoke happens
	// BEFORE the new refresh token is minted, so the caller's own fresh session
	// isn't swept away by its own revoke.
	gomock.InOrder(
		f.repo.EXPECT().AttachPassword(ctx, existingID, gomock.Any()).Return(nil),
		f.refresh.EXPECT().RevokeAllForUser(ctx, existingID).Return(nil),
		f.refresh.EXPECT().Create(ctx, gomock.Any()).Return(nil),
	)
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(existingID, nil)

	tokens, err := f.svc.VerifyEmail(ctx, testEmail, code, nil)
	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.NotEmpty(t, tokens.RefreshToken, "the caller still gets a working session")
}

// If the sessions can't be revoked, the verify must fail rather than quietly
// leave a stolen session alive under a password the attacker no longer knows.
func TestAuthService_VerifyEmail_RevokeFailureFailsVerify(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	code := seedPending(t, f, testUsername)

	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(42), nil)
	f.repo.EXPECT().AttachPassword(ctx, int64(42), gomock.Any()).Return(nil)
	f.refresh.EXPECT().RevokeAllForUser(ctx, int64(42)).Return(errors.New("db down"))
	// No Create expectation: no token pair may be issued.

	tokens, err := f.svc.VerifyEmail(ctx, testEmail, code, nil)
	assert.Nil(t, tokens)
	require.Error(t, err)

	// The pending object survives so the user can simply retry once the DB is back.
	_, found, storeErr := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, storeErr)
	assert.True(t, found)
}

func TestAuthService_VerifyEmail_NoPendingIs400(t *testing.T) {
	f := setupAuthPasswordTest(t)
	tokens, err := f.svc.VerifyEmail(context.Background(), testEmail, "123456", nil)
	assert.Nil(t, tokens)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestAuthService_VerifyEmail_ExpiredPendingIs400(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	code := seedPending(t, f, testUsername)

	// Let the 15-minute TTL lapse (ADR-8) rather than deleting the key — this
	// is the "user came back tomorrow with a valid-looking code" case.
	f.mr.FastForward(f.cfg.EmailCodeTTL + time.Minute)

	_, found, err := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, err)
	require.False(t, found)

	tokens, err := f.svc.VerifyEmail(ctx, testEmail, code, nil)
	assert.Nil(t, tokens)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestAuthService_VerifyEmail_WrongCodeExhaustsAttempts(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	code := seedPending(t, f, testUsername)

	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}

	// EmailCodeMaxAttempts is 3 in the fixture: two wrong codes are plain 400s.
	for i := 0; i < f.cfg.EmailCodeMaxAttempts-1; i++ {
		_, err := f.svc.VerifyEmail(ctx, testEmail, wrong, nil)
		assert.ErrorIs(t, err, ErrInvalidInput, "attempt %d", i+1)

		pending, found, storeErr := f.store.GetPendingReg(ctx, testNormEmail)
		require.NoError(t, storeErr)
		require.True(t, found, "pending survives a merely-wrong code")
		assert.Equal(t, i+1, pending.Attempts)
	}

	// The last one burns the allowance: 429 and the pending object is gone.
	_, err := f.svc.VerifyEmail(ctx, testEmail, wrong, nil)
	assert.ErrorIs(t, err, ErrTooManyRequests)

	_, found, storeErr := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, storeErr)
	assert.False(t, found, "pending is invalidated once attempts are exhausted")

	// Even the RIGHT code no longer works — the registration must be restarted.
	_, err = f.svc.VerifyEmail(ctx, testEmail, code, nil)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestAuthService_VerifyEmail_UsernameRaceKeepsPending(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	code := seedPending(t, f, testUsername)

	// ADR-9: someone took the username between register and verify. The unique
	// index inside the transaction is the arbiter.
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)
	f.repo.EXPECT().
		CreateUserWithPassword(ctx, testNormEmail, testUsername, gomock.Any()).
		Return(int64(0), repo.ErrUsernameTaken)

	tokens, err := f.svc.VerifyEmail(ctx, testEmail, code, nil)
	assert.Nil(t, tokens)
	assert.ErrorIs(t, err, ErrAlreadyExists, "→ 409")

	pending, found, storeErr := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, storeErr)
	require.True(t, found, "ADR-9: the pending object survives a username conflict")
	assert.Zero(t, pending.Attempts, "a username conflict is not a wrong-code attempt")

	// The caller retries verify with a different username and the SAME code.
	other := "otheruser"
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)
	f.repo.EXPECT().
		CreateUserWithPassword(ctx, testNormEmail, other, gomock.Any()).
		Return(int64(101), nil)
	f.refresh.EXPECT().Create(ctx, gomock.Any()).Return(nil)

	tokens, err = f.svc.VerifyEmail(ctx, testEmail, code, &other)
	require.NoError(t, err)
	require.NotNil(t, tokens)

	_, found, storeErr = f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, storeErr)
	assert.False(t, found)
}

// --- ResendCode ------------------------------------------------------------

func TestAuthService_ResendCode_RotatesCodeAndResetsAttempts(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	oldCode := seedPending(t, f, testUsername)

	// Burn one wrong attempt so we can prove the counter resets.
	_, err := f.svc.VerifyEmail(ctx, testEmail, "000000", nil)
	assert.ErrorIs(t, err, ErrInvalidInput)

	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)
	require.NoError(t, f.svc.ResendCode(ctx, testEmail))

	msg := f.mailer.wait(t)
	newCode := codeFrom(t, msg)

	pending, found, err := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, err)
	require.True(t, found)
	assert.Zero(t, pending.Attempts)
	assert.Equal(t, hashCode(newCode), pending.CodeHash)
	assert.NotEqual(t, hashCode(oldCode), pending.CodeHash)
}

func TestAuthService_ResendCode_NoPendingStillSucceeds(t *testing.T) {
	f := setupAuthPasswordTest(t)

	// Anti-enumeration: no pending registration is NOT an error, and no email
	// is sent. The caller cannot tell this apart from a real resend.
	require.NoError(t, f.svc.ResendCode(context.Background(), "stranger@example.com"))
	f.mailer.assertNoMail(t)
}

func TestAuthService_ResendCode_CooldownIs429(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	seedPending(t, f, testUsername)

	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)
	require.NoError(t, f.svc.ResendCode(ctx, testEmail))
	f.mailer.wait(t)

	// Double-click: the cooldown is claimed atomically, so no second email.
	err := f.svc.ResendCode(ctx, testEmail)
	assert.ErrorIs(t, err, ErrTooManyRequests)
	f.mailer.assertNoMail(t)
}

func TestAuthService_ResendCode_QuotaExceededIs429(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	oldCode := seedPending(t, f, testUsername)

	// The hourly allowance is already spent (e.g. by earlier resends). The
	// cooldown is untouched, so this call gets past it and is rejected by the
	// quota specifically.
	exhaustMailQuota(t, f, testNormEmail)

	err := f.svc.ResendCode(ctx, testEmail)
	assert.ErrorIs(t, err, ErrTooManyRequests)
	f.mailer.assertNoMail(t)

	// Rejected before the pending object is touched: the code the user already
	// has in their inbox must stay valid.
	pending, found, storeErr := f.store.GetPendingReg(ctx, testNormEmail)
	require.NoError(t, storeErr)
	require.True(t, found)
	assert.Equal(t, hashCode(oldCode), pending.CodeHash)
}

// --- ForgotPassword ----------------------------------------------------------

func TestAuthService_ForgotPassword_UnknownLogin_SucceedsWithoutMail(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	// Login typed as an email: only FindIDByEmail should be consulted, and no
	// GetByID/cooldown/quota/pending-save work happens for an unknown login.
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)

	err := f.svc.ForgotPassword(ctx, testEmail)
	assert.NoError(t, err, "unknown login must still report success (anti-enumeration)")
	f.mailer.assertNoMail(t)

	_, found, storeErr := f.store.GetPendingReset(ctx, testNormEmail)
	require.NoError(t, storeErr)
	assert.False(t, found)
}

func TestAuthService_ForgotPassword_KnownLogin_SendsMailAndSavesPendingReset(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	const userID = int64(42)
	f.repo.EXPECT().FindIDByUsername(ctx, testUsername).Return(userID, nil)
	f.authRepo.EXPECT().GetByID(gomock.Any(), userID).Return(&domain.User{ID: userID, Email: testNormEmail}, nil)

	err := f.svc.ForgotPassword(ctx, testUsername)
	require.NoError(t, err)

	msg := f.mailer.wait(t)
	assert.Equal(t, testNormEmail, msg.to)
	assert.Contains(t, msg.subject, "Reset your Meetuper password")
	code := codeFrom(t, msg)

	pending, found, storeErr := f.store.GetPendingReset(ctx, testNormEmail)
	require.NoError(t, storeErr)
	require.True(t, found)
	assert.Equal(t, hashCode(code), pending.CodeHash)
}

// Same external nil-ness/error-shape for the found and not-found cases — the
// two must be indistinguishable to the caller (both return nil, i.e. 202).
func TestAuthService_ForgotPassword_SameExternalOutcomeBothCases(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(int64(0), repo.ErrNotFound)
	errUnknown := f.svc.ForgotPassword(ctx, testEmail)

	const userID = int64(7)
	f.repo.EXPECT().FindIDByEmail(ctx, "known@example.com").Return(userID, nil)
	f.authRepo.EXPECT().GetByID(gomock.Any(), userID).Return(&domain.User{ID: userID, Email: "known@example.com"}, nil)
	errKnown := f.svc.ForgotPassword(ctx, "known@example.com")

	assert.NoError(t, errUnknown)
	assert.NoError(t, errKnown)
	assert.Equal(t, errUnknown, errKnown, "both outcomes must be byte-identical (both nil)")
	f.mailer.wait(t) // only the known-login path actually sent mail
}

func TestAuthService_ForgotPassword_CooldownActive_SilentlySkipsSend(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	const userID = int64(42)
	secondGetByIDDone := make(chan struct{})

	f.repo.EXPECT().FindIDByUsername(ctx, testUsername).Return(userID, nil).Times(2)
	first := f.authRepo.EXPECT().GetByID(gomock.Any(), userID).Return(&domain.User{ID: userID, Email: testNormEmail}, nil)
	f.authRepo.EXPECT().GetByID(gomock.Any(), userID).DoAndReturn(func(_ context.Context, _ int64) (*domain.User, error) {
		defer close(secondGetByIDDone)
		return &domain.User{ID: userID, Email: testNormEmail}, nil
	}).After(first)

	require.NoError(t, f.svc.ForgotPassword(ctx, testUsername))
	f.mailer.wait(t)

	// Second call within the cooldown window: still nil (202), but no second
	// email and the pending code from the first call survives untouched. The
	// found-path work now runs in a background goroutine (Critical #2 fix),
	// so we synchronize on the second call's GetByID actually having run
	// before asserting "no mail" — otherwise this assertion could pass
	// trivially just because the goroutine hadn't started yet.
	pendingBefore, _, err := f.store.GetPendingReset(ctx, testNormEmail)
	require.NoError(t, err)

	err = f.svc.ForgotPassword(ctx, testUsername)
	assert.NoError(t, err, "a cooldown hit must not change the external always-202 contract")

	select {
	case <-secondGetByIDDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expected the second call's background goroutine to reach GetByID")
	}
	// GetByID completing doesn't itself guarantee the very next step (the
	// cooldown check, same goroutine) has also finished — it's a single
	// in-process miniredis round trip, not real I/O, so a short grace
	// window is enough without resorting to a raw fixed sleep as the ONLY
	// synchronization (which the GetByID rendezvous above already avoids).
	time.Sleep(50 * time.Millisecond)

	f.mailer.assertNoMail(t)

	pendingAfter, found, err := f.store.GetPendingReset(ctx, testNormEmail)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, pendingBefore.CodeHash, pendingAfter.CodeHash)
}

// ForgotPassword's found-path work (GetByID, cooldown/quota, code
// generation, SavePendingReset, send) must run in the background, not
// before the function returns — otherwise the found/not-found timing
// difference is a measurable account-enumeration oracle (Critical #2: unlike
// Login, nothing in this flow pays a bcrypt cost, so there's no floor to
// hide extra round trips under). This proves it structurally: GetByID is
// blocked until the test releases it, and ForgotPassword must still return
// well before that.
func TestAuthService_ForgotPassword_FoundPathWorkIsAsynchronous(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	const userID = int64(42)
	release := make(chan struct{})
	getByIDCalled := make(chan struct{})

	f.repo.EXPECT().FindIDByUsername(ctx, testUsername).Return(userID, nil)
	f.authRepo.EXPECT().GetByID(gomock.Any(), userID).DoAndReturn(func(_ context.Context, _ int64) (*domain.User, error) {
		close(getByIDCalled)
		<-release
		return &domain.User{ID: userID, Email: testNormEmail}, nil
	})

	start := time.Now()
	err := f.svc.ForgotPassword(ctx, testUsername)
	elapsed := time.Since(start)
	require.NoError(t, err)
	assert.Less(t, elapsed, 200*time.Millisecond,
		"ForgotPassword must return without waiting on the background GetByID call")

	select {
	case <-getByIDCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("background goroutine never called GetByID — found-path work did not run at all")
	}

	close(release)
	f.mailer.wait(t) // background work eventually completes and sends mail
}

// A Telegram-only account (findOrCreateUser creates these with Email: "")
// hitting ForgotPassword must not error, must not send mail, and must not
// touch a shared ""-keyed cooldown/quota/pending namespace across every
// emailless user (Important #3).
func TestAuthService_ForgotPassword_EmaillessUser_SilentNoOp(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	const userID = int64(55)
	getByIDDone := make(chan struct{})
	f.repo.EXPECT().FindIDByUsername(ctx, testUsername).Return(userID, nil)
	f.authRepo.EXPECT().GetByID(gomock.Any(), userID).DoAndReturn(func(_ context.Context, _ int64) (*domain.User, error) {
		defer close(getByIDDone)
		return &domain.User{ID: userID, Email: ""}, nil
	})

	err := f.svc.ForgotPassword(ctx, testUsername)
	assert.NoError(t, err, "a Telegram-only account must still report plain success")

	select {
	case <-getByIDDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expected the background goroutine to reach GetByID")
	}
	f.mailer.assertNoMail(t)
}

// --- ResetPassword -----------------------------------------------------------

func seedPendingReset(t *testing.T, f *authPwdFixture) string {
	t.Helper()
	code, err := generateCode()
	require.NoError(t, err)
	require.NoError(t, f.store.SavePendingReset(context.Background(), testNormEmail, cache.PendingReg{
		CodeHash: hashCode(code),
	}, f.cfg.EmailCodeTTL))
	return code
}

func TestAuthService_ResetPassword_Success_RevokesAllTokens(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	code := seedPendingReset(t, f)

	const userID = int64(42)
	f.repo.EXPECT().FindIDByEmail(ctx, testNormEmail).Return(userID, nil)
	f.credRepo.EXPECT().Upsert(ctx, userID, gomock.Any()).Return(nil)
	f.refresh.EXPECT().RevokeAllForUser(ctx, userID).Return(nil)

	err := f.svc.ResetPassword(ctx, testEmail, code, "a brand new password")
	require.NoError(t, err)

	_, found, storeErr := f.store.GetPendingReset(ctx, testNormEmail)
	require.NoError(t, storeErr)
	assert.False(t, found, "a spent pending reset is deleted")
}

func TestAuthService_ResetPassword_WrongCode_400(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	seedPendingReset(t, f)

	// No repo/credRepo/refresh expectations: a wrong code must not touch any
	// of them.
	err := f.svc.ResetPassword(ctx, testEmail, "000000", "a brand new password")
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestAuthService_ResetPassword_NoPendingIs400(t *testing.T) {
	f := setupAuthPasswordTest(t)
	err := f.svc.ResetPassword(context.Background(), testEmail, "123456", "a brand new password")
	assert.ErrorIs(t, err, ErrInvalidInput)
}

// Mirrors TestAuthService_VerifyEmail_WrongCodeExhaustsAttempts: the reset
// flow's attempt counter (fix round 2: cache.ResetAttemptsKey via
// IncrementResetAttempts, NOT the old pending-object-scoped
// IncrementPendingResetAttempts) must burn out the same way VerifyEmail's
// does, dropping the pending object once EmailCodeMaxAttempts is reached.
func TestAuthService_ResetPassword_WrongCodeExhaustsAttempts(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	code := seedPendingReset(t, f)

	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}

	// EmailCodeMaxAttempts is 3 in the fixture: two wrong codes are plain
	// 400s, and the pending object survives them (only the SEPARATE
	// attempt counter is what tracks progress toward the threshold now —
	// see fix round 2's rejectResetAttempt).
	for i := 0; i < f.cfg.EmailCodeMaxAttempts-1; i++ {
		err := f.svc.ResetPassword(ctx, testEmail, wrong, "a brand new password")
		assert.ErrorIs(t, err, ErrInvalidInput, "attempt %d", i+1)

		_, found, storeErr := f.store.GetPendingReset(ctx, testNormEmail)
		require.NoError(t, storeErr)
		require.True(t, found, "pending survives a merely-wrong code")
	}

	// The last one burns the allowance: 429 and the pending object is gone.
	// No repo/credRepo/refresh EXPECT is set anywhere in this test — a wrong
	// code must never touch any of them.
	err := f.svc.ResetPassword(ctx, testEmail, wrong, "a brand new password")
	assert.ErrorIs(t, err, ErrTooManyRequests)

	_, found, storeErr := f.store.GetPendingReset(ctx, testNormEmail)
	require.NoError(t, storeErr)
	assert.False(t, found, "pending is invalidated once attempts are exhausted")

	// Even the RIGHT code no longer works — the reset must be restarted.
	err = f.svc.ResetPassword(ctx, testEmail, code, "a brand new password")
	assert.ErrorIs(t, err, ErrInvalidInput)
}

// Fix round 2, Critical: a NONEXISTENT email (no PendingReset ever saved for
// it — no forgot-password call, or the account doesn't exist) must reach
// the exact same 429 on the exact same call count as a real account maxing
// out its attempts, via the shared cache.ResetAttemptsKey counter. Before
// this fix, "no pending object" short-circuited to a permanent 400 that
// never became 429 — a deterministic account-existence oracle when combined
// with ForgotPassword's always-202 contract.
func TestAuthService_ResetPassword_NonexistentEmail_EventuallyRateLimited(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	const strangerEmail = "stranger@example.com"

	// No repo/credRepo/refresh/store-seeding at all: this email has no
	// account and no pending reset was ever created for it.
	for i := 0; i < f.cfg.EmailCodeMaxAttempts-1; i++ {
		err := f.svc.ResetPassword(ctx, strangerEmail, "000000", "a brand new password")
		assert.ErrorIs(t, err, ErrInvalidInput, "attempt %d", i+1)
	}

	err := f.svc.ResetPassword(ctx, strangerEmail, "000000", "a brand new password")
	assert.ErrorIs(t, err, ErrTooManyRequests,
		"a nonexistent email must ALSO eventually hit 429, on the same call count a real account would")
}

// Fix round 2: the two sequences — a real account maxing out attempts vs. a
// nonexistent account "maxing out" the same shared counter — must produce
// BYTE-IDENTICAL error sequences over EmailCodeMaxAttempts calls. This is
// the property that actually closes the oracle (not just "both eventually
// 429", but "indistinguishable at every step").
func TestAuthService_ResetPassword_RealAndFakeAccountsProduceIdenticalErrorSequence(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()

	code := seedPendingReset(t, f)
	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}

	var realSeq, fakeSeq []error
	for i := 0; i < f.cfg.EmailCodeMaxAttempts; i++ {
		realSeq = append(realSeq, f.svc.ResetPassword(ctx, testEmail, wrong, "a brand new password"))
	}

	const strangerEmail = "nobody-here@example.com"
	for i := 0; i < f.cfg.EmailCodeMaxAttempts; i++ {
		fakeSeq = append(fakeSeq, f.svc.ResetPassword(ctx, strangerEmail, wrong, "a brand new password"))
	}

	require.Len(t, realSeq, f.cfg.EmailCodeMaxAttempts)
	require.Len(t, fakeSeq, f.cfg.EmailCodeMaxAttempts)
	for i := range realSeq {
		assert.Equalf(t, sentinelKind(realSeq[i]), sentinelKind(fakeSeq[i]),
			"call %d: real account gave %v, fake account gave %v — must match", i, realSeq[i], fakeSeq[i])
	}
	// The last call in particular must be the 429 for both.
	assert.ErrorIs(t, realSeq[len(realSeq)-1], ErrTooManyRequests)
	assert.ErrorIs(t, fakeSeq[len(fakeSeq)-1], ErrTooManyRequests)
}

// sentinelKind is a tiny test-only helper so the identical-sequence
// assertion above compares by SENTINEL (ErrInvalidInput vs
// ErrTooManyRequests), which is what the caller-visible HTTP status code
// actually maps from, rather than by exact error string (which legitimately
// differs in wrapped text between paths).
func sentinelKind(err error) string {
	switch {
	case err == nil:
		return "nil"
	case errors.Is(err, ErrTooManyRequests):
		return "429"
	case errors.Is(err, ErrInvalidInput):
		return "400"
	default:
		return "other"
	}
}

// --- ChangePassword ------------------------------------------------------

func TestAuthService_ChangePassword_Success_RevokesAllTokens(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	const userID = int64(42)

	oldHash, err := bcrypt.GenerateFromPassword([]byte("old password"), bcrypt.MinCost)
	require.NoError(t, err)
	f.credRepo.EXPECT().Get(ctx, userID).Return(&domain.UserCredentials{UserID: userID, PasswordHash: string(oldHash)}, nil)
	f.credRepo.EXPECT().Upsert(ctx, userID, gomock.Any()).Return(nil)
	f.refresh.EXPECT().RevokeAllForUser(ctx, userID).Return(nil)

	err = f.svc.ChangePassword(ctx, userID, "old password", "a brand new password")
	assert.NoError(t, err)
}

func TestAuthService_ChangePassword_WrongCurrentPassword_FailsWithoutRevoking(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	const userID = int64(42)

	oldHash, err := bcrypt.GenerateFromPassword([]byte("old password"), bcrypt.MinCost)
	require.NoError(t, err)
	f.credRepo.EXPECT().Get(ctx, userID).Return(&domain.UserCredentials{UserID: userID, PasswordHash: string(oldHash)}, nil)
	// No Upsert/RevokeAllForUser expectations: a wrong current password must
	// not touch either.

	err = f.svc.ChangePassword(ctx, userID, "totally wrong", "a brand new password")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestAuthService_ChangePassword_SocialOnlyUser_SkipsCurrentPasswordCheck(t *testing.T) {
	f := setupAuthPasswordTest(t)
	ctx := context.Background()
	const userID = int64(42)

	f.credRepo.EXPECT().Get(ctx, userID).Return(nil, repo.ErrNotFound)
	f.credRepo.EXPECT().Upsert(ctx, userID, gomock.Any()).Return(nil)
	f.refresh.EXPECT().RevokeAllForUser(ctx, userID).Return(nil)

	// currentPassword is empty (the client never had one to send) and must not
	// block the first-time password set.
	err := f.svc.ChangePassword(ctx, userID, "", "a brand new password")
	assert.NoError(t, err)
}
