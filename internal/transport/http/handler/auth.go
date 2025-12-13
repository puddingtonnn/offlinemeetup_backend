package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
)

type AuthHandler struct {
	service *service.AuthService
}

func NewAuthHandler(service *service.AuthService) *AuthHandler {
	return &AuthHandler{
		service: service,
	}
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
// @Failure      400  {string}  string "Error"
// @Router       /auth/google [post]
func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	var req googleLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	token, err := h.service.LoginGoogle(r.Context(), req.Token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

// TelegramLogin
// @Summary      Вход через Telegram
// @Description  Принимает данные виджета Telegram, проверяет хеш, возвращает JWT.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        input body service.TelegramAuthData true "Telegram Data"
// @Success      200  {object}  map[string]string
// @Failure      400  {string}  string "Error"
// @Router       /auth/telegram [post]
func (h *AuthHandler) TelegramLogin(w http.ResponseWriter, r *http.Request) {
	var req service.TelegramAuthData
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	token, err := h.service.LoginTelegram(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

// Me - получение своего профиля
// @Summary      Получить мой профиль
// @Description  Возвращает ID текущего пользователя (тест авторизации).
// @Tags         Auth
// @Security     BearerAuth
// @Produce      json
// @Success      200  {string}  string "Приветствие с ID"
// @Failure      401  {string}  string "Пользователь не авторизован"
// @Router       /auth/me [get]
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "user not found in context", http.StatusInternalServerError)
		return
	}
	w.Write([]byte(fmt.Sprintf("Your user ID is: %d", userID)))
}
