package repo

import (
	"context"
	"database/sql"
	"errors"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

type ProfileRepo struct {
	db *bun.DB
}

func NewProfileRepo(db *bun.DB) *ProfileRepo {
	return &ProfileRepo{db: db}
}

func (r *ProfileRepo) GetByUserID(ctx context.Context, userID int64) (*domain.Profile, error) {
	var profile domain.Profile
	err := r.db.NewSelect().
		Model(&profile).
		Relation("AvatarFile").
		Where("user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *ProfileRepo) UpdateProfile(ctx context.Context, profile *domain.Profile) (*domain.Profile, error) {
	_, err := r.db.NewInsert().
		Model(profile).
		On("CONFLICT (user_id) DO UPDATE").
		Set("nickname = EXCLUDED.nickname").
		Set("bio = EXCLUDED.bio").
		Set("avatar_file_id = EXCLUDED.avatar_file_id").
		Returning("*").
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	return profile, nil
}
