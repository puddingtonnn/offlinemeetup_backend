package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/dto"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service/mail"
	"google.golang.org/api/idtoken"
)

type AuthRepository interface {
	GetBySocialID(ctx context.Context, provider, socialID string) (*domain.User, error)
	CreateUserWithSocial(ctx context.Context, user *domain.User, provider, socialID string, profile *domain.Profile) (*domain.User, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
}

// RefreshTokenRepository persists opaque refresh tokens (by hash). Declared at
// the consumer; implemented by *repo.RefreshTokenRepo.
type RefreshTokenRepository interface {
	Create(ctx context.Context, token *domain.RefreshToken) error
	GetByHash(ctx context.Context, hash string) (*domain.RefreshToken, error)
	Revoke(ctx context.Context, id int64) error
	RevokeAllForUser(ctx context.Context, userID int64) error
}

// TokenPair is what a login/refresh yields: a short-lived access JWT plus a
// long-lived opaque refresh token. ExpiresIn is the access lifetime in seconds.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

type AuthService struct {
	repo        AuthRepository
	refreshRepo RefreshTokenRepository
	cfg         *config.Config

	// Email/password flow dependencies (auth_password.go, auth_login.go).
	// Their interfaces are declared in those files, at the consumer, per
	// CLAUDE.md.
	passwordRepo    PasswordUserRepository
	credentialsRepo CredentialsRepository
	authStore       AuthStore
	mailer          Mailer
	mailMetrics     mail.Metrics
	log             *slog.Logger
}

func NewAuthService(
	repo AuthRepository,
	passwordRepo PasswordUserRepository,
	credentialsRepo CredentialsRepository,
	refreshRepo RefreshTokenRepository,
	authStore AuthStore,
	mailer Mailer,
	mailMetrics mail.Metrics,
	cfg *config.Config,
	log *slog.Logger,
) *AuthService {
	if mailMetrics == nil {
		mailMetrics = mail.NopMetrics
	}
	return &AuthService{
		repo:            repo,
		refreshRepo:     refreshRepo,
		cfg:             cfg,
		passwordRepo:    passwordRepo,
		credentialsRepo: credentialsRepo,
		authStore:       authStore,
		mailer:          mailer,
		mailMetrics:     mailMetrics,
		log:             log,
	}
}

func (s *AuthService) LoginGoogle(ctx context.Context, tokenString string) (*TokenPair, error) {
	payload, err := idtoken.Validate(ctx, tokenString, s.cfg.GoogleClientID)
	if err != nil {
		return nil, fmt.Errorf("invalid google token: %w", err)
	}

	socialID := payload.Subject
	email := ""
	if val, ok := payload.Claims["email"].(string); ok {
		email = val
	}

	// Google's `email` claim can be set for an unverified address (e.g. a
	// custom domain the owner never confirmed). Since email becomes a
	// cross-provider account-linking key, an unverified one must not be
	// trusted for that purpose.
	if verified, ok := payload.Claims["email_verified"].(bool); !ok || !verified {
		return nil, errors.New("google email not verified")
	}

	user, err := s.findOrCreateUser(ctx, "google", socialID, email)
	if err != nil {
		return nil, err
	}
	return s.issueTokenPair(ctx, user.ID)
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

func (s *AuthService) LoginTelegram(ctx context.Context, params url.Values) (*TokenPair, error) {

	if !s.validateTelegramHash(params) {
		return nil, errors.New("invalid telegram hash")
	}

	authDate, err := strconv.ParseInt(params.Get("auth_date"), 10, 64)
	if err != nil {
		return nil, errors.New("invalid telegram auth_date")
	}
	if time.Since(time.Unix(authDate, 0)) > telegramAuthTTL {
		return nil, errors.New("telegram auth data expired")
	}

	socialID := params.Get("id")
	user, err := s.findOrCreateUser(ctx, "telegram", socialID, "")
	if err != nil {
		return nil, err
	}
	return s.issueTokenPair(ctx, user.ID)
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

	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "hash" {
			continue
		}
		keys = append(keys, k)
	}

	slices.Sort(keys)

	parts := make([]string, 0, len(keys))
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

// findOrCreateUser looks up a user by (provider, socialID), lazily creating a
// user+profile on first login. It returns the domain user; token issuance is a
// separate step (issueTokenPair) so every login path mints tokens the same way.
func (s *AuthService) findOrCreateUser(ctx context.Context, provider string, socialID string, email string) (*domain.User, error) {
	user, err := s.repo.GetBySocialID(ctx, provider, socialID)
	if err != nil {
		return nil, err
	}

	if user == nil {
		newUser := &domain.User{
			Email:  email,
			Role:   "user",
			Status: domain.UserStatusActive,
		}
		tempUsername := fmt.Sprintf("user_%s", socialID)
		newProfile := &domain.Profile{
			Username:     tempUsername,
			AvatarFileID: uuid.NullUUID{},
			Bio:          "",
		}
		user, err = s.repo.CreateUserWithSocial(ctx, newUser, provider, socialID, newProfile)
		if err != nil {
			return nil, err
		}
	}

	return user, nil
}

func (s *AuthService) CreateDevToken(ctx context.Context, email string) (*TokenPair, error) {
	dummySocialID := "dev_" + email

	user, err := s.findOrCreateUser(ctx, "dev_local", dummySocialID, email)
	if err != nil {
		return nil, err
	}
	return s.issueTokenPair(ctx, user.ID)
}

// issueTokenPair mints a short-lived access JWT and a fresh (rotated) refresh
// token for a user.
func (s *AuthService) issueTokenPair(ctx context.Context, userID int64) (*TokenPair, error) {
	access, err := s.signAccessToken(userID)
	if err != nil {
		return nil, err
	}
	refresh, err := s.newRefreshToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.cfg.JWTAccessTTL.Seconds()),
	}, nil
}

func (s *AuthService) signAccessToken(userID int64) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userID": userID,
		"iat":    now.Unix(),
		"exp":    now.Add(s.cfg.JWTAccessTTL).Unix(),
	})
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *AuthService) newRefreshToken(ctx context.Context, userID int64) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}
	token := hex.EncodeToString(raw)
	rt := &domain.RefreshToken{
		UserID:    userID,
		TokenHash: hashToken(token),
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshTTL),
	}
	if err := s.refreshRepo.Create(ctx, rt); err != nil {
		return "", err
	}
	return token, nil
}

// hashToken hashes a refresh token for storage/lookup. The token is 256 bits of
// CSPRNG output, so a fast SHA-256 is sufficient (no salt/bcrypt needed).
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Refresh validates a refresh token and rotates it: the presented token is
// revoked and a fresh pair is issued. Presenting an already-revoked token is
// treated as a replay — all of that user's tokens are revoked and the caller is
// rejected (ErrUnauthorized).
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	stored, err := s.refreshRepo.GetByHash(ctx, hashToken(refreshToken))
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, ErrUnauthorized
	}
	if stored.RevokedAt != nil {
		// Reuse of a rotated (revoked) token likely means it was stolen. Kill the
		// whole chain so neither the attacker nor the victim can refresh.
		_ = s.refreshRepo.RevokeAllForUser(ctx, stored.UserID)
		return nil, ErrUnauthorized
	}
	if time.Now().After(stored.ExpiresAt) {
		return nil, ErrUnauthorized
	}

	if err := s.refreshRepo.Revoke(ctx, stored.ID); err != nil {
		return nil, err
	}
	return s.issueTokenPair(ctx, stored.UserID)
}

// Logout revokes a refresh token. Idempotent: an unknown or already-revoked
// token is a no-op (no error), so logout always "succeeds" for the client.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	stored, err := s.refreshRepo.GetByHash(ctx, hashToken(refreshToken))
	if err != nil {
		return err
	}
	if stored == nil || stored.RevokedAt != nil {
		return nil
	}
	return s.refreshRepo.Revoke(ctx, stored.ID)
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
