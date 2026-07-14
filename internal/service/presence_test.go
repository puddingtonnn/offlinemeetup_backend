package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/cache"
)

// fakePeers is a hand-rolled chatPeers so presence tests don't need the chat repo.
type fakePeers struct {
	coChat       map[int64][]int64
	participants map[int64][]int64
}

func (f *fakePeers) GetCoChatUserIDs(_ context.Context, userID int64) ([]int64, error) {
	return f.coChat[userID], nil
}

func (f *fakePeers) GetChatParticipantIDs(_ context.Context, chatID int64) ([]int64, error) {
	return f.participants[chatID], nil
}

// fakeNames is a hand-rolled profileNames so presence tests don't need the DB.
type fakeNames struct {
	nicknames map[int64]string
}

func (f *fakeNames) NicknamesByUserIDs(_ context.Context, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	for _, id := range ids {
		if n, ok := f.nicknames[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

func newPresenceService(t *testing.T, peers *fakePeers) *PresenceService {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	names := &fakeNames{nicknames: map[int64]string{1: "alice", 2: "bob", 3: "carol"}}
	return NewPresenceService(cache.NewRedisPresenceStore(rdb), peers, names, time.Minute)
}

func TestPresenceService_ConnectDisconnectTransitions(t *testing.T) {
	svc := newPresenceService(t, &fakePeers{coChat: map[int64][]int64{1: {2, 3}}})
	ctx := context.Background()

	// First connection => online, co-chat members are the recipients.
	online, recipients, err := svc.OnConnect(ctx, 1, "c1")
	require.NoError(t, err)
	assert.True(t, online)
	assert.ElementsMatch(t, []int64{2, 3}, recipients)

	// Second device => not a transition, nobody to notify.
	online, recipients, err = svc.OnConnect(ctx, 1, "c2")
	require.NoError(t, err)
	assert.False(t, online)
	assert.Empty(t, recipients)

	// Losing one of two connections is not yet offline.
	offline, _, _, err := svc.OnDisconnect(ctx, 1, "c1")
	require.NoError(t, err)
	assert.False(t, offline)

	// Losing the last connection => offline, recipients + a last_seen stamp.
	offline, lastSeen, recipients, err := svc.OnDisconnect(ctx, 1, "c2")
	require.NoError(t, err)
	assert.True(t, offline)
	assert.ElementsMatch(t, []int64{2, 3}, recipients)
	assert.False(t, lastSeen.IsZero())
}

func TestPresenceService_StatusForChat(t *testing.T) {
	svc := newPresenceService(t, &fakePeers{participants: map[int64][]int64{100: {1, 2}}})
	ctx := context.Background()

	// A non-member must not read a chat's presence.
	_, err := svc.StatusForChat(ctx, 100, 999)
	require.ErrorIs(t, err, ErrForbidden)

	// Bring user 1 online; user 2 stays offline.
	_, _, err = svc.OnConnect(ctx, 1, "c1")
	require.NoError(t, err)

	statuses, err := svc.StatusForChat(ctx, 100, 1)
	require.NoError(t, err)
	require.Len(t, statuses, 2)

	byID := make(map[int64]PresenceStatus, len(statuses))
	for _, s := range statuses {
		byID[s.UserID] = s
	}
	assert.True(t, byID[1].Online)
	assert.False(t, byID[2].Online)

	// Each status carries the display nickname so clients skip a second lookup.
	assert.Equal(t, "alice", byID[1].Nickname)
	assert.Equal(t, "bob", byID[2].Nickname)
}
