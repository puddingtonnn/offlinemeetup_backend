package service

import (
	"context"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
)

type ProfileService struct {
	repo *repo.ProfileRepo
}

func NewProfileService(repo *repo.ProfileRepo) *ProfileService {
	return &ProfileService{repo: repo}
}

func (s *ProfileService) GetProfile(ctx context.Context, userID int64) (*domain.Profile, error) {
	return s.repo.GetByUserID(ctx, userID)
}

type UpdateProfileInput struct {
	Nickname  string   `json:"nickname"`
	Bio       string   `json:"bio"`
	AvatarURL string   `json:"avatar_url"`
	Tags      []string `json:"tags"`
}

func (s *ProfileService) UpdateProfile(ctx context.Context, userID int64, input *UpdateProfileInput) (*domain.Profile, error) {
	profile := &domain.Profile{
		UserID:    userID,
		Nickname:  input.Nickname,
		Bio:       input.Bio,
		AvatarURL: input.AvatarURL,
	}
	savedProfile, err := s.repo.UpdateProfile(ctx, profile)
	if err != nil {
		return nil, fmt.Errorf("updating profile error: %w", err)
	}

	if err := s.repo.UpdateTags(ctx, savedProfile.ID, input.Tags); err != nil {
		return nil, fmt.Errorf("updating tags error: %w", err)
	}

	return s.repo.GetByUserID(ctx, userID)
}
