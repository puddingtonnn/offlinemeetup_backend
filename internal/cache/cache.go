// Package cache — небольшой слой кеширования, не зависящий от бэкенда.
package cache

import (
	"context"
	"encoding/json"
	"time"
)

// Cache — минимальное key/value хранилище для хелперов кеша. Реализации
// best-effort: сбой бэкенда не фатален, его можно трактовать как промах.
type Cache interface {
	// Get возвращает значение по ключу. found=false — промах; err != nil только при сбое бэкенда.
	Get(ctx context.Context, key string) (value string, found bool, err error)
	// Set сохраняет значение по ключу с заданным TTL.
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	// Del удаляет ключи. Отсутствующий ключ — не ошибка.
	Del(ctx context.Context, keys ...string) error
}

// Load возвращает значение из кеша (JSON). При промахе, ошибке декодирования
// или сбое бэкенда вызывает load, кеширует результат и возвращает его.
func Load[T any](ctx context.Context, c Cache, key string, ttl time.Duration, load func() (T, error)) (T, error) {
	if raw, found, err := c.Get(ctx, key); err == nil && found {
		var cached T
		if json.Unmarshal([]byte(raw), &cached) == nil {
			return cached, nil
		}
		// Битая запись — трактуем как промах: идём в load и перезапишем её ниже.
	}

	value, err := load()
	if err != nil {
		var zero T
		return zero, err
	}

	if raw, err := json.Marshal(value); err == nil {
		_ = c.Set(ctx, key, string(raw), ttl) // best-effort; ошибки логирует реализация
	}
	return value, nil
}
