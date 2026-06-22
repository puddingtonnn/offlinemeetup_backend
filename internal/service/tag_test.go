package service

import (
	"context"
	"errors"
	"testing"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestTagService_ListTags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockTagRepository(ctrl)
	srv := NewTagService(repo)

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		tags := []domain.Tag{
			{ID: 1, Name: "Tech"},
			{ID: 2, Name: "Sport"},
		}

		repo.EXPECT().GetAll(ctx).Return(tags, nil)

		resp, err := srv.ListTags(ctx)
		assert.NoError(t, err)
		assert.Len(t, resp, 2)
		assert.Equal(t, int64(1), resp[0].ID)
		assert.Equal(t, "Tech", resp[0].Name)
		assert.Equal(t, int64(2), resp[1].ID)
		assert.Equal(t, "Sport", resp[1].Name)
	})

	t.Run("repo_error", func(t *testing.T) {
		repoErr := errors.New("db error")
		repo.EXPECT().GetAll(ctx).Return(nil, repoErr)

		resp, err := srv.ListTags(ctx)
		assert.ErrorIs(t, err, repoErr)
		assert.Nil(t, resp)
	})

	t.Run("empty_list", func(t *testing.T) {
		repo.EXPECT().GetAll(ctx).Return([]domain.Tag{}, nil)

		resp, err := srv.ListTags(ctx)
		assert.NoError(t, err)
		assert.Len(t, resp, 0)
	})
}
