package domain

import (
	"time"

	"github.com/uptrace/bun"
)

// UserCredentials holds the bcrypt password hash for a user (ADR-6). Lives in
// its own table, NOT on User — users.* is read on every authenticated request
// (AuthMiddleware -> UserRepo.GetByID) and mapped into UserResponse, so the
// hash must never be reachable through that path.
type UserCredentials struct {
	bun.BaseModel `bun:"table:user_credentials"`

	UserID       int64     `bun:"user_id,pk"`
	PasswordHash string    `bun:"password_hash,notnull"`
	UpdatedAt    time.Time `bun:"updated_at,nullzero,notnull,default:current_timestamp"`
}
