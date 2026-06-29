package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
	"google.golang.org/api/idtoken"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type AuthRepository interface {
	GetBySocialID(ctx context.Context, provider, socialID string) (*domain.User, error)
	CreateUserWithSocial(ctx context.Context, user *domain.User, provider, socialID string, profile *domain.Profile) (*domain.User, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
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

// telegramAuthTTL — максимальный возраст данных авторизации Telegram.
// Защищает от replay-атак повторно использованной login-ссылкой.
const telegramAuthTTL = 24 * time.Hour

func (s *AuthService) LoginTelegram(ctx context.Context, params url.Values) (string, error) {

	if !s.validateTelegramHash(params) {
		return "", errors.New("invalid telegram hash")
	}

	authDate, err := strconv.ParseInt(params.Get("auth_date"), 10, 64)
	if err != nil {
		return "", errors.New("invalid telegram auth_date")
	}
	if time.Since(time.Unix(authDate, 0)) > telegramAuthTTL {
		return "", errors.New("telegram auth data expired")
	}

	socialID := params.Get("id")
	return s.findOrCreateUser(ctx, "telegram", socialID, "")
}

func (s *AuthService) validateTelegramHash(params url.Values) bool {
	// Defense-in-depth: с пустым токеном secretKey вырождается в sha256("") —
	// публичную константу, по которой любой подделает подпись. Конфиг уже не
	// стартует с пустым токеном, но не доверяем этому в security-проверке.
	if s.cfg.TelegramBotToken == "" {
		return false
	}

	receivedHash := params.Get("hash")
	if receivedHash == "" {
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

	// hmac.Equal — сравнение за константное время, без утечки по таймингу.
	return hmac.Equal([]byte(calculatedHash), []byte(receivedHash))
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
			Nickname:     tempNick,
			AvatarFileID: uuid.NullUUID{},
			Bio:          "",
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

// IsActive проверяет, что аккаунт существует и находится в статусе active.
// Используется AuthMiddleware для немедленного отзыва доступа.
func (s *AuthService) IsActive(ctx context.Context, userID int64) (bool, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, nil
	}
	return user.Status == domain.UserStatusActive, nil
}

func (s *AuthService) GetCurrentUser(ctx context.Context, userID int64) (*dto.UserResponse, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("fetching user: %w", err)
	}
	if user == nil {
		return nil, ErrNotFound
	}

	return &dto.UserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Role:      user.Role,
		Status:    string(user.Status),
		CreatedAt: user.CreatedAt,
	}, nil
}
