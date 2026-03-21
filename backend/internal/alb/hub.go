package alb

import (
	pkgAlb "FlightStrips/pkg/events/alb"
	"encoding/json"
	"log/slog"
	"net/http"

	gorilla "github.com/gorilla/websocket"
)

var upgrader = gorilla.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run processes register/unregister events in a select loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			slog.Info("ALB client registered", slog.String("callsign", client.callsign))
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				slog.Info("ALB client unregistered", slog.String("callsign", client.callsign))
			}
		}
	}
}

// Upgrade upgrades an HTTP connection to WebSocket, reads the mandatory login
// event, then registers the client and starts its pumps.
func (h *Hub) Upgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ALB websocket upgrade failed", slog.Any("error", err))
		return
	}

	_, rawLogin, err := conn.ReadMessage()
	if err != nil {
		slog.Info("ALB failed to read login message", slog.Any("error", err))
		conn.Close()
		return
	}

	var loginEvent pkgAlb.LoginEvent
	if err := json.Unmarshal(rawLogin, &loginEvent); err != nil || loginEvent.Type != pkgAlb.Login {
		slog.Info("ALB first message was not a login event")
		conn.Close()
		return
	}

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		callsign: loginEvent.Callsign,
	}

	h.register <- client
	slog.Info("ALB client connected", slog.String("callsign", client.callsign))

	go client.WritePump()
	go client.ReadPump()
}

// BroadcastA2A relays an A2A event to all connected ALB clients when receiver
// is "all", or to the specific client identified by callsign.
func (h *Hub) BroadcastA2A(event pkgAlb.A2AEvent) {
	bytes, err := event.Marshal()
	if err != nil {
		slog.Error("ALB failed to marshal A2A event", slog.Any("error", err))
		return
	}

	for client := range h.clients {
		if event.Receiver == "all" || client.callsign == event.Receiver || client.callsign == event.Sender {
			select {
			case client.send <- bytes:
			default:
				slog.Warn("ALB send buffer full, dropping A2A message", slog.String("callsign", client.callsign))
			}
		}
	}
}
