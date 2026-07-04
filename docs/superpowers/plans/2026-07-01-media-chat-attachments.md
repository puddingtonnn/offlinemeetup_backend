# Media Chat Attachments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let chat messages carry photo / video / audio attachments up to 100 MB, while meetup covers and profile avatars stay image-only.

**Architecture:** Keep the single app-proxied `POST /v1/files/upload` endpoint. Broaden type validation from "image-only" to a media whitelist via a new byte-sniffing package `internal/service/media`. Make the size limit config-driven. Stream a **seekable** body to S3 to avoid buffering 100 MB in RAM. Enforce image-only for covers/avatars at reference time in the repo transaction.

**Tech Stack:** Go 1.26.2, Chi, Bun 1.2.16 (PostgreSQL), aws-sdk-go-v2 S3 (MinIO), gomock + miniredis for tests.

## Global Constraints

- Go 1.26.2; Bun 1.2.16. Do not add new third-party dependencies.
- **Validation is byte-based, never filename/Content-Type-based.** The stored `Content-Type` and object-key extension are always derived from the detected type. This is a load-bearing security invariant.
- Commit messages: English, concise imperative subject + short bullet list; **no watermark / no Co-Authored-By trailer** (user global rule).
- Service-layer tests use `go.uber.org/mock` (gomock); Redis-backed code uses miniredis. **The repo layer has no unit tests** — repo behavior is exercised through service tests with mocked repo interfaces.
- `internal/service/errors.go` holds domain sentinels; repo-internal sentinels live in the `repo` package and are translated to domain sentinels at the service boundary (`errors.Is`, never string matching).
- No DB migration is needed: `files.mime_type` and `messages.file_id` already exist.

---

### Task 1: Config `MaxUploadSize`

**Files:**
- Modify: `internal/config/config.go` (add `int64Env` helper + `MaxUploadSize` field + load line)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config.MaxUploadSize int64` (bytes; default `100 << 20`). Read from env `MAX_UPLOAD_SIZE`. Optional (has a default) — **not** a fail-fast secret.

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestLoad_MaxUploadSizeDefault(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("DB_DSN", "postgres://localhost/test")
	t.Setenv("MAX_UPLOAD_SIZE", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, int64(100<<20), cfg.MaxUploadSize)
}

func TestLoad_MaxUploadSizeOverride(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("DB_DSN", "postgres://localhost/test")
	t.Setenv("MAX_UPLOAD_SIZE", "52428800") // 50 MB

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, int64(52428800), cfg.MaxUploadSize)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_MaxUploadSize -v`
Expected: FAIL — `cfg.MaxUploadSize` undefined (compile error).

- [ ] **Step 3: Add the field, helper, and load line**

In `internal/config/config.go`, add the field to the `Config` struct (near the other scalar fields, e.g. after `WSAllowedOrigins`):

```go
	// MaxUploadSize — максимальный размер загружаемого файла в байтах
	// (MAX_UPLOAD_SIZE, дефолт 100 MB). Применяется и в хендлере (MaxBytesReader),
	// и в FileService.Upload.
	MaxUploadSize int64
```

Add the helper next to `durEnv`:

```go
// int64Env reads an integer (bytes) from env; on an empty or unparseable value
// it returns def.
func int64Env(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}
```

In `Load`, add after the `cfg.PresenceTTL = ...` line:

```go
	cfg.MaxUploadSize = int64Env("MAX_UPLOAD_SIZE", 100<<20)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoad_MaxUploadSize -v`
Expected: PASS (both subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): configurable MAX_UPLOAD_SIZE (default 100MB)"
```

---

### Task 2: `media` byte-sniffing package

**Files:**
- Create: `internal/service/media/detect.go`
- Test: `internal/service/media/detect_test.go`

**Interfaces:**
- Produces: `media.Detect(head []byte) (mime, ext string, ok bool)` — returns the canonical media MIME type and stored extension for a file's first bytes; `ok=false` for anything not in the allowed media whitelist. Callers pass the first ≤512 bytes.

- [ ] **Step 1: Write the failing test**

Create `internal/service/media/detect_test.go`:

```go
package media

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		name     string
		head     []byte
		wantMime string
		wantExt  string
		wantOK   bool
	}{
		{"png", []byte("\x89PNG\r\n\x1a\n" + "payload"), "image/png", ".png", true},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 'J', 'F', 'I', 'F'}, "image/jpeg", ".jpg", true},
		{"gif", []byte("GIF89a" + "payload"), "image/gif", ".gif", true},
		{"mp4 isom", append([]byte{0, 0, 0, 0x20}, []byte("ftypisom....")...), "video/mp4", ".mp4", true},
		{"mov", append([]byte{0, 0, 0, 0x14}, []byte("ftypqt  ....")...), "video/quicktime", ".mov", true},
		{"m4a", append([]byte{0, 0, 0, 0x20}, []byte("ftypM4A ....")...), "audio/mp4", ".m4a", true},
		{"heic", append([]byte{0, 0, 0, 0x18}, []byte("ftypheic....")...), "image/heic", ".heic", true},
		{"3gp", append([]byte{0, 0, 0, 0x18}, []byte("ftyp3gp4....")...), "video/3gpp", ".3gp", true},
		{"webm/ebml", []byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0}, "video/webm", ".webm", true},
		{"ogg", []byte("OggS" + "payload...."), "audio/ogg", ".ogg", true},
		{"flac", []byte("fLaC" + "payload...."), "audio/flac", ".flac", true},
		{"wav", []byte("RIFF" + "size" + "WAVE" + "...."), "audio/wav", ".wav", true},
		{"mp3 id3", []byte("ID3" + "\x03\x00\x00\x00\x00\x00\x00"), "audio/mpeg", ".mp3", true},
		{"pdf rejected", []byte("%PDF-1.7\n%...."), "", "", false},
		{"svg rejected", []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"), "", "", false},
		{"exe rejected", []byte("MZ\x90\x00\x03\x00\x00\x00...."), "", "", false},
		{"empty rejected", []byte{}, "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mime, ext, ok := Detect(tc.head)
			if ok != tc.wantOK || mime != tc.wantMime || ext != tc.wantExt {
				t.Fatalf("Detect() = (%q, %q, %v), want (%q, %q, %v)",
					mime, ext, ok, tc.wantMime, tc.wantExt, tc.wantOK)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/media/ -v`
Expected: FAIL — package/`Detect` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/service/media/detect.go`:

```go
// Package media resolves the MIME type of an uploaded file from its real bytes
// and gates it to an allowed media whitelist (photo / video / audio). The stored
// Content-Type and object-key extension are always derived here from the sniffed
// bytes, never from the client filename — an attacker must not be able to inject
// an active-content extension (.html/.svg/.js) into a public object key or store
// non-media bytes disguised as media.
package media

import (
	"bytes"
	"net/http"
	"strings"
)

// allowed maps a canonical media MIME type to its stored file extension.
var allowed = map[string]string{
	// images
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
	"image/heic": ".heic",
	"image/heif": ".heif",
	// video
	"video/mp4":       ".mp4",
	"video/quicktime": ".mov",
	"video/webm":      ".webm",
	"video/3gpp":      ".3gp",
	"video/x-msvideo": ".avi",
	// audio
	"audio/mpeg": ".mp3",
	"audio/mp4":  ".m4a",
	"audio/aac":  ".aac",
	"audio/ogg":  ".ogg",
	"audio/wav":  ".wav",
	"audio/flac": ".flac",
}

// sniffAlias maps names http.DetectContentType uses to our canonical names.
var sniffAlias = map[string]string{
	"audio/wave":      "audio/wav",
	"application/ogg": "audio/ogg",
	"video/avi":       "video/x-msvideo",
}

// Detect resolves the media MIME type + stored extension from a file's first
// bytes. ok is false for anything not in the allowed media set (documents, svg,
// executables, unknown containers).
func Detect(head []byte) (mime, ext string, ok bool) {
	// 1) Go's built-in sniffer: reliable for images and some a/v.
	if m := normalize(http.DetectContentType(head)); m != "" {
		if e, found := allowed[m]; found {
			return m, e, true
		}
	}
	// 2) Supplementary magic-byte checks for media Go's sniffer misses
	//    (.mov / .m4a / .heic / .mkv / .flac / raw aac).
	if m := detectMagic(head); m != "" {
		if e, found := allowed[m]; found {
			return m, e, true
		}
	}
	return "", "", false
}

// normalize strips any "; charset=..." suffix and maps sniffer names to canonical.
func normalize(m string) string {
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	if alias, ok := sniffAlias[m]; ok {
		return alias
	}
	return m
}

// detectMagic covers media containers the standard library does not sniff.
func detectMagic(b []byte) string {
	// ISO Base Media File Format: [4-byte box size]"ftyp"[major brand].
	if len(b) >= 12 && bytes.Equal(b[4:8], []byte("ftyp")) {
		return byBrand(string(b[8:12]))
	}
	// Matroska / WebM: EBML header. Both are canonicalized to video/webm.
	if len(b) >= 4 && bytes.Equal(b[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		return "video/webm"
	}
	// Ogg container (vorbis/opus).
	if len(b) >= 4 && bytes.Equal(b[:4], []byte("OggS")) {
		return "audio/ogg"
	}
	// FLAC.
	if len(b) >= 4 && bytes.Equal(b[:4], []byte("fLaC")) {
		return "audio/flac"
	}
	// RIFF containers.
	if len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) {
		switch string(b[8:12]) {
		case "WAVE":
			return "audio/wav"
		case "AVI ":
			return "video/x-msvideo"
		}
	}
	// MP3 with an ID3 tag.
	if len(b) >= 3 && bytes.Equal(b[:3], []byte("ID3")) {
		return "audio/mpeg"
	}
	// Raw AAC (ADTS syncword 0xFFFx, layer 00) — check before the broader MP3 sync.
	if len(b) >= 2 && b[0] == 0xFF && (b[1]&0xF6) == 0xF0 {
		return "audio/aac"
	}
	// MPEG audio frame sync (11 bits set).
	if len(b) >= 2 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0 {
		return "audio/mpeg"
	}
	return ""
}

// byBrand maps an ISOBMFF major brand to a canonical MIME type. Unknown brands
// fall back to video/mp4 (the file is still ISOBMFF media).
func byBrand(brand string) string {
	switch brand {
	case "qt  ":
		return "video/quicktime"
	case "M4A ", "M4B ":
		return "audio/mp4"
	case "heic", "heix", "hevc", "heim", "heis":
		return "image/heic"
	case "mif1", "msf1", "heif":
		return "image/heif"
	case "3gp4", "3gp5", "3gp6", "3g2a":
		return "video/3gpp"
	default:
		return "video/mp4"
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/media/ -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/service/media/
git commit -m "feat(media): byte-sniffing detector for photo/video/audio whitelist"
```

---

### Task 3: `FileService.Upload` media support + handler wiring

**Files:**
- Modify: `internal/service/file.go` (rewrite `Upload`; drop `allowedImageTypes`/`imageTypeExt`/`maxFileSize`)
- Modify: `internal/transport/http/handler/file.go` (config-driven limits, seekable body, drop client Content-Type)
- Modify: `internal/app/app.go:104` (pass `cfg.MaxUploadSize` to the handler)
- Test: `internal/service/file_test.go`

**Interfaces:**
- Consumes: `media.Detect` (Task 2), `config.Config.MaxUploadSize` (Task 1).
- Produces: `func (s *FileService) Upload(ctx context.Context, userID int64, fileName string, size int64, reader io.ReadSeeker) (*domain.File, error)` — note the signature drops the client `contentType` param and takes `io.ReadSeeker` (satisfied by `*strings.Reader` in tests and `multipart.File` in the handler). `handler.NewFileHandler(service, log, maxUploadSize int64)`.

- [ ] **Step 1: Update the existing tests to the new signature and media cases**

Rewrite `internal/service/file_test.go`. Set `MaxUploadSize` on the test config, **delete the now-unused `contentType := "image/png"` local** (Go fails to compile an unused variable), drop the `contentType` argument from every `Upload` call, and change the oversize bound + add media/reject cases:

```go
	cfg := &config.Config{
		S3Bucket:      "test-bucket",
		MaxUploadSize: 100 << 20,
	}
```

Replace the existing `Upload(ctx, userID, fileName, contentType, size, ...)` calls with the new 5-arg form, e.g. the success call becomes:

```go
		file, err := srv.Upload(ctx, userID, fileName, size, strings.NewReader(pngBytes))
```

Update these existing subtests to drop `contentType` and (for oversize) use the config bound:

```go
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
```

Add new media subtests (put them inside `TestFileService_Upload`):

```go
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
```

Also update the success subtest's PutObject assertion to check `ContentLength`:

```go
				assert.Equal(t, int64(1024), *params.ContentLength)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestFileService_Upload -v`
Expected: FAIL — signature mismatch (`Upload` still takes 6 args / `contentType`), `maxFileSize` referenced, etc. (compile error).

- [ ] **Step 3: Rewrite `Upload` in `internal/service/file.go`**

Replace the imports block, the three `var`/`const` declarations (`allowedImageTypes`, `imageTypeExt`, `maxFileSize`), and the `Upload` method. Final file:

```go
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

	fileID := uuid.New()
	key := fmt.Sprintf("uploads/%s%s", fileID.String(), ext)

	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.cfg.S3Bucket),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(mimeType),
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
		MimeType:   mimeType,
		UploadedBy: &userID,
	}

	if err := s.repo.Create(ctx, file); err != nil {
		return nil, fmt.Errorf("failed to save file metadata: %w", err)
	}

	return file, nil
}
```

- [ ] **Step 4: Update the handler**

Rewrite `internal/transport/http/handler/file.go`:

```go
package handler

import (
	"log/slog"
	"net/http"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	response "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
)

// uploadMemoryBuffer — сколько байт multipart-парсер держит в памяти; всё сверх
// спиллится во временный файл (он seekable, что и нужно FileService.Upload).
const uploadMemoryBuffer = 16 << 20

type FileHandler struct {
	service       *service.FileService
	maxUploadSize int64
	log           *slog.Logger
}

func NewFileHandler(service *service.FileService, maxUploadSize int64, log *slog.Logger) *FileHandler {
	return &FileHandler{service: service, maxUploadSize: maxUploadSize, log: log}
}

// Upload
// @Summary Загрузить медиафайл (фото/видео/аудио)
// @Security BearerAuth
// @Tags 	Files
// @Accept	multipart/form-data
// @Produce	json
// @Param   file formData file true "Медиафайл"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Router /v1/files/upload [post]
func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadSize)

	if err := r.ParseMultipartForm(uploadMemoryBuffer); err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}

	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		response.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}
	defer file.Close()

	res, err := h.service.Upload(r.Context(), userID, header.Filename, header.Size, file)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusCreated, map[string]interface{}{
		"id": res.ID,
	})
}
```

Update the wiring in `internal/app/app.go:104`:

```go
	fileHandler := handler.NewFileHandler(fileService, cfg.MaxUploadSize, log)
```

- [ ] **Step 5: Run tests + build to verify all pass**

Run: `go build ./... && go test ./internal/service/ -run TestFileService_Upload -v`
Expected: build OK; all `Upload` subtests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/file.go internal/transport/http/handler/file.go internal/app/app.go internal/service/file_test.go
git commit -m "feat(files): accept photo/video/audio up to configurable limit

- validate uploads via media byte-sniffing whitelist
- stream a seekable body to S3 (no full-file RAM buffering)
- drive size cap from config; drop untrusted client Content-Type"
```

---

### Task 4: Image-only gate for covers and avatars

Loosening uploads to media means the shared `/files/upload` could yield a video whose id is then referenced as a meetup cover or profile avatar. Enforce image-only at reference time, in the same transaction as the existing ownership check.

**Files:**
- Modify: `internal/repo/file.go` (add `ErrFileNotImage` + `imageFileOwnedBy`)
- Modify: `internal/repo/meetup.go` (create path ~L35-44, update path ~L235-244)
- Modify: `internal/repo/profile.go` (avatar path ~L36-45)
- Modify: `internal/service/meetup.go` (`mapMeetupRepoError`)
- Modify: `internal/service/profile.go` (inline avatar translation ~L118-121)
- Test: `internal/service/meetup_test.go`, `internal/service/profile_test.go`

**Interfaces:**
- Produces: `repo.ErrFileNotImage` (sentinel); `imageFileOwnedBy(ctx context.Context, idb bun.IDB, fileID uuid.UUID, userID int64) error` (unexported; returns `nil` when owned + image, `ErrFileNotOwned` when missing/not owned, `ErrFileNotImage` when owned but not `image/*`). Service boundary maps `ErrFileNotImage` → `service.ErrInvalidInput`.

- [ ] **Step 1: Write failing service tests**

The repo layer has no unit tests, so assert the boundary translation at the service layer. Find the existing test that drives an `UpdateMeetup`/`UpdateProfile` repo error mapping (search `ErrFileNotOwned` in `internal/service/meetup_test.go` and `profile_test.go`) and add a sibling case that returns `repo.ErrFileNotImage` and expects `service.ErrInvalidInput`.

For `internal/service/meetup_test.go` (mirror the existing cover-file-forbidden test; the mocked repo method name and `setupMeetupTest` helper are already used there):

```go
func TestMeetupService_UpdateMeetup_CoverNotImage(t *testing.T) {
	d := setupMeetupTest(t) // reuse whatever the file's existing helper is called
	// ... arrange an update that reaches repo.Update, which returns repo.ErrFileNotImage ...
	d.repo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(repo.ErrFileNotImage)

	_, err := d.service.UpdateMeetup(/* existing call shape from the neighbouring test */)
	assert.ErrorIs(t, err, service.ErrInvalidInput)
}
```

For `internal/service/profile_test.go` similarly, mock `UpdateProfile` to return `repo.ErrFileNotImage` and assert `service.ErrInvalidInput`.

> Note to implementer: copy the exact arrange/act shape from the adjacent `ErrFileNotOwned` test in each file (helper name, mock method, and call arguments differ per file). If no such neighbouring test exists, add a focused unit test for the pure mapper instead:
> ```go
> func TestMapMeetupRepoError_NotImage(t *testing.T) {
> 	assert.ErrorIs(t, mapMeetupRepoError(repo.ErrFileNotImage), ErrInvalidInput)
> }
> ```
> (`mapMeetupRepoError` is in package `service`, so this test lives in a `_test.go` file with `package service`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/service/ -run 'NotImage' -v`
Expected: FAIL — `repo.ErrFileNotImage` undefined (compile error).

- [ ] **Step 3: Add the sentinel and helper in `internal/repo/file.go`**

Add imports `database/sql` and `strings`. Add:

```go
// ErrFileNotImage means an owned file was referenced where only an image is
// allowed (meetup cover, profile avatar). Reference paths translate it to
// ErrInvalidInput.
var ErrFileNotImage = errors.New("file is not an image")

// imageFileOwnedBy checks that the file exists, was uploaded by userID, and is an
// image. Used by cover/avatar reference paths, which — unlike chat attachments —
// accept images only. Returns ErrFileNotOwned (missing/not owned) or
// ErrFileNotImage (owned but not image/*).
func imageFileOwnedBy(ctx context.Context, idb bun.IDB, fileID uuid.UUID, userID int64) error {
	var mimeType string
	err := idb.NewSelect().
		Model((*domain.File)(nil)).
		Column("mime_type").
		Where("id = ? AND uploaded_by = ?", fileID, userID).
		Scan(ctx, &mimeType)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrFileNotOwned
	}
	if err != nil {
		return fmt.Errorf("checking file ownership: %w", err)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return ErrFileNotImage
	}
	return nil
}
```

- [ ] **Step 4: Wire the helper into the cover paths (`internal/repo/meetup.go`)**

Create path — replace the existing `fileOwnedBy` block:

```go
	if meetup.CoverFileID.Valid {
		if err := imageFileOwnedBy(ctx, tx, meetup.CoverFileID.UUID, meetup.CreatorID); err != nil {
			return nil, err
		}
	}
```

Update path (inside `RunInTx`, which returns just `error`) — replace the existing block:

```go
		if meetup.CoverFileID.Valid {
			if err := imageFileOwnedBy(ctx, tx, meetup.CoverFileID.UUID, meetup.CreatorID); err != nil {
				return err
			}
		}
```

- [ ] **Step 5: Wire the helper into the avatar path (`internal/repo/profile.go`)**

Replace the existing `fileOwnedBy` block in `UpdateProfile`:

```go
	if profile.AvatarFileID.Valid {
		if err := imageFileOwnedBy(ctx, r.db, profile.AvatarFileID.UUID, profile.UserID); err != nil {
			return nil, err
		}
	}
```

- [ ] **Step 6: Map the sentinel at the service boundary**

`internal/service/meetup.go` — extend `mapMeetupRepoError`:

```go
func mapMeetupRepoError(err error) error {
	switch {
	case errors.Is(err, repo.ErrFileNotOwned):
		return fmt.Errorf("cover file: %w", ErrForbidden)
	case errors.Is(err, repo.ErrFileNotImage):
		return fmt.Errorf("cover file must be an image: %w", ErrInvalidInput)
	}
	return err
}
```

`internal/service/profile.go` — extend the inline translation (after the existing `ErrFileNotOwned` branch, ~L118-121):

```go
		if errors.Is(err, repo.ErrFileNotImage) {
			return nil, fmt.Errorf("avatar file must be an image: %w", ErrInvalidInput)
		}
```

- [ ] **Step 7: Run tests + build to verify pass**

Run: `go build ./... && go test ./internal/service/ -run 'NotImage' -v`
Expected: build OK; new tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/repo/file.go internal/repo/meetup.go internal/repo/profile.go internal/service/meetup.go internal/service/profile.go internal/service/meetup_test.go internal/service/profile_test.go
git commit -m "feat(files): keep meetup covers and avatars image-only

- add repo.ErrFileNotImage + imageFileOwnedBy (ownership + image/* check)
- gate cover/avatar references; map to ErrInvalidInput at service boundary"
```

---

### Task 5: Full verification + docs

**Files:**
- Modify: `CLAUDE.md` (security-conventions note about "allowed image" uploads; gotcha for `MAX_UPLOAD_SIZE`)

- [ ] **Step 1: Run the whole test suite + vet + lint**

Run:
```bash
go build ./... && go vet ./... && go test ./... && golangci-lint run
```
Expected: all green. Fix anything that fails before continuing.

- [ ] **Step 2: Update `CLAUDE.md`**

In the **Security conventions** section, update the uploads bullet (currently "Uploads are validated by real bytes… requires the result to be in `imageTypeExt`…") to reflect the media whitelist and the image-only gate for cover/avatar. Replace that bullet with:

```markdown
- **Uploads are validated by real bytes, not the client `Content-Type`/filename.** `FileService.Upload` sniffs the first 512 bytes via `internal/service/media` (`media.Detect`), which gates to a **media whitelist** (photo/video/audio) and derives **both** the stored `Content-Type` and the object-key extension from the detected type. The upload stream must be seekable (`io.ReadSeeker`): the head is read, then the stream is rewound so the S3 SDK signs and streams the body without buffering the whole file in RAM. Max size is `config.MaxUploadSize` (`MAX_UPLOAD_SIZE`, default 100 MB), enforced in the handler (`http.MaxBytesReader`) and the service. **Covers/avatars are image-only**: the shared `/files/upload` accepts any media, so the cover/avatar reference paths call `imageFileOwnedBy` in-tx (ownership **and** `mime_type LIKE 'image/%'`), returning `repo.ErrFileNotImage` (→ `ErrInvalidInput`). Chat attachments accept any media type.
```

In the **Gotchas** section, add:

```markdown
- `MAX_UPLOAD_SIZE` (bytes, default `104857600` = 100 MB) caps media uploads. Uploads are app-proxied through the process (multipart → S3 `PutObject`); the body streams from the seekable `multipart.File`, so raising the limit further is safe up to S3's 5 GB single-PutObject ceiling, beyond which multipart/presigned upload would be needed.
```

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: media uploads (photo/video/audio, 100MB) + image-only covers/avatars"
```

---

## Self-Review

**Spec coverage:**
- Media type whitelist (image/video/audio) → Task 2 (`media.Detect`) + Task 3 (wired into `Upload`). ✅
- HEIC/mov/m4a phone-media detection Go misses → Task 2 (`detectMagic`/`byBrand`). ✅
- SVG / documents / executables rejected → Task 2 tests + Task 3 "rejects pdf". ✅
- 100 MB configurable limit → Task 1 + Task 3 (handler + service). ✅
- Seekable body, no RAM buffering → Task 3 (`io.ReadSeeker`, `Seek`, `ContentLength`). ✅
- Covers/avatars stay image-only → Task 4. ✅
- DTO unchanged → no task (correct; spec says no change). ✅
- Security invariant (extension/type from bytes) → Task 2 comment + Task 3 (no client Content-Type). ✅
- Docs (CLAUDE.md security note + gotcha) → Task 5. ✅
- No migration → confirmed in Global Constraints. ✅

**Placeholder scan:** Task 4 Step 1 intentionally defers to the neighbouring test's exact arrange shape (mock method + call args differ per file); a concrete fallback (pure-mapper unit test) is provided so the task is never blocked. No other TBD/TODO.

**Type consistency:** `Upload(ctx, userID int64, fileName string, size int64, reader io.ReadSeeker)` used consistently in Task 3 service + handler + tests. `media.Detect(head []byte) (mime, ext string, ok bool)` consistent between Task 2 and Task 3. `imageFileOwnedBy(...) error` and `repo.ErrFileNotImage` consistent across Task 4. `NewFileHandler(service, maxUploadSize, log)` consistent between handler + app.go wiring.
