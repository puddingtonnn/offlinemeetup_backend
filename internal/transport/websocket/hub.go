package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
)

// BroadcastMessage is the Hub's internal delivery unit (after decoding from the
// bus). SenderUserID replaces a *Client pointer so room-exclusion survives a
// round-trip through Redis, where the sender lives on a different instance.
type BroadcastMessage struct {
	TargetUserIDs []int64
	RoomID        int64
	Payload       []byte
	SenderUserID  int64
}

// busEnvelope is the wire format published to the bus. Short JSON keys keep the
// per-event overhead small; Payload is an already-encoded WSEvent.
type busEnvelope struct {
	TargetUserIDs []int64         `json:"u,omitempty"`
	RoomID        int64           `json:"r,omitempty"`
	SenderUserID  int64           `json:"s,omitempty"`
	Payload       json.RawMessage `json:"p"`
}

type Hub struct {
	userClients map[int64]map[*Client]bool
	rooms       map[int64]map[*Client]bool
	broadcast   chan *BroadcastMessage
	register    chan *Client
	unregister  chan *Client

	bus        MessageBus
	pubChannel string

	mu  sync.RWMutex
	log *slog.Logger
}

func NewHub(log *slog.Logger, bus MessageBus) *Hub {
	return &Hub{
		broadcast:   make(chan *BroadcastMessage),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		userClients: make(map[int64]map[*Client]bool),
		rooms:       make(map[int64]map[*Client]bool),
		bus:         bus,
		pubChannel:  wsBroadcastChannel,
		log:         log,
	}
}

func (h *Hub) Subscribe(client *Client, roomID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rooms[roomID] == nil {
		h.rooms[roomID] = make(map[*Client]bool)
	}

	h.rooms[roomID][client] = true
}

func (h *Hub) Unsubscribe(client *Client, roomID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.unsubscribeNoLock(client, roomID)
}

func (h *Hub) unsubscribeNoLock(client *Client, roomID int64) {
	if rooms, ok := h.rooms[roomID]; ok {
		delete(rooms, client)
		if len(rooms) == 0 {
			delete(h.rooms, roomID)
		}
	}
}

// BroadcastToUsers delivers payload to every connection of each target user,
// across all instances. It publishes to the bus; local delivery happens only in
// the consumer (so a message is never delivered twice).
func (h *Hub) BroadcastToUsers(targetIDs []int64, payload []byte) {
	h.publish(busEnvelope{TargetUserIDs: targetIDs, Payload: payload})
}

// BroadcastToRooms delivers payload to every subscriber of roomID except the
// sender. senderUserID (not a *Client) is used for exclusion so it works even
// when the sender is connected to another instance.
func (h *Hub) BroadcastToRooms(roomID int64, payload []byte, senderUserID int64) {
	h.publish(busEnvelope{RoomID: roomID, SenderUserID: senderUserID, Payload: payload})
}

// publish encodes an envelope and emits it on the bus. Best-effort: a publish
// failure is logged, never fatal — the message itself is already persisted.
func (h *Hub) publish(env busEnvelope) {
	data, err := json.Marshal(env)
	if err != nil {
		h.log.Error("ws hub: marshal envelope", slog.Any("err", err))
		return
	}
	if err := h.bus.Publish(context.Background(), h.pubChannel, data); err != nil {
		h.log.Error("ws hub: publish to bus", slog.Any("err", err))
	}
}

// StartConsumer subscribes to the bus and feeds decoded broadcasts into Run for
// local delivery. It blocks only to establish the subscription, then runs until
// ctx is cancelled. Call once per instance alongside Run.
func (h *Hub) StartConsumer(ctx context.Context) error {
	ch, err := h.bus.Subscribe(ctx, h.pubChannel)
	if err != nil {
		return err
	}
	go h.consume(ctx, ch)
	return nil
}

func (h *Hub) consume(ctx context.Context, ch <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			var env busEnvelope
			if err := json.Unmarshal(data, &env); err != nil {
				h.log.Error("ws hub: decode envelope", slog.Any("err", err))
				continue
			}
			msg := &BroadcastMessage{
				TargetUserIDs: env.TargetUserIDs,
				RoomID:        env.RoomID,
				SenderUserID:  env.SenderUserID,
				Payload:       env.Payload,
			}
			select {
			case h.broadcast <- msg:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			for _, clients := range h.userClients {
				for client := range clients {
					close(client.send)
				}
			}
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			if h.userClients[client.userID] == nil {
				h.userClients[client.userID] = make(map[*Client]bool)
			}
			h.userClients[client.userID][client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.userClients[client.userID]; ok {
				if _, exists := clients[client]; exists {
					delete(clients, client)
					close(client.send)

					if len(clients) == 0 {
						delete(h.userClients, client.userID)
					}
				}
			}

			for _, roomID := range client.rooms {
				h.unsubscribeNoLock(client, roomID)
			}

			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			if len(message.TargetUserIDs) > 0 {
				for _, targetID := range message.TargetUserIDs {
					if clients, ok := h.userClients[targetID]; ok {
						for client := range clients {
							h.trySend(client, message.Payload)
						}
					}
				}
			}

			if message.RoomID > 0 {
				if clients, ok := h.rooms[message.RoomID]; ok {
					for client := range clients {
						if client.userID == message.SenderUserID {
							continue
						}
						h.trySend(client, message.Payload)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) trySend(client *Client, payload []byte) {
	select {
	case client.send <- payload:
	default:
		// Буфер клиента переполнен — считаем его "зависшим" и отписываем.
		// Запускаем в отдельной горутине: trySend вызывается из горутины Run
		// (под broadcast), а h.unregister читает та же горутина — прямая
		// запись сюда привела бы к deadlock'у всего хаба.
		go func() { h.unregister <- client }()
	}
}
