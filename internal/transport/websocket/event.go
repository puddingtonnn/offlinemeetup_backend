package websocket

import (
	"encoding/json"
)

const (
	EventNewMessage       = "newMessage"
	EventMessageEdited    = "messageEdited"
	EventMessageDeleted   = "messageDeleted"
	EventUserTyping       = "userTyping"
	EventError            = "error"
	EventMessagesRead     = "messagesRead"
	EventUserOnline       = "userOnline"
	EventUserOffline      = "userOffline"
	EventPresenceSnapshot = "presenceSnapshot"
)

type WSEvent struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type WSSendMessagePayload struct {
	ChatID           int64   `json:"chat_id"`
	Content          string  `json:"content"`
	ReplyToMessageID *int64  `json:"reply_to_message_id,omitempty"`
	FileID           *string `json:"file_id,omitempty"`
}

// WSMessageDeletedPayload is broadcast when a message is soft-deleted.
type WSMessageDeletedPayload struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int64 `json:"message_id"`
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

// WSPresencePayload carries one user's online state. Nickname is the display
// name so clients render presence without a second lookup. LastSeen is a unix
// timestamp, set only for offline transitions/snapshots.
type WSPresencePayload struct {
	UserID   int64  `json:"user_id"`
	Online   bool   `json:"online"`
	Nickname string `json:"nickname,omitempty"`
	LastSeen *int64 `json:"last_seen,omitempty"`
}

// WSPresenceSnapshotPayload is the initial presence state of a chat's members,
// pushed to a client right after it connects.
type WSPresenceSnapshotPayload struct {
	Users []WSPresencePayload `json:"users"`
}
