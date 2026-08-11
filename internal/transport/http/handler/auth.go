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

// Register
// @Summary      Регистрация по email и паролю
// @Description  Создаёт «ожидающую» регистрацию и отправляет код подтверждения на email. Пользователь в БД не создаётся до подтверждения. Ответ одинаков независимо от того, существует ли уже аккаунт с таким email.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        input body dto.RegisterRequest true "Email, username, пароль"
// @Success      202  "Accepted"
// @Failure      400  {object}  response.ValidationErrorResponse
// @Router       /v1/auth/register [post]
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		response.RespondValidation(w, errs)
		return
	}

	if err := h.service.Register(r.Context(), req.Email, req.Username, req.Password); err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// VerifyEmail
// @Summary      Подтвердить email и завершить регистрацию
// @Description  Проверяет код из письма, создаёт аккаунт (или добавляет пароль к существующему) и возвращает пару токенов. Если username заняли, пока код был в пути, вернётся 409 — повторите запрос с другим username.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        input body dto.VerifyEmailRequest true "Email, код, опционально другой username"
// @Success      200  {object}  dto.AuthTokensResponse
// @Failure      400  {object}  response.ValidationErrorResponse
// @Failure      409  {object}  response.ErrorResponse
// @Failure      429  {object}  response.ErrorResponse
// @Router       /v1/auth/verify-email [post]
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req dto.VerifyEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		response.RespondValidation(w, errs)
		return
	}

	tokens, err := h.service.VerifyEmail(r.Context(), req.Email, req.Code, req.Username)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, tokenResponse(tokens))
}

// ResendCode
// @Summary      Отправить код подтверждения повторно
// @Description  Генерирует новый код для незавершённой регистрации. Отвечает 202 и в том случае, когда незавершённой регистрации нет — чтобы эндпоинт не подсказывал, какие email заняты.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        input body dto.ResendCodeRequest true "Email"
// @Success      202  "Accepted"
// @Failure      400  {object}  response.ValidationErrorResponse
// @Failure      429  {object}  response.ErrorResponse
// @Router       /v1/auth/resend-code [post]
func (h *AuthHandler) ResendCode(w http.ResponseWriter, r *http.Request) {
	var req dto.ResendCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		response.RespondValidation(w, errs)
		return
	}

	if err := h.service.ResendCode(r.Context(), req.Email); err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// Login
// @Summary      Вход по email/username и паролю
// @Description  Принимает login (email или username — определяется по наличию '@') и пароль, возвращает пару токенов. Неизвестный login и неверный пароль дают одинаковую ошибку.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        input body dto.LoginRequest true "Login, пароль"
// @Success      200  {object}  dto.AuthTokensResponse
// @Failure      400  {object}  response.ValidationErrorResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      429  {object}  response.ErrorResponse
// @Router       /v1/auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		response.RespondValidation(w, errs)
		return
	}

	tokens, err := h.service.Login(r.Context(), req.Login, req.Password)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, tokenResponse(tokens))
}

// ForgotPassword
// @Summary      Запросить сброс пароля
// @Description  Принимает login (email или username) и всегда отвечает 202 — независимо от того, существует ли такой аккаунт, чтобы эндпоинт не подсказывал, какие аккаунты зарегистрированы. Код для сброса отправляется на email, только если аккаунт найден.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        input body dto.ForgotPasswordRequest true "Login"
// @Success      202  "Accepted"
// @Failure      400  {object}  response.ValidationErrorResponse
// @Router       /v1/auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		response.RespondValidation(w, errs)
		return
	}

	if err := h.service.ForgotPassword(r.Context(), req.Login); err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// ResetPassword
// @Summary      Завершить сброс пароля
// @Description  Проверяет код из письма и устанавливает новый пароль. Не логинит — все refresh-токены пользователя отзываются.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        input body dto.ResetPasswordRequest true "Email, код, новый пароль"
// @Success      200  "OK"
// @Failure      400  {object}  response.ValidationErrorResponse
// @Failure      429  {object}  response.ErrorResponse
// @Router       /v1/auth/reset-password [post]
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		response.RespondValidation(w, errs)
		return
	}

	if err := h.service.ResetPassword(r.Context(), req.Email, req.Code, req.NewPassword); err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ChangePassword
// @Summary      Сменить пароль
// @Description  Меняет пароль текущего аккаунта. Если у аккаунта ещё нет пароля (вход был только через Google/Telegram), current_password не проверяется. Все refresh-токены пользователя отзываются, включая тот, которым выполнен этот запрос — клиент должен быть готов заново выполнить вход (или получить 401 при следующем refresh).
// @Tags         Auth
// @Security     BearerAuth
// @Accept       json
// @Param        input body dto.ChangePasswordRequest true "Текущий и новый пароль"
// @Success      204  "No Content"
// @Failure      400  {object}  response.ValidationErrorResponse
// @Failure      401  {object}  response.ErrorResponse
// @Router       /v1/auth/password [patch]
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r, h.log)
	if !ok {
		return
	}

	var req dto.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		response.RespondValidation(w, errs)
		return
	}

	if err := h.service.ChangePassword(r.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
