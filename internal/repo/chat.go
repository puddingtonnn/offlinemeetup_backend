package repo

import (
	"context"

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
	_, err := tx.NewInsert().Model(chatParticipant).Exec(ctx)
	return err
}
func (r *ChatRepo) GetChatByMeetupID(ctx context.Context, tx bun.IDB, meetupID int64) int64 {
	var chat domain.Chat
}
