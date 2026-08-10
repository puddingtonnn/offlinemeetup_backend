package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

// ErrNotFound means the requested user_credentials row does not exist (a
// user who only ever logged in via a social provider has none). The service
// layer translates this into service.ErrNotFound at the boundary, same
// pattern as ErrChatReadOnly/ErrNotChatMember in chat.go.
var ErrNotFound = errors.New("user credentials not found")

// CredentialsRepo owns the password hash for a user (ADR-6). Kept in its own
// table/file, separate from UserRepo, so a plain GetByID of a user never
// touches this data.
type CredentialsRepo struct {
	db *bun.DB
}

// NewCredentialsRepo builds a CredentialsRepo over a Bun DB.
func NewCredentialsRepo(db *bun.DB) *CredentialsRepo {
	return &CredentialsRepo{db: db}
}

// Get returns the credentials row for a user, or ErrNotFound if none exists.
func (r *CredentialsRepo) Get(ctx context.Context, userID int64) (*domain.UserCredentials, error) {
	var creds domain.UserCredentials
	err := r.db.NewSelect().Model(&creds).Where("user_id = ?", userID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting user credentials: %w", err)
	}
	return &creds, nil
}

// Upsert inserts or replaces a user's password hash in one statement (real
// ON CONFLICT upsert, not select-then-insert-or-update — used both by initial
// verify and by change/reset password, all of which want a single atomic
// write regardless of whether a row already exists).
func (r *CredentialsRepo) Upsert(ctx context.Context, userID int64, hash string) error {
	creds := &domain.UserCredentials{UserID: userID, PasswordHash: hash}
	_, err := r.db.NewInsert().
		Model(creds).
		On("CONFLICT (user_id) DO UPDATE").
		Set("password_hash = EXCLUDED.password_hash").
		Set("updated_at = current_timestamp").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("upserting user credentials: %w", err)
	}
	return nil
}
