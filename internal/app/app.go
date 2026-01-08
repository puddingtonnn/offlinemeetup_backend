package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	transport "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/handler"

	"github.com/uptrace/bun"
)

type App struct {
	cfg    *config.Config
	log    *slog.Logger
	router http.Handler
	DB     *bun.DB
}

func New(log *slog.Logger, cfg *config.Config, db *bun.DB) *App {

	userRepo := repo.NewUserRepo(db)
	profileRepo := repo.NewProfileRepo(db)
	meetupRepo := repo.NewMeetupRepo(db)
	tagRepo := repo.NewTagRepo(db)

	authService := service.NewAuthService(userRepo, cfg)
	profileService := service.NewProfileService(profileRepo, userRepo)
	meetupService := service.NewMeetupService(meetupRepo)
	tagService := service.NewTagService(tagRepo)

	authHandler := handler.NewAuthHandler(authService, log)
	profileHandler := handler.NewProfileHandler(profileService, log)
	meetupHandler := handler.NewMeetupHandler(meetupService, log)
	tagHandler := handler.NewTagHandler(tagService, log)

	router := transport.NewRouter(authHandler, profileHandler, meetupHandler, tagHandler, cfg)

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
