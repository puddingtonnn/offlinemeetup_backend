package dto

type ChatResponse struct {
	ID              int64  `json:"id"`
	Type            string `json:"type"`
	MeetupID        *int64 `json:"meetup_id,omitempty"`
	Title           string `json:"title"`
	LastMessageText string `json:"last_message_text"`
	UnreadCount     int    `json:"unread_count"`
}
