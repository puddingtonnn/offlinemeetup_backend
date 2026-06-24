package domain

import (
	"time"

	"github.com/google/uuid"
)

type Chat struct {
	ID              int64     `bun:",pk,autoincrement"`
	Type            string    `bun:"type,notnull"`
	MeetupID        *int64    `bun:"meetup_id"`
	Title           string    `bun:"title"`
	IsReadOnly      bool      `bun:"is_read_only,notnull,default:false"`
	CreatedAt       time.Time `bun:",nullzero,notnull,default:current_timestamp"`
	LastMessageText string    `bun:"last_message_text,scanonly"`
	UnreadCount     int       `bun:"unread_count,scanonly"`

	Meetup *Meetup `bun:"rel:belongs-to,join:meetup_id=id"`
}

type ChatParticipant struct {
	ChatID            int64     `bun:"chat_id,pk"`
	UserID            int64     `bun:"user_id,pk"`
	JoinedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp"`
	LastReadMessageID int64     `bun:"last_read_message_id,default:0"`

	Chat *Chat `bun:"rel:belongs-to,join:chat_id=id"`
	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

type Message struct {
	ID               int64         `bun:",pk,autoincrement"`
	ChatID           int64         `bun:"chat_id,notnull"`
	SenderID         int64         `bun:"sender_id,notnull"`
	Content          string        `bun:"content,notnull"`
	MessageType      string        `bun:"message_type,notnull,default:'text'"`
	ReplyToMessageID *int64        `bun:"reply_to_message_id"`
	FileID           uuid.NullUUID `bun:"file_id,type:uuid"`
	EditedAt         *time.Time    `bun:"edited_at"`
	DeletedAt        *time.Time    `bun:"deleted_at"`
	CreatedAt        time.Time     `bun:",nullzero,notnull,default:current_timestamp"`

	Sender  *User    `bun:"rel:belongs-to,join:sender_id=id"`
	ReplyTo *Message `bun:"rel:belongs-to,join:reply_to_message_id=id"`
	File    *File    `bun:"rel:belongs-to,join:file_id=id"`
}
