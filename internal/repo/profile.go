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
		Relation("Tags").
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
	_, err := r.db.NewInsert().Model(profile).On("CONFLICT (user_id) DO UPDATE").Set("nickname = EXCLUDED.nickname").Set("bio = EXCLUDED.bio").Set("avatar_url = EXCLUDED.avatar_url").Returning("*").Exec(ctx)
	if err != nil {
		return nil, err
	}

	return profile, nil
}

func (r *ProfileRepo) UpdateTags(ctx context.Context, profileID int64, tagNames []string) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().Model((*domain.ProfileTag)(nil)).Where("profile_id = ?", profileID).Exec(ctx)
		if err != nil {
			return err
		}

		if len(tagNames) == 0 {
			return nil
		}

		var profileTags []domain.ProfileTag

		for _, tagName := range tagNames {
			tag := &domain.Tag{Name: tagName}

			exists, err := tx.NewSelect().Model(tag).Where("name = ?", tagName).Exists(ctx)
			if err != nil {
				return err
			}

			if !exists {
				if _, err := tx.NewInsert().Model(tag).Exec(ctx); err != nil {
					return err
				} else {
					if err := tx.NewSelect().Model(tag).Where("name = ?", tagName).Scan(ctx); err != nil {
						return err
					}
				}
			}
			profileTags = append(profileTags, domain.ProfileTag{ProfileID: profileID, TagID: tag.ID})
		}

		if len(profileTags) > 0 {
			_, err := tx.NewInsert().Model(&profileTags).Exec(ctx)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
