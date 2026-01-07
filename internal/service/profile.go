package service

import (
	"context"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
)

type ProfileRepository interface {
	GetByUserID(ctx context.Context, userID int64) (*domain.Profile, error)
	UpdateProfile(ctx context.Context, profile *domain.Profile) (*domain.Profile, error)
}

type UserTagUpdater interface {
	UpdateTags(ctx context.Context, userID int64, tagIDs []int64) error
	GetTagsByUserID(ctx context.Context, userID int64) ([]domain.Tag, error)
}

type ProfileService struct {
	profileRepo ProfileRepository
	userRepo    UserTagUpdater
}

func NewProfileService(profileRepo ProfileRepository, userRepo UserTagUpdater) *ProfileService {
	return &ProfileService{profileRepo: profileRepo, userRepo: userRepo}
}

func (s *ProfileService) GetProfile(ctx context.Context, userID int64) (*dto.ProfileResponse, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, err
	}

	tags, err := s.userRepo.GetTagsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tags: %w", err)
	}

	return &dto.ProfileResponse{
		ID:          profile.ID,
		UserID:      profile.UserID,
		Nickname:    profile.Nickname,
		Bio:         profile.Bio,
		AvatarURL:   profile.AvatarURL,
		IsOrganizer: profile.IsOrganizer,
		Tags:        tags,
	}, nil
}

type UpdateProfileInput struct {
	Nickname  string   `json:"nickname"`
	Bio       string   `json:"bio"`
	AvatarURL string   `json:"avatar_url"`
	Tags      []string `json:"tags"`
}

func (s *ProfileService) UpdateProfile(ctx context.Context, userID int64, req dto.UpdateProfileRequest) (*dto.ProfileResponse, error) {
	profile := &domain.Profile{
		UserID:    userID,
		Nickname:  req.Nickname,
		Bio:       req.Bio,
		AvatarURL: req.AvatarURL,
	}

	_, err := s.profileRepo.UpdateProfile(ctx, profile)
	if err != nil {
		return nil, fmt.Errorf("updating profile error: %w", err)
	}

	if req.TagIDs != nil {
		if err := s.userRepo.UpdateTags(ctx, userID, req.TagIDs); err != nil {
			return nil, fmt.Errorf("updating tags error: %w", err)
		}
	}

	return s.GetProfile(ctx, userID)
}
