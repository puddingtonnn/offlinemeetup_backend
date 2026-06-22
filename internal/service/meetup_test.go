package service

import (
	"context"
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

func setupMeetupTest(t *testing.T) (*miniredis.Miniredis, *redis.Client, *mocks.MockMeetupRepository, *MeetupService) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockMeetupRepository(ctrl)

	s3URL := "http://s3.example.com"
	svc := NewMeetupService(mockRepo, rdb, s3URL)

	return mr, rdb, mockRepo, svc
}

func TestMeetupService_CreateMeetup(t *testing.T) {
	mr, _, mockRepo, svc := setupMeetupTest(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(1)

	req := dto.CreateMeetupRequest{
		Title:       "Test Meetup",
		Description: "Test Description",
		IsPublic:    true,
		StartTime:   time.Now().Add(1 * time.Hour),
		EndTime:     time.Now().Add(2 * time.Hour),
		Address:     "123 Test St",
		Coordinates: dto.Coordinates{Lat: 10.0, Lng: 20.0},
		TagIDs:      []int64{1, 2},
	}

	mockRepo.EXPECT().
		Create(ctx, gomock.Any(), gomock.Any(), req.TagIDs).
		DoAndReturn(func(ctx context.Context, m *domain.Meetup, c *domain.Chat, tIDs []int64) (*domain.Meetup, error) {
			assert.Equal(t, req.Title, m.Title)
			assert.Equal(t, req.Description, m.Description)
			assert.Equal(t, userID, m.CreatorID)
			m.ID = 100
			return m, nil
		})

	resp, err := svc.CreateMeetup(ctx, userID, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(100), resp.ID)
	assert.Equal(t, req.Title, resp.Title)
}

func TestMeetupService_GetMeetup(t *testing.T) {
	mr, _, mockRepo, svc := setupMeetupTest(t)
	defer mr.Close()

	ctx := context.Background()
	meetupID := int64(100)
	userID := int64(1)

	t.Run("success_public", func(t *testing.T) {
		mockRepo.EXPECT().
			GetByID(ctx, meetupID, userID).
			Return(&domain.Meetup{
				ID:       meetupID,
				Title:    "Test Meetup",
				IsPublic: true,
			}, nil)

		resp, err := svc.GetMeetup(ctx, meetupID, userID)
		require.NoError(t, err)
		assert.Equal(t, meetupID, resp.ID)
	})

	t.Run("forbidden_private", func(t *testing.T) {
		mockRepo.EXPECT().
			GetByID(ctx, meetupID, userID).
			Return(&domain.Meetup{
				ID:        meetupID,
				Title:     "Private Meetup",
				IsPublic:  false,
				IsMember:  false,
				CreatorID: 2,
			}, nil)

		resp, err := svc.GetMeetup(ctx, meetupID, userID)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, ErrForbidden)
	})

	t.Run("not_found", func(t *testing.T) {
		mockRepo.EXPECT().
			GetByID(ctx, meetupID, userID).
			Return(nil, nil)

		resp, err := svc.GetMeetup(ctx, meetupID, userID)
		require.ErrorIs(t, err, ErrNotFound)
		assert.Nil(t, resp)
	})
}

func TestMeetupService_JoinMeetup(t *testing.T) {
	ctx := context.Background()
	meetupID := int64(100)
	userID := int64(1)

	activePublic := func() *domain.Meetup {
		return &domain.Meetup{
			ID:       meetupID,
			IsPublic: true,
			Status:   "active",
			EndTime:  time.Now().Add(time.Hour),
			IsMember: false,
		}
	}

	t.Run("success", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).Return(activePublic(), nil)
		mockRepo.EXPECT().Join(ctx, meetupID, userID).Return(nil)

		err := svc.JoinMeetup(ctx, userID, meetupID)
		require.NoError(t, err)
	})

	t.Run("not_found", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).Return(nil, nil)

		err := svc.JoinMeetup(ctx, userID, meetupID)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("private_forbidden", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		m := activePublic()
		m.IsPublic = false
		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).Return(m, nil)

		err := svc.JoinMeetup(ctx, userID, meetupID)
		require.ErrorIs(t, err, ErrForbidden)
	})

	t.Run("cancelled", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		m := activePublic()
		m.Status = "cancelled"
		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).Return(m, nil)

		err := svc.JoinMeetup(ctx, userID, meetupID)
		require.ErrorIs(t, err, ErrMeetupFinished)
	})

	t.Run("finished", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		m := activePublic()
		m.EndTime = time.Now().Add(-time.Hour)
		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).Return(m, nil)

		err := svc.JoinMeetup(ctx, userID, meetupID)
		require.ErrorIs(t, err, ErrMeetupFinished)
	})

	t.Run("already_member", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		m := activePublic()
		m.IsMember = true
		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).Return(m, nil)

		err := svc.JoinMeetup(ctx, userID, meetupID)
		require.ErrorIs(t, err, ErrAlreadyExists)
	})
}
