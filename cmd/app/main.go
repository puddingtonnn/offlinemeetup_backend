package main

import (
	"context"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/app"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/db"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load config", slog.String("err", err.Error()))
		os.Exit(1)
	}

	database, err := db.New(cfg.DBDSN)
	if err != nil {
		logger.Error("Failed to connect to database", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer database.Close()

	application := app.New(logger, cfg, database)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		logger.Error("Failed to run", slog.String("err", err.Error()))
		os.Exit(1)
	}

	logger.Info("Application stopped gracefully")
}
