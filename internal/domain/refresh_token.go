package domain

import (
	"time"

	"github.com/uptrace/bun"
)

// RefreshToken is a long-lived, opaque credential used to mint fresh access
// tokens. Only its SHA-256 hash is stored — the raw token lives on the client.
// Rotation revokes the old row on each use; RevokedAt makes revocation (logout /
// reuse-detection) explicit without deleting the audit row.
type RefreshToken struct {
	bun.BaseModel `bun:"table:refresh_tokens"`

	ID        int64      `bun:",pk,autoincrement"`
	UserID    int64      `bun:"user_id,notnull"`
	TokenHash string     `bun:"token_hash,notnull"`
	ExpiresAt time.Time  `bun:"expires_at,notnull"`
	RevokedAt *time.Time `bun:"revoked_at"`
	CreatedAt time.Time  `bun:"created_at,nullzero,notnull,default:current_timestamp"`
}
