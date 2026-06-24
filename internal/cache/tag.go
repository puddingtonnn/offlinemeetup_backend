package cache

import (
	"context"
	"time"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
)

// TagCache кеширует глобальный список тегов под одним ключом. Теги почти
// статичны и не имеют мутаций в API — инвалидация только по TTL; InvalidateTags
// оставлен для будущего админ-инструментария.
type TagCache struct {
	cache   Cache
	metrics Metrics
	ttl     time.Duration
}

// NewTagCache создаёт TagCache поверх любого Cache.
func NewTagCache(c Cache, m Metrics, ttl time.Duration) *TagCache {
	return &TagCache{cache: c, metrics: m, ttl: ttl}
}

// ListTags возвращает кешированный список тегов или вызывает load при промахе.
func (c *TagCache) ListTags(ctx context.Context, load func() ([]dto.TagResponse, error)) ([]dto.TagResponse, error) {
	return Load(ctx, c.cache, c.metrics, "tags", TagsKey(), c.ttl, load)
}

// InvalidateTags сбрасывает кеш списка тегов.
func (c *TagCache) InvalidateTags(ctx context.Context) error {
	return c.cache.Del(ctx, TagsKey())
}
