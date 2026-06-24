package cache

import (
	"context"
	"time"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
)

// ProfileCache кеширует профили по userID. Профиль пользователя одинаков для
// любого смотрящего, поэтому ключ зависит только от target userID.
type ProfileCache struct {
	cache   Cache
	metrics Metrics
	ttl     time.Duration
}

// NewProfileCache создаёт ProfileCache поверх любого Cache.
func NewProfileCache(c Cache, m Metrics, ttl time.Duration) *ProfileCache {
	return &ProfileCache{cache: c, metrics: m, ttl: ttl}
}

// Profile возвращает кешированный профиль userID или вызывает load при промахе.
func (c *ProfileCache) Profile(ctx context.Context, userID int64, load func() (*dto.ProfileResponse, error)) (*dto.ProfileResponse, error) {
	return Load(ctx, c.cache, c.metrics, "profile", ProfileKey(userID), c.ttl, load)
}

// InvalidateProfile сбрасывает кеш профиля userID.
func (c *ProfileCache) InvalidateProfile(ctx context.Context, userID int64) error {
	return c.cache.Del(ctx, ProfileKey(userID))
}
