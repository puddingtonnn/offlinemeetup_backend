package domain

import "github.com/uptrace/bun"

type Tag struct {
	bun.BaseModel `bun:"table:tags" swaggerignore:"true"`

	ID   int64  `bun:",pk,autoincrement" json:"id"`
	Name string `bun:",unique,notnull" json:"name"`
}

type ProfileTag struct {
	bun.BaseModel `bun:"table:profile_tags" swaggerignore:"true"`

	ProfileID int64 `bun:",pk"`
	TagID     int64 `bun:",pk"`

	Profile *Profile `bun:"rel:belongs-to,join:profile_id=id"`
	Tag     *Tag     `bun:"rel:belongs-to,join:tag_id=id"`
}
