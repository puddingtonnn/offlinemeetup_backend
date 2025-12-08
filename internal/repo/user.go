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

func (r *UserRepo) CreateUserWithSocial(ctx context.Context, user *domain.User, provider, socialID string) (*domain.User, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	social := &domain.SocialAccount{
		UserID:   user.ID,
		Provider: provider,
		SocialID: socialID,
	}

	_, err = tx.NewInsert().Model(social).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating social account: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return user, nil
}
