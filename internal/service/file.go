package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

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

// imageTypeExt сопоставляет провалидированный MIME-тип с расширением. Расширение
// берётся отсюда, а не из имени клиента, чтобы нельзя было задать произвольное
// (active-content) расширение в публичном ключе объекта.
var imageTypeExt = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
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

func (s *FileService) Upload(ctx context.Context, userID int64, fileName string, contentType string, size int64, reader io.Reader) (*domain.File, error) {
	if !allowedImageTypes[contentType] {
		return nil, fmt.Errorf("unsupported file type %q: %w", contentType, ErrInvalidInput)
	}
	if size <= 0 || size > maxFileSize {
		return nil, fmt.Errorf("file size out of range: %w", ErrInvalidInput)
	}

	// Не доверяем заявленному клиентом Content-Type: определяем реальный тип по
	// первым 512 байтам (mime-sniffing) и от него берём и расширение, и
	// сохраняемый Content-Type. Иначе можно залить произвольные байты под видом
	// картинки или подсунуть active-content расширение в публичный ключ объекта.
	head := make([]byte, 512)
	n, err := io.ReadFull(reader, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("reading file head: %w", err)
	}
	head = head[:n]

	detected := http.DetectContentType(head)
	ext, ok := imageTypeExt[detected]
	if !ok {
		return nil, fmt.Errorf("file content is not an allowed image (%s): %w", detected, ErrInvalidInput)
	}

	fileID := uuid.New()
	key := fmt.Sprintf("uploads/%s%s", fileID.String(), ext)

	// Возвращаем прочитанный префикс обратно в поток перед загрузкой.
	body := io.MultiReader(bytes.NewReader(head), reader)

	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.S3Bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(detected),
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
		MimeType:   detected,
		UploadedBy: &userID,
	}

	if err := s.repo.Create(ctx, file); err != nil {
		return nil, fmt.Errorf("failed to save file metadata: %w", err)
	}

	return file, nil
}
