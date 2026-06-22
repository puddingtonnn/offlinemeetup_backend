package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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
	MarkAsRead(ctx context.Context, chatID, userID, lastReadMessageID int64) error
	GetChatParticipantIDs(ctx context.Context, chatID int64) ([]int64, error)
}

type ChatService struct {
	repo        ChatRepository
	rdb         *redis.Client
	log         *slog.Logger
	s3PublicURL string
}

func NewChatService(repo ChatRepository, rdb *redis.Client, log *slog.Logger, s3PublicURL string) *ChatService {
	return &ChatService{repo: repo, rdb: rdb, log: log, s3PublicURL: s3PublicURL}
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
		resp := s.mapChatToResponse(&c)
		if resp != nil {
			dtos = append(dtos, *resp)
		}
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
		resp := s.mapMessageToResponse(&m)
		if resp != nil {
			dtos = append(dtos, *resp)
		}
	}
	return dtos, nil
}

func (s *ChatService) SendMessage(ctx context.Context, chatID, senderID int64, content string) (*dto.MessageResponse, []int64, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, nil, fmt.Errorf("empty message: %w", ErrInvalidInput)
	}
	if len(content) > 4096 {
		return nil, nil, fmt.Errorf("message too long: %w", ErrInvalidInput)
	}

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

	return s.mapMessageToResponse(savedMsg), targetIDs, nil
}

func (s *ChatService) mapMessageToResponse(m *domain.Message) *dto.MessageResponse {
	if m == nil {
		return nil
	}

	var senderDTO *dto.ProfileResponse
	if m.Sender != nil {
		senderDTO = mapProfileToDTO(m.Sender.Profile, s.s3PublicURL)
	}

	return &dto.MessageResponse{
		ID:          m.ID,
		ChatID:      m.ChatID,
		SenderID:    m.SenderID,
		Sender:      senderDTO,
		Content:     m.Content,
		MessageType: m.MessageType,
		CreatedAt:   m.CreatedAt,
	}
}

func (s *ChatService) mapChatToResponse(c *domain.Chat) *dto.ChatResponse {
	if c == nil {
		return nil
	}

	var meetupDTO *dto.MeetupResponse
	if c.Meetup != nil {
		meetupDTO = s.mapMeetupToResponse(c.Meetup)
	}

	title := c.Title
	if title == "" && meetupDTO != nil {
		title = meetupDTO.Title
	}

	return &dto.ChatResponse{
		ID:              c.ID,
		Type:            c.Type,
		MeetupID:        c.MeetupID,
		Meetup:          meetupDTO,
		Title:           title,
		IsReadOnly:      c.IsReadOnly,
		LastMessageText: c.LastMessageText,
		UnreadCount:     c.UnreadCount,
	}
}

func (s *ChatService) mapMeetupToResponse(m *domain.Meetup) *dto.MeetupResponse {
	return mapMeetupToDTO(m, s.s3PublicURL)
}

func (s *ChatService) MarkAsRead(ctx context.Context, chatID, userID, lastReadMessageID int64) ([]int64, error) {
	if err := s.repo.MarkAsRead(ctx, chatID, userID, lastReadMessageID); err != nil {
		return nil, err
	}
	s.rdb.Del(ctx, fmt.Sprintf("user_chats:%d", userID))

	targetIDs, err := s.repo.GetChatParticipantIDs(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants for broadcast: %w", err)
	}

	return targetIDs, nil
}

func (s *ChatService) GetChatParticipantIDs(ctx context.Context, chatID int64) ([]int64, error) {
	participants, err := s.repo.GetChatParticipantIDs(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants ids: %w", err)
	}
	return participants, nil
}
