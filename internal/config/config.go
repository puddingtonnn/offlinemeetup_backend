package config

import (
	"fmt"
	"github.com/joho/godotenv"
	"os"
)

type Config struct {
	AppPort          string
	DBDSN            string
	GoogleClientID   string
	TelegramBotToken string
	JWTSecret        string
	Env              string
	DaDataToken      string
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
