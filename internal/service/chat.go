package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/cache"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
	"github.com/uptrace/bun"
)

type ChatRepository interface {
	CreateGroupChat(ctx context.Context, tx bun.IDB, chat *domain.Chat) error
	AddParticipant(ctx context.Context, tx bun.IDB, chatParticipant *domain.ChatParticipant) error
	GetChatByMeetupID(ctx context.Context, tx bun.IDB, meetupID int64) (*domain.Chat, error)
	GetUserChats(ctx context.Context, userID int64) ([]domain.Chat, error)
	GetMessages(ctx context.Context, userID, chatID, cursor int64, limit int) ([]domain.Message, error)
	SaveMessage(ctx context.Context, msg *domain.Message) (*domain.Message, []int64, error)
	EditMessage(ctx context.Context, msgID, editorID int64, content string) (*domain.Message, []int64, error)
	DeleteMessage(ctx context.Context, msgID, editorID int64) (int64, []int64, error)
	MarkAsRead(ctx context.Context, chatID, userID, lastReadMessageID int64) error
	GetChatParticipantIDs(ctx context.Context, chatID int64) ([]int64, error)
	GetCoChatUserIDs(ctx context.Context, userID int64) ([]int64, error)
}

type ChatService struct {
	repo        ChatRepository
	cache       *cache.ChatCache
	s3PublicURL string
}

func NewChatService(repo ChatRepository, chatCache *cache.ChatCache, s3PublicURL string) *ChatService {
	return &ChatService{repo: repo, cache: chatCache, s3PublicURL: s3PublicURL}
}

func (s *ChatService) GetUserChats(ctx context.Context, userID int64) ([]dto.ChatResponse, error) {
	return s.cache.UserChats(ctx, userID, func() ([]dto.ChatResponse, error) {
		domainChats, err := s.repo.GetUserChats(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to get chats: %w", err)
		}

		dtos := make([]dto.ChatResponse, 0, len(domainChats))
		for _, c := range domainChats {
			if resp := s.mapChatToResponse(&c); resp != nil {
				dtos = append(dtos, *resp)
			}
		}
		return dtos, nil
	})
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

func (s *ChatService) SendMessage(ctx context.Context, chatID, senderID int64, content string, replyToMessageID *int64, fileID *string) (*dto.MessageResponse, []int64, error) {
	content = strings.TrimSpace(content)

	hasAttachment := fileID != nil && *fileID != ""
	// With an attachment the text (caption) may be empty; without one it may not.
	if err := validateMessageContent(content, hasAttachment); err != nil {
		return nil, nil, err
	}

	msg := &domain.Message{
		ChatID:           chatID,
		SenderID:         senderID,
		Content:          content,
		MessageType:      "text",
		ReplyToMessageID: replyToMessageID,
	}

	if hasAttachment {
		id, err := uuid.Parse(*fileID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid file id: %w", ErrInvalidInput)
		}
		msg.FileID = uuid.NullUUID{UUID: id, Valid: true}
		msg.MessageType = "file" // precise kind is carried by attachment.mime_type
	}

	savedMsg, targetIDs, err := s.repo.SaveMessage(ctx, msg)
	if err != nil {
		return nil, nil, mapChatRepoError(err)
	}

	// Best-effort: a stale cache must not fail an already-saved message; the
	// cache layer logs any failure.
	_ = s.cache.InvalidateUserChatsMany(ctx, targetIDs...)

	return s.mapMessageToResponse(savedMsg), targetIDs, nil
}

// EditMessage updates an existing message's text. Only the author may edit, and
// not a deleted message (enforced in the repo). Returns the updated message and
// the chat's participant IDs for broadcast.
func (s *ChatService) EditMessage(ctx context.Context, msgID, editorID int64, content string) (*dto.MessageResponse, []int64, error) {
	content = strings.TrimSpace(content)
	if err := validateMessageContent(content, false); err != nil {
		return nil, nil, err
	}

	updated, targetIDs, err := s.repo.EditMessage(ctx, msgID, editorID, content)
	if err != nil {
		return nil, nil, mapChatRepoError(err)
	}

	// Editing the last message changes the chats list preview text.
	_ = s.cache.InvalidateUserChatsMany(ctx, targetIDs...)

	return s.mapMessageToResponse(updated), targetIDs, nil
}

// DeleteMessage soft-deletes a message (author only). Returns the chat ID and
// participant IDs for broadcast.
func (s *ChatService) DeleteMessage(ctx context.Context, msgID, editorID int64) (int64, []int64, error) {
	chatID, targetIDs, err := s.repo.DeleteMessage(ctx, msgID, editorID)
	if err != nil {
		return 0, nil, mapChatRepoError(err)
	}

	_ = s.cache.InvalidateUserChatsMany(ctx, targetIDs...)

	return chatID, targetIDs, nil
}

// validateMessageContent rejects over-long bodies, and empty ones unless
// allowEmpty (a message that carries an attachment may have no text).
func validateMessageContent(content string, allowEmpty bool) error {
	switch {
	case content == "" && !allowEmpty:
		return fmt.Errorf("empty message: %w", ErrInvalidInput)
	case len(content) > 4096:
		return fmt.Errorf("message too long: %w", ErrInvalidInput)
	default:
		return nil
	}
}

func (s *ChatService) mapMessageToResponse(m *domain.Message) *dto.MessageResponse {
	if m == nil {
		return nil
	}

	var senderDTO *dto.ProfileResponse
	if m.Sender != nil {
		senderDTO = mapProfileToDTO(m.Sender.Profile, s.s3PublicURL)
	}

	isDeleted := m.DeletedAt != nil
	content := m.Content
	if isDeleted {
		content = "" // never leak a soft-deleted body
	}

	// A deleted message hides its attachment too.
	var attachment *dto.AttachmentResponse
	if !isDeleted && m.File != nil {
		attachment = &dto.AttachmentResponse{
			URL:      publicURL(s.s3PublicURL, m.File),
			FileName: m.File.FileName,
			MimeType: m.File.MimeType,
			Size:     m.File.Size,
		}
	}

	return &dto.MessageResponse{
		ID:          m.ID,
		ChatID:      m.ChatID,
		SenderID:    m.SenderID,
		Sender:      senderDTO,
		Content:     content,
		MessageType: m.MessageType,
		Attachment:  attachment,
		ReplyTo:     mapMessagePreview(m.ReplyTo),
		EditedAt:    m.EditedAt,
		IsDeleted:   isDeleted,
		CreatedAt:   m.CreatedAt,
	}
}

// mapMessagePreview builds the compact quoted-message preview for a reply. A
// deleted target is shown as a tombstone (blank content, is_deleted=true).
func mapMessagePreview(m *domain.Message) *dto.MessagePreview {
	if m == nil {
		return nil
	}

	isDeleted := m.DeletedAt != nil
	content := m.Content
	if isDeleted {
		content = ""
	}

	var nickname string
	if m.Sender != nil && m.Sender.Profile != nil {
		nickname = m.Sender.Profile.Nickname
	}

	return &dto.MessagePreview{
		ID:             m.ID,
		SenderID:       m.SenderID,
		SenderNickname: nickname,
		Content:        content,
		IsDeleted:      isDeleted,
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
	_ = s.cache.InvalidateUserChats(ctx, userID) // best-effort; cache layer logs failures

	targetIDs, err := s.repo.GetChatParticipantIDs(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants for broadcast: %w", err)
	}

	return targetIDs, nil
}

// mapChatRepoError translates infrastructure errors from the repo into the
// service's domain sentinels at the layer boundary, so handlers map them to the
// right HTTP status (403/404/409) instead of a blanket 500. The original cause
// is kept via %w for logging and errors.Is.
func mapChatRepoError(err error) error {
	switch {
	case errors.Is(err, repo.ErrNotChatMember), errors.Is(err, repo.ErrNotMessageAuthor):
		return fmt.Errorf("chat access denied: %w", ErrForbidden)
	case errors.Is(err, repo.ErrChatReadOnly):
		return fmt.Errorf("chat: %w", ErrChatReadOnly)
	case errors.Is(err, repo.ErrMessageNotFound):
		return fmt.Errorf("message: %w", ErrNotFound)
	default:
		return fmt.Errorf("chat repo error: %w", err)
	}
}

func (s *ChatService) GetChatParticipantIDs(ctx context.Context, chatID int64) ([]int64, error) {
	participants, err := s.repo.GetChatParticipantIDs(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants ids: %w", err)
	}
	return participants, nil
}

// GetCoChatUserIDs returns the IDs of users who share a chat with userID. Used
// by presence to address online/offline notifications.
func (s *ChatService) GetCoChatUserIDs(ctx context.Context, userID int64) ([]int64, error) {
	ids, err := s.repo.GetCoChatUserIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get co-chat user ids: %w", err)
	}
	return ids, nil
}
