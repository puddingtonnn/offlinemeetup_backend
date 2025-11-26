package domain

import (
	"github.com/uptrace/bun"
	"time"
)

type Event struct {
	bun.BaseModel `bun:"table:events"`

	ID          int64  `bun:",pk,autoincrement"`
	Title       string `bun:",notnull"`
	Description string
	IsPublic    bool      `bun:",notnull"`
	CreatedAt   time.Time `bun:",nullzero,notnull,default:current_timestamp"`
	StartTime   time.Time `bun:",notnull"`
	EndTime     time.Time `bun:",notnull"`
	CreatorID   int64     `bun:",notnull"`
	Location    string    `bun:"type:geography(POINT,4326)"`

	Creator *User `bun:"rel-belongs-to,join:creator_id=id"`
}

type Tag struct {
	bun.BaseModel `bun:"table:tags"`

	ID   int64  `bun:",pk,autoincrement"`
	Name string `bun:",notnull,unique"`
	Icon string `bun:",notnull"`
}

type EventTag struct {
	bun.BaseModel `bun:"table:event_tags"`

	EventID int64  `bun:",pk"`
	Event   *Event `bun:"rel:belongs-to,join:event_id=id"`

	TagID int64 `bun:",pk"`
	Tag   *Tag  `bun:"rel:belongs-to,join:tag_id=id"`
}
