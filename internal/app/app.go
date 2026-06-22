package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	transport "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/handler"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
)

type App struct {
	cfg    *config.Config
	log    *slog.Logger
	router http.Handler
	DB     *bun.DB
	hub    *websocket.Hub
}

func New(log *slog.Logger, cfg *config.Config, db *bun.DB) *App {
	hub := websocket.NewHub(log)

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Error("failed to connect to redis", "err", err)
	}

	// AWS SDK v2 Config
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")),
	)
	if err != nil {
		log.Error("Failed to load AWS config", slog.String("error", err.Error()))
		panic(fmt.Errorf("failed to load AWS config: %w", err))
	}

	// S3 Client with modern endpoint configuration
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.S3Endpoint)
		}
		o.UsePathStyle = true
	})

	userRepo := repo.NewUserRepo(db)
	profileRepo := repo.NewProfileRepo(db)
	meetupRepo := repo.NewMeetupRepo(db)
	tagRepo := repo.NewTagRepo(db)
	chatRepo := repo.NewChatRepo(db)
	fileRepo := repo.NewFileRepo(db)

	authService := service.NewAuthService(userRepo, cfg)
	profileService := service.NewProfileService(profileRepo, userRepo, cfg.S3PublicURL)
	meetupService := service.NewMeetupService(meetupRepo, rdb, cfg.S3PublicURL)
	tagService := service.NewTagService(tagRepo)
	geoService := service.NewGeoService(cfg.DaDataToken)
	chatService := service.NewChatService(chatRepo, rdb, log, cfg.S3PublicURL)
	fileService := service.NewFileService(fileRepo, s3Client, cfg)

	authHandler := handler.NewAuthHandler(authService, log)
	profileHandler := handler.NewProfileHandler(profileService, log)
	meetupHandler := handler.NewMeetupHandler(meetupService, log)
	tagHandler := handler.NewTagHandler(tagService, log)
	geoHandler := handler.NewGeoHandler(geoService)
	chatHandler := handler.NewChatHandler(chatService, hub, log)
	wsHandler := websocket.NewWebSocketHandler(hub, log, chatService, profileService)
	fileHandler := handler.NewFileHandler(fileService, log)

	router := transport.NewRouter(authHandler, profileHandler, meetupHandler, tagHandler, geoHandler, chatHandler, wsHandler, fileHandler, authService, cfg)

	return &App{
		cfg:    cfg,
		log:    log,
		router: router,
		DB:     db,
		hub:    hub,
	}
}

func (a *App) Run(ctx context.Context) error {
	go a.hub.Run(ctx)

	server := &http.Server{
		Addr:    ":" + a.cfg.AppPort,
		Handler: a.router,
	}

	serverErr := make(chan error, 1)
	go func() {
		a.log.Info("Starting server", slog.String("port", a.cfg.AppPort))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed to start: %w", err)
	case <-ctx.Done():
		a.log.Info("Shutting down server")
	}

	<-ctx.Done()
	a.log.Info("Shutting down server", slog.String("port", a.cfg.AppPort))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	return nil
}
