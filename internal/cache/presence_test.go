package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPresenceStore(t *testing.T) (*RedisPresenceStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewRedisPresenceStore(rdb), mr
}

func TestRedisPresenceStore_ConnectionAccounting(t *testing.T) {
	store, _ := newTestPresenceStore(t)
	ctx := context.Background()
	ttl := time.Minute

	// First connection => offline->online transition.
	became, err := store.Connect(ctx, 1, "c1", ttl)
	require.NoError(t, err)
	assert.True(t, became, "first connection must be an online transition")

	// Second device => still online, not a transition.
	became, err = store.Connect(ctx, 1, "c2", ttl)
	require.NoError(t, err)
	assert.False(t, became)

	status, err := store.OnlineStatus(ctx, []int64{1, 2})
	require.NoError(t, err)
	assert.True(t, status[1])
	assert.False(t, status[2], "user with no connections must be offline")

	// Drop one of two => not yet offline.
	became, err = store.Disconnect(ctx, 1, "c1")
	require.NoError(t, err)
	assert.False(t, became)

	// Drop the last => online->offline transition.
	became, err = store.Disconnect(ctx, 1, "c2")
	require.NoError(t, err)
	assert.True(t, became)

	status, err = store.OnlineStatus(ctx, []int64{1})
	require.NoError(t, err)
	assert.False(t, status[1])

	// Removing an already-gone connection is idempotent (no double offline).
	became, err = store.Disconnect(ctx, 1, "c2")
	require.NoError(t, err)
	assert.False(t, became)
}

func TestRedisPresenceStore_LastSeen(t *testing.T) {
	store, _ := newTestPresenceStore(t)
	ctx := context.Background()

	ts := time.Unix(1_700_000_000, 0)
	require.NoError(t, store.SetLastSeen(ctx, 5, ts))

	seen, err := store.LastSeen(ctx, []int64{5, 6})
	require.NoError(t, err)
	assert.Equal(t, ts.Unix(), seen[5].Unix())
	_, ok := seen[6]
	assert.False(t, ok, "users without a last_seen key must be absent")
}

func TestRedisPresenceStore_TTLExpiry(t *testing.T) {
	store, mr := newTestPresenceStore(t)
	ctx := context.Background()

	_, err := store.Connect(ctx, 7, "x", 100*time.Millisecond)
	require.NoError(t, err)

	status, err := store.OnlineStatus(ctx, []int64{7})
	require.NoError(t, err)
	require.True(t, status[7])

	// A crashed instance never sends the final Disconnect; the TTL must reap it.
	mr.FastForward(200 * time.Millisecond)

	status, err = store.OnlineStatus(ctx, []int64{7})
	require.NoError(t, err)
	assert.False(t, status[7], "presence must decay to offline once the TTL lapses")
}
