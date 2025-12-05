package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	transport "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http"
	"log/slog"
	"net/http"
	"time"

	"github.com/uptrace/bun"
)

type App struct {
	cfg    *config.Config
	log    *slog.Logger
	router http.Handler
	DB     *bun.DB
}

func New(log *slog.Logger, cfg *config.Config, db *bun.DB) *App {
	router := transport.NewRouter()

	return &App{
		cfg:    cfg,
		log:    log,
		router: router,
		DB:     db,
	}
}

func (a *App) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:    ":" + a.cfg.AppPort,
		Handler: a.router,
	}
	go func() {
		a.log.Info("Starting server", slog.String("port", a.cfg.AppPort))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Error("Server execution failed", slog.String("error", err.Error()))
		}
	}()
	<-ctx.Done()
	a.log.Info("Shutting down server", slog.String("port", a.cfg.AppPort))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	return nil
}
