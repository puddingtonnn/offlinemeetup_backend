package websocket

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testHub(t *testing.T) (*Hub, context.CancelFunc) {
	t.Helper()
	hub := NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)), NewLocalBus())
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)
	// Local delivery now flows bus -> consumer -> Run, so the consumer must run.
	require.NoError(t, hub.StartConsumer(ctx))
	return hub, cancel
}

func recvWithTimeout(t *testing.T, ch chan []byte, d time.Duration) ([]byte, bool) {
	t.Helper()
	select {
	case msg, ok := <-ch:
		return msg, ok
	case <-time.After(d):
		return nil, false
	}
}

func (h *Hub) hasUser(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.userClients[userID]
	return ok
}

func TestHub_RegisterAndBroadcastToUser(t *testing.T) {
	hub, cancel := testHub(t)
	defer cancel()

	client := &Client{userID: 1, hub: hub, send: make(chan []byte, 4)}
	hub.register <- client

	// Payloads cross the bus as JSON (envelope.Payload is json.RawMessage), so a
	// test payload must itself be valid JSON.
	hub.BroadcastToUsers([]int64{1}, []byte(`{"v":"hello"}`))

	msg, ok := recvWithTimeout(t, client.send, time.Second)
	require.True(t, ok)
	assert.JSONEq(t, `{"v":"hello"}`, string(msg))
}

func TestHub_Unregister(t *testing.T) {
	hub, cancel := testHub(t)
	defer cancel()

	client := &Client{userID: 1, hub: hub, send: make(chan []byte, 4)}
	hub.register <- client
	require.Eventually(t, func() bool { return hub.hasUser(1) }, time.Second, 5*time.Millisecond)

	hub.unregister <- client

	// После отписки пользователь убран из карты, а его канал закрыт.
	require.Eventually(t, func() bool { return !hub.hasUser(1) }, time.Second, 5*time.Millisecond)
	_, ok := recvWithTimeout(t, client.send, time.Second)
	assert.False(t, ok, "send channel must be closed after unregister")
}

func TestHub_BroadcastToRoomsExcludesSender(t *testing.T) {
	hub, cancel := testHub(t)
	defer cancel()

	sender := &Client{userID: 1, hub: hub, send: make(chan []byte, 4)}
	other := &Client{userID: 2, hub: hub, send: make(chan []byte, 4)}

	roomID := int64(99)
	hub.Subscribe(sender, roomID)
	hub.Subscribe(other, roomID)

	hub.BroadcastToRooms(roomID, []byte(`{"v":"typing"}`), sender.userID)

	// Получатель видит сообщение...
	msg, ok := recvWithTimeout(t, other.send, time.Second)
	require.True(t, ok)
	assert.JSONEq(t, `{"v":"typing"}`, string(msg))

	// ...а отправитель — нет.
	_, ok = recvWithTimeout(t, sender.send, 100*time.Millisecond)
	assert.False(t, ok, "sender must not receive its own room broadcast")
}

// TestHub_BroadcastDoesNotDeadlockOnFullBuffer — регрессионный тест на deadlock
// в trySend: если буфер одного клиента переполнен, это не должно блокировать
// горутину Run и доставку сообщений остальным клиентам.
func TestHub_BroadcastDoesNotDeadlockOnFullBuffer(t *testing.T) {
	hub, cancel := testHub(t)
	defer cancel()

	// "Зависший" клиент: буфер размера 1 и заранее заполнен, никто не читает.
	slow := &Client{userID: 1, hub: hub, send: make(chan []byte, 1)}
	slow.send <- []byte("prefill")

	// Здоровый клиент.
	healthy := &Client{userID: 2, hub: hub, send: make(chan []byte, 8)}

	hub.register <- slow
	hub.register <- healthy

	hub.BroadcastToUsers([]int64{1, 2}, []byte(`{"v":"hello"}`))

	// Здоровый клиент должен получить сообщение, несмотря на зависшего:
	// если бы Run заблокировался на slow, доставки бы не было.
	msg, ok := recvWithTimeout(t, healthy.send, 2*time.Second)
	require.True(t, ok, "healthy client did not receive — hub likely deadlocked")
	assert.JSONEq(t, `{"v":"hello"}`, string(msg))

	// Зависший клиент должен быть автоматически отписан.
	require.Eventually(t, func() bool { return !hub.hasUser(1) }, 2*time.Second, 10*time.Millisecond)
}
