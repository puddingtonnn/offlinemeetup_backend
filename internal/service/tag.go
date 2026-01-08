package service

import (
	"context"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"

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

func (s *TagService) ListTags(ctx context.Context) ([]dto.TagResponse, error) {
	tags, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	res := make([]dto.TagResponse, len(tags))
	for i, t := range tags {
		res[i] = dto.TagResponse{
			ID:   t.ID,
			Name: t.Name,
		}
	}
	return res, nil
}
