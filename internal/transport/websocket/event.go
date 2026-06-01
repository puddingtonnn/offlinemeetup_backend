package websocket

import (
	"encoding/json"
)

const (
	EventNewMessage   = "newMessage"
	EventUserTyping   = "userTyping"
	EventError        = "error"
	EventMessagesRead = "messagesRead"
	EventUserOnline   = "userOnline"
)

type WSEvent struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type WSSendMessagePayload struct {
	ChatID  int64  `json:"chat_id"`
	Content string `json:"content"`
}

type WSTypingPayload struct {
	ChatID   int64  `json:"chat_id"`
	UserID   int64  `json:"user_id,omitempty"`
	Nickname string `json:"nickname,omitempty"`
}

type WSMessagesReadPayload struct {
	ChatID            int64 `json:"chat_id"`
	LastReadMessageID int64 `json:"last_read_message_id"`
}
