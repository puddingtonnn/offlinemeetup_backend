package domain

import "time"

type Chat struct {
	ID       int64     `bun:",pk,autoincrement"`
	Type     string    `bun:"type,notnull"`
	MeetupID *int64    `bun:"meetup_id"`
	Title    string    `bun:"title"`
	Created  time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}

type ChatParticipant struct {
	ChatID            int64     `bun:"chat_id,pk"`
	UserID            int64     `bun:"user_id,pk"`
	JoinedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp"`
	LastReadMessageID int64     `bun:"last_read_message_id,default:0"`
}

type Message struct {
	ID          int64     `bun:",pk,autoincrement"`
	ChatID      int64     `bun:"chat_id,notnull"`
	SenderID    int64     `bun:"sender_id,notnull"`
	Content     string    `bun:"content,notnull"`
	MessageType string    `bun:"message_type,notnull,default:'text'"`
	CreatedAt   time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}
