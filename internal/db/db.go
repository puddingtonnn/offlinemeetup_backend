package db

import (
	"database/sql"
	"fmt"
	"runtime"
	"time"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func New(dsn string) (*bun.DB, error) {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))

	// Bun/pgdriver default to an unbounded pool: under load the app could open
	// more connections than Postgres' max_connections and cause refused-connection
	// cascades. Cap it (the bun-recommended 4×CPU) and recycle idle conns.
	maxConns := 4 * runtime.GOMAXPROCS(0)
	sqldb.SetMaxOpenConns(maxConns)
	sqldb.SetMaxIdleConns(maxConns)
	sqldb.SetConnMaxLifetime(5 * time.Minute)

	db := bun.NewDB(sqldb, pgdialect.New())

	db.RegisterModel(
		(*domain.UserTag)(nil),
		(*domain.MeetupTag)(nil),
		(*domain.Participant)(nil),
		(*domain.ChatParticipant)(nil),
	)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("db connection failed: %w", err)
	}

	return db, nil
}
