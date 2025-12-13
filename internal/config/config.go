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
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AppPort:          os.Getenv("APP_PORT"),
		DBDSN:            os.Getenv("DB_DSN"),
		GoogleClientID:   os.Getenv("GOOGLE_CLIENT_ID"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
	}

	if cfg.AppPort == "" {
		cfg.AppPort = "8080"
	}

	if cfg.DBDSN == "" {
		return nil, fmt.Errorf("DB_DSN is not set")
	}
	if cfg.GoogleClientID == "" {
		fmt.Println("WARNING: GOOGLE_CLIENT_ID is not set")
	}
	if cfg.TelegramBotToken == "" {
		fmt.Println("WARNING: TELEGRAM_BOT_TOKEN is not set")
	}

	return cfg, nil
}
