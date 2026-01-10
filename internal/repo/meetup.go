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

	meetup.ParticipantsCount = 1

	_, err = r.db.NewInsert().Model(meetup).Value("location", "ST_GeomFromText(?, 4326)", meetup.Location.String()).Returning("id, created_at").Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("meetup creation failed: %w", err)
	}

	creatorParticipant := &domain.Participant{
		MeetupID: meetup.ID,
		UserID:   meetup.CreatorID,
		Role:     "organizer",
		Status:   "approved",
	}

	_, err = tx.NewInsert().Model(creatorParticipant).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("adding creator to participants failed: %w", err)
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

	return r.GetByID(ctx, meetup.ID)
}

func (r *MeetupRepo) GetByID(ctx context.Context, id int64) (*domain.Meetup, error) {
	var meetup domain.Meetup

	err := r.db.NewSelect().
		Model(&meetup).
		Relation("Creator").
		Relation("Tags").
		Where("meetup.id = ?", id).
		Scan(ctx)

	if err != nil {
		return nil, err
	}
	return &meetup, nil
}

func (r *MeetupRepo) List(ctx context.Context, filter dto.MeetupFilter, currentUserID int64) ([]domain.Meetup, error) {
	var meetups []domain.Meetup

	q := r.db.NewSelect().Model(&meetups)
	q.Relation("Creator")
	q.Relation("Tags")

	if filter.OnlyMy && currentUserID != 0 {
		q.Join("JOIN participants AS p ON p.meetup_id = meetup.id")
		q.Where("p.user_id = ?", currentUserID)
	}

	if filter.Lat != 0 && filter.Lng != 0 {
		if filter.Radius > 0 {
			q.Where("ST_DWithin(?TableAlias.location, ST_MakePoint(?, ?)::geography, ?)",
				filter.Lng, filter.Lat, filter.Radius)
		}
		// Вычисление расстояния
		q.ColumnExpr("meetup.*, ST_Distance(location, ST_MakePoint(?, ?)::geography) AS distance_meters",
			filter.Lng, filter.Lat)

		// Сортировка
		q.Order("distance_meters ASC")
	} else {
		q.Order("start_time ASC")
	}

	if currentUserID != 0 {
		q.ColumnExpr("EXISTS (SELECT 1 FROM participants p WHERE p.meetup_id = meetup.id AND p.user_id = ?) AS is_participant", currentUserID)
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
		_, err := tx.NewUpdate().Model(meetup).Value("location", "ST_GeomFromText(?, 4326)", meetup.Location.String()).WherePK().Exec(ctx)
		if err != nil {
			return err
		}

		_, err = tx.NewDelete().Model((*domain.MeetupTag)(nil)).Where("meetup_id = ?", meetup.ID).Exec(ctx)
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

func (r *MeetupRepo) Join(ctx context.Context, meetupID, userID int64) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		participant := &domain.Participant{
			MeetupID: meetupID,
			UserID:   userID,
			Role:     "member",
		}

		_, err := tx.NewInsert().Model(participant).On("CONFLICT DO NOTHING").Exec(ctx)
		if err != nil {
			return err
		}

		res, err := tx.NewInsert().Model(participant).On("CONFLICT DO NOTHING").Exec(ctx)
		if err != nil {
			return err
		}

		rows, _ := res.RowsAffected()
		if rows == 0 {
			return nil // уже участник
		}

		_, err = tx.NewUpdate().Table("meetups").Set("participants_count = participants_count + 1").
			Where("id = ?", meetupID).
			Exec(ctx)

		return err
	})
}

func (r *MeetupRepo) Leave(ctx context.Context, meetupID, userID int64) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.NewDelete().Model((*domain.Participant)(nil)).Where("meetup_id = ? AND user_id = ?", meetupID, userID).Exec(ctx)
		if err != nil {
			return err
		}

		rows, _ := res.RowsAffected()
		if rows == 0 {
			return nil // не был участником
		}

		_, err = tx.NewUpdate().Table("meetups").Set("participants_count = participants_count - 1").
			Where("id = ?", meetupID).
			Exec(ctx)

		return err
	})
}
