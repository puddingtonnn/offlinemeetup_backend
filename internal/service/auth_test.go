package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

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
