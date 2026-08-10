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
)

func newTestAuthStore(t *testing.T) (*RedisAuthStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewRedisAuthStore(rdb, slog.Default()), mr
}

func TestRedisAuthStore_PendingReg_SaveGetDelete(t *testing.T) {
	store, _ := newTestAuthStore(t)
	ctx := context.Background()

	_, found, err := store.GetPendingReg(ctx, "User@Example.com")
	require.NoError(t, err)
	assert.False(t, found, "no pending registration yet")

	data := PendingReg{PasswordHash: "hash1", CodeHash: "code1", Attempts: 0, Username: "alice"}
	require.NoError(t, store.SavePendingReg(ctx, "user@example.com", data, 15*time.Minute))

	got, found, err := store.GetPendingReg(ctx, "User@Example.com") // case-insensitive lookup
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, data, got)

	// A repeated register overwrites, not merges.
	data2 := PendingReg{PasswordHash: "hash2", CodeHash: "code2", Attempts: 0, Username: "alice2"}
	require.NoError(t, store.SavePendingReg(ctx, "user@example.com", data2, 15*time.Minute))
	got, found, err = store.GetPendingReg(ctx, "user@example.com")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, data2, got)

	require.NoError(t, store.DeletePendingReg(ctx, "user@example.com"))
	_, found, err = store.GetPendingReg(ctx, "user@example.com")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestRedisAuthStore_PendingReset_SaveGetDelete(t *testing.T) {
	store, _ := newTestAuthStore(t)
	ctx := context.Background()

	data := PendingReg{PasswordHash: "newhash", CodeHash: "codeh", Attempts: 0}
	require.NoError(t, store.SavePendingReset(ctx, "bob@example.com", data, 15*time.Minute))

	got, found, err := store.GetPendingReset(ctx, "bob@example.com")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, data, got)

	// Reset and registration live in separate key spaces.
	_, found, err = store.GetPendingReg(ctx, "bob@example.com")
	require.NoError(t, err)
	assert.False(t, found)

	require.NoError(t, store.DeletePendingReset(ctx, "bob@example.com"))
	_, found, err = store.GetPendingReset(ctx, "bob@example.com")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestRedisAuthStore_IncrementPendingRegAttempts_PreservesTTL(t *testing.T) {
	store, mr := newTestAuthStore(t)
	ctx := context.Background()

	data := PendingReg{PasswordHash: "h", CodeHash: "c", Attempts: 0, Username: "carl"}
	require.NoError(t, store.SavePendingReg(ctx, "carl@example.com", data, 15*time.Minute))

	// Simulate some time elapsed so the remaining TTL is less than 15m.
	mr.FastForward(5 * time.Minute)
	ttlBefore := mr.TTL(PendingRegKey("carl@example.com"))
	require.Greater(t, ttlBefore, time.Duration(0))

	n, err := store.IncrementPendingRegAttempts(ctx, "carl@example.com")
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	n, err = store.IncrementPendingRegAttempts(ctx, "carl@example.com")
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	got, found, err := store.GetPendingReg(ctx, "carl@example.com")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 2, got.Attempts)
	assert.Equal(t, "h", got.PasswordHash, "other fields must be preserved across increment")

	ttlAfter := mr.TTL(PendingRegKey("carl@example.com"))
	assert.LessOrEqual(t, ttlAfter, ttlBefore, "increment must not reset the TTL clock")
	assert.Greater(t, ttlAfter, time.Duration(0), "increment must not accidentally drop the TTL")
}

func TestRedisAuthStore_IncrementPendingRegAttempts_MissingKey(t *testing.T) {
	store, _ := newTestAuthStore(t)
	ctx := context.Background()

	_, err := store.IncrementPendingRegAttempts(ctx, "ghost@example.com")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPendingNotFound))
}

func TestRedisAuthStore_IncrementPendingResetAttempts_PreservesTTL(t *testing.T) {
	store, _ := newTestAuthStore(t)
	ctx := context.Background()

	data := PendingReg{PasswordHash: "h", CodeHash: "c"}
	require.NoError(t, store.SavePendingReset(ctx, "dana@example.com", data, 15*time.Minute))

	n, err := store.IncrementPendingResetAttempts(ctx, "dana@example.com")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestRedisAuthStore_LoginFail_IncrementAndReset(t *testing.T) {
	store, mr := newTestAuthStore(t)
	ctx := context.Background()
	window := 15 * time.Minute

	n, err := store.IncrementLoginFail(ctx, "Denis", window)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	// "denis" and "Denis" share one counter (lower-cased in the key builder).
	n, err = store.IncrementLoginFail(ctx, "denis", window)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	ttl := mr.TTL(LoginFailKey("denis"))
	assert.Greater(t, ttl, time.Duration(0), "TTL must be set on first increment")

	require.NoError(t, store.ResetLoginFail(ctx, "denis"))
	n, err = store.IncrementLoginFail(ctx, "denis", window)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "counter must restart after reset")
}

func TestRedisAuthStore_MailCooldown_DoubleFireProtection(t *testing.T) {
	store, _ := newTestAuthStore(t)
	ctx := context.Background()
	cooldown := time.Minute

	allowed, err := store.CheckAndSetMailCooldown(ctx, "eve@example.com", cooldown)
	require.NoError(t, err)
	assert.True(t, allowed, "first send must be allowed")

	allowed, err = store.CheckAndSetMailCooldown(ctx, "eve@example.com", cooldown)
	require.NoError(t, err)
	assert.False(t, allowed, "second send within cooldown must be rejected")
}

func TestRedisAuthStore_MailQuota_Increments(t *testing.T) {
	store, _ := newTestAuthStore(t)
	ctx := context.Background()
	window := time.Hour

	for i := 1; i <= 3; i++ {
		n, err := store.IncrementMailQuota(ctx, "frank@example.com", window)
		require.NoError(t, err)
		assert.Equal(t, i, n)
	}

	// A different email has an independent counter.
	n, err := store.IncrementMailQuota(ctx, "gina@example.com", window)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
