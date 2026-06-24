package websocket

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRedisBus_CrossInstanceDelivery proves the core scaling guarantee: a
// broadcast emitted on one instance reaches a recipient connected to a different
// instance, and every connection receives it exactly once (no double delivery).
func TestRedisBus_CrossInstanceDelivery(t *testing.T) {
	mr := miniredis.RunT(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	newInstance := func() *Hub {
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		hub := NewHub(log, NewRedisBus(rdb))
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go hub.Run(ctx)
		// StartConsumer blocks until the Redis subscription is confirmed, so the
		// broadcast below cannot race ahead of the subscription.
		require.NoError(t, hub.StartConsumer(ctx))
		return hub
	}

	hubA := newInstance()
	hubB := newInstance()

	// The same user has one connection on each instance.
	clientA := &Client{userID: 7, hub: hubA, send: make(chan []byte, 4)}
	clientB := &Client{userID: 7, hub: hubB, send: make(chan []byte, 4)}
	hubA.register <- clientA
	hubB.register <- clientB

	// Broadcast originates on instance A.
	hubA.BroadcastToUsers([]int64{7}, []byte(`{"v":"hi"}`))

	msgB, ok := recvWithTimeout(t, clientB.send, 2*time.Second)
	require.True(t, ok, "client on instance B did not receive the cross-instance broadcast")
	assert.JSONEq(t, `{"v":"hi"}`, string(msgB))

	msgA, ok := recvWithTimeout(t, clientA.send, 2*time.Second)
	require.True(t, ok, "client on instance A did not receive its own-instance broadcast")
	assert.JSONEq(t, `{"v":"hi"}`, string(msgA))

	// Exactly once per connection — no duplicate from the publisher also
	// consuming its own message.
	_, ok = recvWithTimeout(t, clientB.send, 200*time.Millisecond)
	assert.False(t, ok, "client B received a duplicate")
	_, ok = recvWithTimeout(t, clientA.send, 200*time.Millisecond)
	assert.False(t, ok, "client A received a duplicate")
}
