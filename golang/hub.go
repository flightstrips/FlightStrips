package main

// BaseHub represents the common functionality for all hubs
type BaseHub[T WebsocketClient] struct {
	// Registered clients.
	clients map[T]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan T

	// Unregister requests from clients.
	unregister chan T

	// Server reference
	server *Server
}

// NewBaseHub creates a new base hub
func NewBaseHub[T WebsocketClient](server *Server) *BaseHub[T] {
	return &BaseHub[T]{
		broadcast:  make(chan []byte),
		register:   make(chan T),
		unregister: make(chan T),
		clients:    make(map[T]bool),
		server:     server,
	}
}

func (h *BaseHub[T]) SendToPosition(position string, message []byte) error {
	for client := range h.clients {
		if client.GetPosition() == position {
			client.Send(message)
		}
	}
	return nil
}

func (h *BaseHub[T]) Run() {
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
