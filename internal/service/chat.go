package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
	"github.com/redis/go-redis/v9"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

type ChatRepository interface {
	CreateGroupChat(ctx context.Context, tx bun.IDB, chat *domain.Chat) error
	AddParticipant(ctx context.Context, tx bun.IDB, chatParticipant *domain.ChatParticipant) error
	GetChatByMeetupID(ctx context.Context, tx bun.IDB, meetupID int64) (*domain.Chat, error)
	GetUserChats(ctx context.Context, userID int64) ([]domain.Chat, error)
	GetMessages(ctx context.Context, userID, chatID, cursor int64, limit int) ([]domain.Message, error)
	SaveMessage(ctx context.Context, msg *domain.Message) (*domain.Message, []int64, error)
}

type ChatService struct {
	repo ChatRepository
	rdb  *redis.Client
	log  *slog.Logger
}

func NewChatService(repo ChatRepository, rdb *redis.Client, log *slog.Logger) *ChatService {
	return &ChatService{repo: repo, rdb: rdb, log: log}
}

func (s *ChatService) GetUserChats(ctx context.Context, userID int64) ([]dto.ChatResponse, error) {
	cacheKey := fmt.Sprintf("user_chats:%d", userID)
	cachedData, err := s.rdb.Get(ctx, cacheKey).Result()

	if err == nil {
		var chats []dto.ChatResponse
		if err := json.Unmarshal([]byte(cachedData), &chats); err == nil {
			return chats, nil
		}
	} else if err != redis.Nil {
		s.log.Error("Redis GET error", slog.Any("error", err))
	}

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

	if dtosToCache, err := json.Marshal(dtos); err == nil {
		s.rdb.Set(ctx, cacheKey, dtosToCache, time.Minute*5)
	}

	return dtos, nil
}

func (s *ChatService) GetMessages(ctx context.Context, userID, chatID, cursor int64, limit int) ([]dto.MessageResponse, error) {
	domainMessages, err := s.repo.GetMessages(ctx, userID, chatID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	dtos := make([]dto.MessageResponse, 0, len(domainMessages))

	for _, m := range domainMessages {
		messageDTO := dto.MessageResponse{
			ID:          m.ID,
			ChatID:      m.ChatID,
			SenderID:    m.SenderID,
			Content:     m.Content,
			MessageType: m.MessageType,
			CreatedAt:   m.CreatedAt,
		}
		dtos = append(dtos, messageDTO)
	}
	return dtos, nil
}

func (s *ChatService) SendMessage(ctx context.Context, chatID, senderID int64, content string) (*dto.MessageResponse, []int64, error) {
	msg := &domain.Message{
		ChatID:      chatID,
		SenderID:    senderID,
		Content:     content,
		MessageType: "text",
	}

	savedMsg, targetIDs, err := s.repo.SaveMessage(ctx, msg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to save message: %w", err)
	}

	for _, targetID := range targetIDs {
		s.rdb.Del(ctx, fmt.Sprintf("user_chats:%d", targetID))
	}

	response := &dto.MessageResponse{
		ID:          savedMsg.ID,
		ChatID:      savedMsg.ChatID,
		SenderID:    savedMsg.SenderID,
		Content:     savedMsg.Content,
		MessageType: savedMsg.MessageType,
		CreatedAt:   savedMsg.CreatedAt,
	}

	return response, targetIDs, nil
}
