package dto

import "time"

type ChatResponse struct {
	ID              int64           `json:"id"`
	Type            string          `json:"type"`
	MeetupID        *int64          `json:"meetup_id,omitempty"`
	Meetup          *MeetupResponse `json:"meetup,omitempty"`
	Title           string          `json:"title"`
	IsReadOnly      bool            `json:"is_read_only"`
	LastMessageText string          `json:"last_message_text"`
	UnreadCount     int             `json:"unread_count"`
}

type MessageResponse struct {
	ID          int64               `json:"id"`
	ChatID      int64               `json:"chat_id"`
	SenderID    int64               `json:"sender_id"`
	Sender      *ProfileResponse    `json:"sender"`
	Content     string              `json:"content"`
	MessageType string              `json:"message_type"`
	Attachment  *AttachmentResponse `json:"attachment,omitempty"`
	ReplyTo     *MessagePreview     `json:"reply_to,omitempty"`
	RequestID   *string             `json:"request_id,omitempty"`
	EditedAt    *time.Time          `json:"edited_at,omitempty"`
	IsDeleted   bool                `json:"is_deleted"`
	CreatedAt   time.Time           `json:"created_at"`
}

// AttachmentResponse is a file attached to a message, with a ready public URL.
type AttachmentResponse struct {
	URL      string `json:"url"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

// MessagePreview is a compact quoted message shown above a reply.
type MessagePreview struct {
	ID             int64  `json:"id"`
	SenderID       int64  `json:"sender_id"`
	SenderNickname string `json:"sender_nickname,omitempty"`
	Content        string `json:"content"`
	IsDeleted      bool   `json:"is_deleted"`
}

// PresenceResponse is one chat member's online state. LastSeen is a unix
// timestamp (seconds), present only for offline members with a known last visit.
type PresenceResponse struct {
	UserID   int64  `json:"user_id"`
	Online   bool   `json:"online"`
	LastSeen *int64 `json:"last_seen,omitempty"`
}
