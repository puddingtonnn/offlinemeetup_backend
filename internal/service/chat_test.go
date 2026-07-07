package service

import (
	"context"
	"encoding/json"
	"errors"
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

	"github.com/puddingtonnn/offlinemeetup_backend/internal/cache"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
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
	chatCache := cache.NewChatCache(cache.NewRedisCache(rdb, logger), cache.NopMetrics, time.Minute)
	svc := NewChatService(mockRepo, chatCache, s3URL)

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
		DoAndReturn(func(ctx context.Context, msg *domain.Message) (*domain.Message, []int64, bool, error) {
			assert.Equal(t, chatID, msg.ChatID)
			assert.Equal(t, userID, msg.SenderID)
			assert.Equal(t, content, msg.Content)

			msg.ID = 10
			return msg, []int64{1, 2, 3}, true, nil
		})

	resp, targetIDs, created, err := svc.SendMessage(ctx, chatID, userID, content, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, created)
	assert.Equal(t, int64(10), resp.ID)
	assert.Equal(t, content, resp.Content)
	assert.ElementsMatch(t, []int64{1, 2, 3}, targetIDs)

	// verify cache invalidation
	assert.False(t, mr.Exists("user_chats:1"))
	assert.False(t, mr.Exists("user_chats:2"))
	assert.False(t, mr.Exists("user_chats:3"))
}

// TestChatService_SendMessage_Idempotent — при повторной отправке с тем же
// request_id репозиторий возвращает created=false; сервис прокидывает флаг и НЕ
// инвалидирует кэш (сообщение не новое, превью чатов не изменилось).
func TestChatService_SendMessage_Idempotent(t *testing.T) {
	mr, rdb, mockRepo, svc := setupChatTest(t)
	defer mr.Close()

	ctx := context.Background()
	reqID := "idem-key-1"
	require.NoError(t, rdb.Set(ctx, "user_chats:2", "[]", 0).Err())

	mockRepo.EXPECT().
		SaveMessage(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, msg *domain.Message) (*domain.Message, []int64, bool, error) {
			require.NotNil(t, msg.RequestID, "request_id must be threaded to the repo")
			assert.Equal(t, reqID, *msg.RequestID)
			msg.ID = 42
			return msg, []int64{2}, false, nil // created=false: дубль
		})

	resp, _, created, err := svc.SendMessage(ctx, 100, 1, "hi", nil, nil, &reqID)
	require.NoError(t, err)
	assert.False(t, created, "duplicate must report created=false")
	require.NotNil(t, resp)
	assert.Equal(t, int64(42), resp.ID)
	assert.True(t, mr.Exists("user_chats:2"), "cache must NOT be invalidated on a duplicate")
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

// Не-участник чата не должен инициировать рассылку messagesRead: репозиторий
// возвращает ErrNotChatMember, сервис переводит его в ErrForbidden и НЕ зовёт
// GetChatParticipantIDs (broadcast не происходит).
func TestChatService_MarkAsRead_NotMember(t *testing.T) {
	mr, _, mockRepo, svc := setupChatTest(t)
	defer mr.Close()

	ctx := context.Background()

	mockRepo.EXPECT().
		MarkAsRead(ctx, int64(100), int64(7), int64(50)).
		Return(repo.ErrNotChatMember)
	// GetChatParticipantIDs не ожидается — gomock провалит тест, если его вызовут.

	targetIDs, err := svc.MarkAsRead(ctx, 100, 7, 50)
	require.ErrorIs(t, err, ErrForbidden)
	assert.Nil(t, targetIDs)
}

func TestChatService_SendMessage_Validation(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects empty content", func(t *testing.T) {
		mr, _, _, svc := setupChatTest(t)
		defer mr.Close()

		// Репозиторий не должен вызываться (нет EXPECT).
		_, _, _, err := svc.SendMessage(ctx, 100, 1, "   ", nil, nil, nil)
		require.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("rejects too long content", func(t *testing.T) {
		mr, _, _, svc := setupChatTest(t)
		defer mr.Close()

		_, _, _, err := svc.SendMessage(ctx, 100, 1, strings.Repeat("a", 4097), nil, nil, nil)
		require.ErrorIs(t, err, ErrInvalidInput)
	})
}

func TestChatService_SendMessage_Attachment(t *testing.T) {
	ctx := context.Background()

	t.Run("attachment with empty caption is allowed and mapped to a public URL", func(t *testing.T) {
		mr, _, mockRepo, svc := setupChatTest(t)
		defer mr.Close()

		fileID := "11111111-1111-1111-1111-111111111111"
		mockRepo.EXPECT().
			SaveMessage(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, msg *domain.Message) (*domain.Message, []int64, bool, error) {
				assert.True(t, msg.FileID.Valid, "file id must be set on the saved message")
				assert.Equal(t, "file", msg.MessageType)
				msg.ID = 7
				msg.File = &domain.File{FileName: "pic.png", Key: "uploads/pic.png", MimeType: "image/png", Size: 1234}
				return msg, []int64{1}, true, nil
			})

		resp, _, _, err := svc.SendMessage(ctx, 100, 1, "", nil, &fileID, nil)
		require.NoError(t, err)
		require.NotNil(t, resp.Attachment)
		assert.Equal(t, "http://s3.example.com/uploads/pic.png", resp.Attachment.URL)
		assert.Equal(t, "image/png", resp.Attachment.MimeType)
		assert.Equal(t, int64(1234), resp.Attachment.Size)
	})

	t.Run("invalid file id is rejected before hitting repo", func(t *testing.T) {
		mr, _, _, svc := setupChatTest(t)
		defer mr.Close()

		bad := "not-a-uuid"
		_, _, _, err := svc.SendMessage(ctx, 100, 1, "hi", nil, &bad, nil)
		require.ErrorIs(t, err, ErrInvalidInput)
	})
}

func TestChatService_SendMessage_RepoErrorMapping(t *testing.T) {
	ctx := context.Background()
	userID := int64(1)
	chatID := int64(100)

	tests := []struct {
		name    string
		repoErr error
		wantErr error
	}{
		{"not a member maps to forbidden", repo.ErrNotChatMember, ErrForbidden},
		{"read-only maps to chat read-only", repo.ErrChatReadOnly, ErrChatReadOnly},
		{"unknown error stays generic", errors.New("db down"), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr, _, mockRepo, svc := setupChatTest(t)
			defer mr.Close()

			mockRepo.EXPECT().
				SaveMessage(ctx, gomock.Any()).
				Return(nil, nil, false, tt.repoErr)

			_, _, _, err := svc.SendMessage(ctx, chatID, userID, "hello", nil, nil, nil)
			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				// Unknown errors must not be misclassified as a domain sentinel.
				assert.NotErrorIs(t, err, ErrForbidden)
				assert.NotErrorIs(t, err, ErrChatReadOnly)
			}
		})
	}
}

func TestChatService_EditMessage(t *testing.T) {
	ctx := context.Background()

	t.Run("success invalidates participants and returns updated", func(t *testing.T) {
		mr, rdb, mockRepo, svc := setupChatTest(t)
		defer mr.Close()
		require.NoError(t, rdb.Set(ctx, "user_chats:2", "[]", 0).Err())

		edited := time.Now()
		mockRepo.EXPECT().
			EditMessage(ctx, int64(5), int64(10), int64(1), "new text").
			Return(&domain.Message{ID: 10, ChatID: 5, SenderID: 1, Content: "new text", EditedAt: &edited}, []int64{2}, nil)

		resp, targetIDs, err := svc.EditMessage(ctx, 5, 10, 1, "new text")
		require.NoError(t, err)
		assert.Equal(t, "new text", resp.Content)
		require.NotNil(t, resp.EditedAt)
		assert.ElementsMatch(t, []int64{2}, targetIDs)
		assert.False(t, mr.Exists("user_chats:2"))
	})

	t.Run("non-author maps to forbidden", func(t *testing.T) {
		mr, _, mockRepo, svc := setupChatTest(t)
		defer mr.Close()

		mockRepo.EXPECT().
			EditMessage(ctx, int64(5), int64(10), int64(99), "x").
			Return(nil, nil, repo.ErrNotMessageAuthor)

		_, _, err := svc.EditMessage(ctx, 5, 10, 99, "x")
		require.ErrorIs(t, err, ErrForbidden)
	})

	t.Run("rejects empty content before hitting repo", func(t *testing.T) {
		mr, _, _, svc := setupChatTest(t)
		defer mr.Close()

		_, _, err := svc.EditMessage(ctx, 5, 10, 1, "   ")
		require.ErrorIs(t, err, ErrInvalidInput)
	})
}

func TestChatService_DeleteMessage(t *testing.T) {
	ctx := context.Background()

	t.Run("success returns chat and participants", func(t *testing.T) {
		mr, _, mockRepo, svc := setupChatTest(t)
		defer mr.Close()

		mockRepo.EXPECT().
			DeleteMessage(ctx, int64(5), int64(10), int64(1)).
			Return(int64(5), []int64{1, 2}, nil)

		chatID, targetIDs, err := svc.DeleteMessage(ctx, 5, 10, 1)
		require.NoError(t, err)
		assert.Equal(t, int64(5), chatID)
		assert.ElementsMatch(t, []int64{1, 2}, targetIDs)
	})

	t.Run("missing message maps to not found", func(t *testing.T) {
		mr, _, mockRepo, svc := setupChatTest(t)
		defer mr.Close()

		mockRepo.EXPECT().
			DeleteMessage(ctx, int64(5), int64(10), int64(1)).
			Return(int64(0), nil, repo.ErrMessageNotFound)

		_, _, err := svc.DeleteMessage(ctx, 5, 10, 1)
		require.ErrorIs(t, err, ErrNotFound)
	})
}

func TestChatService_GetMessages_DeletedAndReply(t *testing.T) {
	mr, _, mockRepo, svc := setupChatTest(t)
	defer mr.Close()

	ctx := context.Background()
	deletedAt := time.Now()
	replyTo := int64(1)

	mockRepo.EXPECT().
		GetMessages(ctx, int64(1), int64(100), int64(0), 10).
		Return([]domain.Message{
			// A live reply that quotes message #1.
			{ID: 2, ChatID: 100, SenderID: 1, Content: "answer", ReplyToMessageID: &replyTo,
				ReplyTo: &domain.Message{ID: 1, SenderID: 9, Content: "question"}},
			// A soft-deleted message: content must be hidden.
			{ID: 3, ChatID: 100, SenderID: 1, Content: "secret", DeletedAt: &deletedAt},
		}, nil)

	resp, err := svc.GetMessages(ctx, 1, 100, 0, 10)
	require.NoError(t, err)
	require.Len(t, resp, 2)

	require.NotNil(t, resp[0].ReplyTo)
	assert.Equal(t, int64(1), resp[0].ReplyTo.ID)
	assert.Equal(t, "question", resp[0].ReplyTo.Content)

	assert.True(t, resp[1].IsDeleted)
	assert.Empty(t, resp[1].Content, "soft-deleted body must not leak")
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
