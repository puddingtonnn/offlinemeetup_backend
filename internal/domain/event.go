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
