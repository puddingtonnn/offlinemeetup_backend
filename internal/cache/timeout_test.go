package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowCache — тестовый Cache, операции которого длятся delay, но уважают отмену
// контекста (возвращают ctx.Err при истечении таймаута).
type slowCache struct{ delay time.Duration }

func (s slowCache) Get(ctx context.Context, _ string) (string, bool, error) {
	select {
	case <-time.After(s.delay):
		return "", false, nil
	case <-ctx.Done():
		return "", false, ctx.Err()
	}
}

func (s slowCache) Set(ctx context.Context, _, _ string, _ time.Duration) error {
	select {
	case <-time.After(s.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s slowCache) Del(ctx context.Context, _ ...string) error {
	select {
	case <-time.After(s.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestTimeoutCache_DegradesToLoader(t *testing.T) {
	ctx := context.Background()
	// Бэкенд «висит» секунду, таймаут — миллисекунда: Get обязан вернуть ошибку.
	tc := NewTimeoutCache(slowCache{delay: time.Second}, time.Millisecond)
	m := &fakeMetrics{}

	loaderCalled := false
	start := time.Now()
	got, err := Load(ctx, tc, m, "test", "k", time.Minute, func() (int, error) {
		loaderCalled = true
		return 5, nil
	})

	require.NoError(t, err, "таймаут кеша не должен валить запрос")
	assert.Equal(t, 5, got)
	assert.True(t, loaderCalled, "при таймауте Get должны деградировать к loader")
	assert.Less(t, time.Since(start), 500*time.Millisecond, "не должны ждать медленный бэкенд целиком")
	assert.Positive(t, m.errors, "таймаут Get учитывается как Error")
}

func TestNewTimeoutCache_ZeroTimeoutReturnsInner(t *testing.T) {
	inner := slowCache{}
	assert.Equal(t, Cache(inner), NewTimeoutCache(inner, 0), "timeout<=0 отключает обёртку")
}
