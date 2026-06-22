package websocket

import (
	"context"
	"log"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
)

type WSHandler struct {
	hub            *Hub
	log            *slog.Logger
	chatService    *service.ChatService
	profileService *service.ProfileService
	upgrader       websocket.Upgrader
}

func NewWebSocketHandler(hub *Hub, log *slog.Logger, chatService *service.ChatService, profileService *service.ProfileService, allowedOrigins []string) *WSHandler {
	return &WSHandler{
		hub:            hub,
		log:            log,
		chatService:    chatService,
		profileService: profileService,
		upgrader:       newUpgrader(allowedOrigins),
	}
}

// ServeWs
// @Summary     Установка WebSocket соединения
// @Description Открывает постоянное WebSocket соединение для получения событий чата (новые сообщения, статусы). Требуется Bearer токен в заголовке Authorization.
// @Tags        WebSockets
// @Security    BearerAuth
// @Success     101     {string}  string "Switching Protocols (Успешное подключение)"
// @Failure     401     {object}  response.ErrorResponse "Не авторизован"
// @Failure     500     {object}  response.ErrorResponse "Ошибка при апгрейде соединения"
// @Router      /v1/ws [get]
func (h *WSHandler) ServeWs(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		response.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// Получаем никнейм один раз при подключении
	nickname := "User"
	profile, err := h.profileService.GetProfile(r.Context(), userID)
	if err == nil && profile != nil {
		nickname = profile.Nickname
	}

	// Создаем базовый контекст для этого конкретного соединения
	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		userID:      userID,
		nickname:    nickname,
		hub:         h.hub,
		conn:        conn,
		send:        make(chan []byte, 256),
		chatService: h.chatService,
		log:         h.log,
	}
	client.hub.register <- client

	chats, err := h.chatService.GetUserChats(context.Background(), userID)
	if err == nil {
		for _, chat := range chats {
			client.rooms = append(client.rooms, chat.ID)
			client.hub.Subscribe(client, chat.ID)
		}
	} else {
		h.log.Error("failed to get user chats for ws subscription", slog.Any("err", err))
	}

	// Передаем контекст и функцию отмены в горутины
	go client.writePump(ctx, cancel)
	go client.readPump(ctx, cancel)
}
