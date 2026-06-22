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

type S3PutObjectAPI interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// allowedImageTypes — разрешённые MIME-типы для загрузки (аватары, обложки).
var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// maxFileSize — максимальный размер файла (10 MB), согласован с лимитом тела в хендлере.
const maxFileSize = 10 << 20

type FileService struct {
	repo     FileRepository
	s3Client S3PutObjectAPI
	cfg      *config.Config
}

func NewFileService(repo FileRepository, s3Client S3PutObjectAPI, cfg *config.Config) *FileService {
	return &FileService{
		repo:     repo,
		s3Client: s3Client,
		cfg:      cfg,
	}
}

func (s *FileService) Upload(ctx context.Context, fileName string, contentType string, size int64, reader io.Reader) (*domain.File, error) {
	if !allowedImageTypes[contentType] {
		return nil, fmt.Errorf("unsupported file type %q: %w", contentType, ErrInvalidInput)
	}
	if size <= 0 || size > maxFileSize {
		return nil, fmt.Errorf("file size out of range: %w", ErrInvalidInput)
	}

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
