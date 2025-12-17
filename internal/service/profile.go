package service

import (
	"context"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
)

type ProfileService struct {
	profileRepo *repo.ProfileRepo
	userRepo    *repo.UserRepo
}

func NewProfileService(profileRepo *repo.ProfileRepo, userRepo *repo.UserRepo) *ProfileService {
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
