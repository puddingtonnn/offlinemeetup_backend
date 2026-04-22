package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
)

type ChatHandler struct {
	service *service.ChatService
	log     *slog.Logger
}

func NewChatHandler(service *service.ChatService, log *slog.Logger) *ChatHandler {
	return &ChatHandler{service: service, log: log}
}

// GetUserChats
// @Summary     Список чатов пользователя
// @Description Возвращает список всех чатов (групповых и личных), в которых состоит текущий авторизованный пользователь. Отсортированы по активности (чаты с новыми сообщениями — сверху). Включает текст последнего сообщения и счетчик непрочитанных.
// @Tags        Chats
// @Security    BearerAuth
// @Produce     json
// @Success     200     {array}   dto.ChatResponse
// @Failure     401     {object}  response.ErrorResponse "Не авторизован"
// @Failure     500     {object}  response.ErrorResponse "Внутренняя ошибка сервера"
// @Router      /v1/chats [get]
func (h *ChatHandler) GetUserChats(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		response.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	chats, err := h.service.GetUserChats(r.Context(), userID)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, chats)
}

func (h *ChatHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		response.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	idStr := chi.URLParam(r, "id")
	chatID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.RespondError(w, fmt.Errorf("invalid chat id"), h.log)
		return
	}

	query := r.URL.Query()

	cursor, _ := strconv.ParseInt(query.Get("cursor"), 10, 64)
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	messages, err := h.service.GetMessages(r.Context(), chatID, cursor, limit)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, messages)
}
