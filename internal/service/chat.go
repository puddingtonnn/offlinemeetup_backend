package service

import (
	"context"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

type ChatRepository interface {
	CreateGroupChat(ctx context.Context, tx bun.IDB, chat *domain.Chat) error
	AddParticipant(ctx context.Context, tx bun.IDB, chatParticipant *domain.ChatParticipant) error
	GetChatByMeetupID(ctx context.Context, tx bun.IDB, meetupID int64) (*domain.Chat, error)
	GetUserChats(ctx context.Context, userID int64) ([]domain.Chat, error)
}

type ChatService struct {
	repo ChatRepository
}

func NewChatService(repo ChatRepository) *ChatService {
	return &ChatService{repo: repo}
}

func (s *ChatService) GetUserChats(ctx context.Context, userID int64) ([]dto.ChatResponse, error) {
	domainChats, err := s.repo.GetUserChats(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chats: %w", err)
	}

	dtos := make([]dto.ChatResponse, 0, len(domainChats))

	for _, c := range domainChats {
		chatDTO := dto.ChatResponse{
			ID:              c.ID,
			Type:            c.Type,
			MeetupID:        c.MeetupID,
			Title:           c.Title,
			LastMessageText: c.LastMessageText,
			UnreadCount:     c.UnreadCount,
		}
		dtos = append(dtos, chatDTO)
	}

	return dtos, nil
}
