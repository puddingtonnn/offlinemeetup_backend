package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo/mocks"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
)

func setupChatTest(t *testing.T) (*miniredis.Miniredis, *redis.Client, *mocks.MockChatRepository, *ChatService) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockChatRepository(ctrl)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s3URL := "http://s3.example.com"
	svc := NewChatService(mockRepo, rdb, logger, s3URL)

	return mr, rdb, mockRepo, svc
}

func TestChatService_GetMessages(t *testing.T) {
	mr, _, mockRepo, svc := setupChatTest(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(1)
	chatID := int64(100)

	now := time.Now()
	mockRepo.EXPECT().
		GetMessages(ctx, userID, chatID, int64(0), 10).
		Return([]domain.Message{
			{ID: 1, ChatID: chatID, SenderID: userID, Content: "Hello", CreatedAt: now},
			{ID: 2, ChatID: chatID, SenderID: 2, Content: "Hi", CreatedAt: now},
		}, nil)

	resp, err := svc.GetMessages(ctx, userID, chatID, 0, 10)
	require.NoError(t, err)
	require.Len(t, resp, 2)
	assert.Equal(t, int64(1), resp[0].ID)
	assert.Equal(t, int64(2), resp[1].ID)
}

func TestChatService_SendMessage(t *testing.T) {
	mr, _, mockRepo, svc := setupChatTest(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(1)
	chatID := int64(100)
	content := "Hello World"

	mockRepo.EXPECT().
		SaveMessage(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, msg *domain.Message) (*domain.Message, []int64, error) {
			assert.Equal(t, chatID, msg.ChatID)
			assert.Equal(t, userID, msg.SenderID)
			assert.Equal(t, content, msg.Content)
			
			msg.ID = 10
			return msg, []int64{1, 2, 3}, nil
		})

	resp, targetIDs, err := svc.SendMessage(ctx, chatID, userID, content)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(10), resp.ID)
	assert.Equal(t, content, resp.Content)
	assert.ElementsMatch(t, []int64{1, 2, 3}, targetIDs)

	// verify cache invalidation
	assert.False(t, mr.Exists("user_chats:1"))
	assert.False(t, mr.Exists("user_chats:2"))
	assert.False(t, mr.Exists("user_chats:3"))
}

func TestChatService_GetUserChats(t *testing.T) {
	ctx := context.Background()
	userID := int64(1)

	t.Run("cache miss loads from repo and caches", func(t *testing.T) {
		mr, _, mockRepo, svc := setupChatTest(t)
		defer mr.Close()

		meetupID := int64(500)
		mockRepo.EXPECT().
			GetUserChats(ctx, userID).
			Return([]domain.Chat{
				// Чат с привязанным митапом — задействует mapMeetupToResponse,
				// при пустом Title заголовок берётся из митапа.
				{ID: 10, Type: "group", MeetupID: &meetupID, Meetup: &domain.Meetup{ID: meetupID, Title: "Митап A"}},
				{ID: 11, Type: "group", Title: "Chat B"},
			}, nil)

		resp, err := svc.GetUserChats(ctx, userID)
		require.NoError(t, err)
		require.Len(t, resp, 2)
		assert.Equal(t, int64(10), resp[0].ID)
		assert.Equal(t, "Митап A", resp[0].Title)
		require.NotNil(t, resp[0].Meetup)

		// Результат должен быть закэширован.
		assert.True(t, mr.Exists("user_chats:1"))
	})

	t.Run("cache hit returns without touching repo", func(t *testing.T) {
		mr, rdb, _, svc := setupChatTest(t)
		defer mr.Close()

		cached, _ := json.Marshal([]dto.ChatResponse{{ID: 99, Title: "Cached"}})
		require.NoError(t, rdb.Set(ctx, "user_chats:1", cached, 0).Err())

		// Отсутствие EXPECT на mockRepo => вызов репозитория провалит тест.
		resp, err := svc.GetUserChats(ctx, userID)
		require.NoError(t, err)
		require.Len(t, resp, 1)
		assert.Equal(t, int64(99), resp[0].ID)
		assert.Equal(t, "Cached", resp[0].Title)
	})
}

func TestChatService_MarkAsRead(t *testing.T) {
	mr, rdb, mockRepo, svc := setupChatTest(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(1)
	chatID := int64(100)
	lastReadID := int64(50)

	// Заранее кладём кэш, чтобы проверить его инвалидацию.
	require.NoError(t, rdb.Set(ctx, "user_chats:1", "[]", 0).Err())

	mockRepo.EXPECT().MarkAsRead(ctx, chatID, userID, lastReadID).Return(nil)
	mockRepo.EXPECT().GetChatParticipantIDs(ctx, chatID).Return([]int64{1, 2, 3}, nil)

	targetIDs, err := svc.MarkAsRead(ctx, chatID, userID, lastReadID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 2, 3}, targetIDs)

	// Кэш читателя должен быть сброшен.
	assert.False(t, mr.Exists("user_chats:1"))
}

func TestChatService_SendMessage_Validation(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects empty content", func(t *testing.T) {
		mr, _, _, svc := setupChatTest(t)
		defer mr.Close()

		// Репозиторий не должен вызываться (нет EXPECT).
		_, _, err := svc.SendMessage(ctx, 100, 1, "   ")
		require.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("rejects too long content", func(t *testing.T) {
		mr, _, _, svc := setupChatTest(t)
		defer mr.Close()

		_, _, err := svc.SendMessage(ctx, 100, 1, strings.Repeat("a", 4097))
		require.ErrorIs(t, err, ErrInvalidInput)
	})
}

func TestChatService_GetChatParticipantIDs(t *testing.T) {
	mr, _, mockRepo, svc := setupChatTest(t)
	defer mr.Close()

	ctx := context.Background()
	chatID := int64(100)

	mockRepo.EXPECT().GetChatParticipantIDs(ctx, chatID).Return([]int64{1, 2, 3}, nil)

	ids, err := svc.GetChatParticipantIDs(ctx, chatID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 2, 3}, ids)
}
