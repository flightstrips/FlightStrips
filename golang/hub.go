package main

// BaseHub represents the common functionality for all hubs
type BaseHub struct {
	// Registered clients.
	clients map[WebsocketClient]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan WebsocketClient

	// Unregister requests from clients.
	unregister chan WebsocketClient

	// Server reference
	server *Server
}

// NewBaseHub creates a new base hub
func NewBaseHub(server *Server) *BaseHub {
	return &BaseHub{
		broadcast:  make(chan []byte),
		register:   make(chan WebsocketClient),
		unregister: make(chan WebsocketClient),
		clients:    make(map[WebsocketClient]bool),
		server:     server,
	}
}

func (h *BaseHub) SendToPosition(position string, message []byte) error {
	for client := range h.clients {
		if client.GetPosition() == position {
			client.Send(message)
		}
	}
	return nil
}

func (h *BaseHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				client.Send(message)
			}
		}
	}
}
