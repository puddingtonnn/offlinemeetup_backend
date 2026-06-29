package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
)

const (
	writeWait = 10 * time.Second

	pongWait = 60 * time.Second

	pingPeriod = (pongWait * 9) / 10

	maxMessageSize = 4096
)

// newUpgrader строит WebSocket upgrader с проверкой Origin по allowlist.
// Пустой allowedOrigins => разрешены любые источники (нативные клиенты,
// обратная совместимость). Запросы без заголовка Origin (мобильные приложения)
// пропускаются всегда.
func newUpgrader(allowedOrigins []string) websocket.Upgrader {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}

	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if len(allowed) == 0 {
				return true
			}
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			return allowed[origin]
		},
	}
}

type Client struct {
	userID      int64
	connID      string
	nickname    string
	hub         *Hub
	conn        *websocket.Conn
	send        chan []byte
	chatService *service.ChatService
	presence    *service.PresenceService
	log         *slog.Logger
	rooms       []int64
}

func (c *Client) readPump(ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		cancel()
		c.hub.unregister <- c
		c.conn.Close()
		c.markOffline()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				c.log.Error("websocket unexpected close", slog.Any("err", err))
			} else if !errors.As(err, &closeErr) {
				// Log non-close errors (e.g., protocol violations, unmasked frames, invalid utf-8, read timeouts)
				c.log.Error("websocket read error", slog.Any("err", err))
			}
			break
		}

		var event WSEvent
		if err := json.Unmarshal(message, &event); err != nil {
			c.log.Error("failed to unmarshal WS event", slog.Any("err", err))
			continue
		}

		c.handleEvent(ctx, event)
	}
}

func (c *Client) handleEvent(ctx context.Context, event WSEvent) {
	switch event.Type {
	case EventNewMessage:
		var req WSSendMessagePayload
		if err := json.Unmarshal(event.Payload, &req); err != nil {
			c.sendError(ctx, event.RequestID, "invalid payload for newMessage")
			return
		}

		go func() {
			timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			resp, targetIDs, err := c.chatService.SendMessage(timeoutCtx, req.ChatID, c.userID, req.Content, req.ReplyToMessageID, req.FileID)
			if err != nil {
				c.log.Error("failed to send message via WS", slog.Any("err", err))
				c.sendError(ctx, event.RequestID, wsErrorMessage(err))
				return
			}

			respPayload, err := json.Marshal(resp)
			if err != nil {
				c.log.Error("failed to marshal response via WS", slog.Any("err", err))
				c.sendError(ctx, event.RequestID, "internal server error")
				return
			}

			responseEvent := WSEvent{
				Type:      EventNewMessage,
				RequestID: event.RequestID,
				Payload:   respPayload,
			}

			finalData, _ := json.Marshal(responseEvent)
			c.hub.BroadcastToUsers(targetIDs, finalData)
		}()

	case EventUserTyping:
		var req WSTypingPayload
		if err := json.Unmarshal(event.Payload, &req); err != nil {
			return
		}

		req.UserID = c.userID
		req.Nickname = c.nickname
		newPayload, _ := json.Marshal(req)

		responseEvent := WSEvent{
			Type:      EventUserTyping,
			RequestID: event.RequestID,
			Payload:   newPayload,
		}

		finalData, _ := json.Marshal(responseEvent)
		c.hub.BroadcastToRooms(req.ChatID, finalData, c.userID)

	case EventMessagesRead:
		var req WSMessagesReadPayload
		if err := json.Unmarshal(event.Payload, &req); err != nil {
			c.sendError(ctx, event.RequestID, "invalid payload for messagesRead")
			return
		}
		go func() {
			timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			targetIDs, err := c.chatService.MarkAsRead(timeoutCtx, req.ChatID, c.userID, req.LastReadMessageID)
			if err != nil {
				c.log.Error("failed to mark messages as read via WS", slog.Any("err", err))
				c.sendError(ctx, event.RequestID, wsErrorMessage(err))
				return
			}

			// Рассылаем серверный payload, а не отражаем клиентский: иначе
			// произвольные/спуфнутые поля ушли бы участникам чата.
			readPayload, _ := json.Marshal(WSMessagesReadPayload{
				ChatID:            req.ChatID,
				LastReadMessageID: req.LastReadMessageID,
			})
			responseEvent := WSEvent{
				Type:      EventMessagesRead,
				RequestID: event.RequestID,
				Payload:   readPayload,
			}

			finalData, _ := json.Marshal(responseEvent)
			c.hub.BroadcastToUsers(targetIDs, finalData)

		}()

	default:
		c.log.Warn("unknown WS event type", slog.String("type", event.Type))
		c.sendError(ctx, event.RequestID, "unknown event type")
	}
}

// markOffline reports the lost connection to presence and, if it was the user's
// last connection anywhere, broadcasts userOffline. Idempotent per connID, so a
// backpressure-drop and the readPump exit can both run without double-firing. It
// uses a fresh context: the connection ctx is already cancelled when this runs,
// which would otherwise abort the SREM/last_seen writes.
func (c *Client) markOffline() {
	if c.presence == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	offline, lastSeen, recipients, err := c.presence.OnDisconnect(ctx, c.userID, c.connID)
	if err != nil {
		c.log.Error("presence: on disconnect", slog.Any("err", err))
		return
	}
	if offline && len(recipients) > 0 {
		ls := lastSeen.Unix()
		c.hub.BroadcastToUsers(recipients, presenceEvent(EventUserOffline, c.userID, false, &ls))
	}
}

// wsErrorMessage turns a service error into a short, client-facing reason so
// the WS peer can tell apart "not a member" / "read-only" / "bad input" from a
// generic failure, mirroring the HTTP status mapping in response.RespondError.
func wsErrorMessage(err error) string {
	switch {
	case errors.Is(err, service.ErrForbidden):
		return "you are not a member of this chat"
	case errors.Is(err, service.ErrChatReadOnly):
		return "chat is read-only"
	case errors.Is(err, service.ErrInvalidInput):
		return "invalid message"
	default:
		return "failed to send message"
	}
}

func (c *Client) sendError(ctx context.Context, requestID, message string) {
	errPayload, err := json.Marshal(map[string]string{"message": message})
	if err != nil {
		c.log.Error("failed to marshal error message", slog.Any("err", err))
		return
	}
	errEvent := WSEvent{
		Type:      EventError,
		RequestID: requestID,
		Payload:   errPayload,
	}
	data, _ := json.Marshal(errEvent)

	select {
	case <-ctx.Done():
		return
	case c.send <- data:
	}
}

func (c *Client) writePump(ctx context.Context, cancel context.CancelFunc) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			// Refresh presence TTL on the same tick as the WS ping so a live
			// connection never decays to offline. Best-effort, off the write path.
			if c.presence != nil {
				go func() {
					hbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := c.presence.Heartbeat(hbCtx, c.userID); err != nil {
						c.log.Error("presence: heartbeat", slog.Any("err", err))
					}
				}()
			}

			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
