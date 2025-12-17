package db

import (
	"database/sql"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func New(dsn string) (*bun.DB, error) {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))

	db := bun.NewDB(sqldb, pgdialect.New())

	db.RegisterModel(
		(*domain.UserTag)(nil),
	)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("db connection failed: %w", err)
	}

	return db, nil
}
