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

func (r *MeetupRepo) Create(ctx context.Context, meetup *domain.Meetup) (*domain.Meetup, error) {
	_, err := r.db.NewInsert().Model(meetup).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("meetup creation failed: %w", err)
	}
	return meetup, nil
}

func (r *MeetupRepo) GetByID(ctx context.Context, id int64) (*domain.Meetup, error) {
	var meetup domain.Meetup

	// ST_AsText конвертирует бинарную геометрию обратно в читаемый формат POINT(...)
	// Bun автоматически замапит результат колонки location в поле Location struct
	err := r.db.NewSelect().
		Model(&meetup).
		ColumnExpr("*, ST_AsText(location) as location").
		Where("id = ?", id).
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
		Relation("Creator")

	if filter.Radius > 0 {
		q.Where("ST_DWithin(location, ST_MakePoint(?, ?)::geography, ?)",
			filter.Lng, filter.Lat, filter.Radius)
		q.OrderExpr("ST_Distance(location, ST_MakePoint(?, ?)::geography) ASC",
			filter.Lng, filter.Lat)
	} else {
		q.Order("start_time ASC")
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

func (r *MeetupRepo) Update(ctx context.Context, meetup *domain.Meetup) error {
	_, err := r.db.NewUpdate().Model(meetup).WherePK().Exec(ctx)
	return err
}

func (r *MeetupRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*domain.Meetup)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}
