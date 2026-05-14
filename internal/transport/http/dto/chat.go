package dto

import "time"

type ChatResponse struct {
	ID              int64  `json:"id"`
	Type            string `json:"type"`
	MeetupID        *int64 `json:"meetup_id,omitempty"`
	Title           string `json:"title"`
	LastMessageText string `json:"last_message_text"`
	UnreadCount     int    `json:"unread_count"`
}

type MessageResponse struct {
	ID          int64            `json:"id"`
	ChatID      int64            `json:"chat_id"`
	SenderID    int64            `json:"sender_id"`
	Sender      *ProfileResponse `json:"sender"`
	Content     string           `json:"content"`
	MessageType string           `json:"message_type"`
	CreatedAt   time.Time        `json:"created_at"`
}
