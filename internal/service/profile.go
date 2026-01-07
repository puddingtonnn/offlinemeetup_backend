package service

import (
	"context"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
)

type ProfileRepository interface {
	GetByUserID(ctx context.Context, userID int64) (*domain.Profile, error)
	UpdateProfile(ctx context.Context, profile *domain.Profile) (*domain.Profile, error)
}

type UserTagUpdater interface {
	UpdateTags(ctx context.Context, userID int64, tagNames []string) error
}

type ProfileService struct {
	profileRepo ProfileRepository
	userRepo    UserTagUpdater
}

func NewProfileService(profileRepo ProfileRepository, userRepo UserTagUpdater) *ProfileService {
	return &ProfileService{profileRepo: profileRepo, userRepo: userRepo}
}

func (s *ProfileService) GetProfile(ctx context.Context, userID int64) (*domain.Profile, error) {
	return s.profileRepo.GetByUserID(ctx, userID)
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

	updatedProfile, err := s.profileRepo.UpdateProfile(ctx, profile)
	if err != nil {
		return nil, fmt.Errorf("updating profile error: %w", err)
	}

	if input.Tags != nil {
		if err := s.userRepo.UpdateTags(ctx, userID, input.Tags); err != nil {
			return nil, fmt.Errorf("updating tags error: %w", err)
		}
	}

	return updatedProfile, nil
}
