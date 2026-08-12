package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	repopkg "github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
	repomocks "github.com/puddingtonnn/offlinemeetup_backend/internal/repo/mocks"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// computeTelegramHash повторяет алгоритм подписи Telegram, чтобы сгенерировать
// валидный hash для теста.
func computeTelegramHash(botToken string, params url.Values) string {
	var keys []string
	for k := range params {
		if k == "hash" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+params.Get(k))
	}
	dataCheckString := strings.Join(parts, "\n")

	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(dataCheckString))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestAuthService_validateTelegramHash(t *testing.T) {
	cfg := &config.Config{TelegramBotToken: "bot-token"}
	srv := NewAuthService(nil, nil, nil, nil, nil, nil, nil, cfg, discardLog())

	baseParams := func() url.Values {
		p := url.Values{}
		p.Set("id", "12345")
		p.Set("first_name", "John")
		p.Set("auth_date", "1700000000")
		return p
	}

	t.Run("valid hash", func(t *testing.T) {
		p := baseParams()
		p.Set("hash", computeTelegramHash("bot-token", p))
		assert.True(t, srv.validateTelegramHash(p))
	})

	t.Run("tampered field", func(t *testing.T) {
		p := baseParams()
		p.Set("hash", computeTelegramHash("bot-token", p))
		// Подменяем данные после расчёта подписи.
		p.Set("first_name", "Mallory")
		assert.False(t, srv.validateTelegramHash(p))
	})

	t.Run("wrong hash", func(t *testing.T) {
		p := baseParams()
		p.Set("hash", "deadbeef")
		assert.False(t, srv.validateTelegramHash(p))
	})

	t.Run("missing hash", func(t *testing.T) {
		assert.False(t, srv.validateTelegramHash(baseParams()))
	})

	t.Run("wrong bot token", func(t *testing.T) {
		p := baseParams()
		p.Set("hash", computeTelegramHash("other-token", p))
		assert.False(t, srv.validateTelegramHash(p))
	})
}

func TestAuthService_LoginTelegram(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		TelegramBotToken: "bot-token",
		JWTSecret:        "secret",
		JWTAccessTTL:     15 * time.Minute,
		JWTRefreshTTL:    24 * time.Hour,
	}

	params := url.Values{}
	params.Set("id", "555")
	params.Set("first_name", "John")
	params.Set("auth_date", strconv.FormatInt(time.Now().Unix(), 10))

	t.Run("valid hash for existing user", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockAuthRepository(ctrl)
		refresh := mocks.NewMockRefreshTokenRepository(ctrl)
		srv := NewAuthService(repo, nil, nil, refresh, nil, nil, nil, cfg, discardLog())

		p := url.Values{}
		for k, v := range params {
			p[k] = v
		}
		p.Set("hash", computeTelegramHash("bot-token", p))

		repo.EXPECT().GetBySocialID(ctx, "telegram", "555").Return(&domain.User{ID: 7}, nil)
		refresh.EXPECT().Create(ctx, gomock.Any()).Return(nil)

		tokens, err := srv.LoginTelegram(ctx, p)
		assert.NoError(t, err)
		assert.NotNil(t, tokens)
		assert.NotEmpty(t, tokens.AccessToken)
		assert.NotEmpty(t, tokens.RefreshToken)
	})

	t.Run("invalid hash rejected without repo call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockAuthRepository(ctrl)
		srv := NewAuthService(repo, nil, nil, nil, nil, nil, nil, cfg, discardLog())

		p := url.Values{}
		for k, v := range params {
			p[k] = v
		}
		p.Set("hash", "deadbeef")

		tokens, err := srv.LoginTelegram(ctx, p)
		assert.Error(t, err)
		assert.Nil(t, tokens)
	})

	t.Run("expired auth_date rejected (replay protection)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockAuthRepository(ctrl)
		srv := NewAuthService(repo, nil, nil, nil, nil, nil, nil, cfg, discardLog())

		p := url.Values{}
		p.Set("id", "555")
		p.Set("first_name", "John")
		// auth_date двухдневной давности — за пределами TTL.
		p.Set("auth_date", strconv.FormatInt(time.Now().Add(-48*time.Hour).Unix(), 10))
		p.Set("hash", computeTelegramHash("bot-token", p))

		// Хэш валиден, но данные просрочены => репозиторий не вызывается.
		tokens, err := srv.LoginTelegram(ctx, p)
		assert.Error(t, err)
		assert.Nil(t, tokens)
	})
}

func TestAuthService_CreateDevToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockAuthRepository(ctrl)
	pwdRepo := repomocks.NewMockPasswordUserRepository(ctrl)
	refresh := mocks.NewMockRefreshTokenRepository(ctrl)
	cfg := &config.Config{JWTSecret: "test_secret", JWTAccessTTL: 15 * time.Minute, JWTRefreshTTL: 24 * time.Hour}
	srv := NewAuthService(repo, pwdRepo, nil, refresh, nil, nil, nil, cfg, discardLog())

	ctx := context.Background()
	email := "dev@test.com"
	dummySocialID := "dev_" + email

	t.Run("user_exists", func(t *testing.T) {
		existingUser := &domain.User{
			ID:    1,
			Email: email,
		}

		repo.EXPECT().
			GetBySocialID(ctx, "dev_local", dummySocialID).
			Return(existingUser, nil)
		refresh.EXPECT().Create(ctx, gomock.Any()).Return(nil)

		tokens, err := srv.CreateDevToken(ctx, email)
		assert.NoError(t, err)
		assert.NotNil(t, tokens)
		assert.NotEmpty(t, tokens.RefreshToken)

		// Verify access token
		parsedToken, err := jwt.Parse(tokens.AccessToken, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWTSecret), nil
		})
		assert.NoError(t, err)
		assert.True(t, parsedToken.Valid)
		claims, ok := parsedToken.Claims.(jwt.MapClaims)
		assert.True(t, ok)
		assert.Equal(t, float64(existingUser.ID), claims["userID"])
	})

	t.Run("user_not_exists", func(t *testing.T) {
		repo.EXPECT().
			GetBySocialID(ctx, "dev_local", dummySocialID).
			Return(nil, nil)
		// No account on this email yet, so the link-by-email step falls through
		// to the create path.
		pwdRepo.EXPECT().FindIDByEmail(ctx, email).Return(int64(0), repopkg.ErrNotFound)

		newUser := &domain.User{
			ID:    2,
			Email: email,
		}

		repo.EXPECT().
			CreateUserWithSocial(ctx, gomock.Any(), "dev_local", dummySocialID, gomock.Any()).
			Return(newUser, nil)
		refresh.EXPECT().Create(ctx, gomock.Any()).Return(nil)

		tokens, err := srv.CreateDevToken(ctx, email)
		assert.NoError(t, err)
		assert.NotNil(t, tokens)

		// Verify access token
		parsedToken, err := jwt.Parse(tokens.AccessToken, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWTSecret), nil
		})
		assert.NoError(t, err)
		assert.True(t, parsedToken.Valid)
		claims, ok := parsedToken.Claims.(jwt.MapClaims)
		assert.True(t, ok)
		assert.Equal(t, float64(newUser.ID), claims["userID"])
	})

	t.Run("repo_error_get", func(t *testing.T) {
		repoErr := errors.New("db error")
		repo.EXPECT().
			GetBySocialID(ctx, "dev_local", dummySocialID).
			Return(nil, repoErr)

		tokens, err := srv.CreateDevToken(ctx, email)
		assert.ErrorIs(t, err, repoErr)
		assert.Nil(t, tokens)
	})

	t.Run("repo_error_create", func(t *testing.T) {
		repo.EXPECT().
			GetBySocialID(ctx, "dev_local", dummySocialID).
			Return(nil, nil)
		pwdRepo.EXPECT().FindIDByEmail(ctx, email).Return(int64(0), repopkg.ErrNotFound)

		repoErr := errors.New("create error")
		repo.EXPECT().
			CreateUserWithSocial(ctx, gomock.Any(), "dev_local", dummySocialID, gomock.Any()).
			Return(nil, repoErr)

		tokens, err := srv.CreateDevToken(ctx, email)
		assert.ErrorIs(t, err, repoErr)
		assert.Nil(t, tokens)
	})
}

// TestAuthService_findOrCreateUser_LinksExistingEmail covers the fix for
// one-way account linking. Registering with a password on an email that
// already has a social account attaches the password to it (ADR-7); the
// reverse — social login on an email that already has a password account —
// used to take the create path and die on the users-email unique index,
// permanently locking the owner out of that login method.
func TestAuthService_findOrCreateUser_LinksExistingEmail(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{JWTSecret: "secret", JWTAccessTTL: 15 * time.Minute, JWTRefreshTTL: 24 * time.Hour}

	t.Run("links instead of creating a second account", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockAuthRepository(ctrl)
		pwdRepo := repomocks.NewMockPasswordUserRepository(ctrl)
		srv := NewAuthService(repo, pwdRepo, nil, nil, nil, nil, nil, cfg, discardLog())

		existing := &domain.User{ID: 42, Email: "bob@example.com"}
		repo.EXPECT().GetBySocialID(ctx, "google", "sub-1").Return(nil, nil)
		// The email is normalized (ADR-3) before it is used as the linking key.
		pwdRepo.EXPECT().FindIDByEmail(ctx, "bob@example.com").Return(int64(42), nil)
		repo.EXPECT().LinkSocialAccount(ctx, int64(42), "google", "sub-1").Return(nil)
		repo.EXPECT().GetByID(ctx, int64(42)).Return(existing, nil)
		// No CreateUserWithSocial expectation: creating a second row here is
		// exactly the bug, and gomock fails the test if it happens.

		user, err := srv.findOrCreateUser(ctx, "google", "sub-1", " Bob@Example.COM ")
		assert.NoError(t, err)
		assert.Equal(t, existing, user)
	})

	t.Run("creates when the email is free", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockAuthRepository(ctrl)
		pwdRepo := repomocks.NewMockPasswordUserRepository(ctrl)
		srv := NewAuthService(repo, pwdRepo, nil, nil, nil, nil, nil, cfg, discardLog())

		created := &domain.User{ID: 43, Email: "new@example.com"}
		repo.EXPECT().GetBySocialID(ctx, "google", "sub-2").Return(nil, nil)
		pwdRepo.EXPECT().FindIDByEmail(ctx, "new@example.com").Return(int64(0), repopkg.ErrNotFound)
		repo.EXPECT().
			CreateUserWithSocial(ctx, gomock.Any(), "google", "sub-2", gomock.Any()).
			Return(created, nil)

		user, err := srv.findOrCreateUser(ctx, "google", "sub-2", "new@example.com")
		assert.NoError(t, err)
		assert.Equal(t, created, user)
	})

	t.Run("no email means no linking attempt", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockAuthRepository(ctrl)
		pwdRepo := repomocks.NewMockPasswordUserRepository(ctrl)
		srv := NewAuthService(repo, pwdRepo, nil, nil, nil, nil, nil, cfg, discardLog())

		created := &domain.User{ID: 44}
		repo.EXPECT().GetBySocialID(ctx, "telegram", "555").Return(nil, nil)
		// Telegram supplies no email, so linking must be skipped entirely —
		// an unexpected FindIDByEmail("") would fail the controller. Linking
		// every emailless account together would merge strangers.
		repo.EXPECT().
			CreateUserWithSocial(ctx, gomock.Any(), "telegram", "555", gomock.Any()).
			Return(created, nil)

		user, err := srv.findOrCreateUser(ctx, "telegram", "555", "")
		assert.NoError(t, err)
		assert.Equal(t, created, user)
	})
}

func TestAuthService_IsActive(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	tests := []struct {
		name       string
		user       *domain.User
		repoErr    error
		wantActive bool
		wantErr    bool
	}{
		{"active", &domain.User{ID: 1, Status: domain.UserStatusActive}, nil, true, false},
		{"banned", &domain.User{ID: 1, Status: domain.UserStatusBanned}, nil, false, false},
		{"inactive", &domain.User{ID: 1, Status: domain.UserStatusInactive}, nil, false, false},
		{"not found", nil, nil, false, false},
		{"repo error", nil, errors.New("db down"), false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			repo := mocks.NewMockAuthRepository(ctrl)
			srv := NewAuthService(repo, nil, nil, nil, nil, nil, nil, cfg, discardLog())

			repo.EXPECT().GetByID(ctx, int64(1)).Return(tt.user, tt.repoErr)

			active, err := srv.IsActive(ctx, 1)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantActive, active)
		})
	}
}

func TestAuthService_GetCurrentUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockAuthRepository(ctrl)
	cfg := &config.Config{}
	srv := NewAuthService(repo, nil, nil, nil, nil, nil, nil, cfg, discardLog())

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		user := &domain.User{
			ID:        1,
			Email:     "test@test.com",
			Role:      "user",
			Status:    domain.UserStatusActive,
			CreatedAt: time.Now(),
		}

		repo.EXPECT().GetByID(ctx, int64(1)).Return(user, nil)

		resp, err := srv.GetCurrentUser(ctx, 1)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, user.ID, resp.ID)
		assert.Equal(t, user.Email, resp.Email)
		assert.Equal(t, user.Role, resp.Role)
		assert.Equal(t, string(user.Status), resp.Status)
		assert.Equal(t, user.CreatedAt, resp.CreatedAt)
	})

	t.Run("not_found", func(t *testing.T) {
		repo.EXPECT().GetByID(ctx, int64(1)).Return(nil, nil)

		resp, err := srv.GetCurrentUser(ctx, 1)
		assert.ErrorIs(t, err, ErrNotFound)
		assert.Nil(t, resp)
	})

	t.Run("repo_error", func(t *testing.T) {
		repoErr := errors.New("db error")
		repo.EXPECT().GetByID(ctx, int64(1)).Return(nil, repoErr)

		resp, err := srv.GetCurrentUser(ctx, 1)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "fetching user")
		assert.Nil(t, resp)
	})
}

func TestAuthService_Refresh(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{JWTSecret: "secret", JWTAccessTTL: 15 * time.Minute, JWTRefreshTTL: 24 * time.Hour}

	t.Run("valid token rotates: revoke old + issue new pair", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		refresh := mocks.NewMockRefreshTokenRepository(ctrl)
		srv := NewAuthService(nil, nil, nil, refresh, nil, nil, nil, cfg, discardLog())

		stored := &domain.RefreshToken{ID: 5, UserID: 7, ExpiresAt: time.Now().Add(time.Hour)}
		refresh.EXPECT().GetByHash(ctx, gomock.Any()).Return(stored, nil)
		refresh.EXPECT().Revoke(ctx, int64(5)).Return(nil)
		refresh.EXPECT().Create(ctx, gomock.Any()).Return(nil)

		tokens, err := srv.Refresh(ctx, "raw-refresh-token")
		assert.NoError(t, err)
		assert.NotNil(t, tokens)
		assert.NotEmpty(t, tokens.AccessToken)
		assert.NotEmpty(t, tokens.RefreshToken)
	})

	t.Run("unknown token is unauthorized", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		refresh := mocks.NewMockRefreshTokenRepository(ctrl)
		srv := NewAuthService(nil, nil, nil, refresh, nil, nil, nil, cfg, discardLog())

		refresh.EXPECT().GetByHash(ctx, gomock.Any()).Return(nil, nil)

		tokens, err := srv.Refresh(ctx, "nope")
		assert.ErrorIs(t, err, ErrUnauthorized)
		assert.Nil(t, tokens)
	})

	t.Run("revoked token triggers reuse-detection (revoke all)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		refresh := mocks.NewMockRefreshTokenRepository(ctrl)
		srv := NewAuthService(nil, nil, nil, refresh, nil, nil, nil, cfg, discardLog())

		revokedAt := time.Now().Add(-time.Minute)
		stored := &domain.RefreshToken{ID: 5, UserID: 7, ExpiresAt: time.Now().Add(time.Hour), RevokedAt: &revokedAt}
		refresh.EXPECT().GetByHash(ctx, gomock.Any()).Return(stored, nil)
		refresh.EXPECT().RevokeAllForUser(ctx, int64(7)).Return(nil)

		tokens, err := srv.Refresh(ctx, "reused")
		assert.ErrorIs(t, err, ErrUnauthorized)
		assert.Nil(t, tokens)
	})

	t.Run("expired token is unauthorized", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		refresh := mocks.NewMockRefreshTokenRepository(ctrl)
		srv := NewAuthService(nil, nil, nil, refresh, nil, nil, nil, cfg, discardLog())

		stored := &domain.RefreshToken{ID: 5, UserID: 7, ExpiresAt: time.Now().Add(-time.Hour)}
		refresh.EXPECT().GetByHash(ctx, gomock.Any()).Return(stored, nil)

		tokens, err := srv.Refresh(ctx, "old")
		assert.ErrorIs(t, err, ErrUnauthorized)
		assert.Nil(t, tokens)
	})
}

func TestAuthService_Logout(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	t.Run("revokes a live token", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		refresh := mocks.NewMockRefreshTokenRepository(ctrl)
		srv := NewAuthService(nil, nil, nil, refresh, nil, nil, nil, cfg, discardLog())

		stored := &domain.RefreshToken{ID: 9, UserID: 3}
		refresh.EXPECT().GetByHash(ctx, gomock.Any()).Return(stored, nil)
		refresh.EXPECT().Revoke(ctx, int64(9)).Return(nil)

		assert.NoError(t, srv.Logout(ctx, "tok"))
	})

	t.Run("unknown token is an idempotent no-op", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		refresh := mocks.NewMockRefreshTokenRepository(ctrl)
		srv := NewAuthService(nil, nil, nil, refresh, nil, nil, nil, cfg, discardLog())

		refresh.EXPECT().GetByHash(ctx, gomock.Any()).Return(nil, nil)

		assert.NoError(t, srv.Logout(ctx, "whatever"))
	})
}
