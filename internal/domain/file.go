package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type File struct {
	bun.BaseModel `bun:"table:files"`

	ID       uuid.UUID `bun:"type:uuid,pk,default:gen_random_uuid()"`
	FileName string    `bun:",notnull"`
	Key      string    `bun:",notnull,unique"` // S3 Key
	Bucket   string    `bun:",notnull"`
	Size     int64     `bun:",notnull"`
	MimeType string    `bun:",notnull"`
	// UploadedBy — кто загрузил файл; по нему проверяется владение при ссылке на
	// файл (cover/avatar/attachment). Nullable: legacy-строки и ON DELETE SET NULL.
	UploadedBy *int64 `bun:"uploaded_by"`
	// Медиа-метаданные, извлечённые при загрузке (best-effort). Nullable: не-медиа,
	// форматы без такой информации и legacy-строки их не имеют.
	DurationMS *int64    `bun:"duration_ms"`
	Width      *int      `bun:"width"`
	Height     *int      `bun:"height"`
	CreatedAt  time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}
