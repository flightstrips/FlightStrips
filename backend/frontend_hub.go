package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
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

func (hub *FrontendHub) Register(client *FrontendClient) {
	hub.register <- client
}

func (hub *FrontendHub) Unregister(client *FrontendClient) {
	hub.unregister <- client
}

func (hub *FrontendHub) OnRegister(client *FrontendClient) {
	if client.session == WaitingForEuroscopeConnectionSessionId {
		return
	}

	hub.sendInitialEvent(client)
}

func (hub *FrontendHub) sendInitialEvent(client *FrontendClient) {
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
		stripModels = append(stripModels, MapStripToFrontendModel(&strip))
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

func MapStripToFrontendModel(strip *data.Strip) FrontendStrip {
	return FrontendStrip{
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
		Eobt:              strip.Eobt.String,
	}
}

func (hub *FrontendHub) OnUnregister(client *FrontendClient) {
}

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

func (hub *FrontendHub) CidOnline(session int32, cid string) {
	for client := range hub.clients {
		if client.user.cid == cid {
			client.session = session
			hub.sendInitialEvent(client)
			return
		}
	}
}

func (hub *FrontendHub) CidDisconnect(cid string) {
	for client := range hub.clients {
		if client.user.cid == cid {
			client.session = WaitingForEuroscopeConnectionSessionId
			SendFrontendEvent(client, FrontendDisconnectEvent{})
			return
		}
	}
}

func (hub *FrontendHub) SendStripUpdate(session int32, callsign string) {
	db := data.New(hub.server.DBPool)
	strip, err := db.GetStrip(context.Background(), data.GetStripParams{Callsign: callsign, Session: session})
	if err != nil {
		return
	}

	model := MapStripToFrontendModel(&strip)

	message, err := json.Marshal(FrontendStripUpdateEvent{FrontendStrip: model})
	if err != nil {
		return
	}

	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendControllerOnline(session int32, callsign string, position string) {
	message, err := json.Marshal(FrontendControllerOnlineEvent{
		FrontendController: FrontendController{
			Callsign: callsign,
			Position: position,
		},
	})
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendControllerOffline(session int32, callsign string, position string) {
	message, err := json.Marshal(FrontendControllerOfflineEvent{
		FrontendController: FrontendController{
			Callsign: callsign,
			Position: position,
		},
	})
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendAssignedSquawkEvent(session int32, callsign string, squawk string) {
	message, err := json.Marshal(FrontendAssignedSquawkEvent{
		Callsign: callsign,
		Squawk:   squawk,
	})
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendSquawkEvent(session int32, callsign string, squawk string) {
	message, err := json.Marshal(FrontendSquawkEvent{
		Callsign: callsign,
		Squawk:   squawk,
	})
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendRequestedAltitudeEvent(session int32, callsign string, altitude int) {
	message, err := json.Marshal(FrontendRequestedAltitudeEvent{
		Callsign: callsign,
		Altitude: altitude,
	})
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendClearedAltitudeEvent(session int32, callsign string, altitude int) {
	message, err := json.Marshal(FrontendClearedAltitudeEvent{
		Callsign: callsign,
		Altitude: altitude,
	})
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendBayEvent(session int32, callsign string, bay string) {
	message, err := json.Marshal(FrontendBayEvent{
		Callsign: callsign,
		Bay:      bay,
	})
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendAircraftDisconnect(session int32, callsign string) {
	message, err := json.Marshal(FrontendAircraftDisconnectEvent{
		Callsign: callsign,
	})
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendStandEvent(session int32, callsign string, stand string) {
	message, err := json.Marshal(FrontendStandEvent{
		Callsign: callsign,
		Stand:    stand,
	})
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendSetHeadingEvent(session int32, callsign string, heading int) {
	message, err := json.Marshal(FrontendSetHeadingEvent{
		Callsign: callsign,
		Heading:  heading,
	})
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendCommunicationTypeEvent(session int32, callsign string, communicationType string) {
	message, err := json.Marshal(FrontendCommunicationTypeEvent{
		Callsign:          callsign,
		CommunicationType: communicationType,
	})
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendCoordinationTransfer(session int32, callsign, from, to string) {
	event := CoordinationTransferBroadcastEvent{
		Callsign: callsign,
		From:     from,
		To:       to,
	}
	message, err := json.Marshal(event)
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendCoordinationAssume(session int32, callsign, position string) {
	event := CoordinationAssumeBroadcastEvent{
		Callsign: callsign,
		Position: position,
	}
	message, err := json.Marshal(event)
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendCoordinationReject(session int32, callsign, position string) {
	event := CoordinationRejectBroadcastEvent{
		Callsign: callsign,
		Position: position,
	}
	message, err := json.Marshal(event)
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) SendCoordinationFree(session int32, callsign string) {
	event := CoordinationFreeBroadcastEvent{
		Callsign: callsign,
	}
	message, err := json.Marshal(event)
	if err != nil {
		return
	}
	hub.broadcast <- FrontendBroadcastMessage{session: session, message: message}
}

func (hub *FrontendHub) Run() {
	for {
		select {
		case client := <-hub.register:
			hub.clients[client] = true
			hub.OnRegister(client)
		case client := <-hub.unregister:
			if _, ok := hub.clients[client]; ok {
				delete(hub.clients, client)
				client.Close()
			}
			hub.OnUnregister(client)
		case message := <-hub.broadcast:
			for client := range hub.clients {
				if message.session == client.session {
					client.send <- message.message
				}
			}
		case message := <-hub.send:
			for client := range hub.clients {
				if message.session == client.session && message.position == client.position {
					client.send <- message.message
				}
			}
		}
	}
}
