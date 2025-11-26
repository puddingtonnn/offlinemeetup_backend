package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/db"
	httpHandler "github.com/puddingtonnn/offlinemeetup_backend/internal/handler/http"
	"github.com/uptrace/bun"
)

type App struct {
	router http.Handler
	DB     *bun.DB
}

// New создаёт новое приложение с подключением к БД и инициализацией схемы
func New(dbDSN string) *App {
	// Инициализация БД
	database := db.New(dbDSN)

	// Инициализация схемы (миграции)
	if err := db.InitSchema(context.Background(), database); err != nil {
		panic(err)
	}

	// Инициализация маршрутов
	router := httpHandler.LoadRoutes()

	app := &App{
		router: router,
		DB:     database,
	}

	return app
}

// Start запускает HTTP сервер
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
