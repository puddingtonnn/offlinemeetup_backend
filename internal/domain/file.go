package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type File struct {
	bun.BaseModel `bun:"table:files"`

	ID        uuid.UUID `bun:"type:uuid,pk,default:gen_random_uuid()"`
	FileName  string    `bun:",notnull"`
	Key       string    `bun:",notnull,unique"` // S3 Key
	Bucket    string    `bun:",notnull"`
	Size      int64     `bun:",notnull"`
	MimeType  string    `bun:",notnull"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}
