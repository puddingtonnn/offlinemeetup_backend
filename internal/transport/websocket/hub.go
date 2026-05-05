package websocket

type BroadcastMessage struct {
	TargetUserIDs []int64
	Payload       []byte
}

type Hub struct {
	clients    map[int64]*Client
	broadcast  chan *BroadcastMessage
	register   chan *Client
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan *BroadcastMessage),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[int64]*Client),
	}
}

func (h *Hub) BroadcastToUsers(targetIDs []int64, payload []byte) {
	h.broadcast <- &BroadcastMessage{
		TargetUserIDs: targetIDs,
		Payload:       payload,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client.userID] = client
		case client := <-h.unregister:
			if _, ok := h.clients[client.userID]; ok {
				delete(h.clients, client.userID)
				close(client.send)
			}
		case message := <-h.broadcast:
			for _, targetID := range message.TargetUserIDs {
				if client, ok := h.clients[targetID]; ok {
					select {
					case client.send <- message.Payload:
					default:
						close(client.send)
						delete(h.clients, targetID)
					}
				}
			}
		}
	}
}
