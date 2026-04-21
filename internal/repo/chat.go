package repo

import (
	"context"
	"fmt"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

type ChatRepo struct {
	db *bun.DB
}

func NewChatRepo(db *bun.DB) *ChatRepo {
	return &ChatRepo{db: db}
}

func (r *ChatRepo) CreateGroupChat(ctx context.Context, tx bun.IDB, chat *domain.Chat) error {
	_, err := tx.NewInsert().Model(chat).Returning("id").Exec(ctx)
	return err
}

func (r *ChatRepo) AddParticipant(ctx context.Context, tx bun.IDB, chatParticipant *domain.ChatParticipant) error {
	_, err := tx.NewInsert().Model(chatParticipant).Ignore().Exec(ctx)
	return err
}

func (r *ChatRepo) GetChatByMeetupID(ctx context.Context, tx bun.IDB, meetupID int64) (*domain.Chat, error) {
	var chat domain.Chat
	err := tx.NewSelect().Model(&chat).Where("meetup_id = ?", meetupID).Scan(ctx)
	if err != nil {
		return nil, err
	}

	return &chat, nil
}

func (r *ChatRepo) GetUserChats(ctx context.Context, userID int64) ([]domain.Chat, error) {
	var chats []domain.Chat
	err := r.db.NewSelect().Model(&chats).
		ColumnExpr("chat.*").
		ColumnExpr("(SELECT content FROM messages m WHERE m.chat_id = chat.id ORDER BY m.created_at DESC LIMIT 1) AS last_message_text").
		ColumnExpr("(SELECT COUNT(id) FROM messages m WHERE m.chat_id = chat.id AND m.id > cp.last_read_message_id ) AS unread_count").
		Join("JOIN chat_participants cp ON cp.chat_id = chat.id").
		Where("cp.user_id = ?", userID).
		OrderExpr("(SELECT MAX(id) FROM messages WHERE chat_id = chat.id) DESC NULLS LAST").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("getting user's chats failed: %w", err)
	}

	return chats, nil
}
