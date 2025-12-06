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

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Register - ручка регистрации
// @Summary      Регистрация нового пользователя
// @Description  Создает пользователя в базе и возвращает его ID.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        input body registerRequest true "Данные для регистрации"
// @Success      201  {object}  map[string]interface{} "Успешная регистрация, возвращает ID"
// @Failure      400  {string}  string "Неверный формат запроса"
// @Failure      500  {string}  string "Ошибка сервера (например, email уже занят)"
// @Router       /auth/register [post]
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	id, err := h.service.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id})
}

// Login - ручка входа
// @Summary      Вход в систему
// @Description  Проверяет email/пароль и выдает JWT токен.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        input body loginRequest true "Данные для входа"
// @Success      200  {object}  map[string]string "Успешный вход, возвращает token"
// @Failure      400  {string}  string "Неверный запрос"
// @Failure      401  {string}  string "Неверный email или пароль"
// @Router       /auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	token, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"token": token})
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
