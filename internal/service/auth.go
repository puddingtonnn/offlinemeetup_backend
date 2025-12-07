package service

import (
	"context"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
	"golang.org/x/crypto/bcrypt"
	"time"
)

var jwtSecret = []byte("super-secret-key")

type AuthService struct {
	repo *repo.UserRepo
}

func NewAuthService(repo *repo.UserRepo, cfg *config.Config) *AuthService {
	return &AuthService{repo: repo, cfg: cfg}
}

func (s *AuthService) Register(ctx context.Context, email, password string) (int64, error) {
	passHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	user := &domain.User{
		Email:        email,
		PasswordHash: string(passHash),
		Status:       domain.UserStatusActive,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return 0, err
	}

	return user.ID, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, error) {
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return "", errors.New("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", errors.New("invalid email or password")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID,
		"exp":     time.Now().Add(time.Hour * 720).Unix(),
	})

	tokensString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return tokensString, nil
}
