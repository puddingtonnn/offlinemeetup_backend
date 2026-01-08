package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Meetup struct {
	bun.BaseModel `bun:"table:meetups"`

	ID          int64  `bun:",pk,autoincrement"`
	Title       string `bun:",notnull"`
	Description string `bun:""`

	CoverFileID uuid.NullUUID `bun:"type:uuid"`

	IsPublic  bool  `bun:",notnull"`
	CreatorID int64 `bun:",notnull"`

	StartTime time.Time `bun:",notnull"`
	EndTime   time.Time `bun:",notnull"`

	Location    Location `bun:"location,type:geography(POINT,4326)"`
	AddressText string   `bun:""`

	CreatedAt time.Time  `bun:",nullzero,notnull,default:current_timestamp"`
	DeletedAt *time.Time `bun:",soft_delete,nullzero"`

	// Relations
	Creator *User  `bun:"rel:belongs-to,join:creator_id=id"`
	Tags    []*Tag `bun:"m2m:meetup_tags,join:Meetup=Tag"`
}
