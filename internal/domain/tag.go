package domain

import "github.com/uptrace/bun"

type Tag struct {
	bun.BaseModel `bun:"table:tags" swaggerignore:"true"`

	ID   int64  `bun:",pk,autoincrement" json:"id"`
	Name string `bun:",unique,notnull" json:"name"`
}

type UserTag struct {
	bun.BaseModel `bun:"table:user_tags" swaggerignore:"true"`

	UserID int64 `bun:",pk" json:"user_id"`
	TagID  int64 `bun:",pk" json:"tag_id"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
	Tag  *Tag  `bun:"rel:belongs-to,join:tag_id=id"`
}
