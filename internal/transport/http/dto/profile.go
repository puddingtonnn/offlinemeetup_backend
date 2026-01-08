package dto

type ProfileResponse struct {
	ID          int64         `json:"id"`
	UserID      int64         `json:"user_id"`
	Nickname    string        `json:"nickname"`
	Bio         string        `json:"bio"`
	AvatarURL   string        `json:"avatar_url"`
	IsOrganizer bool          `json:"is_organizer"`
	Tags        []TagResponse `json:"tags"`
}

type UpdateProfileRequest struct {
	Nickname  string  `json:"nickname"`
	Bio       string  `json:"bio"`
	AvatarURL string  `json:"avatar_url"`
	TagIDs    []int64 `json:"tags"`
}
