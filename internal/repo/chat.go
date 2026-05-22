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

func (r *ChatRepo) RemoveParticipant(ctx context.Context, tx bun.IDB, chatID, userID int64) error {
	_, err := tx.NewDelete().
		Table("chat_participants").
		Where("chat_id = ? AND user_id = ?", chatID, userID).
		Exec(ctx)
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
		ColumnExpr("(SELECT content FROM messages m2 WHERE m2.chat_id = chat.id ORDER BY m2.created_at DESC LIMIT 1) AS last_message_text").
		ColumnExpr("(SELECT COUNT(id) FROM messages m2 WHERE m2.chat_id = chat.id AND m2.id > cp.last_read_message_id ) AS unread_count").
		Relation("Meetup").
		Relation("Meetup.Creator").
		Relation("Meetup.Creator.Profile").
		Relation("Meetup.Creator.Profile.AvatarFile").
		Relation("Meetup.Tags").
		Relation("Meetup.CoverFile").
		Join("JOIN chat_participants cp ON cp.chat_id = chat.id").
		Where("cp.user_id = ?", userID).
		OrderExpr("GREATEST(COALESCE((SELECT MAX(created_at) FROM messages WHERE chat_id = chat.id), '1970-01-01'::timestamp), chat.created_at) DESC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("getting user's chats failed: %w", err)
	}

	return chats, nil
}

func (r *ChatRepo) GetMessages(ctx context.Context, userID, chatID, cursor int64, limit int) ([]domain.Message, error) {
	var messages []domain.Message

	query := r.db.NewSelect().Model(&messages).
		Relation("Sender").
		Relation("Sender.Profile").
		Relation("Sender.Profile.AvatarFile").
		Where("message.chat_id = ? AND EXISTS (SELECT 1 FROM chat_participants WHERE chat_id = ? AND user_id = ?)", chatID, chatID, userID)

	if cursor > 0 {
		query = query.Where("message.id < ?", cursor)
	}

	err := query.Order("message.id DESC").Limit(limit).Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("getting messages failed: %w", err)
	}

	return messages, nil
}

func (r *ChatRepo) SaveMessage(ctx context.Context, msg *domain.Message) (*domain.Message, []int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	exists, err := tx.NewSelect().
		TableExpr("chat_participants").
		Where("chat_id = ? AND user_id = ?", msg.ChatID, msg.SenderID).
		Exists(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("checking if chat_participants exists failed: %w", err)
	}
	if !exists {
		return nil, nil, fmt.Errorf("access denied: user is not a member of this chat")
	}

	_, err = tx.NewInsert().Model(msg).Returning("id, created_at").Exec(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to insert message: %w", err)
	}

	_, err = tx.NewUpdate().Table("chat_participants").Set("last_read_message_id = ?", msg.ID).
		Where("chat_id = ? AND user_id = ?", msg.ChatID, msg.SenderID).
		Exec(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to update sender read status: %w", err)
	}

	var targetIDs []int64
	err = tx.NewSelect().
		TableExpr("chat_participants").
		Column("user_id").
		Where("chat_id = ?", msg.ChatID).
		Scan(ctx, &targetIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get targetIDs: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	err = r.db.NewSelect().Model(msg).
		Relation("Sender").
		Relation("Sender.Profile").
		Relation("Sender.Profile.AvatarFile").
		WherePK().
		Scan(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load sender after save: %w", err)
	}

	return msg, targetIDs, nil
}

func (r *ChatRepo) MarkAsRead(ctx context.Context, chatID, userID, lastReadMessageID int64) error {
	_, err := r.db.NewUpdate().
		Table("chat_participants").
		Set("last_read_message_id = ?", lastReadMessageID).
		Where("chat_id = ? AND user_id = ?", chatID, userID).
		Where("last_read_message_id < ?", lastReadMessageID).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to mark message as read: %w", err)
	}
	return nil
}

func (r *ChatRepo) GetChatParticipantIDs(ctx context.Context, chatID int64) ([]int64, error) {
	var targetIDs []int64
	err := r.db.NewSelect().
		TableExpr("chat_participants").
		Column("user_id").
		Where("chat_id = ?", chatID).
		Scan(ctx, &targetIDs)

	return targetIDs, err
}
