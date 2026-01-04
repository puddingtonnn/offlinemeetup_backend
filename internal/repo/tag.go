package repo

import (
	"context"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

type TagRepo struct {
	db *bun.DB
}

func NewTagRepo(db *bun.DB) *TagRepo {
	return &TagRepo{db: db}
}

func (r *TagRepo) GetAll(ctx context.Context) ([]domain.Tag, error) {
	var tags []domain.Tag

	err := r.db.NewSelect().
		Model(&tags).
		Order("name ASC").Scan(ctx)

	return tags, err
}
