package repo

import (
	"context"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

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
