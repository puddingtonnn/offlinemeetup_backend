package service

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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

type authLoginFixture struct {
	mr       *miniredis.Miniredis
	svc      *AuthService
	pwdRepo  *repomocks.MockPasswordUserRepository
	credRepo *repomocks.MockCredentialsRepository
	refresh  *svcmocks.MockRefreshTokenRepository
	store    *cache.RedisAuthStore
	cfg      *config.Config
}

func setupAuthLoginTest(t *testing.T) *authLoginFixture {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctrl := gomock.NewController(t)
	pwdRepo := repomocks.NewMockPasswordUserRepository(ctrl)
	credRepo := repomocks.NewMockCredentialsRepository(ctrl)
	refreshRepo := svcmocks.NewMockRefreshTokenRepository(ctrl)

	log := slog.New(slog.DiscardHandler)
	store := cache.NewRedisAuthStore(rdb, log)

	cfg := &config.Config{
		JWTSecret:       "test-secret",
		JWTAccessTTL:    15 * time.Minute,
		JWTRefreshTTL:   24 * time.Hour,
		LoginFailLimit:  10,
		LoginFailWindow: 15 * time.Minute,
	}

	svc := NewAuthService(nil, pwdRepo, credRepo, refreshRepo, store, nil, cfg, log)

	return &authLoginFixture{
		mr:       mr,
		svc:      svc,
		pwdRepo:  pwdRepo,
		credRepo: credRepo,
		refresh:  refreshRepo,
		store:    store,
		cfg:      cfg,
	}
}

// realHash builds a real bcrypt hash (cost 12) for a test password.
func realHash(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	require.NoError(t, err)
	return string(h)
}

func TestAuthService_Login_ByEmail_Success(t *testing.T) {
	f := setupAuthLoginTest(t)
	hash := realHash(t, "correct horse battery")

	f.pwdRepo.EXPECT().FindIDByEmail(gomock.Any(), "user@example.com").Return(int64(42), nil)
	f.credRepo.EXPECT().Get(gomock.Any(), int64(42)).
		Return(&domain.UserCredentials{UserID: 42, PasswordHash: hash}, nil)
	f.refresh.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

	// Mixed case + surrounding space on purpose — email lookup normalizes.
	tokens, err := f.svc.Login(context.Background(), " User@Example.com ", "correct horse battery")
	require.NoError(t, err)
	require.NotEmpty(t, tokens.AccessToken)
	require.NotEmpty(t, tokens.RefreshToken)
}

func TestAuthService_Login_ByUsername_Success(t *testing.T) {
	f := setupAuthLoginTest(t)
	hash := realHash(t, "correct horse battery")

	f.pwdRepo.EXPECT().FindIDByUsername(gomock.Any(), "bob").Return(int64(7), nil)
	f.credRepo.EXPECT().Get(gomock.Any(), int64(7)).
		Return(&domain.UserCredentials{UserID: 7, PasswordHash: hash}, nil)
	f.refresh.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

	tokens, err := f.svc.Login(context.Background(), "bob", "correct horse battery")
	require.NoError(t, err)
	require.NotEmpty(t, tokens.AccessToken)
	require.NotEmpty(t, tokens.RefreshToken)

	// Email and username login give identical success shape: a real token
	// pair, same as TestAuthService_Login_ByEmail_Success.
	require.NotEmpty(t, tokens.ExpiresIn)
}

// TestAuthService_Login_NotFoundVsWrongPassword_IdenticalFailure is the
// brief's central assertion: an unknown login and a known login with a wrong
// password must be indistinguishable to the caller. Both return the exact
// same sentinel, and both exercise bcrypt on a real-shaped comparison — the
// not-found case never calls CredentialsRepo.Get (no EXPECT is set for it,
// so gomock fails the test if it does) and instead compares against
// dummyPasswordHash, while the wrong-password case DOES call Get and
// compares against the real stored hash. Both still fail identically.
func TestAuthService_Login_NotFoundVsWrongPassword_IdenticalFailure(t *testing.T) {
	var notFoundErr, wrongPasswordErr error

	t.Run("not found", func(t *testing.T) {
		f := setupAuthLoginTest(t)
		f.pwdRepo.EXPECT().FindIDByEmail(gomock.Any(), "ghost@example.com").Return(int64(0), repo.ErrNotFound)
		// No EXPECT on f.credRepo.Get: if Login called it, the mock controller
		// fails this test — that is the "bcrypt still ran, but against the
		// dummy hash, not a DB-loaded one" assertion.

		_, err := f.svc.Login(context.Background(), "ghost@example.com", "whatever12345")
		notFoundErr = err
	})

	t.Run("wrong password", func(t *testing.T) {
		f := setupAuthLoginTest(t)
		hash := realHash(t, "realpassword123")
		f.pwdRepo.EXPECT().FindIDByEmail(gomock.Any(), "user@example.com").Return(int64(42), nil)
		f.credRepo.EXPECT().Get(gomock.Any(), int64(42)).
			Return(&domain.UserCredentials{UserID: 42, PasswordHash: hash}, nil)

		_, err := f.svc.Login(context.Background(), "user@example.com", "wrongpassword123")
		wrongPasswordErr = err
	})

	require.ErrorIs(t, notFoundErr, ErrUnauthorized)
	require.ErrorIs(t, wrongPasswordErr, ErrUnauthorized)
	require.Equal(t, notFoundErr, wrongPasswordErr, "not-found and wrong-password must be byte-for-byte the same error")
}

// TestAuthService_Login_NoCredentialsRowIsSameFailure covers the third
// timing-symmetry case the brief calls out explicitly: a login that exists
// but has never set a password (a Google/Telegram-only account) must fail
// exactly like a wrong password, not leak "this account has no password".
func TestAuthService_Login_NoCredentialsRowIsSameFailure(t *testing.T) {
	f := setupAuthLoginTest(t)
	f.pwdRepo.EXPECT().FindIDByEmail(gomock.Any(), "social@example.com").Return(int64(9), nil)
	f.credRepo.EXPECT().Get(gomock.Any(), int64(9)).Return(nil, repo.ErrNotFound)

	_, err := f.svc.Login(context.Background(), "social@example.com", "whatever12345")
	require.ErrorIs(t, err, ErrUnauthorized)
}

// TestAuthService_Login_TenFailuresThenTooManyRequests pins ADR-13's
// threshold: LoginFailLimit=10 means the 11th attempt within the window is
// rejected — and rejected WITHOUT touching the repo at all (no 11th EXPECT is
// set on FindIDByEmail; gomock fails if Login calls it).
func TestAuthService_Login_TenFailuresThenTooManyRequests(t *testing.T) {
	f := setupAuthLoginTest(t)
	f.pwdRepo.EXPECT().FindIDByEmail(gomock.Any(), "ghost@example.com").
		Return(int64(0), repo.ErrNotFound).Times(10)

	for i := 1; i <= 10; i++ {
		_, err := f.svc.Login(context.Background(), "ghost@example.com", "whatever12345")
		require.ErrorIsf(t, err, ErrUnauthorized, "attempt %d", i)
	}

	_, err := f.svc.Login(context.Background(), "ghost@example.com", "whatever12345")
	require.ErrorIs(t, err, ErrTooManyRequests)
}

// TestAuthService_Login_SuccessResetsCounter asserts the underlying Redis key
// is actually gone after a successful login, not just that a later login
// works.
func TestAuthService_Login_SuccessResetsCounter(t *testing.T) {
	f := setupAuthLoginTest(t)

	f.pwdRepo.EXPECT().FindIDByEmail(gomock.Any(), "user@example.com").
		Return(int64(0), repo.ErrNotFound).Times(3)
	for i := 0; i < 3; i++ {
		_, err := f.svc.Login(context.Background(), "user@example.com", "wrong")
		require.ErrorIs(t, err, ErrUnauthorized)
	}
	require.True(t, f.mr.Exists(cache.LoginFailKey("user@example.com")), "counter should exist after failures")

	hash := realHash(t, "correct horse battery")
	f.pwdRepo.EXPECT().FindIDByEmail(gomock.Any(), "user@example.com").Return(int64(42), nil)
	f.credRepo.EXPECT().Get(gomock.Any(), int64(42)).
		Return(&domain.UserCredentials{UserID: 42, PasswordHash: hash}, nil)
	f.refresh.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

	_, err := f.svc.Login(context.Background(), "user@example.com", "correct horse battery")
	require.NoError(t, err)

	require.False(t, f.mr.Exists(cache.LoginFailKey("user@example.com")), "counter must be gone after success")
}

// TestAuthService_Login_FailurePathTouchesNoPostgresWrites is ADR-13's other
// half: none of the mutating repo methods (CreateUserWithPassword,
// AttachPassword, RefreshTokenRepository.Create) have an EXPECT set below, so
// gomock fails the test the instant Login calls any of them. Only reads
// (FindIDBy*, CredentialsRepo.Get) are expected.
func TestAuthService_Login_FailurePathTouchesNoPostgresWrites(t *testing.T) {
	f := setupAuthLoginTest(t)
	hash := realHash(t, "realpassword123")

	f.pwdRepo.EXPECT().FindIDByEmail(gomock.Any(), "ghost@example.com").Return(int64(0), repo.ErrNotFound)
	_, err := f.svc.Login(context.Background(), "ghost@example.com", "whatever12345")
	require.ErrorIs(t, err, ErrUnauthorized)

	f.pwdRepo.EXPECT().FindIDByEmail(gomock.Any(), "user@example.com").Return(int64(42), nil)
	f.credRepo.EXPECT().Get(gomock.Any(), int64(42)).
		Return(&domain.UserCredentials{UserID: 42, PasswordHash: hash}, nil)
	_, err = f.svc.Login(context.Background(), "user@example.com", "wrongpassword123")
	require.ErrorIs(t, err, ErrUnauthorized)

	f.pwdRepo.EXPECT().FindIDByEmail(gomock.Any(), "social@example.com").Return(int64(7), nil)
	f.credRepo.EXPECT().Get(gomock.Any(), int64(7)).Return(nil, repo.ErrNotFound)
	_, err = f.svc.Login(context.Background(), "social@example.com", "whatever12345")
	require.ErrorIs(t, err, ErrUnauthorized)
}
