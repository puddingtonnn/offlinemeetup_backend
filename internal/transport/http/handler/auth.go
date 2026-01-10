package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	response "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
)

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

	token, err := h.service.LoginGoogle(r.Context(), req.Token)
	if err != nil {
		response.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"token": token})
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

	token, err := h.service.LoginTelegram(r.Context(), q)
	if err != nil {
		h.log.Error("Telegram login failed", slog.String("err", err.Error()))

		redirectURL := fmt.Sprintf("meetuper://auth/error?message=%s", "login_failed")
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	redirectSuccess := fmt.Sprintf("meetuper://auth/success?token=%s", token)
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

	token, err := h.service.CreateDevToken(r.Context(), req.Email)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"token": token,
		"note":  "THIS IS A DEV TOKEN! DO NOT USE IN PRODUCTION",
	})
}
