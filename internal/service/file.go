package service

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service/media"
)

type FileRepository interface {
	Create(ctx context.Context, file *domain.File) error
}

type S3PutObjectAPI interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

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

// Upload validates a file by its real bytes (media whitelist), then stores it in
// S3 and records its metadata. reader must be seekable: the first 512 bytes are
// read to sniff the type, then it is rewound so the SDK can sign and stream the
// body without buffering the whole file in memory.
func (s *FileService) Upload(ctx context.Context, userID int64, fileName string, size int64, reader io.ReadSeeker) (*domain.File, error) {
	if size <= 0 || size > s.cfg.MaxUploadSize {
		return nil, fmt.Errorf("file size out of range: %w", ErrInvalidInput)
	}

	head := make([]byte, 512)
	n, err := io.ReadFull(reader, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("reading file head: %w", err)
	}
	head = head[:n]

	mimeType, ext, ok := media.Detect(head)
	if !ok {
		return nil, fmt.Errorf("file content is not an allowed media type: %w", ErrInvalidInput)
	}

	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewinding upload stream: %w", err)
	}

	// Best-effort metadata pass over the seekable stream: extracts duration +
	// dimensions and corrects an audio-only mp4 that sniffed as video. A parse
	// failure degrades to the Detect result, so it never blocks the upload.
	meta := media.ExtractMeta(mimeType, ext, reader)
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewinding upload stream: %w", err)
	}

	fileID := uuid.New()
	key := fmt.Sprintf("uploads/%s%s", fileID.String(), meta.Ext)

	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.cfg.S3Bucket),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(meta.Mime),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload to s3: %w", err)
	}

	file := &domain.File{
		ID:         fileID,
		FileName:   fileName,
		Key:        key,
		Bucket:     s.cfg.S3Bucket,
		Size:       size,
		MimeType:   meta.Mime,
		UploadedBy: &userID,
		DurationMS: meta.DurationMS,
		Width:      meta.Width,
		Height:     meta.Height,
	}

	if err := s.repo.Create(ctx, file); err != nil {
		return nil, fmt.Errorf("failed to save file metadata: %w", err)
	}

	return file, nil
}
