package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
	"github.com/redis/go-redis/v9"
)

type MeetupRepository interface {
	Create(ctx context.Context, meetup *domain.Meetup, chat *domain.Chat, tagIDs []int64) (*domain.Meetup, error)
	GetByID(ctx context.Context, id int64, currentUserID int64) (*domain.Meetup, error)
	GetByInviteToken(ctx context.Context, token uuid.UUID, currentUserID int64) (*domain.Meetup, error)
	List(ctx context.Context, filter dto.MeetupFilter, currentUserID int64) ([]domain.Meetup, error)
	Update(ctx context.Context, meetup *domain.Meetup, newTagIDs []int64) error
	Delete(ctx context.Context, id int64) error
	Join(ctx context.Context, meetupID, userID int64) error
	Leave(ctx context.Context, meetupID, userID int64) error
}

type MeetupService struct {
	repo        MeetupRepository
	rdb         *redis.Client
	s3PublicURL string
}

func NewMeetupService(repo MeetupRepository, rdb *redis.Client, s3PublicURL string) *MeetupService {
	return &MeetupService{repo: repo, rdb: rdb, s3PublicURL: s3PublicURL}
}

func (s *MeetupService) CreateMeetup(ctx context.Context, userID int64, req dto.CreateMeetupRequest) (*dto.MeetupResponse, error) {
	meetup := &domain.Meetup{
		Title:       req.Title,
		Description: req.Description,
		IsPublic:    req.IsPublic,
		CreatorID:   userID,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		Location: domain.Location{
			Lat: req.Coordinates.Lat,
			Lng: req.Coordinates.Lng,
		},
		AddressText: req.Address,
	}

	if req.CoverFileID != nil && *req.CoverFileID != "" {
		id, err := uuid.Parse(*req.CoverFileID)
		if err == nil {
			meetup.CoverFileID = uuid.NullUUID{UUID: id, Valid: true}
		}
	}

	chat := &domain.Chat{
		Type: "group",
	}

	created, err := s.repo.Create(ctx, meetup, chat, req.TagIDs)
	if err != nil {
		return nil, err
	}

	s.rdb.Del(ctx, fmt.Sprintf("user_chats:%d", userID))

	return s.mapToResponse(created), nil
}

func (s *MeetupService) mapToResponse(m *domain.Meetup) *dto.MeetupResponse {
	if m == nil {
		return nil
	}

	var tagsDTO []dto.TagResponse
	if len(m.Tags) > 0 {
		tagsDTO = make([]dto.TagResponse, len(m.Tags))
		for i, t := range m.Tags {
			if t == nil {
				continue
			}
			tagsDTO[i] = dto.TagResponse{
				ID:   t.ID,
				Name: t.Name,
			}
		}
	} else {
		tagsDTO = []dto.TagResponse{}
	}

	var dist *int
	if m.DistanceMeters != 0 {
		d := int(m.DistanceMeters)
		dist = &d
	}

	coverURL := ""
	if m.CoverFile != nil {
		coverURL = fmt.Sprintf("%s/%s", s.s3PublicURL, m.CoverFile.Key)
	}

	var creatorDTO *dto.ProfileResponse
	if m.Creator != nil && m.Creator.Profile != nil {
		p := m.Creator.Profile
		avatarURL := ""
		if p.AvatarFile != nil {
			avatarURL = fmt.Sprintf("%s/%s", s.s3PublicURL, p.AvatarFile.Key)
		}
		creatorDTO = &dto.ProfileResponse{
			ID:          p.ID,
			UserID:      p.UserID,
			Nickname:    p.Nickname,
			Bio:         p.Bio,
			AvatarURL:   avatarURL,
			IsOrganizer: p.IsOrganizer,
			Tags:        []dto.TagResponse{},
		}
	}

	participantsDTO := make([]*dto.ProfileResponse, 0, len(m.Participants))
	for _, part := range m.Participants {
		if part != nil && part.Profile != nil {
			p := part.Profile
			avatarURL := ""
			if p.AvatarFile != nil {
				avatarURL = fmt.Sprintf("%s/%s", s.s3PublicURL, p.AvatarFile.Key)
			}
			participantsDTO = append(participantsDTO, &dto.ProfileResponse{
				ID:          p.ID,
				UserID:      p.UserID,
				Nickname:    p.Nickname,
				Bio:         p.Bio,
				AvatarURL:   avatarURL,
				IsOrganizer: p.IsOrganizer,
				Tags:        []dto.TagResponse{},
			})
		}
	}

	return &dto.MeetupResponse{
		ID:          m.ID,
		Title:       m.Title,
		Description: m.Description,
		StartTime:   m.StartTime,
		EndTime:     m.EndTime,
		Coordinates: dto.Coordinates{
			Lat: m.Location.Lat,
			Lng: m.Location.Lng,
		},
		Address:           m.AddressText,
		CreatorID:         m.CreatorID,
		Creator:           creatorDTO,
		Tags:              tagsDTO,
		ParticipantsCount: m.ParticipantsCount,
		Participants:      participantsDTO,
		DistanceMeters:    dist,
		IsMember:          m.IsMember,
		CoverURL:          coverURL,
	}
}

func (s *MeetupService) GetMeetup(ctx context.Context, id int64, userID int64) (*dto.MeetupResponse, error) {
	m, err := s.repo.GetByID(ctx, id, userID)

	if err != nil {
		return nil, fmt.Errorf("getting meetup: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("meetup %d: %w", id, ErrNotFound)
	}

	if !m.IsPublic && !m.IsMember && m.CreatorID != userID {
		return nil, ErrForbidden
	}

	return s.mapToResponse(m), nil
}

func (s *MeetupService) ListMeetups(ctx context.Context, userID int64, filter dto.MeetupFilter) ([]*dto.MeetupResponse, error) {
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	meetups, err := s.repo.List(ctx, filter, userID)
	if err != nil {
		return nil, err
	}

	response := make([]*dto.MeetupResponse, len(meetups))
	for i, m := range meetups {
		response[i] = s.mapToResponse(&m)
	}
	return response, nil
}

func (s *MeetupService) UpdateMeetup(ctx context.Context, userID int64, meetupID int64, req dto.UpdateMeetupRequest) (*dto.MeetupResponse, error) {
	existing, err := s.repo.GetByID(ctx, meetupID, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("getting meetup: %w", err)
	}
	if existing.CreatorID != userID {
		return nil, ErrForbidden
	}

	if req.Title != nil {
		existing.Title = *req.Title
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.IsPublic != nil {
		existing.IsPublic = *req.IsPublic
	}
	if req.StartTime != nil {
		existing.StartTime = *req.StartTime
	}
	if req.EndTime != nil {
		existing.EndTime = *req.EndTime
	}
	if req.Address != nil {
		existing.AddressText = *req.Address
	}
	if req.Coordinates != nil {
		existing.Location = domain.Location{
			Lat: req.Coordinates.Lat,
			Lng: req.Coordinates.Lng,
		}
	}
	if req.CoverFileID != nil {
		if *req.CoverFileID == "" {
			existing.CoverFileID = uuid.NullUUID{}
		} else {
			id, err := uuid.Parse(*req.CoverFileID)
			if err == nil {
				existing.CoverFileID = uuid.NullUUID{UUID: id, Valid: true}
			}
		}
	}

	var tagIDs []int64
	if req.TagIDs != nil {
		tagIDs = *req.TagIDs
	}

	if err := s.repo.Update(ctx, existing, tagIDs); err != nil {
		return nil, err
	}
	return s.mapToResponse(existing), nil
}

func (s *MeetupService) DeleteMeetup(ctx context.Context, userID int64, meetupID int64) error {
	existing, err := s.repo.GetByID(ctx, meetupID, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("getting meetup: %w", err)
	}

	if existing.CreatorID != userID {
		return ErrForbidden
	}

	return s.repo.Delete(ctx, meetupID)
}

func (s *MeetupService) JoinMeetup(ctx context.Context, userID, meetupID int64) error {
	meetup, err := s.repo.GetByID(ctx, meetupID, userID)
	if err != nil {
		return err
	}
	if meetup == nil {
		return ErrNotFound
	}

	if time.Now().After(meetup.EndTime) {
		return ErrMeetupFinished
	}

	if err := s.repo.Join(ctx, meetupID, userID); err != nil {
		return err
	}

	s.rdb.Del(ctx, fmt.Sprintf("user_chats:%d", userID))
	return nil
}

func (s *MeetupService) LeaveMeetup(ctx context.Context, userID, meetupID int64) error {
	if err := s.repo.Leave(ctx, meetupID, userID); err != nil {
		return err
	}

	s.rdb.Del(ctx, fmt.Sprintf("user_chats:%d", userID))
	return nil
}

func (s *MeetupService) JoinMeetupByToken(ctx context.Context, userID int64, token string) error {
	inviteUUID, err := uuid.Parse(token)
	if err != nil {
		return fmt.Errorf("invalid token format: %w", ErrInvalidInput)
	}

	meetup, err := s.repo.GetByInviteToken(ctx, inviteUUID, userID)
	if err != nil {
		return err
	}
	if meetup == nil {
		return ErrNotFound
	}

	if time.Now().After(meetup.EndTime) {
		return ErrMeetupFinished
	}

	if err := s.repo.Join(ctx, meetup.ID, userID); err != nil {
		return err
	}

	s.rdb.Del(ctx, fmt.Sprintf("user_chats:%d", userID))
	return nil
}
