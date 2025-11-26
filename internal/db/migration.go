package db

import (
	"context"
	"fmt"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
)

// InitSchema инициализирует схему БД и создаёт необходимые таблицы
func InitSchema(ctx context.Context, db *bun.DB) error {
	// Создание расширения PostGIS
	_, err := db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS postgis;")
	if err != nil {
		return fmt.Errorf("failed to create postgis extension: %w", err)
	}

	// 1. Users
	_, err = db.NewCreateTable().Model((*domain.User)(nil)).IfNotExists().Exec(ctx)
	if err != nil {
		return err
	}

	// 2. Tags
	_, err = db.NewCreateTable().Model((*domain.Tag)(nil)).IfNotExists().Exec(ctx)
	if err != nil {
		return err
	}

	// 3. Events (ссылается на Users)
	_, err = db.NewCreateTable().
		Model((*domain.Event)(nil)).
		IfNotExists().
		ForeignKey(`("creator_id") REFERENCES "users" ("id") ON DELETE CASCADE`).
		Exec(ctx)
	if err != nil {
		return err
	}

	// 4. EventTags (ссылается на Events и Tags)
	_, err = db.NewCreateTable().
		Model((*domain.EventTag)(nil)).
		IfNotExists().
		ForeignKey(`("event_id") REFERENCES "events" ("id") ON DELETE CASCADE`).
		ForeignKey(`("tag_id") REFERENCES "tags" ("id") ON DELETE CASCADE`).
		Exec(ctx)
	if err != nil {
		return err
	}

	// Индекс для геолокации
	_, err = db.ExecContext(ctx, `
        CREATE INDEX IF NOT EXISTS idx_events_location 
        ON events USING GIST (location);
    `)

	return err
}
