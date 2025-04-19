package main

type FrontendBroadcastMessage struct {
	session int32
	message []byte
}

type FrontendPositionMessage struct {
	session int32
	position string
	message []byte
}

type FrontendHub struct {
	server *Server
	clients map[*FrontendClient]bool

	broadcast chan FrontendBroadcastMessage
	send chan FrontendPositionMessage

	register chan *FrontendClient
	unregister chan *FrontendClient

}

func (h *FrontendHub) Register(client *FrontendClient) {
	h.register <- client
}

func (h *FrontendHub) Unregister(client *FrontendClient) {
	h.unregister <- client
}

// NewBaseHub creates a new base hub
func NewFrontendHub(server *Server) *FrontendHub {
	return &FrontendHub{
		broadcast:  make(chan FrontendBroadcastMessage),
		send:       make(chan FrontendPositionMessage),
		register:   make(chan *FrontendClient),
		unregister: make(chan *FrontendClient),
		clients:    make(map[*FrontendClient]bool),
		server:     server,
	}
}


func (h *FrontendHub) Run() {
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
				if message.session == client.session {
					client.send <- message.message
				}
			}
		case message := <-h.send:
			for client := range h.clients {
				if message.session == client.session && message.position == client.position {
					client.send <- message.message
				}
			}
		}
	}
}

