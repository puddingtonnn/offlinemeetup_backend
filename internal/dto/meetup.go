package dto

import (
	"time"
	"unicode/utf8"
)

// Границы длины текстовых полей митапа (в символах, не байтах — важно для
// кириллицы). Ограничивают раздувание строк БД и кеш-снапшотов.
const (
	maxTitleLen       = 200
	maxDescriptionLen = 5000
	maxAddressLen     = 500
)

type Coordinates struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type CreateMeetupRequest struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	IsPublic    bool        `json:"is_public"`
	StartTime   time.Time   `json:"start_time"`
	EndTime     time.Time   `json:"end_time"`
	Coordinates Coordinates `json:"coordinates"` // Вложенный JSON
	Address     string      `json:"address"`
	TagIDs      []int64     `json:"tags"`
	CoverFileID *string     `json:"cover_file_id"`
}

func (r *CreateMeetupRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if n := utf8.RuneCountInString(r.Title); n < 3 || n > maxTitleLen {
		errs["title"] = "must be 3–200 chars"
	}
	if utf8.RuneCountInString(r.Description) > maxDescriptionLen {
		errs["description"] = "must be at most 5000 chars"
	}
	if utf8.RuneCountInString(r.Address) > maxAddressLen {
		errs["address"] = "must be at most 500 chars"
	}
	if r.StartTime.Before(time.Now()) {
		errs["start_time"] = "cannot be in the past"
	}
	if r.EndTime.Before(r.StartTime) {
		errs["end_time"] = "must be after start time"
	}
	if r.Coordinates.Lat < -90 || r.Coordinates.Lat > 90 {
		errs["lat"] = "invalid latitude"
	}
	if r.Coordinates.Lng < -180 || r.Coordinates.Lng > 180 {
		errs["lng"] = "invalid longitude"
	}
	return errs
}

func (r *UpdateMeetupRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if r.Title != nil {
		if n := utf8.RuneCountInString(*r.Title); n < 3 || n > maxTitleLen {
			errs["title"] = "must be 3–200 chars"
		}
	}
	if r.Description != nil && utf8.RuneCountInString(*r.Description) > maxDescriptionLen {
		errs["description"] = "must be at most 5000 chars"
	}
	if r.Address != nil && utf8.RuneCountInString(*r.Address) > maxAddressLen {
		errs["address"] = "must be at most 500 chars"
	}
	if r.StartTime != nil && r.EndTime != nil && r.EndTime.Before(*r.StartTime) {
		errs["end_time"] = "must be after start time"
	}
	if r.Coordinates != nil {
		if r.Coordinates.Lat < -90 || r.Coordinates.Lat > 90 {
			errs["lat"] = "invalid latitude"
		}
		if r.Coordinates.Lng < -180 || r.Coordinates.Lng > 180 {
			errs["lng"] = "invalid longitude"
		}
	}
	return errs
}

type MeetupResponse struct {
	ID                int64              `json:"id"`
	Title             string             `json:"title"`
	Description       string             `json:"description"`
	IsPublic          bool               `json:"is_public"`
	InviteToken       string             `json:"invite_token"`
	Tags              []TagResponse      `json:"tags"`
	StartTime         time.Time          `json:"start_time"`
	EndTime           time.Time          `json:"end_time"`
	Coordinates       Coordinates        `json:"coordinates"`
	Address           string             `json:"address"`
	Status            string             `json:"status"`
	CreatorID         int64              `json:"creator_id"`
	Creator           *ProfileResponse   `json:"creator"`
	ParticipantsCount int                `json:"participants_count"`
	Participants      []*ProfileResponse `json:"participants,omitempty"`
	DistanceMeters    *int               `json:"distance_meters,omitempty"`
	IsMember          bool               `json:"is_member"`
	CoverURL          string             `json:"cover_url"`
}

type MeetupFilter struct {
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	Radius      int     `json:"radius"`
	Limit       int     `json:"limit"`
	Offset      int     `json:"offset"`
	Tags        []int64 `json:"tags"`
	OnlyMy      bool    `json:"only_my"`
	ShowPast    bool    `json:"show_past"`
	ExcludeOwn  bool    `json:"exclude_own"`
	OnlyCreated bool    `json:"only_created"`
}

type UpdateMeetupRequest struct {
	Title       *string      `json:"title"`
	Description *string      `json:"description"`
	TagIDs      *[]int64     `json:"tags"`
	IsPublic    *bool        `json:"is_public"`
	StartTime   *time.Time   `json:"start_time"`
	EndTime     *time.Time   `json:"end_time"`
	Coordinates *Coordinates `json:"coordinates"`
	Address     *string      `json:"address"`
	CoverFileID *string      `json:"cover_file_id"`
}
