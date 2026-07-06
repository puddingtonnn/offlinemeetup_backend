package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisPresenceStore tracks per-user WS connections in Redis so presence is
// correct across devices and backend instances. A user is online while their
// connection set is non-empty; the set carries a TTL refreshed by heartbeats so
// a crashed instance can't leave a user stuck "online".
type RedisPresenceStore struct {
	rdb *redis.Client
}

// NewRedisPresenceStore builds a presence store over a Redis client.
func NewRedisPresenceStore(rdb *redis.Client) *RedisPresenceStore {
	return &RedisPresenceStore{rdb: rdb}
}

// connectScript adds a connection, refreshes the TTL, and reports the
// offline->online transition atomically (so two instances racing the first
// connection don't both broadcast). KEYS[1]=conns set, ARGV[1]=connID,
// ARGV[2]=ttl ms. Returns 1 on transition, else 0.
var connectScript = redis.NewScript(`
local added = redis.call('SADD', KEYS[1], ARGV[1])
redis.call('PEXPIRE', KEYS[1], ARGV[2])
local n = redis.call('SCARD', KEYS[1])
if added == 1 and n == 1 then return 1 else return 0 end
`)

// disconnectScript removes a connection and reports the online->offline
// transition (last connection gone) atomically. KEYS[1]=conns set,
// ARGV[1]=connID. Returns 1 on transition, else 0.
var disconnectScript = redis.NewScript(`
local removed = redis.call('SREM', KEYS[1], ARGV[1])
local n = redis.call('SCARD', KEYS[1])
if removed == 1 and n == 0 then return 1 else return 0 end
`)

func (s *RedisPresenceStore) Connect(ctx context.Context, userID int64, connID string, ttl time.Duration) (bool, error) {
	res, err := connectScript.Run(ctx, s.rdb, []string{PresenceConnsKey(userID)}, connID, ttl.Milliseconds()).Int()
	if err != nil {
		return false, fmt.Errorf("presence connect: %w", err)
	}
	return res == 1, nil
}

func (s *RedisPresenceStore) Disconnect(ctx context.Context, userID int64, connID string) (bool, error) {
	res, err := disconnectScript.Run(ctx, s.rdb, []string{PresenceConnsKey(userID)}, connID).Int()
	if err != nil {
		return false, fmt.Errorf("presence disconnect: %w", err)
	}
	return res == 1, nil
}

// Refresh extends the TTL of a still-connected user's set (heartbeat). PEXPIRE
// on a missing key is a no-op, so a fully-disconnected user is not revived.
func (s *RedisPresenceStore) Refresh(ctx context.Context, userID int64, ttl time.Duration) error {
	if err := s.rdb.PExpire(ctx, PresenceConnsKey(userID), ttl).Err(); err != nil {
		return fmt.Errorf("presence refresh: %w", err)
	}
	return nil
}

// lastSeenTTL bounds how long an offline user's last_seen is retained. Without
// it (TTL 0 = never expires) the keyspace grows forever — one immortal key per
// user who ever disconnected. A stale last_seen past this horizon is not useful
// to show anyway.
const lastSeenTTL = 90 * 24 * time.Hour

func (s *RedisPresenceStore) SetLastSeen(ctx context.Context, userID int64, ts time.Time) error {
	if err := s.rdb.Set(ctx, PresenceLastSeenKey(userID), ts.Unix(), lastSeenTTL).Err(); err != nil {
		return fmt.Errorf("presence set last seen: %w", err)
	}
	return nil
}

// OnlineStatus reports online state for many users in a single pipeline.
func (s *RedisPresenceStore) OnlineStatus(ctx context.Context, userIDs []int64) (map[int64]bool, error) {
	out := make(map[int64]bool, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}

	pipe := s.rdb.Pipeline()
	cmds := make(map[int64]*redis.IntCmd, len(userIDs))
	for _, id := range userIDs {
		cmds[id] = pipe.SCard(ctx, PresenceConnsKey(id))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("presence online status: %w", err)
	}
	for id, cmd := range cmds {
		out[id] = cmd.Val() > 0
	}
	return out, nil
}

// LastSeen returns the last-offline timestamp for users that have one. Missing
// keys (never offline / never seen) are simply absent from the result.
func (s *RedisPresenceStore) LastSeen(ctx context.Context, userIDs []int64) (map[int64]time.Time, error) {
	out := make(map[int64]time.Time, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}

	pipe := s.rdb.Pipeline()
	cmds := make(map[int64]*redis.StringCmd, len(userIDs))
	for _, id := range userIDs {
		cmds[id] = pipe.Get(ctx, PresenceLastSeenKey(id))
	}
	// redis.Nil for missing keys is expected and surfaces as the pipeline error.
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("presence last seen: %w", err)
	}
	for id, cmd := range cmds {
		v, err := cmd.Int64()
		if err != nil {
			continue // missing or unparseable last_seen
		}
		out[id] = time.Unix(v, 0)
	}
	return out, nil
}
