package handler

import (
	"log/slog"
	"net/http"

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
