package cache

import (
	"context"
	"time"
)

// timeoutCache оборачивает Cache пер-операционным таймаутом. Зависший Redis не
// должен тормозить запрос: при истечении таймаута Get вернёт ошибку, которую
// Load трактует как промах и идёт в источник правды (Set/Del — best-effort).
type timeoutCache struct {
	inner   Cache
	timeout time.Duration
}

// NewTimeoutCache оборачивает Cache таймаутом на каждую операцию. При timeout<=0
// возвращает inner без обёртки (таймаут отключён).
func NewTimeoutCache(inner Cache, timeout time.Duration) Cache {
	if timeout <= 0 {
		return inner
	}
	return &timeoutCache{inner: inner, timeout: timeout}
}

func (c *timeoutCache) Get(ctx context.Context, key string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.inner.Get(ctx, key)
}

func (c *timeoutCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.inner.Set(ctx, key, value, ttl)
}

func (c *timeoutCache) Del(ctx context.Context, keys ...string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.inner.Del(ctx, keys...)
}
