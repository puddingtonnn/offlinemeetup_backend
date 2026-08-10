package dto

import (
	"regexp"
	"unicode/utf8"
)

var usernameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_.]{2,32}$`)

type ProfileResponse struct {
	ID          int64         `json:"id"`
	UserID      int64         `json:"user_id"`
	Username    string        `json:"username"`
	DisplayName *string       `json:"display_name"`
	Bio         string        `json:"bio"`
	AvatarURL   string        `json:"avatar_url"`
	IsOrganizer bool          `json:"is_organizer"`
	Tags        []TagResponse `json:"tags"`
}

type UpdateProfileRequest struct {
	Username     *string `json:"username"`
	DisplayName  *string `json:"display_name"`
	Bio          *string `json:"bio"`
	AvatarFileID *string `json:"avatar_file_id"`
	TagIDs       []int64 `json:"tags"`
}

func (r *UpdateProfileRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if r.Username != nil && !usernameRegexp.MatchString(*r.Username) {
		errs["username"] = "must be 2-32 chars, letters/digits/underscore/dot only"
	}
	if r.DisplayName != nil {
		n := utf8.RuneCountInString(*r.DisplayName)
		if n < 2 || n > 32 {
			errs["display_name"] = "must be between 2 and 32 chars"
		}
	}
	if r.Bio != nil && len(*r.Bio) > 500 {
		errs["bio"] = "must be at most 500 chars"
	}
	return errs
}
