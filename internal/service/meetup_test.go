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

func TestMeetupService_JoinMeetupByToken(t *testing.T) {
	ctx := context.Background()
	userID := int64(1)
	token := "11111111-1111-1111-1111-111111111111"

	activeMeetup := func() *domain.Meetup {
		return &domain.Meetup{
			ID:      100,
			Status:  "active",
			EndTime: time.Now().Add(time.Hour),
		}
	}

	t.Run("success", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		m := activeMeetup()
		mockRepo.EXPECT().GetByInviteToken(ctx, gomock.Any(), userID).Return(m, nil)
		mockRepo.EXPECT().Join(ctx, m.ID, userID).Return(nil)

		require.NoError(t, svc.JoinMeetupByToken(ctx, userID, token))
	})

	t.Run("invalid token format", func(t *testing.T) {
		mr, _, _, svc := setupMeetupTest(t)
		defer mr.Close()

		err := svc.JoinMeetupByToken(ctx, userID, "not-a-uuid")
		require.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("not found", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByInviteToken(ctx, gomock.Any(), userID).Return(nil, nil)

		require.ErrorIs(t, svc.JoinMeetupByToken(ctx, userID, token), ErrNotFound)
	})

	t.Run("cancelled", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		m := activeMeetup()
		m.Status = "cancelled"
		mockRepo.EXPECT().GetByInviteToken(ctx, gomock.Any(), userID).Return(m, nil)

		require.ErrorIs(t, svc.JoinMeetupByToken(ctx, userID, token), ErrMeetupFinished)
	})

	t.Run("already member", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		m := activeMeetup()
		m.IsMember = true
		mockRepo.EXPECT().GetByInviteToken(ctx, gomock.Any(), userID).Return(m, nil)

		require.ErrorIs(t, svc.JoinMeetupByToken(ctx, userID, token), ErrAlreadyExists)
	})
}

func TestMeetupService_UpdateMeetup(t *testing.T) {
	ctx := context.Background()
	meetupID := int64(100)
	userID := int64(1)

	t.Run("success", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).
			Return(&domain.Meetup{ID: meetupID, CreatorID: userID, Title: "old"}, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any(), gomock.Any()).Return(nil)

		newTitle := "new title"
		resp, err := svc.UpdateMeetup(ctx, userID, meetupID, dto.UpdateMeetupRequest{Title: &newTitle})
		require.NoError(t, err)
		assert.Equal(t, "new title", resp.Title)
	})

	t.Run("not owner", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).
			Return(&domain.Meetup{ID: meetupID, CreatorID: 999}, nil)

		_, err := svc.UpdateMeetup(ctx, userID, meetupID, dto.UpdateMeetupRequest{})
		require.ErrorIs(t, err, ErrForbidden)
	})

	t.Run("not found", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).Return(nil, nil)

		_, err := svc.UpdateMeetup(ctx, userID, meetupID, dto.UpdateMeetupRequest{})
		require.ErrorIs(t, err, ErrNotFound)
	})
}

func TestMeetupService_DeleteMeetup(t *testing.T) {
	ctx := context.Background()
	meetupID := int64(100)
	userID := int64(1)

	t.Run("success", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).
			Return(&domain.Meetup{ID: meetupID, CreatorID: userID}, nil)
		mockRepo.EXPECT().Delete(ctx, meetupID).Return(nil)

		require.NoError(t, svc.DeleteMeetup(ctx, userID, meetupID))
	})

	t.Run("not owner", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).
			Return(&domain.Meetup{ID: meetupID, CreatorID: 999}, nil)

		require.ErrorIs(t, svc.DeleteMeetup(ctx, userID, meetupID), ErrForbidden)
	})

	t.Run("not found", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).Return(nil, nil)

		require.ErrorIs(t, svc.DeleteMeetup(ctx, userID, meetupID), ErrNotFound)
	})
}

func TestMeetupService_LeaveMeetup(t *testing.T) {
	ctx := context.Background()
	userID := int64(1)
	meetupID := int64(100)

	t.Run("participant leaves", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).
			Return(&domain.Meetup{ID: meetupID, CreatorID: 999}, nil)
		mockRepo.EXPECT().Leave(ctx, meetupID, userID).Return(nil)

		require.NoError(t, svc.LeaveMeetup(ctx, userID, meetupID))
	})

	t.Run("organizer cannot leave", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).
			Return(&domain.Meetup{ID: meetupID, CreatorID: userID}, nil)
		// Leave не должен вызываться.

		require.ErrorIs(t, svc.LeaveMeetup(ctx, userID, meetupID), ErrOrganizerCannotLeave)
	})

	t.Run("not found", func(t *testing.T) {
		mr, _, mockRepo, svc := setupMeetupTest(t)
		defer mr.Close()

		mockRepo.EXPECT().GetByID(ctx, meetupID, userID).Return(nil, nil)

		require.ErrorIs(t, svc.LeaveMeetup(ctx, userID, meetupID), ErrNotFound)
	})
}

func TestMeetupService_ListMeetups(t *testing.T) {
	mr, _, mockRepo, svc := setupMeetupTest(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(1)

	// Limit == 0 должен подставить дефолт 20 на уровне сервиса.
	mockRepo.EXPECT().
		List(ctx, gomock.Any(), userID).
		DoAndReturn(func(_ context.Context, f dto.MeetupFilter, _ int64) ([]domain.Meetup, error) {
			assert.Equal(t, 20, f.Limit, "service must default empty limit to 20")
			return []domain.Meetup{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}}, nil
		})

	list, err := svc.ListMeetups(ctx, userID, dto.MeetupFilter{})
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, int64(1), list[0].ID)
	assert.Equal(t, "B", list[1].Title)
}
