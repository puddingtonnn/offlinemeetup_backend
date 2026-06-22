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
		S3Bucket: "test-bucket",
	}

	ctx := context.Background()
	fileName := "test.png"
	contentType := "image/png"
	size := int64(1024)
	reader := strings.NewReader("dummy content")

	t.Run("success", func(t *testing.T) {
		s3Client := &mockS3Client{
			putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
				assert.Equal(t, "test-bucket", *params.Bucket)
				assert.Equal(t, contentType, *params.ContentType)
				assert.True(t, strings.HasPrefix(*params.Key, "uploads/"))
				assert.True(t, strings.HasSuffix(*params.Key, ".png"))
				return &s3.PutObjectOutput{}, nil
			},
		}

		srv := NewFileService(repo, s3Client, cfg)

		repo.EXPECT().Create(ctx, gomock.Any()).Return(nil)

		file, err := srv.Upload(ctx, fileName, contentType, size, reader)
		assert.NoError(t, err)
		assert.NotNil(t, file)
		assert.Equal(t, fileName, file.FileName)
		assert.Equal(t, "test-bucket", file.Bucket)
		assert.Equal(t, size, file.Size)
		assert.Equal(t, contentType, file.MimeType)
		assert.True(t, strings.HasPrefix(file.Key, "uploads/"))
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

		file, err := srv.Upload(ctx, fileName, contentType, size, reader)
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

		file, err := srv.Upload(ctx, fileName, contentType, size, reader)
		assert.ErrorIs(t, err, repoErr)
		assert.ErrorContains(t, err, "failed to save file metadata")
		assert.Nil(t, file)
	})
}
