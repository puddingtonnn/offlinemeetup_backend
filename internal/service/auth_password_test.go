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
	mr      *miniredis.Miniredis
	svc     *AuthService
	repo    *repomocks.MockPasswordUserRepository
	refresh *svcmocks.MockRefreshTokenRepository
	store   *cache.RedisAuthStore
	mailer  *fakeMailer
	cfg     *config.Config
}

func setupAuthPasswordTest(t *testing.T) *authPwdFixture {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctrl := gomock.NewController(t)
	pwdRepo := repomocks.NewMockPasswordUserRepository(ctrl)
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

	svc := NewAuthService(nil, pwdRepo, nil, refreshRepo, store, mailer, cfg, log)

	return &authPwdFixture{
		mr:      mr,
		svc:     svc,
		repo:    pwdRepo,
		refresh: refreshRepo,
		store:   store,
		mailer:  mailer,
		cfg:     cfg,
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
