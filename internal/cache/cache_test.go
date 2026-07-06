package cache

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
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
		got, err := Load(ctx, c, NopMetrics, "test", "k", ttl, func() (int, error) {
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
		got, err := Load(ctx, c, NopMetrics, "test", "k", ttl, func() (int, error) {
			t.Fatal("loader must not run on a cache hit")
			return 0, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 7, got)
	})

	t.Run("corrupt entry falls through to loader and overwrites", func(t *testing.T) {
		mr, c := newTestCache(t)
		require.NoError(t, mr.Set("k", "not-json"))
		got, err := Load(ctx, c, NopMetrics, "test", "k", ttl, func() (int, error) {
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
		_, err := Load(ctx, c, NopMetrics, "test", "k", ttl, func() (int, error) {
			return 0, wantErr
		})
		require.ErrorIs(t, err, wantErr)
		assert.False(t, mr.Exists("k"))
	})

	t.Run("nil pointer result is not cached (no null-poisoning)", func(t *testing.T) {
		mr, c := newTestCache(t)
		calls := 0
		load := func() (*int, error) {
			calls++
			return nil, nil // "not found" от указательного loader'а
		}
		got, err := Load(ctx, c, NopMetrics, "test", "k", ttl, load)
		require.NoError(t, err)
		assert.Nil(t, got)
		// Значение "null" не должно попасть в кеш: иначе следующий Load вернул бы
		// nil как "хит" до истечения TTL, минуя loader.
		assert.False(t, mr.Exists("k"), "nil result must not be cached")

		_, err = Load(ctx, c, NopMetrics, "test", "k", ttl, load)
		require.NoError(t, err)
		assert.Equal(t, 2, calls, "nil-возврат не кешируется — повторный Load снова идёт в loader")
	})
}

// TestLoad_SingleflightCollapsesConcurrentMisses — конкурентные промахи по
// одному ключу должны схлопнуться в один вызов loader.
func TestLoad_SingleflightCollapsesConcurrentMisses(t *testing.T) {
	ctx := context.Background()
	_, c := newTestCache(t)

	const n = 20
	var calls int32
	started := make(chan struct{})
	release := make(chan struct{})

	load := func() (int, error) {
		atomic.AddInt32(&calls, 1)
		close(started) // выполняет только лидер singleflight — безопасно, один раз
		<-release
		return 7, nil
	}

	var wg sync.WaitGroup
	results := make([]int, n)
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// уникальный ключ кеша не нужен — наоборот, все по одному ключу "k".
			v, err := Load(ctx, c, NopMetrics, "test", "k", time.Minute, load)
			require.NoError(t, err)
			results[idx] = v
		}(i)
	}

	<-started                         // лидер вошёл в load() и заблокирован
	time.Sleep(50 * time.Millisecond) // даём остальным дойти до singleflight.Do
	close(release)
	wg.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "loader должен вызваться один раз")
	for _, v := range results {
		assert.Equal(t, 7, v)
	}
}

// fakeMetrics считает вызовы Metrics для проверки инструментирования.
type fakeMetrics struct {
	hits, misses, errors, latency int32
}

func (m *fakeMetrics) Hit(string)   { atomic.AddInt32(&m.hits, 1) }
func (m *fakeMetrics) Miss(string)  { atomic.AddInt32(&m.misses, 1) }
func (m *fakeMetrics) Error(string) { atomic.AddInt32(&m.errors, 1) }
func (m *fakeMetrics) ObserveLatency(string, string, time.Duration) {
	atomic.AddInt32(&m.latency, 1)
}

func TestLoad_Metrics(t *testing.T) {
	ctx := context.Background()
	_, c := newTestCache(t)
	m := &fakeMetrics{}

	// Первый вызов — промах.
	_, err := Load(ctx, c, m, "test", "k", time.Minute, func() (int, error) { return 1, nil })
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&m.misses))
	assert.Equal(t, int32(0), atomic.LoadInt32(&m.hits))

	// Второй вызов — попадание.
	_, err = Load(ctx, c, m, "test", "k", time.Minute, func() (int, error) {
		t.Fatal("loader must not run on a hit")
		return 0, nil
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&m.hits))
	assert.Positive(t, atomic.LoadInt32(&m.latency), "должна замеряться латентность get")
}

func TestLoad_Metrics_BackendErrorCountsError(t *testing.T) {
	ctx := context.Background()
	m := &fakeMetrics{}

	got, err := Load(ctx, errGetCache{}, m, "test", "k", time.Minute, func() (int, error) {
		return 42, nil // деградация к loader при сбое бэкенда
	})
	require.NoError(t, err)
	assert.Equal(t, 42, got)
	assert.Positive(t, atomic.LoadInt32(&m.errors), "сбой Get должен инкрементить Error")
	assert.Zero(t, atomic.LoadInt32(&m.misses), "сбой Get не должен ещё и считаться Miss (двойной учёт)")
}

func TestJitter_WithinBounds(t *testing.T) {
	t.Run("zero ttl unchanged", func(t *testing.T) {
		assert.Equal(t, time.Duration(0), jitter(0))
	})
	t.Run("stays within ±10%", func(t *testing.T) {
		ttl := 100 * time.Second
		for range 1000 {
			j := jitter(ttl)
			assert.GreaterOrEqual(t, j, 90*time.Second)
			assert.LessOrEqual(t, j, 110*time.Second)
		}
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
	cc := NewChatCache(rc, NopMetrics, time.Minute)

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
		cc := NewChatCache(rc, NopMetrics, time.Minute)

		require.NoError(t, cc.InvalidateUserChats(ctx, 1))
		assert.False(t, mr.Exists("user_chats:1"))
	})

	t.Run("InvalidateUserChatsMany drops every listed key", func(t *testing.T) {
		mr, rc := newTestCache(t)
		for _, id := range []int64{1, 2, 3} {
			require.NoError(t, mr.Set(UserChatsKey(id), "[]"))
		}
		cc := NewChatCache(rc, NopMetrics, time.Minute)

		require.NoError(t, cc.InvalidateUserChatsMany(ctx, 1, 2, 3))
		assert.False(t, mr.Exists("user_chats:1"))
		assert.False(t, mr.Exists("user_chats:2"))
		assert.False(t, mr.Exists("user_chats:3"))
	})

	t.Run("InvalidateUserChatsMany with no ids is a no-op", func(t *testing.T) {
		_, rc := newTestCache(t)
		cc := NewChatCache(rc, NopMetrics, time.Minute)
		require.NoError(t, cc.InvalidateUserChatsMany(ctx))
	})
}

// errGetCache — тестовый Cache, чей Get всегда возвращает ошибку бэкенда.
type errGetCache struct{}

func (errGetCache) Get(context.Context, string) (string, bool, error) {
	return "", false, errors.New("backend down")
}
func (errGetCache) Set(context.Context, string, string, time.Duration) error { return nil }
func (errGetCache) Del(context.Context, ...string) error                     { return nil }
