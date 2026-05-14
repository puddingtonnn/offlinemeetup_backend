package domain

import (
	"github.com/uptrace/bun"
	"time"
)

type User struct {
	bun.BaseModel `bun:"table:users" swaggerignore:"true"`
	ID            int64            `bun:",pk,autoincrement"`
	Email         string           `bun:",unique,nullzero"`
	Role          string           `bun:"default:'user'"`
	Status        UserStatus       `bun:",type:varchar(20),default:'active'"`
	CreatedAt     time.Time        `bun:",nullzero,notnull,default:current_timestamp"`
	UpdatedAt     time.Time        `bun:",nullzero,notnull,default:current_timestamp"`
	Socials       []*SocialAccount `bun:"rel:has-many,join:id=user_id"`

	Tags []*Tag `bun:"m2m:user_tags,join:User=User,join:Tag=Tag"`

	Profile *Profile `bun:"rel:has-one,join:id=user_id"`
}

type SocialAccount struct {
	bun.BaseModel `bun:"table:social_accounts" swaggerignore:"true"`

	ID        int64     `bun:",pk,autoincrement" json:"id"`
	UserID    int64     `bun:",notnull" json:"user_id"`
	Provider  string    `bun:",notnull" json:"provider"`
	SocialID  string    `bun:"social_id,notnull" json:"social_id"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`

	User *User `bun:"rel:belongs-to,join:user_id=id" json:"-"`
}

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusInactive UserStatus = "inactive"
	UserStatusBanned   UserStatus = "banned"
)
