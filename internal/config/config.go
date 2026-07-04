package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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

	CacheTimeout    time.Duration
	CacheTTLChats   time.Duration
	CacheTTLTags    time.Duration
	CacheTTLProfile time.Duration
	CacheTTLMeetup  time.Duration

	// PresenceTTL — срок жизни ключа присутствия в Redis; обновляется heartbeat'ом
	// из writePump. Должен быть заметно больше pingPeriod (~54s), чтобы один
	// пропущенный heartbeat не выкинул юзера в offline, но достаточно мал, чтобы
	// упавший инстанс быстро «отпустил» свои соединения.
	PresenceTTL time.Duration

	WSAllowedOrigins []string

	// MaxUploadSize — максимальный размер загружаемого файла в байтах
	// (MAX_UPLOAD_SIZE, дефолт 100 MB). Применяется и в хендлере (MaxBytesReader),
	// и в FileService.Upload.
	MaxUploadSize int64
}

// durEnv reads a duration from env (e.g. "200ms", "5m"); on an empty or
// unparseable value it returns def.
func durEnv(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// int64Env reads an integer (bytes) from env; on an empty or unparseable value
// it returns def.
func int64Env(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
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

	cfg.CacheTimeout = durEnv("CACHE_TIMEOUT", 200*time.Millisecond)
	cfg.CacheTTLChats = durEnv("CACHE_TTL_CHATS", 5*time.Minute)
	cfg.CacheTTLTags = durEnv("CACHE_TTL_TAGS", time.Hour)
	cfg.CacheTTLProfile = durEnv("CACHE_TTL_PROFILE", 10*time.Minute)
	cfg.CacheTTLMeetup = durEnv("CACHE_TTL_MEETUP", 2*time.Minute)
	cfg.PresenceTTL = durEnv("PRESENCE_TTL", 2*time.Minute)
	cfg.MaxUploadSize = int64Env("MAX_UPLOAD_SIZE", 100<<20)

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
	// Секреты-капабилити обязаны быть заданы: пустой JWT-секрет означает подпись/
	// проверку HMAC-ключом нулевой длины (форж токена под любой userID), пустой
	// токен Telegram вырождает HMAC-ключ в sha256("") (форж подписи). Падаем на
	// старте во всех окружениях, как уже делает проверка DB_DSN выше.
	if cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is not set")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET_KEY is not set")
	}
	if cfg.Env == "" {
		cfg.Env = "local"
	}
	if cfg.DaDataToken == "" {
		fmt.Println("WARNING: DADATA_TOKEN is empty")
	}

	return cfg, nil
}
