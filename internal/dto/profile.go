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
	Nickname     *string `json:"nickname"`
	Bio          *string `json:"bio"`
	AvatarFileID *string `json:"avatar_file_id"`
	TagIDs       []int64 `json:"tags"`
}

func (r *UpdateProfileRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if r.Nickname != nil {
		n := len(*r.Nickname)
		if n < 2 || n > 32 {
			errs["nickname"] = "must be between 2 and 32 chars"
		}
	}
	if r.Bio != nil && len(*r.Bio) > 500 {
		errs["bio"] = "must be at most 500 chars"
	}
	return errs
}
