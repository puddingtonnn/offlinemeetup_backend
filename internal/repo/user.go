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

func (r *UserRepo) GetTagsByUserID(ctx context.Context, userID int64) ([]domain.Tag, error) {
	var tags []domain.Tag

	err := r.db.NewSelect().
		Model(&tags).
		Join("JOIN user_tags ut ON ut.tag_id = tag.id").
		Where("ut.user_id = ?", userID).
		Order("tag.name ASC").
		Scan(ctx)
	return tags, err
}

func (r *UserRepo) UpdateTags(ctx context.Context, userID int64, tagIDs []int64) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().Model((*domain.UserTag)(nil)).Where("user_id = ?", userID).Exec(ctx)
		if err != nil {
			return err
		}

		if len(tagIDs) == 0 {
			return nil
		}

		userTags := make([]domain.UserTag, len(tagIDs))
		for i, id := range tagIDs {
			userTags[i] = domain.UserTag{
				UserID: userID,
				TagID:  id,
			}
		}

		_, err = tx.NewInsert().Model(&userTags).Exec(ctx)
		return err
	})
}
