package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

// ErrFileNotOwned means a user referenced a file uploaded by someone else (or a
// non-existent / owner-less file). Reference paths translate it to ErrForbidden.
var ErrFileNotOwned = errors.New("file is not owned by user")

type FileRepo struct {
	db *bun.DB
}

func NewFileRepo(db *bun.DB) *FileRepo {
	return &FileRepo{db: db}
}

func (r *FileRepo) Create(ctx context.Context, file *domain.File) error {
	_, err := r.db.NewInsert().Model(file).Exec(ctx)
	return err
}

// fileOwnedBy reports whether the file exists and was uploaded by userID. Used
// to stop a user attaching another user's uploaded file (cover, avatar, message
// attachment) by replaying its id. Runs on the given IDB so it can join an
// existing transaction.
func fileOwnedBy(ctx context.Context, idb bun.IDB, fileID uuid.UUID, userID int64) (bool, error) {
	exists, err := idb.NewSelect().
		Model((*domain.File)(nil)).
		Where("id = ? AND uploaded_by = ?", fileID, userID).
		Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("checking file ownership: %w", err)
	}
	return exists, nil
}
