package cache

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
)

func newTestCache(t *testing.T) (*miniredis.Miniredis, *RedisCache) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, NewRedisCache(rdb, slog.New(slog.DiscardHandler))
}

func TestUserChatsKey(t *testing.T) {
	assert.Equal(t, "user_chats:42", UserChatsKey(42))
}

func TestLoad(t *testing.T) {
	ctx := context.Background()
	ttl := time.Minute

	t.Run("miss calls loader and caches result", func(t *testing.T) {
		mr, c := newTestCache(t)
		calls := 0
		got, err := Load(ctx, c, "k", ttl, func() (int, error) {
			calls++
			return 7, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 7, got)
		assert.Equal(t, 1, calls)
		assert.True(t, mr.Exists("k"))
	})

	t.Run("hit returns cached value without calling loader", func(t *testing.T) {
		mr, c := newTestCache(t)
		require.NoError(t, mr.Set("k", "7"))
		got, err := Load(ctx, c, "k", ttl, func() (int, error) {
			t.Fatal("loader must not run on a cache hit")
			return 0, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 7, got)
	})

	t.Run("corrupt entry falls through to loader and overwrites", func(t *testing.T) {
		mr, c := newTestCache(t)
		require.NoError(t, mr.Set("k", "not-json"))
		got, err := Load(ctx, c, "k", ttl, func() (int, error) {
			return 9, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 9, got)
		cached, _ := mr.Get("k")
		assert.Equal(t, "9", cached, "corrupt value should be replaced by the fresh one")
	})

	t.Run("loader error propagates and nothing is cached", func(t *testing.T) {
		mr, c := newTestCache(t)
		wantErr := errors.New("boom")
		_, err := Load(ctx, c, "k", ttl, func() (int, error) {
			return 0, wantErr
		})
		require.ErrorIs(t, err, wantErr)
		assert.False(t, mr.Exists("k"))
	})
}

func TestRedisCache_GetMiss(t *testing.T) {
	_, c := newTestCache(t)
	value, found, err := c.Get(context.Background(), "absent")
	require.NoError(t, err, "a miss is not an error")
	assert.False(t, found)
	assert.Empty(t, value)
}

func TestChatCache_UserChats(t *testing.T) {
	ctx := context.Background()
	mr, rc := newTestCache(t)
	cc := NewChatCache(rc)

	calls := 0
	load := func() ([]dto.ChatResponse, error) {
		calls++
		return []dto.ChatResponse{{ID: 1, Title: "A"}}, nil
	}

	got, err := cc.UserChats(ctx, 1, load)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, calls)
	assert.True(t, mr.Exists("user_chats:1"))

	// Second read is served from cache: loader must not run again.
	got, err = cc.UserChats(ctx, 1, load)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, calls)
}

func TestChatCache_Invalidate(t *testing.T) {
	ctx := context.Background()

	t.Run("InvalidateUserChats drops one user's key", func(t *testing.T) {
		mr, rc := newTestCache(t)
		require.NoError(t, mr.Set(UserChatsKey(1), "[]"))
		cc := NewChatCache(rc)

		require.NoError(t, cc.InvalidateUserChats(ctx, 1))
		assert.False(t, mr.Exists("user_chats:1"))
	})

	t.Run("InvalidateUserChatsMany drops every listed key", func(t *testing.T) {
		mr, rc := newTestCache(t)
		for _, id := range []int64{1, 2, 3} {
			require.NoError(t, mr.Set(UserChatsKey(id), "[]"))
		}
		cc := NewChatCache(rc)

		require.NoError(t, cc.InvalidateUserChatsMany(ctx, 1, 2, 3))
		assert.False(t, mr.Exists("user_chats:1"))
		assert.False(t, mr.Exists("user_chats:2"))
		assert.False(t, mr.Exists("user_chats:3"))
	})

	t.Run("InvalidateUserChatsMany with no ids is a no-op", func(t *testing.T) {
		_, rc := newTestCache(t)
		cc := NewChatCache(rc)
		require.NoError(t, cc.InvalidateUserChatsMany(ctx))
	})
}
