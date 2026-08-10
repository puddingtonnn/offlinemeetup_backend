package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

// ErrNotFound means a row the caller asked for by key does not exist — a
// user_credentials row (a user who only ever logged in via a social provider
// has none), or a user/profile looked up by email/username. The service layer
// translates this into service.ErrNotFound at the boundary, same pattern as
// ErrChatReadOnly/ErrNotChatMember in chat.go.
var ErrNotFound = errors.New("row not found")

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
	return upsertCredentials(ctx, r.db, userID, hash)
}

// upsertCredentials is the single INSERT ... ON CONFLICT that writes a
// password hash. It takes a bun.IDB so it works both standalone (CredentialsRepo)
// and inside another repo's transaction (UserRepo's registration/attach paths,
// which must write credentials in the SAME tx as the user/profile rows).
func upsertCredentials(ctx context.Context, idb bun.IDB, userID int64, hash string) error {
	creds := &domain.UserCredentials{UserID: userID, PasswordHash: hash}
	_, err := idb.NewInsert().
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
