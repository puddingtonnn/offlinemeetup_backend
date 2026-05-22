package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
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

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	userID      int64
	hub         *Hub
	conn        *websocket.Conn
	send        chan []byte
	chatService *service.ChatService
	log         *slog.Logger
	rooms       []int64
}

func (c *Client) readPump(ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		cancel()
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.log.Error("websocket unexpected close", slog.Any("err", err))
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

			resp, targetIDs, err := c.chatService.SendMessage(timeoutCtx, req.ChatID, c.userID, req.Content)
			if err != nil {
				c.log.Error("failed to send message via WS", slog.Any("err", err))
				c.sendError(ctx, event.RequestID, "failed to send message")
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
		newPayload, _ := json.Marshal(req)

		responseEvent := WSEvent{
			Type:      EventUserTyping,
			RequestID: event.RequestID,
			Payload:   newPayload,
		}

		finalData, _ := json.Marshal(responseEvent)
		c.hub.BroadcastToRooms(req.ChatID, finalData, c)

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
				c.sendError(ctx, event.RequestID, "failed to mark messages as read")
				return
			}

			responseEvent := WSEvent{
				Type:      EventMessagesRead,
				RequestID: event.RequestID,
				Payload:   event.Payload,
			}

			finalData, _ := json.Marshal(responseEvent)
			c.hub.BroadcastToUsers(targetIDs, finalData)

		}()

	default:
		c.log.Warn("unknown WS event type", slog.String("type", event.Type))
		c.sendError(ctx, event.RequestID, "unknown event type")
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
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
