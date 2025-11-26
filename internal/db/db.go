package db

import (
	"database/sql"
	"fmt"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// New создаёт и возвращает подключение к базе данных
func New(dbDSN string) *bun.DB {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dbDSN)))

	database := bun.NewDB(sqldb, pgdialect.New())
	if err := database.Ping(); err != nil {
		panic(fmt.Errorf("db connection failed: %w", err))
	}

	return database
}
