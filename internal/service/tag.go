package service

import (
	"context"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
)

type TagService struct {
	repo *repo.TagRepo
}

func NewTagService(repo *repo.TagRepo) *TagService {
	return &TagService{repo: repo}
}

func (s *TagService) ListTags(ctx context.Context) ([]domain.Tag, error) {
	return s.repo.GetAll(ctx)
}
