package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	transport "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/handler"
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

	_, err := db.NewCreateTable().Model((*domain.User)(nil)).IfNotExists().Exec(context.Background())
	if err != nil {
		log.Error("failed to create users table", slog.String("error", err.Error()))
	}
	_, err = db.NewCreateTable().Model((*domain.SocialAccount)(nil)).IfNotExists().Exec(context.Background())
	if err != nil {
		log.Error("failed to create social accounts table", slog.String("error", err.Error()))
	}
	userRepo := repo.NewUserRepo(db)

	authService := service.NewAuthService(userRepo, cfg)

	authHandler := handler.NewAuthHandler(authService)

	router := transport.NewRouter(authHandler)

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
