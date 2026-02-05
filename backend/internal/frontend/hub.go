package frontend

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/helpers"
	"context"
	"errors"
	"log"

	gorilla "github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
)

type internalMessage struct {
	session int32
	message frontend.OutgoingMessage
	cid     *string
}

type Hub struct {
	server       shared.Server
	stripService shared.StripService
	clients      map[*Client]bool

	send chan internalMessage

	register   chan *Client
	unregister chan *Client

	handlers shared.MessageHandlers[frontend.EventType, *Client]
}

func NewHub(stripService shared.StripService) *Hub {
	handlers := shared.NewMessageHandlers[frontend.EventType, *Client]()

	handlers.Add(frontend.GenerateSquawk, handleGenerateSquawk)
	handlers.Add(frontend.Move, handleMove)
	handlers.Add(frontend.StripUpdate, handleStripUpdate)
	handlers.Add(frontend.CoordinationTransferRequestType, handleCoordinationTransferRequest)
	handlers.Add(frontend.CoordinationAssumeRequestType, handleCoordinationAssumeRequest)
	handlers.Add(frontend.CoordinationRejectRequestType, handleCoordinationRejectRequest)
	handlers.Add(frontend.CoordinationFreeRequestType, handleCoordinationFreeRequest)
	handlers.Add(frontend.UpdateOrder, handleUpdateOrder)
	handlers.Add(frontend.SendMessage, handleSendMessage)
	handlers.Add(frontend.CdmReady, handleCdmReady)
	handlers.Add(frontend.ReleasePoint, handleReleasePoint)
	handlers.Add(frontend.IssuePdcClearance, handleIssuePdcClearance)
	handlers.Add(frontend.PdcManualStateChange, handlePdcManualStateChange)
	handlers.Add(frontend.RevertToVoice, handleRevertToVoice)

	hub := &Hub{
		send:         make(chan internalMessage),
		register:     make(chan *Client),
		unregister:   make(chan *Client),
		clients:      make(map[*Client]bool),
		handlers:     handlers,
		stripService: stripService,
	}

	go hub.Run()

	return hub
}

func (hub *Hub) Unregister(client *Client) {
	hub.unregister <- client
}

func (hub *Hub) GetMessageHandlers() shared.MessageHandlers[frontend.EventType, *Client] {
	return hub.handlers
}

func (hub *Hub) Broadcast(session int32, message frontend.OutgoingMessage) {
	hub.send <- internalMessage{
		session: session,
		message: message,
		cid:     nil,
	}
}

func (hub *Hub) Send(session int32, cid string, message frontend.OutgoingMessage) {
	hub.send <- internalMessage{
		session: session,
		message: message,
		cid:     &cid,
	}
}

func (hub *Hub) GetServer() shared.Server {
	return hub.server
}

func (hub *Hub) SetServer(server shared.Server) {
	hub.server = server
}

func (hub *Hub) HandleNewConnection(conn *gorilla.Conn, user shared.AuthenticatedUser) (*Client, error) {
	controllerRepo := hub.server.GetControllerRepository()
	sessionRepo := hub.server.GetSessionRepository()
	
	controller, err := controllerRepo.GetByCid(context.Background(), user.GetCid())

	var session int32
	var position, airport, callsign string
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}

		session = WaitingForEuroscopeConnectionSessionId
		position = WaitingForEuroscopeConnectionPosition
		airport = WaitingForEuroscopeConnectionAirport
		callsign = WaitingForEuroscopeConnectionCallsign
	} else {
		dbSession, err := sessionRepo.GetByID(context.Background(), controller.Session)

		if err != nil {
			// this should not really happen due to the foreign key constraint on the controller table
			return nil, err
		}

		session = dbSession.ID
		position = controller.Position
		airport = dbSession.Airport
		callsign = controller.Callsign
	}

	// Create and return the client
	client := &Client{
		conn:     conn,
		user:     user,
		session:  session,
		send:     make(chan events.OutgoingMessage),
		hub:      hub,
		position: position,
		airport:  airport,
		callsign: callsign,
	}

	hub.register <- client

	return client, nil
}

func (hub *Hub) sendInitialEvent(client *Client) {
	controllerRepo := hub.server.GetControllerRepository()
	stripRepo := hub.server.GetStripRepository()
	sectorRepo := hub.server.GetSectorOwnerRepository()

	controllers, err := controllerRepo.ListBySession(context.Background(), client.session)
	if err != nil {
		log.Println("Failed to list controllers:", err)
		return
	}

	strips, err := stripRepo.List(context.Background(), client.session)
	if err != nil {
		log.Println("Failed to list strips:", err)
		return
	}

	sectors, err := sectorRepo.ListBySession(context.Background(), client.session)
	if err != nil {
		log.Println("Failed to list sectors:", err)
		return
	}

	sectorsMap := make(map[string]*internalModels.SectorOwner)
	for _, sector := range sectors {
		sectorsMap[sector.Position] = sector
	}

	controllerModels := make([]frontend.Controller, 0, len(controllers))
	stripModels := make([]frontend.Strip, 0, len(strips))
	layout := ""

	me := frontend.Controller{}

	for _, controller := range controllers {
		identifier := ""
		if sector, ok := sectorsMap[controller.Position]; ok {
			identifier = sector.Identifier
		}
		c := frontend.Controller{
			Callsign:   controller.Callsign,
			Position:   controller.Position,
			Identifier: identifier,
		}
		controllerModels = append(controllerModels, c)

		if controller.Callsign == client.callsign {
			me = c
			if controller.Layout != nil {
				layout = *controller.Layout
			}
		}
	}

	for _, strip := range strips {
		stripModels = append(stripModels, MapStripToFrontendModel(strip))
	}

	event := frontend.InitialEvent{
		Contsollers: controllerModels,
		Strips:      stripModels,
		Me:          me,
		Callsign:    client.callsign,
		Airport:     client.airport,
		Layout:      layout,
		RunwaySetup: frontend.RunwayConfiguration{
			Departure: make([]string, 0),
			Arrival:   make([]string, 0),
		},
	}

	client.send <- event
}

func MapStripToFrontendModel(strip *internalModels.Strip) frontend.Strip {

	return frontend.Strip{
		Callsign:            strip.Callsign,
		Origin:              strip.Origin,
		Destination:         strip.Destination,
		Alternate:           helpers.ValueOrDefault(strip.Alternative),
		Route:               helpers.ValueOrDefault(strip.Route),
		Remarks:             helpers.ValueOrDefault(strip.Remarks),
		Runway:              helpers.ValueOrDefault(strip.Runway),
		Squawk:              helpers.ValueOrDefault(strip.Squawk),
		AssignedSquawk:      helpers.ValueOrDefault(strip.AssignedSquawk),
		Sid:                 helpers.ValueOrDefault(strip.Sid),
		ClearedAltitude:     helpers.ValueOrDefault(strip.ClearedAltitude),
		RequestedAltitude:   helpers.ValueOrDefault(strip.RequestedAltitude),
		Heading:             helpers.ValueOrDefault(strip.Heading),
		AircraftType:        helpers.ValueOrDefault(strip.AircraftType),
		AircraftCategory:    helpers.ValueOrDefault(strip.AircraftCategory),
		Stand:               helpers.ValueOrDefault(strip.Stand),
		Capabilities:        helpers.ValueOrDefault(strip.Capabilities),
		CommunicationType:   helpers.ValueOrDefault(strip.CommunicationType),
		Bay:                 strip.Bay,
		ReleasePoint:        helpers.ValueOrDefault(strip.ReleasePoint),
		Version:             strip.Version,
		Sequence:            helpers.ValueOrDefault(strip.Sequence),
		NextControllers:     strip.NextOwners,
		PreviousControllers: strip.PreviousOwners,
		Owner:               helpers.ValueOrDefault(strip.Owner),
		Eobt:                helpers.ValueOrDefault(strip.Eobt),
		Tobt:                helpers.ValueOrDefault(strip.Tobt),
		Tsat:                helpers.ValueOrDefault(strip.Tsat),
		Ctot:                helpers.ValueOrDefault(strip.Ctot),
		PdcState:            strip.PdcState,
	}
}

func (hub *Hub) CidOnline(session int32, cid string) {
	for client := range hub.clients {
		if client.user.GetCid() == cid {
			client.session = session
			hub.sendInitialEvent(client)
			return
		}
	}
}

func (hub *Hub) CidDisconnect(cid string) {
	for client := range hub.clients {
		if client.user.GetCid() == cid {
			client.session = WaitingForEuroscopeConnectionSessionId
			client.send <- frontend.DisconnectEvent{}
			return
		}
	}
}

func (hub *Hub) SendStripUpdate(session int32, callsign string) {
	stripRepo := hub.server.GetStripRepository()
	strip, err := stripRepo.GetByCallsign(context.Background(), session, callsign)
	if err != nil {
		return
	}

	model := MapStripToFrontendModel(strip)

	event := frontend.StripUpdateEvent{
		Strip: model,
	}

	hub.Broadcast(session, event)
}

func (hub *Hub) SendControllerOnline(session int32, callsign string, position string, identifier string) {
	event := frontend.ControllerOnlineEvent{
		Controller: frontend.Controller{
			Callsign:   callsign,
			Position:   position,
			Identifier: identifier,
		},
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendControllerOffline(session int32, callsign string, position string, identifier string) {
	event := frontend.ControllerOfflineEvent{
		Controller: frontend.Controller{
			Callsign:   callsign,
			Position:   position,
			Identifier: identifier,
		},
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendAssignedSquawkEvent(session int32, callsign string, squawk string) {
	event := frontend.AssignedSquawkEvent{
		Callsign: callsign,
		Squawk:   squawk,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendSquawkEvent(session int32, callsign string, squawk string) {
	event := frontend.SquawkEvent{
		Callsign: callsign,
		Squawk:   squawk,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendRequestedAltitudeEvent(session int32, callsign string, altitude int32) {
	event := frontend.RequestedAltitudeEvent{
		Callsign: callsign,
		Altitude: altitude,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendClearedAltitudeEvent(session int32, callsign string, altitude int32) {
	event := frontend.ClearedAltitudeEvent{
		Callsign: callsign,
		Altitude: altitude,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendBayEvent(session int32, callsign string, bay string, sequence int32) {
	event := frontend.BayEvent{
		Callsign: callsign,
		Bay:      bay,
		Sequence: sequence,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendAircraftDisconnect(session int32, callsign string) {
	event := frontend.AircraftDisconnectEvent{
		Callsign: callsign,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendStandEvent(session int32, callsign string, stand string) {
	event := frontend.StandEvent{
		Callsign: callsign,
		Stand:    stand,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendSetHeadingEvent(session int32, callsign string, heading int32) {
	event := frontend.SetHeadingEvent{
		Callsign: callsign,
		Heading:  heading,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendCommunicationTypeEvent(session int32, callsign string, communicationType string) {
	event := frontend.CommunicationTypeEvent{
		Callsign:          callsign,
		CommunicationType: communicationType,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendCoordinationTransfer(session int32, callsign, from, to string) {
	event := frontend.CoordinationTransferBroadcastEvent{
		Callsign: callsign,
		From:     from,
		To:       to,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendCoordinationAssume(session int32, callsign, position string) {
	event := frontend.CoordinationAssumeBroadcastEvent{
		Callsign: callsign,
		Position: position,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendCoordinationReject(session int32, callsign, position string) {
	event := frontend.CoordinationRejectBroadcastEvent{
		Callsign: callsign,
		Position: position,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendCoordinationFree(session int32, callsign string) {
	event := frontend.CoordinationFreeBroadcastEvent{
		Callsign: callsign,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendOwnersUpdate(session int32, callsign string, nextOwners []string, previousOwners []string) {
	event := frontend.OwnersUpdateEvent{
		Callsign:       callsign,
		NextOwners:     nextOwners,
		PreviousOwners: previousOwners,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendLayoutUpdates(session int32, layoutMap map[string]string) {
	for client, _ := range hub.clients {
		if layout, ok := layoutMap[client.callsign]; client.session == session && ok {
			event := frontend.LayoutUpdateEvent{
				Layout: layout,
			}
			client.send <- event
		}
	}
}

func (hub *Hub) SendCdmUpdate(session int32, callsign, eobt, tobt, tsat, ctot string) {
	event := frontend.CdmDataEvent{
		Callsign: callsign,
		Eobt:     eobt,
		Tobt:     tobt,
		Tsat:     tsat,
		Ctot:     ctot,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendCdmWait(session int32, callsign string) {
	event := frontend.CdmWaitEvent{
		Callsign: callsign,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendPdcStateChange(session int32, callsign, state string) {
	event := frontend.PdcStateChangeEvent{
		Callsign: callsign,
		State:    state,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendServerMessage(session int32, message string) {
	event := frontend.BroadcastEvent{
		Message: message,
		From:    "SERVER",
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendToPosition(session int32, position string, message frontend.OutgoingMessage) {
	for client := range hub.clients {
		if client.session == session && client.position == position {
			client.send <- message
		}
	}
}

func (hub *Hub) OnRegister(client *Client) {
	log.Println("Client registered:", client.user.GetCid())
	if client.session != WaitingForEuroscopeConnectionSessionId {
		hub.sendInitialEvent(client)
	}
}

func (hub *Hub) OnUnregister(client *Client) {
	log.Println("Client unregistered:", client.user.GetCid())
}

func (hub *Hub) Run() {
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
		case message := <-hub.send:
			if message.cid != nil {
				for client := range hub.clients {
					if message.session == client.session && *message.cid == client.GetCid() {
						client.send <- message.message
					}
				}
			} else {
				for client := range hub.clients {
					if message.session == client.session {
						client.send <- message.message
					}
				}
			}
		}
	}
}
