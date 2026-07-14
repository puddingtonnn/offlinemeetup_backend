package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// pngBytes — валидная PNG-сигнатура (8 байт), которой достаточно, чтобы
// media.Detect распознал image/png. Загрузка теперь валидирует реальные
// байты, поэтому тестовое тело должно быть настоящей картинкой.
const pngBytes = "\x89PNG\r\n\x1a\n" + "rest-of-the-image-payload"

type mockS3Client struct {
	putObjectFunc func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putObjectFunc != nil {
		return m.putObjectFunc(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

func TestFileService_Upload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockFileRepository(ctrl)
	cfg := &config.Config{
		S3Bucket:      "test-bucket",
		MaxUploadSize: 100 << 20,
	}

	ctx := context.Background()
	const userID = int64(7)
	fileName := "test.png"
	size := int64(1024)

	t.Run("success", func(t *testing.T) {
		s3Client := &mockS3Client{
			putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
				assert.Equal(t, "test-bucket", *params.Bucket)
				assert.Equal(t, "image/png", *params.ContentType) // тип — из реальных байт
				assert.True(t, strings.HasPrefix(*params.Key, "uploads/"))
				assert.True(t, strings.HasSuffix(*params.Key, ".png")) // расширение — из типа
				assert.Equal(t, int64(1024), *params.ContentLength)
				return &s3.PutObjectOutput{}, nil
			},
		}

		srv := NewFileService(repo, s3Client, cfg)

		repo.EXPECT().Create(ctx, gomock.Any()).Return(nil)

		file, err := srv.Upload(ctx, userID, fileName, size, strings.NewReader(pngBytes))
		assert.NoError(t, err)
		assert.NotNil(t, file)
		assert.Equal(t, fileName, file.FileName)
		assert.Equal(t, "test-bucket", file.Bucket)
		assert.Equal(t, size, file.Size)
		assert.Equal(t, "image/png", file.MimeType)
		assert.True(t, strings.HasPrefix(file.Key, "uploads/"))
		if assert.NotNil(t, file.UploadedBy) {
			assert.Equal(t, userID, *file.UploadedBy) // владелец зафиксирован
		}
	})

	t.Run("s3_upload_error", func(t *testing.T) {
		s3Err := errors.New("s3 error")
		s3Client := &mockS3Client{
			putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
				return nil, s3Err
			},
		}

		srv := NewFileService(repo, s3Client, cfg)

		// repo.Create should not be called

		file, err := srv.Upload(ctx, userID, fileName, size, strings.NewReader(pngBytes))
		assert.Error(t, err)
		assert.ErrorContains(t, err, "failed to upload to s3")
		assert.Nil(t, file)
	})

	t.Run("repo_create_error", func(t *testing.T) {
		s3Client := &mockS3Client{
			putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
				return &s3.PutObjectOutput{}, nil
			},
		}

		srv := NewFileService(repo, s3Client, cfg)

		repoErr := errors.New("db error")
		repo.EXPECT().Create(ctx, gomock.Any()).Return(repoErr)

		file, err := srv.Upload(ctx, userID, fileName, size, strings.NewReader(pngBytes))
		assert.ErrorIs(t, err, repoErr)
		assert.ErrorContains(t, err, "failed to save file metadata")
		assert.Nil(t, file)
	})

	t.Run("rejects unsupported mime type", func(t *testing.T) {
		srv := NewFileService(repo, &mockS3Client{}, cfg)
		file, err := srv.Upload(ctx, userID, "evil.html", size, strings.NewReader("<!DOCTYPE html>"))
		assert.ErrorIs(t, err, ErrInvalidInput)
		assert.Nil(t, file)
	})

	t.Run("rejects content-type spoofing (html bytes)", func(t *testing.T) {
		srv := NewFileService(repo, &mockS3Client{}, cfg)
		body := strings.NewReader("<!DOCTYPE html><script>alert(1)</script>")
		file, err := srv.Upload(ctx, userID, "evil.png", size, body)
		assert.ErrorIs(t, err, ErrInvalidInput)
		assert.Nil(t, file)
	})

	t.Run("rejects oversize file", func(t *testing.T) {
		srv := NewFileService(repo, &mockS3Client{}, cfg)
		file, err := srv.Upload(ctx, userID, "big.png", cfg.MaxUploadSize+1, strings.NewReader(pngBytes))
		assert.ErrorIs(t, err, ErrInvalidInput)
		assert.Nil(t, file)
	})

	t.Run("rejects zero size", func(t *testing.T) {
		srv := NewFileService(repo, &mockS3Client{}, cfg)
		file, err := srv.Upload(ctx, userID, "empty.png", 0, strings.NewReader(""))
		assert.ErrorIs(t, err, ErrInvalidInput)
		assert.Nil(t, file)
	})

	t.Run("accepts mp4 video", func(t *testing.T) {
		s3Client := &mockS3Client{
			putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
				assert.Equal(t, "video/mp4", *params.ContentType)
				assert.True(t, strings.HasSuffix(*params.Key, ".mp4"))
				return &s3.PutObjectOutput{}, nil
			},
		}
		srv := NewFileService(repo, s3Client, cfg)
		repo.EXPECT().Create(ctx, gomock.Any()).Return(nil)

		body := strings.NewReader("\x00\x00\x00\x20ftypisom" + "rest-of-video")
		file, err := srv.Upload(ctx, userID, "clip.mp4", size, body)
		assert.NoError(t, err)
		assert.Equal(t, "video/mp4", file.MimeType)
	})

	t.Run("accepts mp3 audio", func(t *testing.T) {
		s3Client := &mockS3Client{
			putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
				assert.Equal(t, "audio/mpeg", *params.ContentType)
				assert.True(t, strings.HasSuffix(*params.Key, ".mp3"))
				return &s3.PutObjectOutput{}, nil
			},
		}
		srv := NewFileService(repo, s3Client, cfg)
		repo.EXPECT().Create(ctx, gomock.Any()).Return(nil)

		body := strings.NewReader("ID3\x03\x00\x00\x00\x00\x00\x00rest-of-audio")
		file, err := srv.Upload(ctx, userID, "voice.mp3", size, body)
		assert.NoError(t, err)
		assert.Equal(t, "audio/mpeg", file.MimeType)
	})

	t.Run("rejects pdf document", func(t *testing.T) {
		srv := NewFileService(repo, &mockS3Client{}, cfg)
		file, err := srv.Upload(ctx, userID, "doc.pdf", size, strings.NewReader("%PDF-1.7\n%...."))
		assert.ErrorIs(t, err, ErrInvalidInput)
		assert.Nil(t, file)
	})
}
