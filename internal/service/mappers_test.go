package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testS3 = "https://s3.example.com"

func TestMapMeetupToDTO_Nil(t *testing.T) {
	assert.Nil(t, mapMeetupToDTO(nil, testS3))
	assert.Nil(t, mapProfileToDTO(nil, testS3))
}

func TestMapMeetupToDTO_Full(t *testing.T) {
	m := &domain.Meetup{
		ID:                7,
		Title:             "Go meetup",
		IsPublic:          true,
		Status:            "active",
		CreatorID:         1,
		ParticipantsCount: 2,
		DistanceMeters:    1234.7,
		IsMember:          true,
		CoverFile:         &domain.File{Key: "uploads/cover.png"},
		Creator: &domain.User{
			Profile: &domain.Profile{ID: 10, UserID: 1, Nickname: "host", AvatarFile: &domain.File{Key: "uploads/a.png"}},
		},
		Tags: []*domain.Tag{{ID: 1, Name: "Go"}, nil, {ID: 2, Name: "Backend"}},
		Participants: []*domain.User{
			{Profile: &domain.Profile{ID: 11, UserID: 2, Nickname: "guest"}},
			{Profile: nil}, // должен быть пропущен
		},
	}

	dto := mapMeetupToDTO(m, testS3)
	require.NotNil(t, dto)

	assert.Equal(t, int64(7), dto.ID)
	assert.Equal(t, "active", dto.Status)
	assert.True(t, dto.IsMember)
	assert.Equal(t, testS3+"/uploads/cover.png", dto.CoverURL)

	require.NotNil(t, dto.DistanceMeters)
	assert.Equal(t, 1234, *dto.DistanceMeters)

	require.NotNil(t, dto.Creator)
	assert.Equal(t, "host", dto.Creator.Nickname)
	assert.Equal(t, testS3+"/uploads/a.png", dto.Creator.AvatarURL)

	// nil-тег пропущен.
	require.Len(t, dto.Tags, 2)
	assert.Equal(t, "Go", dto.Tags[0].Name)

	// участник без профиля пропущен.
	require.Len(t, dto.Participants, 1)
	assert.Equal(t, "guest", dto.Participants[0].Nickname)
}

func TestMapMeetupToDTO_NoDistanceNoCover(t *testing.T) {
	m := &domain.Meetup{ID: 1, InviteToken: uuid.Nil}
	dto := mapMeetupToDTO(m, testS3)
	require.NotNil(t, dto)
	assert.Nil(t, dto.DistanceMeters, "нулевая дистанция => nil")
	assert.Empty(t, dto.CoverURL)
	assert.Empty(t, dto.Tags)
	assert.Nil(t, dto.Creator)
}
