package app

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"net/http"
)

type App struct {
	router http.Handler
	DB     *bun.DB
}

func New(dbDSN string) *App {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dbDSN)))

	db := bun.NewDB(sqldb, pgdialect.New())
	if err := db.Ping(); err != nil {
		panic(fmt.Errorf("db connection failed: %w", err))
	}
	app := &App{
		DB: db,
	}

	if err := app.initSchema(context.Background()); err != nil {
		panic(err)
	}

	app.router = loadRoutes()

	return app
}

func (a *App) Start(ctx context.Context) error {
	server := &http.Server{
		Addr:    ":8080",
		Handler: a.router,
	}
	err := server.ListenAndServe()
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}

func (a *App) initSchema(ctx context.Context) error {
	_, err := a.DB.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS postgis;")
	if err != nil {
		return fmt.Errorf("failed to create postgis: %w", err)
	}

	models := []interface{}{
		(*domain.User)(nil),
		(*domain.Profile)(nil),
		(*domain.Tag)(nil),
		(*domain.Event)(nil),
		(*domain.EventTag)(nil),
	}

	for _, model := range models {
		_, err := a.DB.NewCreateTable().
			Model(model).
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to create table for model %T: %w", model, err)
		}
	}

	_, err = a.DB.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_events_location ON events USING GIST (location);`)
	if err != nil {
		return fmt.Errorf("failed to create gist index: %w", err)
	}

	fmt.Println("Database schema initialized successfully!")
	return nil
}
