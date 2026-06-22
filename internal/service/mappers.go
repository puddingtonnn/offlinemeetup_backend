package service

import (
	"fmt"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
)

// publicURL строит полный публичный URL файла в S3 или "" если файла нет.
func publicURL(s3URL string, f *domain.File) string {
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s", s3URL, f.Key)
}

// mapTagsToResponse конвертирует теги домена в DTO, пропуская nil-элементы.
func mapTagsToResponse(tags []*domain.Tag) []dto.TagResponse {
	result := make([]dto.TagResponse, 0, len(tags))
	for _, t := range tags {
		if t == nil {
			continue
		}
		result = append(result, dto.TagResponse{ID: t.ID, Name: t.Name})
	}
	return result
}

// mapProfileToDTO конвертирует профиль в DTO без тегов (теги подгружаются отдельно
// там, где это нужно). Возвращает nil, если профиль не задан.
func mapProfileToDTO(p *domain.Profile, s3URL string) *dto.ProfileResponse {
	if p == nil {
		return nil
	}
	return &dto.ProfileResponse{
		ID:          p.ID,
		UserID:      p.UserID,
		Nickname:    p.Nickname,
		Bio:         p.Bio,
		AvatarURL:   publicURL(s3URL, p.AvatarFile),
		IsOrganizer: p.IsOrganizer,
		Tags:        []dto.TagResponse{},
	}
}

// mapMeetupToDTO — единый маппер митапа в DTO, используемый meetup- и chat-сервисами.
func mapMeetupToDTO(m *domain.Meetup, s3URL string) *dto.MeetupResponse {
	if m == nil {
		return nil
	}

	var dist *int
	if m.DistanceMeters != 0 {
		d := int(m.DistanceMeters)
		dist = &d
	}

	var creator *dto.ProfileResponse
	if m.Creator != nil {
		creator = mapProfileToDTO(m.Creator.Profile, s3URL)
	}

	participants := make([]*dto.ProfileResponse, 0, len(m.Participants))
	for _, part := range m.Participants {
		if part != nil && part.Profile != nil {
			participants = append(participants, mapProfileToDTO(part.Profile, s3URL))
		}
	}

	return &dto.MeetupResponse{
		ID:                m.ID,
		Title:             m.Title,
		Description:       m.Description,
		IsPublic:          m.IsPublic,
		InviteToken:       m.InviteToken.String(),
		StartTime:         m.StartTime,
		EndTime:           m.EndTime,
		Coordinates:       dto.Coordinates{Lat: m.Location.Lat, Lng: m.Location.Lng},
		Address:           m.AddressText,
		Status:            m.Status,
		CreatorID:         m.CreatorID,
		Creator:           creator,
		Tags:              mapTagsToResponse(m.Tags),
		ParticipantsCount: m.ParticipantsCount,
		Participants:      participants,
		DistanceMeters:    dist,
		IsMember:          m.IsMember,
		CoverURL:          publicURL(s3URL, m.CoverFile),
	}
}
