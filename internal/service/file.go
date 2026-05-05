package service

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
)

type FileRepository interface {
	Create(ctx context.Context, file *domain.File) error
}

type FileService struct {
	repo     FileRepository
	s3Client *s3.Client
	cfg      *config.Config
}

func NewFileService(repo FileRepository, s3Client *s3.Client, cfg *config.Config) *FileService {
	return &FileService{
		repo:     repo,
		s3Client: s3Client,
		cfg:      cfg,
	}
}

func (s *FileService) Upload(ctx context.Context, fileName string, contentType string, size int64, reader io.Reader) (*domain.File, error) {
	fileID := uuid.New()
	ext := filepath.Ext(fileName)
	key := fmt.Sprintf("uploads/%s%s", fileID.String(), ext)

	_, err := s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.S3Bucket),
		Key:         aws.String(key),
		Body:        reader,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload to s3: %w", err)
	}

	file := &domain.File{
		ID:       fileID,
		FileName: fileName,
		Key:      key,
		Bucket:   s.cfg.S3Bucket,
		Size:     size,
		MimeType: contentType,
	}

	if err := s.repo.Create(ctx, file); err != nil {
		return nil, fmt.Errorf("failed to save file metadata: %w", err)
	}

	return file, nil
}
