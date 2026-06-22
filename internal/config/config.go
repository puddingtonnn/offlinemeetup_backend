package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort          string
	DBDSN            string
	GoogleClientID   string
	TelegramBotToken string
	JWTSecret        string
	Env              string
	DaDataToken      string

	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3PublicURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	WSAllowedOrigins []string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AppPort:          os.Getenv("APP_PORT"),
		DBDSN:            os.Getenv("DB_DSN"),
		GoogleClientID:   os.Getenv("GOOGLE_WEB_CLIENT_ID"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		JWTSecret:        os.Getenv("JWT_SECRET_KEY"),
		Env:              os.Getenv("APP_ENV"),
		DaDataToken:      os.Getenv("DADATA_TOKEN"),

		S3Endpoint:  os.Getenv("S3_ENDPOINT"),
		S3Region:    os.Getenv("S3_REGION"),
		S3Bucket:    os.Getenv("S3_BUCKET"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("S3_SECRET_KEY"),
		S3PublicURL: os.Getenv("S3_PUBLIC_URL"),

		RedisAddr:     os.Getenv("REDIS_ADDR"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
	}

	if cfg.RedisAddr == "" {
		cfg.RedisAddr = "meetuper_redis:6379"
	}
	if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
		if n, err := strconv.Atoi(dbStr); err == nil {
			cfg.RedisDB = n
		}
	}

	if origins := os.Getenv("WS_ALLOWED_ORIGINS"); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			if o = strings.TrimSpace(o); o != "" {
				cfg.WSAllowedOrigins = append(cfg.WSAllowedOrigins, o)
			}
		}
	}

	if cfg.AppPort == "" {
		cfg.AppPort = "9090"
	}

	if cfg.DBDSN == "" {
		return nil, fmt.Errorf("DB_DSN is not set")
	}
	if cfg.GoogleClientID == "" {
		fmt.Println("WARNING: GOOGLE_WEB_CLIENT_ID is not set")
	}
	if cfg.TelegramBotToken == "" {
		fmt.Println("WARNING: TELEGRAM_BOT_TOKEN is not set")
	}
	if cfg.JWTSecret == "" {
		fmt.Println("WARNING: JWT_SECRET_KEY is not set")
	}
	if cfg.Env == "" {
		cfg.Env = "local"
	}
	if cfg.DaDataToken == "" {
		fmt.Println("WARNING: DADATA_TOKEN is empty")
	}

	return cfg, nil
}
