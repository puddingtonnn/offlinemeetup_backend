package domain

import (
	"github.com/uptrace/bun"
	"time"
)

type User struct {
	bun.BaseModel `bun:"table:users"`
	ID            int64            `bun:",pk,autoincrement" json:"id"`
	Email         string           `bun:",unique,nullzero" json:"email"`
	Role          string           `bun:"default:'user'" json:"role"`
	Status        UserStatus       `bun:",type:varchar(20),default:'active'" json:"status"`
	CreatedAt     time.Time        `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time        `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
	Socials       []*SocialAccount `bun:"rel:has-many,join:id=user_id" json:"socials,omitempty"`
}

type SocialAccount struct {
	bun.BaseModel `bun:"table:social_accounts"`

	ID        int64     `bun:",pk,autoincrement"`
	UserID    int64     `bun:",notnull"`
	Provider  string    `bun:",notnull"`
	SocialID  string    `bun:"social_id,notnull"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
}

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusInactive UserStatus = "inactive"
	UserStatusBanned   UserStatus = "banned"
)

type Profile struct {
	bun.BaseModel `bun:"table:profile"`

	ID          int64  `bun:",pk,autoincrement"`
	UserID      int64  `bun:",notnull"`
	Nickname    string `bun:",unique,notnull"`
	Bio         string
	AvatarURL   string    `bun:",notnull"`
	IsOrganizer bool      `bun:",notnull"`
	Gender      string    `bun:",notnull"`
	UpdatedAt   time.Time `bun:",nullzero,notnull,default:current_timestamp"`

	User *User `bun:"rel:belongs-to,join:user_id=id"`
}
