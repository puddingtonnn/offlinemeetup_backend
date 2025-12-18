package domain

import (
	"github.com/uptrace/bun"
	"time"
)

type Profile struct {
	bun.BaseModel `bun:"table:profile" swaggerignore:"true"`

	ID          int64     `bun:",pk,autoincrement" json:"id"`
	UserID      int64     `bun:",notnull" json:"user_id"`
	Nickname    string    `bun:",unique,notnull" json:"nickname"`
	Bio         string    `bun:"" json:"bio"`
	AvatarURL   string    `bun:",notnull" json:"avatar_url"`
	IsOrganizer bool      `bun:",notnull,default:False" json:"is_organizer"`
	UpdatedAt   time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`

	User *User `bun:"rel:belongs-to,join:user_id=id" json:"-"`
}
