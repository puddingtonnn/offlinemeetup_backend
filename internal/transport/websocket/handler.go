package websocket

import (
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
	"log"
	"log/slog"
	"net/http"
)

type WSHandler struct {
	hub *Hub
	log *slog.Logger
}

func NewWebSocketHandler(hub *Hub, log *slog.Logger) *WSHandler {
	return &WSHandler{hub: hub, log: log}
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

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	client := &Client{userID: userID, hub: h.hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}
