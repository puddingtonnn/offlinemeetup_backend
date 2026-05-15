package websocket

import (
	"encoding/json"
)

const (
	EventNewMessage = "newMessage"
	EventUserTyping = "userTyping"
	EventError      = "error"
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
	ChatID int64 `json:"chat_id"`
}
