// Package cache — небольшой слой кеширования, не зависящий от бэкенда.
package cache

import (
	"context"
	"encoding/json"
	"time"

	"golang.org/x/sync/singleflight"
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

// loadGroup коллапсирует конкурентные промахи по одному ключу в один вызов
// load(). Ключи глобально уникальны по префиксам доменов, поэтому одна общая
// группа на процесс не даёт коллизий между доменами.
var loadGroup singleflight.Group

// Load возвращает значение из кеша (JSON). При промахе, ошибке декодирования
// или сбое бэкенда вызывает load (под singleflight), кеширует результат с
// jitter-TTL и возвращает его. name — имя кеша для метрик.
func Load[T any](ctx context.Context, c Cache, m Metrics, name, key string, ttl time.Duration, load func() (T, error)) (T, error) {
	start := time.Now()
	raw, found, err := c.Get(ctx, key)
	m.ObserveLatency(name, "get", time.Since(start))
	if err != nil {
		m.Error(name)
	}
	if err == nil && found {
		var cached T
		if json.Unmarshal([]byte(raw), &cached) == nil {
			m.Hit(name)
			return cached, nil
		}
		// Битая запись — трактуем как промах: идём в load и перезапишем её ниже.
	}
	m.Miss(name)

	value, err, _ := loadGroup.Do(key, func() (any, error) {
		v, err := load()
		if err != nil {
			return v, err
		}
		if data, err := json.Marshal(v); err == nil {
			setStart := time.Now()
			setErr := c.Set(ctx, key, string(data), jitter(ttl)) // best-effort; ошибки логирует реализация
			m.ObserveLatency(name, "set", time.Since(setStart))
			if setErr != nil {
				m.Error(name)
			}
		}
		return v, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return value.(T), nil
}
