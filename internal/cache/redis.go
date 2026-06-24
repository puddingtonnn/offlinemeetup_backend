package cache

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache — реализация Cache поверх Redis. Ошибки чтения/записи логируются
// и возвращаются, но не фатальны — вызывающий деградирует к источнику правды.
type RedisCache struct {
	rdb *redis.Client
	log *slog.Logger
}

// NewRedisCache оборачивает клиент Redis в Cache.
func NewRedisCache(rdb *redis.Client, log *slog.Logger) *RedisCache {
	return &RedisCache{rdb: rdb, log: log}
}

// Get реализует Cache. Отсутствующий ключ → found=false, err=nil.
func (c *RedisCache) Get(ctx context.Context, key string) (string, bool, error) {
	value, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		c.log.Error("cache get failed", slog.String("key", key), slog.Any("error", err))
		return "", false, err
	}
	return value, true, nil
}

// Set реализует Cache.
func (c *RedisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		c.log.Error("cache set failed", slog.String("key", key), slog.Any("error", err))
		return err
	}
	return nil
}

// Del реализует Cache.
func (c *RedisCache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
		c.log.Error("cache del failed", slog.Any("keys", keys), slog.Any("error", err))
		return err
	}
	return nil
}
