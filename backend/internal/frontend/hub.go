package frontend

import (
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/helpers"
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"

	gorilla "github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
)

type internalMessage struct {
	session int32
	message frontend.OutgoingMessage
	cid     *string
}

type cidOnlineMessage struct {
	session int32
	cid     string
}

type Hub struct {
	server                shared.Server
	stripService          shared.StripService
	authenticationService shared.AuthenticationService
	clients               map[*Client]bool

	send chan internalMessage

	register   chan *Client
	unregister chan *Client
	cidOnline  chan cidOnlineMessage

	handlers shared.MessageHandlers[frontend.EventType, *Client]

	msgMu      sync.Mutex
	msgCounter int64 // accessed atomically
	messages   map[int32][]frontend.MessageReceivedEvent

	metarMu    sync.RWMutex
	metarCache map[int32]string // session ID → latest METAR string
}

func NewHub(stripService shared.StripService, authenticationService shared.AuthenticationService) *Hub {
	handlers := shared.NewMessageHandlers[frontend.EventType, *Client]()

	handlers.Add(frontend.Token, handleTokenEvent)
	handlers.Add(frontend.GenerateSquawk, handleGenerateSquawk)
	handlers.Add(frontend.Move, handleMove)
	handlers.Add(frontend.UpdateStripData, handleStripUpdate)
	handlers.Add(frontend.CoordinationTransferRequestType, handleCoordinationTransferRequest)
	handlers.Add(frontend.CoordinationAssumeRequestType, handleCoordinationAssumeRequest)
	handlers.Add(frontend.CoordinationRejectRequestType, handleCoordinationRejectRequest)
	handlers.Add(frontend.CoordinationFreeRequestType, handleCoordinationFreeRequest)
	handlers.Add(frontend.CoordinationCancelTransferRequest, handleCoordinationCancelTransferRequest)
	handlers.Add(frontend.UpdateOrder, handleUpdateOrder)
	handlers.Add(frontend.SendMessage, handleSendMessage)
	handlers.Add(frontend.CdmReady, handleCdmReady)
	handlers.Add(frontend.ReleasePoint, handleReleasePoint)
	handlers.Add(frontend.Marked, handleMarked)
	handlers.Add(frontend.RunwayClearance, handleRunwayClearance)
	handlers.Add(frontend.AcknowledgeUnexpectedChange, handleAcknowledgeUnexpectedChange)
	handlers.Add(frontend.IssuePdcClearance, handleIssuePdcClearance)
	handlers.Add(frontend.PdcManualStateChange, handlePdcManualStateChange)
	handlers.Add(frontend.RevertToVoice, handleRevertToVoice)
	handlers.Add(frontend.ActionCreateTacticalStrip, handleCreateTacticalStrip)
	handlers.Add(frontend.ActionDeleteTacticalStrip, handleDeleteTacticalStrip)
	handlers.Add(frontend.ActionConfirmTacticalStrip, handleConfirmTacticalStrip)
	handlers.Add(frontend.ActionStartTacticalTimer, handleStartTacticalTimer)
	handlers.Add(frontend.ActionMoveTacticalStrip, handleMoveTacticalStrip)

	hub := &Hub{
		send:                  make(chan internalMessage),
		register:              make(chan *Client),
		unregister:            make(chan *Client),
		cidOnline:             make(chan cidOnlineMessage),
		clients:               make(map[*Client]bool),
		handlers:              handlers,
		stripService:          stripService,
		authenticationService: authenticationService,
		messages:              make(map[int32][]frontend.MessageReceivedEvent),
		metarCache:            make(map[int32]string),
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

	// Gate: reject the connection if no EuroScope client is currently online for this airport.
	// Clients waiting for an ES connection (airport == "") are allowed through without checking.
	if airport != WaitingForEuroscopeConnectionAirport {
		esHub := hub.server.GetEuroscopeHub()
		if esHub != nil && !esHub.HasActiveClientForAirport(airport) {
			slog.Info("Rejecting frontend connection: no EuroScope client online",
				slog.String("cid", user.GetCid()),
				slog.String("airport", airport),
			)
			rejection := frontend.ConnectRejectedEvent{Reason: "no EuroScope client connected"}
			if data, err := rejection.Marshal(); err == nil {
				_ = conn.WriteMessage(gorilla.TextMessage, data)
			}
			_ = conn.Close()
			return nil, errors.New("no EuroScope client connected for airport: " + airport)
		}
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
	sessionRepo := hub.server.GetSessionRepository()

	controllers, err := controllerRepo.ListBySession(context.Background(), client.session)
	if err != nil {
		slog.Error("Failed to list controllers", slog.Any("error", err), slog.Int("session", int(client.session)))
		return
	}

	dbSession, err := sessionRepo.GetByID(context.Background(), client.session)
	if err != nil {
		slog.Error("Failed to get session", slog.Any("error", err), slog.Int("session", int(client.session)))
		return
	}

	strips, err := stripRepo.List(context.Background(), client.session)
	if err != nil {
		slog.Error("Failed to list strips", slog.Any("error", err), slog.Int("session", int(client.session)))
		return
	}

	sectors, err := sectorRepo.ListBySession(context.Background(), client.session)
	if err != nil {
		slog.Error("Failed to list sectors", slog.Any("error", err), slog.Int("session", int(client.session)))
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
		section := ""
		if pos, err := config.GetPositionBasedOnFrequency(controller.Position); err == nil {
			section = pos.Section
		}
		c := frontend.Controller{
			Callsign:   controller.Callsign,
			Position:   controller.Position,
			Identifier: identifier,
			Section:    section,
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

	coordRepo := hub.server.GetCoordinationRepository()
	coordinations, err := coordRepo.ListBySession(context.Background(), client.session)
	if err != nil {
		slog.Error("Failed to list coordinations", slog.Any("error", err), slog.Int("session", int(client.session)))
		return
	}

	stripCallsignByID := make(map[int32]string)
	for _, strip := range strips {
		stripCallsignByID[strip.ID] = strip.Callsign
	}

	coordinationModels := make([]frontend.SyncCoordination, 0, len(coordinations))
	for _, coord := range coordinations {
		callsign, ok := stripCallsignByID[coord.StripID]
		if !ok {
			continue
		}
		coordinationModels = append(coordinationModels, frontend.SyncCoordination{
			Callsign: callsign,
			From:     coord.FromPosition,
			To:       coord.ToPosition,
		})
	}

	departure := dbSession.ActiveRunways.DepartureRunways
	if departure == nil {
		departure = make([]string, 0)
	}
	arrival := dbSession.ActiveRunways.ArrivalRunways
	if arrival == nil {
		arrival = make([]string, 0)
	}

	// Load tactical strips
	tacticalStripModels := make([]frontend.TacticalStripPayload, 0)
	tacticalRepo := hub.server.GetTacticalStripRepository()
	if tacticalRepo != nil {
		tacticalStrips, err := tacticalRepo.ListBySession(context.Background(), client.session)
		if err != nil {
			slog.Error("Failed to list tactical strips", slog.Any("error", err), slog.Int("session", int(client.session)))
		} else {
			for _, ts := range tacticalStrips {
				tacticalStripModels = append(tacticalStripModels, MapTacticalStripToPayload(ts))
			}
		}
	}

	hub.msgMu.Lock()
	storedMsgs := make([]frontend.MessageReceivedEvent, len(hub.messages[client.session]))
	copy(storedMsgs, hub.messages[client.session])
	hub.msgMu.Unlock()

	event := frontend.InitialEvent{
		Contsollers:    controllerModels,
		Strips:         stripModels,
		TacticalStrips: tacticalStripModels,
		Me:             me,
		Callsign:       client.callsign,
		Airport:        client.airport,
		Layout:         layout,
		RunwaySetup: frontend.RunwayConfiguration{
			Departure: departure,
			Arrival:   arrival,
		},
		Coordinations: coordinationModels,
		Messages:      storedMsgs,
	}

	client.send <- event

	hub.metarMu.RLock()
	cachedMetar := hub.metarCache[client.session]
	hub.metarMu.RUnlock()

	if cachedMetar != "" {
		client.send <- frontend.AtisUpdateEvent{Metar: cachedMetar}
	}
}

func MapTacticalStripToPayload(ts *internalModels.TacticalStrip) frontend.TacticalStripPayload {
	aircraft := ""
	if ts.Aircraft != nil {
		aircraft = *ts.Aircraft
	}
	confirmedBy := ""
	if ts.ConfirmedBy != nil {
		confirmedBy = *ts.ConfirmedBy
	}
	return frontend.TacticalStripPayload{
		ID:          ts.ID,
		SessionID:   ts.SessionID,
		Type:        ts.Type,
		Bay:         ts.Bay,
		Label:       ts.Label,
		Aircraft:    aircraft,
		ProducedBy:  ts.ProducedBy,
		Sequence:    ts.Sequence,
		TimerStart:  ts.TimerStart,
		Confirmed:   ts.Confirmed,
		ConfirmedBy: confirmedBy,
		CreatedAt:   ts.CreatedAt,
	}
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
		Marked:                 strip.Marked,
		Registration:           helpers.ValueOrDefault(strip.Registration),
		TrackingController:     strip.TrackingController,
		RunwayCleared:          strip.RunwayCleared,
		UnexpectedChangeFields:  strip.UnexpectedChangeFields,
		ControllerModifiedFields: strip.ControllerModifiedFields,
	}
}

func (hub *Hub) CidOnline(session int32, cid string) {
	hub.cidOnline <- cidOnlineMessage{session: session, cid: cid}
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
	section := ""
	if pos, err := config.GetPositionBasedOnFrequency(position); err == nil {
		section = pos.Section
	}
	event := frontend.ControllerOnlineEvent{
		Controller: frontend.Controller{
			Callsign:   callsign,
			Position:   position,
			Identifier: identifier,
			Section:    section,
		},
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendControllerOffline(session int32, callsign string, position string, identifier string) {
	section := ""
	if pos, err := config.GetPositionBasedOnFrequency(position); err == nil {
		section = pos.Section
	}
	event := frontend.ControllerOfflineEvent{
		Controller: frontend.Controller{
			Callsign:   callsign,
			Position:   position,
			Identifier: identifier,
			Section:    section,
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

func (hub *Hub) SendOwnersUpdate(session int32, callsign, owner string, nextOwners []string, previousOwners []string) {
	event := frontend.OwnersUpdateEvent{
		Callsign:       callsign,
		Owner:          owner,
		NextOwners:     nextOwners,
		PreviousOwners: previousOwners,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendLayoutUpdates(session int32, layoutMap map[string]string) {
	for client, _ := range hub.clients {
		if layout, ok := layoutMap[client.position]; client.session == session && ok {
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

func (hub *Hub) SendRunwayConfiguration(session int32, departure, arrival []string) {
	event := frontend.RunwayConfigurationEvent{
		RunwaySetup: frontend.RunwayConfiguration{
			Departure: departure,
			Arrival:   arrival,
		},
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendTacticalStripCreated(session int32, strip frontend.TacticalStripPayload) {
	hub.Broadcast(session, frontend.TacticalStripCreatedEvent{Strip: strip})
}

func (hub *Hub) SendTacticalStripDeleted(session int32, id int64, bay string) {
	hub.Broadcast(session, frontend.TacticalStripDeletedEvent{
		ID:        id,
		SessionID: session,
		Bay:       bay,
	})
}

func (hub *Hub) SendTacticalStripUpdated(session int32, strip frontend.TacticalStripPayload) {
	hub.Broadcast(session, frontend.TacticalStripUpdatedEvent{Strip: strip})
}

func (hub *Hub) SendTacticalStripMoved(session int32, id int64, bay string, sequence int32) {
	hub.Broadcast(session, frontend.TacticalStripMovedEvent{
		ID:        id,
		SessionID: session,
		Bay:       bay,
		Sequence:  sequence,
	})
}

func (hub *Hub) SendBroadcast(session int32, message string, from string) {
	event := frontend.BroadcastEvent{
		Message: message,
		From:    from,
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

func (hub *Hub) SendAtisUpdate(session int32, metar string) {
	hub.metarMu.Lock()
	hub.metarCache[session] = metar
	hub.metarMu.Unlock()

	hub.Broadcast(session, frontend.AtisUpdateEvent{Metar: metar})
}

func (hub *Hub) SendToPosition(session int32, position string, message frontend.OutgoingMessage) {
	for client := range hub.clients {
		if client.session == session && client.position == position {
			client.send <- message
		}
	}
}

func (hub *Hub) NextMessageID() int64 {
	return atomic.AddInt64(&hub.msgCounter, 1)
}

func (hub *Hub) storeMessage(sessionID int32, msg frontend.MessageReceivedEvent) {
	hub.msgMu.Lock()
	defer hub.msgMu.Unlock()
	msgs := hub.messages[sessionID]
	msgs = append([]frontend.MessageReceivedEvent{msg}, msgs...)
	if len(msgs) > 100 {
		msgs = msgs[:100]
	}
	hub.messages[sessionID] = msgs
}

func (hub *Hub) dispatchMessage(session int32, msg frontend.MessageReceivedEvent, senderCID string) {
	if msg.IsBroadcast {
		hub.Broadcast(session, msg)
		return
	}

	// Resolve area names → positions, find first active position per area
	areaMap := config.GetMessageAreas()
	recipientPositions := make(map[string]bool)
	for _, area := range msg.Recipients {
		positions, ok := areaMap[area]
		if !ok {
			continue
		}
		for _, pos := range positions {
			found := false
			for client := range hub.clients {
				if client.session == session && client.position == pos {
					recipientPositions[pos] = true
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	for client := range hub.clients {
		if client.session != session {
			continue
		}
		if client.user.GetCid() == senderCID || recipientPositions[client.position] {
			client.send <- msg
		}
	}
}

func (hub *Hub) OnRegister(client *Client) {
	slog.Debug("Client registered", slog.String("cid", client.user.GetCid()))
	if client.session != WaitingForEuroscopeConnectionSessionId {
		hub.sendInitialEvent(client)
	}
}

func (hub *Hub) OnUnregister(client *Client) {
	slog.Debug("Client unregistered", slog.String("cid", client.user.GetCid()))
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
		case msg := <-hub.cidOnline:
			for client := range hub.clients {
				if client.user.GetCid() == msg.cid {
					slog.Debug("Associating frontend client with session",
						slog.String("cid", msg.cid),
						slog.Int("session", int(msg.session)))
					client.session = msg.session

					// Populate callsign, position, and airport from DB so that
					// sendInitialEvent can find the correct controller and layout.
					// These fields are empty when the client connected before ES.
					if client.callsign == WaitingForEuroscopeConnectionCallsign {
						controllerRepo := hub.server.GetControllerRepository()
						sessionRepo := hub.server.GetSessionRepository()
						if controller, err := controllerRepo.GetByCid(context.Background(), msg.cid); err == nil {
							if dbSession, err := sessionRepo.GetByID(context.Background(), controller.Session); err == nil {
								client.callsign = controller.Callsign
								client.position = controller.Position
								client.airport = dbSession.Airport
							} else {
								slog.Error("Failed to get session for CID online client",
									slog.String("cid", msg.cid), slog.Any("error", err))
							}
						} else {
							slog.Error("Failed to get controller for CID online client",
								slog.String("cid", msg.cid), slog.Any("error", err))
						}
					}

					hub.sendInitialEvent(client)
					break
				}
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
