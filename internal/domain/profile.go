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
	Nickname     string        `bun:",unique,notnull" json:"nickname"`
	Bio          string        `bun:"" json:"bio"`
	AvatarFileID uuid.NullUUID `bun:"type:uuid" json:"avatar_file_id"`
	AvatarFile   *File         `bun:"rel:belongs-to,join:avatar_file_id=id" json:"avatar_file"`
	IsOrganizer  bool          `bun:",notnull,default:False" json:"is_organizer"`
	UpdatedAt    time.Time     `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`

	User *User `bun:"rel:belongs-to,join:user_id=id" json:"-"`
}
