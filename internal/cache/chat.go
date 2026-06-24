package cache

import (
	"context"
	"time"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
)

// ChatCache кеширует списки чатов по пользователям. Владеет ключом, TTL и
// метриками, поэтому вызывающему не нужно знать детали кеширования.
type ChatCache struct {
	cache   Cache
	metrics Metrics
	ttl     time.Duration
}

// NewChatCache создаёт ChatCache поверх любого Cache.
func NewChatCache(c Cache, m Metrics, ttl time.Duration) *ChatCache {
	return &ChatCache{cache: c, metrics: m, ttl: ttl}
}

// UserChats возвращает кешированный список чатов userID или вызывает load при промахе.
func (c *ChatCache) UserChats(ctx context.Context, userID int64, load func() ([]dto.ChatResponse, error)) ([]dto.ChatResponse, error) {
	return Load(ctx, c.cache, c.metrics, "chats", UserChatsKey(userID), c.ttl, load)
}

// InvalidateUserChats сбрасывает кеш списка чатов userID.
func (c *ChatCache) InvalidateUserChats(ctx context.Context, userID int64) error {
	return c.cache.Del(ctx, UserChatsKey(userID))
}

// InvalidateUserChatsMany сбрасывает кеш списков чатов для нескольких пользователей за один вызов.
func (c *ChatCache) InvalidateUserChatsMany(ctx context.Context, userIDs ...int64) error {
	if len(userIDs) == 0 {
		return nil
	}
	keys := make([]string, len(userIDs))
	for i, id := range userIDs {
		keys[i] = UserChatsKey(id)
	}
	return c.cache.Del(ctx, keys...)
}
