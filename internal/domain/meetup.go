package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Meetup struct {
	bun.BaseModel `bun:"table:meetups"`

	ID          int64  `bun:",pk,autoincrement"`
	Title       string `bun:",notnull"`
	Description string `bun:""`

	CoverFileID uuid.NullUUID `bun:"type:uuid"`
	CoverFile   *File         `bun:"rel:belongs-to,join:cover_file_id=id"`

	IsPublic  bool  `bun:",notnull"`
	CreatorID int64 `bun:",notnull"`

	StartTime time.Time `bun:",notnull"`
	EndTime   time.Time `bun:",notnull"`

	Location    Location `bun:"location,type:geography(POINT,4326)"`
	AddressText string   `bun:""`

	CreatedAt time.Time  `bun:",nullzero,notnull,default:current_timestamp"`
	DeletedAt *time.Time `bun:",soft_delete,nullzero"`

	ParticipantsCount int `bun:"participants_count"`

	DistanceMeters float64 `bun:"distance_meters,scanonly"`
	IsMember       bool    `bun:"is_member,scanonly"`

	// Relations
	Creator      *User   `bun:"rel:belongs-to,join:creator_id=id"`
	Tags         []*Tag  `bun:"m2m:meetup_tags,join:Meetup=Tag"`
	Participants []*User `bun:"m2m:participants,join:Meetup=User"`
}

type Participant struct {
	bun.BaseModel `bun:"table:participants"`

	MeetupID int64 `bun:",pk"`
	UserID   int64 `bun:",pk"`

	Role     string    `bun:",default:'member'"`
	Status   string    `bun:",default:'approved'"`
	JoinedAt time.Time `bun:",default:current_timestamp"`
}
