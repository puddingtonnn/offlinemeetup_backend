package cache

import (
	"context"
	"time"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
)

// userChatsTTL — сколько живёт кешированный список чатов пользователя.
const userChatsTTL = 5 * time.Minute

// ChatCache кеширует списки чатов по пользователям. Владеет форматом ключа и TTL,
// поэтому вызывающему не нужно знать детали кеширования.
type ChatCache struct {
	cache Cache
}

// NewChatCache создаёт ChatCache поверх любого Cache.
func NewChatCache(c Cache) *ChatCache {
	return &ChatCache{cache: c}
}

// UserChats возвращает кешированный список чатов userID или вызывает load при промахе.
func (c *ChatCache) UserChats(ctx context.Context, userID int64, load func() ([]dto.ChatResponse, error)) ([]dto.ChatResponse, error) {
	return Load(ctx, c.cache, UserChatsKey(userID), userChatsTTL, load)
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
