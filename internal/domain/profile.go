package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Profile struct {
	bun.BaseModel `bun:"table:profile" swaggerignore:"true"`

	ID           int64         `bun:",pk,autoincrement" json:"id"`
	UserID       int64         `bun:",notnull" json:"user_id"`
	Username     string        `bun:",unique,notnull" json:"username"`
	DisplayName  *string       `bun:"" json:"display_name"`
	Bio          string        `bun:"" json:"bio"`
	AvatarFileID uuid.NullUUID `bun:"type:uuid" json:"avatar_file_id"`
	AvatarFile   *File         `bun:"rel:belongs-to,join:avatar_file_id=id" json:"avatar_file"`
	IsOrganizer  bool          `bun:",notnull,default:False" json:"is_organizer"`
	UpdatedAt    time.Time     `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`

	User *User `bun:"rel:belongs-to,join:user_id=id" json:"-"`
}

// DisplayNameOf implements the shared "name shown to other users" rule:
// display_name if set (non-empty), else username. Used everywhere a profile's
// name is surfaced (mappers, chat, presence) so the rule lives in exactly one
// place instead of a repeated if at every call site.
func DisplayNameOf(username string, displayName *string) string {
	if displayName != nil && *displayName != "" {
		return *displayName
	}
	return username
}
