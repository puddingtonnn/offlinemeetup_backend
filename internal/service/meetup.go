package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
)

var (
	ErrMeetupNotFound = errors.New("meetup not found")
	ErrForbidden      = errors.New("you are not the owner of this meetup")
)

type MeetupRepository interface {
	Create(ctx context.Context, meetup *domain.Meetup, tagIDs []int64) (*domain.Meetup, error)
	GetByID(ctx context.Context, id int64) (*domain.Meetup, error)
	List(ctx context.Context, filter dto.MeetupFilter) ([]domain.Meetup, error)
	Update(ctx context.Context, meetup *domain.Meetup, newTagIDs []int64) error
	Delete(ctx context.Context, id int64) error
}

type MeetupService struct {
	repo MeetupRepository
}

func NewMeetupService(repo MeetupRepository) *MeetupService {
	return &MeetupService{repo: repo}
}

func (s *MeetupService) CreateMeetup(ctx context.Context, userID int64, req dto.CreateMeetupRequest) (*dto.MeetupResponse, error) {
	wktLocation := fmt.Sprintf("POINT(%.6f %.6f)", req.Coordinates.Lng, req.Coordinates.Lat)

	meetup := &domain.Meetup{
		Title:       req.Title,
		Description: req.Description,
		IsPublic:    req.IsPublic,
		CreatorID:   userID,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		Location:    wktLocation,
		AddressText: req.Address,
	}

	created, err := s.repo.Create(ctx, meetup, req.TagIDs)
	if err != nil {
		return nil, err
	}

	return s.mapToResponse(created), nil
}

func (s *MeetupService) mapToResponse(m *domain.Meetup) *dto.MeetupResponse {

	var lat, lng float64

	if m.Location != "" {
		cleanStr := strings.TrimPrefix(m.Location, "POINT(")
		cleanStr = strings.TrimSuffix(cleanStr, ")")
		fmt.Sscanf(cleanStr, "%f %f", &lng, &lat)
	}

	tagsResp := make([]dto.TagResponse, 0)

	if len(m.Tags) > 0 {
		for _, t := range m.Tags {
			tagsResp = append(tagsResp, dto.TagResponse{
				ID:   t.ID,
				Name: t.Name,
			})
		}
	}

	return &dto.MeetupResponse{
		ID:          m.ID,
		Title:       m.Title,
		Description: m.Description,
		StartTime:   m.StartTime,
		EndTime:     m.EndTime,
		Coordinates: dto.Coordinates{Lat: lat, Lng: lng},
		Address:     m.AddressText,
		CreatorID:   m.CreatorID,
		Tags:        tagsResp,
	}
}

func (s *MeetupService) GetMeetup(ctx context.Context, id int64) (*dto.MeetupResponse, error) {
	m, err := s.repo.GetByID(ctx, id)

	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrMeetupNotFound
	}

	return s.mapToResponse(m), nil
}

func (s *MeetupService) ListMeetups(ctx context.Context, filter dto.MeetupFilter) ([]*dto.MeetupResponse, error) {
	if filter.Limit == 0 {
		filter.Limit = 20
	}

	meetups, err := s.repo.List(ctx, filter)
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
	existing, err := s.repo.GetByID(ctx, meetupID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrMeetupNotFound
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
		existing.Location = fmt.Sprintf("POINT(%f %f)", req.Coordinates.Lng, req.Coordinates.Lat)
	}

	if err := s.repo.Update(ctx, existing, *req.TagIDs); err != nil {
		return nil, err
	}
	return s.mapToResponse(existing), nil
}

func (s *MeetupService) DeleteMeetup(ctx context.Context, userID int64, meetupID int64) error {
	existing, err := s.repo.GetByID(ctx, meetupID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrMeetupNotFound
	}

	if existing.CreatorID != userID {
		return ErrForbidden
	}

	return s.repo.Delete(ctx, meetupID)
}
