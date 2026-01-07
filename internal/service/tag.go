package service

import (
	"context"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
)

type TagRepository interface {
	GetAll(ctx context.Context) ([]domain.Tag, error)
}

type TagService struct {
	repo TagRepository
}

func NewTagService(repo TagRepository) *TagService {
	return &TagService{repo: repo}
}

func (s *TagService) ListTags(ctx context.Context) ([]domain.Tag, error) {
	return s.repo.GetAll(ctx)
}
