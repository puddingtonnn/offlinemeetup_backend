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

// DisplayNamesByUserIDs returns a user_id -> display name map for the given
// users in a single round-trip, applying the shared display-name rule
// (domain.DisplayNameOf). Users without a profile are simply absent from the
// map, so the caller gets an empty name for them rather than an error.
func (r *ProfileRepo) DisplayNamesByUserIDs(ctx context.Context, ids []int64) (map[int64]string, error) {
	if len(ids) == 0 {
		return map[int64]string{}, nil
	}

	var rows []struct {
		UserID      int64   `bun:"user_id"`
		Username    string  `bun:"username"`
		DisplayName *string `bun:"display_name"`
	}
	err := r.db.NewSelect().
		Model((*domain.Profile)(nil)).
		Column("user_id", "username", "display_name").
		Where("user_id IN (?)", bun.In(ids)).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}

	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		out[row.UserID] = domain.DisplayNameOf(row.Username, row.DisplayName)
	}
	return out, nil
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
	// Аватар должен принадлежать владельцу профиля и быть изображением.
	if profile.AvatarFileID.Valid {
		if err := imageFileOwnedBy(ctx, r.db, profile.AvatarFileID.UUID, profile.UserID); err != nil {
			return nil, err
		}
	}

	_, err := r.db.NewInsert().
		Model(profile).
		On("CONFLICT (user_id) DO UPDATE").
		Set("username = EXCLUDED.username").
		Set("display_name = EXCLUDED.display_name").
		Set("bio = EXCLUDED.bio").
		Set("avatar_file_id = EXCLUDED.avatar_file_id").
		Returning("*").
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	return profile, nil
}
