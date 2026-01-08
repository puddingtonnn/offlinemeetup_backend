package repo

import (
	"context"
	"fmt"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
	"github.com/uptrace/bun"
)

type MeetupRepo struct {
	db *bun.DB
}

func NewMeetupRepo(db *bun.DB) *MeetupRepo {
	return &MeetupRepo{db: db}
}

func (r *MeetupRepo) Create(ctx context.Context, meetup *domain.Meetup, tagIDs []int64) (*domain.Meetup, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = r.db.NewInsert().Model(meetup).Value("location", "ST_GeomFromText(?, 4326)", meetup.Location).Returning("*, ST_AsText(location) AS location").Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("meetup creation failed: %w", err)
	}

	if len(tagIDs) > 0 {
		meetupTags := make([]domain.MeetupTag, len(tagIDs))
		for i, tagID := range tagIDs {
			meetupTags[i] = domain.MeetupTag{
				MeetupID: meetup.ID,
				TagID:    tagID,
			}
		}
		if _, err := tx.NewInsert().Model(&meetupTags).Exec(ctx); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("transaction commit failed: %w", err)
	}

	return meetup, nil
}

func (r *MeetupRepo) GetByID(ctx context.Context, id int64) (*domain.Meetup, error) {
	var meetup domain.Meetup

	// ST_AsText конвертирует бинарную геометрию обратно в читаемый формат POINT(...)
	// Bun автоматически замапит результат колонки location в поле Location struct
	err := r.db.NewSelect().
		Model(&meetup).
		Relation("Creator").
		Relation("Tags").
		ExcludeColumn("location").
		ColumnExpr("ST_AsText(meetup.location) AS location").
		Where("meetup.id = ?", id).
		Scan(ctx)

	if err != nil {
		return nil, err
	}
	return &meetup, nil
}

func (r *MeetupRepo) List(ctx context.Context, filter dto.MeetupFilter) ([]domain.Meetup, error) {
	var meetups []domain.Meetup

	q := r.db.NewSelect().
		Model(&meetups).
		Relation("Creator").
		Relation("Tags").
		ExcludeColumn("location").
		ColumnExpr("ST_AsText(meetup.location) AS location")

	if filter.Radius > 0 {
		q.Where("ST_DWithin(location, ST_MakePoint(?, ?)::geography, ?)",
			filter.Lng, filter.Lat, filter.Radius)
		q.OrderExpr("ST_Distance(location, ST_MakePoint(?, ?)::geography) ASC",
			filter.Lng, filter.Lat)
	} else {
		q.Order("start_time ASC")
	}

	if len(filter.Tags) > 0 {
		q.Where("meetup.id IN (SELECT meetup_id FROM meetup_tags WHERE tag_id IN (?))", bun.In(filter.Tags))
	}

	if filter.Limit > 0 {
		q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q.Offset(filter.Offset)
	}

	err := q.Scan(ctx)
	return meetups, err
}

func (r *MeetupRepo) Update(ctx context.Context, meetup *domain.Meetup, newTagIDs []int64) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewUpdate().Model(meetup).WherePK().Exec(ctx)
		if err != nil {
			return err
		}

		_, err = tx.NewDelete().Model((*domain.MeetupTag)(nil)).Where("meetup.id = ?", meetup.ID).Exec(ctx)
		if err != nil {
			return err
		}

		if len(newTagIDs) > 0 {
			meetupTags := make([]domain.MeetupTag, len(newTagIDs))
			for i, tagID := range newTagIDs {
				meetupTags[i] = domain.MeetupTag{
					MeetupID: meetup.ID,
					TagID:    tagID,
				}
			}
			_, err := tx.NewInsert().Model(&meetupTags).Exec(ctx)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *MeetupRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*domain.Meetup)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}
