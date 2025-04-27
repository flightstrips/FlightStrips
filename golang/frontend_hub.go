package main

import (
	"FlightStrips/data"
	"context"
	"log"
)

type FrontendBroadcastMessage struct {
	session int32
	message []byte
}

type FrontendPositionMessage struct {
	session  int32
	position string
	message  []byte
}

type FrontendHub struct {
	server  *Server
	clients map[*FrontendClient]bool

	broadcast chan FrontendBroadcastMessage
	send      chan FrontendPositionMessage

	register   chan *FrontendClient
	unregister chan *FrontendClient
}

func (h *FrontendHub) Register(client *FrontendClient) {
	h.register <- client
}

func (h *FrontendHub) Unregister(client *FrontendClient) {
	h.unregister <- client
}

func (hub *FrontendHub) OnRegister(client *FrontendClient) {
	if client.session == WaitingForEuroscopeConnectionSessionId {
		return
	}

	db := data.New(hub.server.DBPool)

	controllers, err := db.ListControllers(context.Background(), client.session)
	if err != nil {
		log.Println("Failed to list controllers:", err)
		return
	}

	strips, err := db.ListStrips(context.Background(), client.session)
	if err != nil {
		log.Println("Failed to list strips:", err)
		return
	}

	controllerModels := make([]FrontendController, 0, len(controllers))
	stripModels := make([]FrontendStrip, 0, len(strips))

	for _, controller := range controllers {
		controllerModels = append(controllerModels, FrontendController{
			Callsign: controller.Callsign,
			Position: controller.Position,
		})
	}

	for _, strip := range strips {
		stripModels = append(stripModels, FrontendStrip{
			Callsign:          strip.Callsign,
			Origin:            strip.Origin,
			Destination:       strip.Destination,
			Alternate:         strip.Alternative.String,
			Route:             strip.Route.String,
			Remarks:           strip.Remarks.String,
			Runway:            strip.Remarks.String,
			Squawk:            strip.Squawk.String,
			AssignedSquawk:    strip.AssignedSquawk.String,
			Sid:               strip.Sid.String,
			Cleared:           strip.Cleared.Bool,
			ClearedAltitude:   int(strip.ClearedAltitude.Int32),
			RequestedAltitude: int(strip.RequestedAltitude.Int32),
			Heading:           int(strip.Heading.Int32),
			AircraftType:      strip.AircraftType.String,
			AircraftCategory:  strip.AircraftCategory.String,
			Stand:             strip.Stand.String,
			Capabilities:      strip.Capabilities.String,
			CommunicationType: strip.CommunicationType.String,
			Bay:               strip.Bay.String,
			ReleasePoint:      "",
			Version:           int(strip.Version),
			Sequence:          int(strip.Sequence.Int32),
		})
	}

	event := FrontendInitialEvent{
		Controllers: controllerModels,
		Strips:      stripModels,
		Position:    client.position,
		Callsign:    client.callsign,
		Airport:     client.airport,
		RunwaySetup: RunwayConfiguration{
			Departure: make([]string, 0),
			Arrival:   make([]string, 0),
		},
	}

	SendFrontendEvent(client, event)
}

func (hub *FrontendHub) OnUnregister(client *FrontendClient) {
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
			h.OnRegister(client)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
			h.OnUnregister(client)
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
