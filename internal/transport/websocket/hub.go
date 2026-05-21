package websocket

import (
	"context"
	"log/slog"
	"sync"
)

type BroadcastMessage struct {
	TargetUserIDs []int64
	RoomID        int64
	Payload       []byte
	SenderClient  *Client
}

type Hub struct {
	userClients map[int64]map[*Client]bool
	rooms       map[int64]map[*Client]bool
	broadcast   chan *BroadcastMessage
	register    chan *Client
	unregister  chan *Client

	mu  sync.RWMutex
	log *slog.Logger
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		broadcast:   make(chan *BroadcastMessage),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		userClients: make(map[int64]map[*Client]bool),
		rooms:       make(map[int64]map[*Client]bool),
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

	if rooms, ok := h.rooms[roomID]; ok {
		delete(rooms, client)
		if len(rooms) == 0 {
			delete(h.rooms, roomID)
		}
	}
}

func (h *Hub) BroadcastToUsers(targetIDs []int64, payload []byte) {
	h.broadcast <- &BroadcastMessage{
		TargetUserIDs: targetIDs,
		Payload:       payload,
	}
}

func (h *Hub) BroadcastToRooms(roomID int64, payload []byte, sender *Client) {
	h.broadcast <- &BroadcastMessage{
		RoomID:       roomID,
		Payload:      payload,
		SenderClient: sender,
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
				delete(clients, client)
				close(client.send)

				if len(clients) == 0 {
					delete(h.userClients, client.userID)
				}
			}

			for _, roomID := range client.rooms {
				h.Unsubscribe(client, roomID)
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
						if client == message.SenderClient {
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
		close(client.send)
		h.unregister <- client
	}
}
