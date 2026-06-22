package service

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo/mocks"
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
