package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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
	s3PublicURL string
}

func NewProfileService(profileRepo ProfileRepository, userRepo UserTagUpdater, s3PublicURL string) *ProfileService {
	return &ProfileService{profileRepo: profileRepo, userRepo: userRepo, s3PublicURL: s3PublicURL}
}

func mapTagsToDTO(tags []domain.Tag) []dto.TagResponse {
	if tags == nil {
		return []dto.TagResponse{}
	}
	dtos := make([]dto.TagResponse, len(tags))
	for i, t := range tags {
		dtos[i] = dto.TagResponse{
			ID:   t.ID,
			Name: t.Name,
		}
	}
	return dtos
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

	avatarURL := ""
	if profile.AvatarFile != nil {
		avatarURL = fmt.Sprintf("%s/%s", s.s3PublicURL, profile.AvatarFile.Key)
	}

	return &dto.ProfileResponse{
		ID:          profile.ID,
		UserID:      profile.UserID,
		Nickname:    profile.Nickname,
		Bio:         profile.Bio,
		AvatarURL:   avatarURL,
		IsOrganizer: profile.IsOrganizer,
		Tags:        mapTagsToDTO(tags),
	}, nil
}

func (s *ProfileService) UpdateProfile(ctx context.Context, userID int64, req dto.UpdateProfileRequest) (*dto.ProfileResponse, error) {
	profile := &domain.Profile{
		UserID:   userID,
		Nickname: req.Nickname,
		Bio:      req.Bio,
	}

	if req.AvatarFileID != nil {
		if *req.AvatarFileID == "" {
			profile.AvatarFileID = uuid.NullUUID{}
		} else {
			id, err := uuid.Parse(*req.AvatarFileID)
			if err == nil {
				profile.AvatarFileID = uuid.NullUUID{UUID: id, Valid: true}
			}
		}
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
