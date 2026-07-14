package websocket

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
)

type WSHandler struct {
	hub             *Hub
	log             *slog.Logger
	chatService     *service.ChatService
	profileService  *service.ProfileService
	presenceService *service.PresenceService
	upgrader        websocket.Upgrader
}

func NewWebSocketHandler(hub *Hub, log *slog.Logger, chatService *service.ChatService, profileService *service.ProfileService, presenceService *service.PresenceService, allowedOrigins []string) *WSHandler {
	return &WSHandler{
		hub:             hub,
		log:             log,
		chatService:     chatService,
		profileService:  profileService,
		presenceService: presenceService,
		upgrader:        newUpgrader(allowedOrigins),
	}
}

// newConnID returns a random per-connection identifier. It must be unique across
// a user's devices and across instances so presence accounting (a Redis set of
// connection IDs) stays correct; crypto/rand gives that without an extra dep.
func newConnID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
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
		h.log.Error("websocket upgrade failed", slog.Any("err", err))
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
		connID:      newConnID(),
		nickname:    nickname,
		hub:         h.hub,
		conn:        conn,
		send:        make(chan []byte, 256),
		work:        make(chan WSEvent, workBuffer),
		chatService: h.chatService,
		presence:    h.presenceService,
		log:         h.log,
		cancel:      cancel,
	}

	// Полностью заполняем client.rooms ДО публикации клиента в хаб: send в
	// register-канал переносит готовый слайс через happens-before границу, иначе
	// горутина хаба (unregister) могла бы читать rooms, пока ServeWs ещё аппендит.
	chats, err := h.chatService.GetUserChats(context.Background(), userID)
	if err == nil {
		for _, chat := range chats {
			client.rooms = append(client.rooms, chat.ID)
			client.hub.Subscribe(client, chat.ID)
		}
	} else {
		h.log.Error("failed to get user chats for ws subscription", slog.Any("err", err))
	}

	client.hub.register <- client

	// Регистрируем присутствие и шлём начальный снапшот до старта насосов: канал
	// send буферизирован, поэтому снапшот дождётся writePump. Best-effort.
	h.announcePresence(userID, client)

	// Каждая горутина соединения — под safeGo: паника в одной изолирована и не
	// роняет процесс. eventPump — единственный воркер, обрабатывающий события по
	// порядку.
	safeGo(h.log, func() { client.writePump(ctx, cancel) })
	safeGo(h.log, func() { client.eventPump(ctx) })
	safeGo(h.log, func() { client.readPump(ctx, cancel) })
}

// announcePresence marks the user online, broadcasts userOnline to co-chat
// members on the offline->online transition, and pushes the connecting client a
// snapshot of its peers' presence. All best-effort: a presence failure must not
// break the chat connection.
func (h *WSHandler) announcePresence(userID int64, client *Client) {
	if h.presenceService == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	online, recipients, err := h.presenceService.OnConnect(ctx, userID, client.connID)
	if err != nil {
		h.log.Error("presence: on connect", slog.Any("err", err))
	} else if online && len(recipients) > 0 {
		h.hub.BroadcastToUsers(recipients, presenceEvent(EventUserOnline, userID, true, client.nickname, nil))
	}

	statuses, err := h.presenceService.SnapshotFor(ctx, userID)
	if err != nil {
		h.log.Error("presence: snapshot", slog.Any("err", err))
		return
	}
	select {
	case client.send <- presenceSnapshotEvent(statuses):
	default:
		// send buffer full at connect time is implausible; drop rather than block.
	}
}
