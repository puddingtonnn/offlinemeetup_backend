package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

type UserRepo struct {
	db *bun.DB
}

func NewUserRepo(db *bun.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) GetBySocialID(ctx context.Context, provider, socialID string) (*domain.User, error) {
	var socialAccount domain.SocialAccount

	err := r.db.NewSelect().Model(&socialAccount).Relation("User").Where("provider = ?", provider).Where("social_id = ?", socialID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return socialAccount.User, nil
}

func (r *UserRepo) CreateUserWithSocial(ctx context.Context, user *domain.User, provider, socialID string, profile *domain.Profile) (*domain.User, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	_, err = tx.NewInsert().Model(user).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("error inserting user: %w", err)
	}

	social := &domain.SocialAccount{
		UserID:   user.ID,
		Provider: provider,
		SocialID: socialID,
	}

	_, err = tx.NewInsert().Model(social).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating social account: %w", err)
	}

	profile.UserID = user.ID
	_, err = tx.NewInsert().Model(profile).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating profile: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return user, nil
}

func (r *UserRepo) UpdateTags(ctx context.Context, userID int64, tagNames []string) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().Model((*domain.UserTag)(nil)).Where("user_id = ?", userID).Exec(ctx)
		if err != nil {
			return err
		}

		if len(tagNames) == 0 {
			return nil
		}

		var userTags []domain.UserTag

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
			userTags = append(userTags, domain.UserTag{UserID: userID, TagID: tag.ID})
		}

		if len(userTags) > 0 {
			_, err := tx.NewInsert().Model(&userTags).Exec(ctx)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	var user domain.User
	err := r.db.NewSelect().
		Model(&user).
		Relation("Tags").
		Where("id = ?", id).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}
