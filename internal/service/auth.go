package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"google.golang.org/api/idtoken"
	"net/url"
	"sort"
	"strings"
	"time"
)

type AuthRepository interface {
	GetBySocialID(ctx context.Context, provider, socialID string) (*domain.User, error)
	CreateUserWithSocial(ctx context.Context, user *domain.User, provider, socialID string, profile *domain.Profile) (*domain.User, error)
}

type AuthService struct {
	repo AuthRepository
	cfg  *config.Config
}

func NewAuthService(repo AuthRepository, cfg *config.Config) *AuthService {
	return &AuthService{repo: repo, cfg: cfg}
}

func (s *AuthService) LoginGoogle(ctx context.Context, tokenString string) (string, error) {
	payload, err := idtoken.Validate(ctx, tokenString, s.cfg.GoogleClientID)
	if err != nil {
		return "", fmt.Errorf("invalid google token: %w", err)
	}

	socialID := payload.Subject
	email := ""
	if val, ok := payload.Claims["email"].(string); ok {
		email = val
	}

	return s.findOrCreateUser(ctx, "google", socialID, email)
}

type TelegramAuthData struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	PhotoURL  string `json:"photo_url"`
	AuthDate  int64  `json:"auth_date"`
	Hash      string `json:"hash"`
}

func (s *AuthService) LoginTelegram(ctx context.Context, params url.Values) (string, error) {

	if !s.validateTelegramHash(params) {
		return "", errors.New("invalid telegram hash")
	}

	socialID := params.Get("id")
	return s.findOrCreateUser(ctx, "telegram", socialID, "")
}

func (s *AuthService) validateTelegramHash(params url.Values) bool {
	receivedHash := params.Get("hash")
	if receivedHash == "" {
		fmt.Println("DEBUG: No hash found in params")
		return false
	}

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
		parts = append(parts, fmt.Sprintf("%s=%s", k, params.Get(k)))
	}
	dataCheckString := strings.Join(parts, "\n")

	sha256hash := sha256.New()
	sha256hash.Write([]byte(s.cfg.TelegramBotToken))
	secretKey := sha256hash.Sum(nil)

	hmacHash := hmac.New(sha256.New, secretKey)
	hmacHash.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(hmacHash.Sum(nil))

	return calculatedHash == receivedHash
}

func (s *AuthService) findOrCreateUser(ctx context.Context, provider string, socialID string, email string) (string, error) {
	user, err := s.repo.GetBySocialID(ctx, provider, socialID)
	if err != nil {
		return "", err
	}

	if user == nil {
		newUser := &domain.User{
			Email:  email,
			Role:   "user",
			Status: domain.UserStatusActive,
		}
		tempNick := fmt.Sprintf("user_%s", socialID)
		newProfile := &domain.Profile{
			Nickname:  tempNick,
			AvatarURL: "",
			Bio:       "",
		}
		user, err = s.repo.CreateUserWithSocial(ctx, newUser, provider, socialID, newProfile)
		if err != nil {
			return "", err
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userID": user.ID,
		"exp":    time.Now().Add(time.Hour * 24 * 30).Unix(),
	})
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *AuthService) CreateDevToken(ctx context.Context, email string) (string, error) {
	dummySocialID := "dev_" + email

	token, err := s.findOrCreateUser(ctx, "dev_local", dummySocialID, email)
	return token, err
}
