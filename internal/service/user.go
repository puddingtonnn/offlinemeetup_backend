package service

import (
	"context"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
)

type UserService struct {
	userRepo    *repo.UserRepo
	profileRepo *repo.ProfileRepo
}

func NewUserService(userRepo *repo.UserRepo, profileRepo *repo.ProfileRepo) *UserService {
	return &UserService{userRepo: userRepo, profileRepo: profileRepo}
}

func (s *UserService) GetUser(ctx context.Context, userID int64) (*domain.User, error) {
	return s.userRepo.GetByID(ctx, userID)
}

type UpdateUserProfileInput struct {
	Nickname  string   `json:"nickname"`
	Bio       string   `json:"bio"`
	AvatarURL string   `json:"avatar_url"`
	Tags      []string `json:"tags"`
}

func (s *UserService) UpdateUserProfile(
	ctx context.Context,
	userID int64,
	input *UpdateUserProfileInput,
) (*domain.Profile, error) {
	returnedProfile := (*domain.Profile)(nil)

	profile := &domain.Profile{
		UserID:    userID,
		Nickname:  input.Nickname,
		Bio:       input.Bio,
		AvatarURL: input.AvatarURL,
	}

	p, err := s.profileRepo.UpdateProfile(ctx, profile)
	if err != nil {
		return nil, err
	}

	returnedProfile = p

	if err := s.userRepo.UpdateTags(ctx, userID, input.Tags); err != nil {
		return nil, err
	}

	return returnedProfile, nil
}
