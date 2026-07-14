package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	response "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/dto"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
)

// tokenResponse maps the service token pair to the wire DTO.
func tokenResponse(t *service.TokenPair) dto.AuthTokensResponse {
	return dto.AuthTokensResponse{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    t.ExpiresIn,
	}
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthHandler struct {
	service *service.AuthService
	log     *slog.Logger
}

func NewAuthHandler(service *service.AuthService, log *slog.Logger) *AuthHandler {
	return &AuthHandler{service: service, log: log}
}

type googleLoginRequest struct {
	Token string `json:"token"`
}

// GoogleLogin
// @Summary      Вход через Google
// @Description  Принимает ID Token от Google SDK, возвращает JWT.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        input body googleLoginRequest true "Google ID Token"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  response.ErrorResponse
// @Router       /v1/auth/google [post]
func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	var req googleLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}

	tokens, err := h.service.LoginGoogle(r.Context(), req.Token)
	if err != nil {
		response.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	response.JSON(w, http.StatusOK, tokenResponse(tokens))
}

// TelegramCallBack TelegramCallback
// @Summary      Callback для виджета Telegram
// @Description  Принимает GET параметры от Telegram, валидирует, логинит и редиректит в приложение с токеном.
// @Tags         Auth
// @Router       /v1/auth/telegram/callback [get]
func (h *AuthHandler) TelegramCallBack(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	if q.Get("hash") == "" || q.Get("id") == "" {
		response.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	tokens, err := h.service.LoginTelegram(r.Context(), q)
	if err != nil {
		h.log.Error("Telegram login failed", slog.String("err", err.Error()))

		redirectURL := fmt.Sprintf("meetuper://auth/error?message=%s", "login_failed")
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	// Токены уходят в query URL диплинка — экранируем оба (без QueryEscape
	// url-значимые символы сломали бы разбор ссылки на клиенте).
	redirectSuccess := fmt.Sprintf("meetuper://auth/success?access_token=%s&refresh_token=%s&expires_in=%d",
		url.QueryEscape(tokens.AccessToken), url.QueryEscape(tokens.RefreshToken), tokens.ExpiresIn)
	http.Redirect(w, r, redirectSuccess, http.StatusFound)
}

// Me - получение своего аккаунта
// @Summary      Получить данные моего аккаунта
// @Description  Возвращает ID, email, роль и статус.
// @Tags         Auth
// @Security     BearerAuth
// @Produce      json
// @Success      200  {string}  dto.UserResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Router       /v1/auth/me [get]
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		response.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	userDTO, err := h.service.GetCurrentUser(r.Context(), userID)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, userDTO)
}

type devLoginRequest struct {
	Email string `json:"email"`
}

func (h *AuthHandler) DevLogin(w http.ResponseWriter, r *http.Request) {
	var req devLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	if req.Email == "" {
		req.Email = "test@example.com" // Дефолтный тестовый юзер
	}

	tokens, err := h.service.CreateDevToken(r.Context(), req.Email)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, tokenResponse(tokens))
}

// Refresh
// @Summary      Обновить токены
// @Description  Принимает refresh_token, ротирует его и возвращает новую пару токенов.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        input body refreshRequest true "Refresh token"
// @Success      200  {object}  dto.AuthTokensResponse
// @Failure      401  {object}  response.ErrorResponse
// @Router       /v1/auth/refresh [post]
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}

	tokens, err := h.service.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, tokenResponse(tokens))
}

// Logout
// @Summary      Выход
// @Description  Отзывает переданный refresh_token. Идемпотентно.
// @Tags         Auth
// @Accept       json
// @Param        input body refreshRequest true "Refresh token"
// @Success      204  "No Content"
// @Router       /v1/auth/logout [post]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}

	if err := h.service.Logout(r.Context(), req.RefreshToken); err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
