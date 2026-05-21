package dto

import "time"

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
	errors := make(map[string]string)
	if len(r.Title) < 3 {
		errors["title"] = "must be at least 3 chars"
	}
	if r.StartTime.Before(time.Now()) {
		errors["start_time"] = "cannot be in the past"
	}
	if r.EndTime.Before(r.StartTime) {
		errors["end_time"] = "must be after start time"
	}
	// Валидация координат (bounds check)
	if r.Coordinates.Lat < -90 || r.Coordinates.Lat > 90 {
		errors["coordinates"] = "invalid latitude"
	}
	return errors
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
