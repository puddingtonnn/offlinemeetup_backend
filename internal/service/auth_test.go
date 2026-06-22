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
	srv := NewAuthService(nil, cfg)

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
	cfg := &config.Config{TelegramBotToken: "bot-token", JWTSecret: "secret"}

	params := url.Values{}
	params.Set("id", "555")
	params.Set("first_name", "John")
	params.Set("auth_date", strconv.FormatInt(time.Now().Unix(), 10))

	t.Run("valid hash for existing user", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockAuthRepository(ctrl)
		srv := NewAuthService(repo, cfg)

		p := url.Values{}
		for k, v := range params {
			p[k] = v
		}
		p.Set("hash", computeTelegramHash("bot-token", p))

		repo.EXPECT().GetBySocialID(ctx, "telegram", "555").Return(&domain.User{ID: 7}, nil)

		token, err := srv.LoginTelegram(ctx, p)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("invalid hash rejected without repo call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockAuthRepository(ctrl)
		srv := NewAuthService(repo, cfg)

		p := url.Values{}
		for k, v := range params {
			p[k] = v
		}
		p.Set("hash", "deadbeef")

		token, err := srv.LoginTelegram(ctx, p)
		assert.Error(t, err)
		assert.Empty(t, token)
	})

	t.Run("expired auth_date rejected (replay protection)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockAuthRepository(ctrl)
		srv := NewAuthService(repo, cfg)

		p := url.Values{}
		p.Set("id", "555")
		p.Set("first_name", "John")
		// auth_date двухдневной давности — за пределами TTL.
		p.Set("auth_date", strconv.FormatInt(time.Now().Add(-48*time.Hour).Unix(), 10))
		p.Set("hash", computeTelegramHash("bot-token", p))

		// Хэш валиден, но данные просрочены => репозиторий не вызывается.
		token, err := srv.LoginTelegram(ctx, p)
		assert.Error(t, err)
		assert.Empty(t, token)
	})
}

func TestAuthService_CreateDevToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockAuthRepository(ctrl)
	cfg := &config.Config{JWTSecret: "test_secret"}
	srv := NewAuthService(repo, cfg)

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

		token, err := srv.CreateDevToken(ctx, email)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		// Verify token
		parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
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

		newUser := &domain.User{
			ID:    2,
			Email: email,
		}

		repo.EXPECT().
			CreateUserWithSocial(ctx, gomock.Any(), "dev_local", dummySocialID, gomock.Any()).
			Return(newUser, nil)

		token, err := srv.CreateDevToken(ctx, email)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		// Verify token
		parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
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

		token, err := srv.CreateDevToken(ctx, email)
		assert.ErrorIs(t, err, repoErr)
		assert.Empty(t, token)
	})
	
	t.Run("repo_error_create", func(t *testing.T) {
		repo.EXPECT().
			GetBySocialID(ctx, "dev_local", dummySocialID).
			Return(nil, nil)
			
		repoErr := errors.New("create error")
		repo.EXPECT().
			CreateUserWithSocial(ctx, gomock.Any(), "dev_local", dummySocialID, gomock.Any()).
			Return(nil, repoErr)

		token, err := srv.CreateDevToken(ctx, email)
		assert.ErrorIs(t, err, repoErr)
		assert.Empty(t, token)
	})
}

func TestAuthService_GetCurrentUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockAuthRepository(ctrl)
	cfg := &config.Config{}
	srv := NewAuthService(repo, cfg)

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
