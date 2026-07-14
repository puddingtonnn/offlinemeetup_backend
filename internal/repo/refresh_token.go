package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

type RefreshTokenRepo struct {
	db *bun.DB
}

func NewRefreshTokenRepo(db *bun.DB) *RefreshTokenRepo {
	return &RefreshTokenRepo{db: db}
}

// Create stores a new refresh token (its hash) for a user.
func (r *RefreshTokenRepo) Create(ctx context.Context, token *domain.RefreshToken) error {
	_, err := r.db.NewInsert().Model(token).Returning("id, created_at").Exec(ctx)
	if err != nil {
		return fmt.Errorf("creating refresh token: %w", err)
	}
	return nil
}

// GetByHash looks a token up by its SHA-256 hash. Returns (nil, nil) when absent
// so a missing token is distinct from a backend error.
func (r *RefreshTokenRepo) GetByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	var token domain.RefreshToken
	err := r.db.NewSelect().Model(&token).Where("token_hash = ?", hash).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting refresh token: %w", err)
	}
	return &token, nil
}

// Revoke marks a single token revoked. Idempotent: a WHERE on revoked_at IS NULL
// keeps a re-revoke a no-op rather than moving the timestamp.
func (r *RefreshTokenRepo) Revoke(ctx context.Context, id int64) error {
	_, err := r.db.NewUpdate().
		Table("refresh_tokens").
		Set("revoked_at = ?", time.Now()).
		Where("id = ? AND revoked_at IS NULL", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}
	return nil
}

// RevokeAllForUser revokes every live token of a user (logout-everywhere and the
// reuse-detection response to a replayed rotated token).
func (r *RefreshTokenRepo) RevokeAllForUser(ctx context.Context, userID int64) error {
	_, err := r.db.NewUpdate().
		Table("refresh_tokens").
		Set("revoked_at = ?", time.Now()).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("revoking user refresh tokens: %w", err)
	}
	return nil
}
