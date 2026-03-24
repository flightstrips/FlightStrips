package euroscope

import (
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testing/recorder"
	"FlightStrips/pkg/events"
	"FlightStrips/pkg/events/euroscope"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
)

type internalMessage struct {
	session int32
	message euroscope.OutgoingMessage
	cid     *string
}

type Hub struct {
	server                shared.Server
	stripService          shared.StripService
	controllerService     shared.ControllerService
	authenticationService shared.AuthenticationService
	clients               map[*Client]bool

	// airportClientsMu guards airportClientCount for concurrent reads from other goroutines.
	airportClientsMu   sync.RWMutex
	airportClientCount map[string]int // airport → number of connected ES clients

	send chan internalMessage

	register   chan *Client
	unregister chan *Client

	master map[int32]*Client

	handlers shared.MessageHandlers[euroscope.EventType, *Client]

	// Recording support
	recorders map[int32]*recorder.Recorder // One recorder per session

	// Offline timer support — cancellable per-position delayed offline processing
	offlineMu     sync.Mutex
	offlineTimers map[string]context.CancelFunc // key: "<sessionID>:<positionName>"
}

func NewHub(stripService shared.StripService, controllerService shared.ControllerService, authenticationService shared.AuthenticationService) *Hub {
	handlers := shared.NewMessageHandlers[euroscope.EventType, *Client]()

	handlers.Add(euroscope.Authentication, handleTokenEvent)
	handlers.Add(euroscope.ControllerOnline, handleControllerOnline)
	handlers.Add(euroscope.ControllerOffline, handleControllerOffline)
	handlers.Add(euroscope.CommunicationType, handleCommunicationType)
	handlers.Add(euroscope.AssignedSquawk, handleAssignedSquawk)
	handlers.Add(euroscope.Squawk, handleSquawk)
	handlers.Add(euroscope.GroundState, handleGroundState)
	handlers.Add(euroscope.ClearedFlag, handleClearedFlag)
	handlers.Add(euroscope.Stand, handleStand)
	handlers.Add(euroscope.RequestedAltitude, handleRequestedAltitude)
	handlers.Add(euroscope.ClearedAltitude, handleClearedAltitude)
	handlers.Add(euroscope.PositionUpdate, handlePositionUpdate)
	handlers.Add(euroscope.SetHeading, handleSetHeading)
	handlers.Add(euroscope.AircraftDisconnected, handleAircraftDisconnected)
	handlers.Add(euroscope.Sync, handleSync)
	handlers.Add(euroscope.StripUpdate, handleStripUpdateEvent)
	handlers.Add(euroscope.Runway, handleRunways)
	handlers.Add(euroscope.CdmLocalData, handleCdmLocalData)
	handlers.Add(euroscope.TrackingControllerChanged, handleTrackingControllerChanged)
	handlers.Add(euroscope.CoordinationReceived, handleCoordinationReceived)

	hub := &Hub{
		register:              make(chan *Client),
		unregister:            make(chan *Client),
		clients:               make(map[*Client]bool),
		send:                  make(chan internalMessage),
		master:                make(map[int32]*Client),
		handlers:              handlers,
		stripService:          stripService,
		controllerService:     controllerService,
		authenticationService: authenticationService,
		recorders:             make(map[int32]*recorder.Recorder),
		offlineTimers:         make(map[string]context.CancelFunc),
		airportClientCount:    make(map[string]int),
	}

	go hub.Run()

	return hub
}

func (hub *Hub) Register(client *Client) {
	hub.register <- client
}

func (hub *Hub) Unregister(client *Client) {
	hub.unregister <- client
}

func (hub *Hub) Broadcast(session int32, message euroscope.OutgoingMessage) {
	hub.send <- internalMessage{
		session: session,
		message: message,
		cid:     nil,
	}
}

func (hub *Hub) Send(session int32, cid string, message euroscope.OutgoingMessage) {
	hub.send <- internalMessage{
		session: session,
		message: message,
		cid:     &cid,
	}
}

func (hub *Hub) OnRegister(client *Client) {
	// Track per-airport client count for HasActiveClientForAirport queries.
	hub.airportClientsMu.Lock()
	hub.airportClientCount[client.airport]++
	hub.airportClientsMu.Unlock()
	// Start recording if in record mode and not already recording this session
	if config.IsRecordMode() && !hub.IsRecording(client.session) {
		err := hub.StartRecording(client.session, client.airport, "LIVE", "Auto-recorded session")
		if err != nil {
			slog.Error("Failed to start recording", slog.Any("error", err))
		} else {
			// Set login info in the recorder
			if rec, ok := hub.recorders[client.session]; ok {
				rec.SetLoginInfo(client.position, client.callsign, 0) // range not stored in client
			}
		}
	}

	// Determine master role immediately to avoid race conditions
	isMaster := false
	if _, ok := hub.master[client.session]; !ok {
		slog.Debug("Euroscope client is master", slog.String("cid", client.GetCid()))
		hub.master[client.session] = client
		isMaster = true
	}

	// Send BackendSync first, then delay, then SessionInfo
	go func() {
		hub.sendBackendSyncIfNeeded(client)
		time.Sleep(2 * time.Second)
		if isMaster {
			client.send <- euroscope.SessionInfoEvent{Role: euroscope.SessionInfoMaster}
		} else {
			client.send <- euroscope.SessionInfoEvent{Role: euroscope.SessionInfoSlave}
			// For slaves, layouts are already calculated by the master; notify the frontend now.
			hub.server.GetFrontendHub().CidOnline(client.session, client.user.GetCid())
		}
	}()
}

// sendBackendSyncIfNeeded fetches all existing strips for the client's session
// and sends a BackendSyncEvent to the client so it can apply the backend-authoritative
// state to EuroScope before assuming master or slave duties.
func (hub *Hub) sendBackendSyncIfNeeded(client *Client) {
	stripRepo := hub.server.GetStripRepository()
	strips, err := stripRepo.List(context.Background(), client.session)
	if err != nil {
		slog.Error("Failed to fetch strips for backend sync", slog.Any("error", err))
		return
	}

	syncStrips := make([]euroscope.BackendSyncStrip, 0, len(strips))
	for _, strip := range strips {
		entry := euroscope.BackendSyncStrip{
			Callsign: strip.Callsign,
			Cleared:  strip.Cleared,
		}
		if strip.AssignedSquawk != nil {
			entry.AssignedSquawk = *strip.AssignedSquawk
		}
		if strip.State != nil {
			entry.GroundState = *strip.State
		}
		if strip.Stand != nil {
			entry.Stand = *strip.Stand
		}
		syncStrips = append(syncStrips, entry)
	}

	lat, lon := config.GetAirportCoordinates()
	client.send <- euroscope.BackendSyncEvent{
		Strips:    syncStrips,
		Latitude:  lat,
		Longitude: lon,
	}
	slog.Debug("Sent backend sync to connecting EuroScope client",
		slog.String("cid", client.GetCid()),
		slog.Int("session", int(client.session)),
		slog.Int("strips", len(syncStrips)),
	)
}

func (hub *Hub) GetMessageHandlers() shared.MessageHandlers[euroscope.EventType, *Client] {
	return hub.handlers
}

func (hub *Hub) GetServer() shared.Server {
	return hub.server
}

func (hub *Hub) SetServer(server shared.Server) {
	hub.server = server
}

func (hub *Hub) SetControllerService(controllerService shared.ControllerService) {
	hub.controllerService = controllerService
}

func (hub *Hub) HandleNewConnection(conn *gorilla.Conn, user shared.AuthenticatedUser) (*Client, error) {
	slog.Debug("Euroscope client connected", slog.String("cid", user.GetCid()))
	// Read the login message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read login message: %w", err)
	}

	// Handle the login
	event, sessionID, err := hub.handleLogin(msg, user)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	client := &Client{
		conn:     conn,
		session:  sessionID,
		send:     make(chan events.OutgoingMessage),
		hub:      hub,
		user:     user,
		position: event.Position,
		callsign: event.Callsign,
		airport:  event.Airport,
	}

	hub.register <- client

	return client, nil
}

func (hub *Hub) handleLogin(msg []byte, user shared.AuthenticatedUser) (event euroscope.LoginEvent, sessionID int32, err error) {
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return
	}

	if event.Type != euroscope.Login {
		err = fmt.Errorf("invalid initial event type, expected login")
		return
	}

	sessionName := "LIVE"
	if sessionName == "PLAYBACK" {
		sessionName = sessionName + "_" + strconv.Itoa(rand.Int())
	}

	slog.Debug("Euroscope client logged in", slog.String("cid", user.GetCid()), slog.String("session", sessionName))

	session, err := hub.server.GetOrCreateSession(event.Airport, sessionName)
	if err != nil {
		return
	}

	// Since the login is sent on first logon and when a position is changed we need to check if the controller is
	// already in the database. It may also already be in the database if the master have synced it before a new
	// controller connects to FlightStrips

	controllerRepo := hub.server.GetControllerRepository()
	controller, err := controllerRepo.Get(context.Background(), event.Callsign, session.Id)

	cid := user.GetCid()
	now := time.Now().UTC()
	if errors.Is(err, pgx.ErrNoRows) {
		newController := &internalModels.Controller{
			Callsign:          event.Callsign,
			Session:           session.Id,
			Position:          event.Position,
			Cid:               &cid,
			LastSeenEuroscope: &now,
		}

		err = controllerRepo.Create(context.Background(), newController)

		if err != nil {
			return event, session.Id, err
		}

		return event, session.Id, nil
	} else if err != nil {
		return event, session.Id, err
	} else {
		// Set CID
		controllerRepo.SetCid(context.Background(), session.Id, event.Callsign, &cid)
	}

	if controller.Position != event.Position {
		_, err = controllerRepo.SetPosition(context.TODO(), session.Id, event.Callsign, event.Position)

		if err != nil {
			return event, session.Id, err
		}
	}

	return event, session.Id, err
}

func (hub *Hub) OnUnregister(client *Client) {
	// Update per-airport client count.
	hub.airportClientsMu.Lock()
	if hub.airportClientCount[client.airport] > 0 {
		hub.airportClientCount[client.airport]--
		if hub.airportClientCount[client.airport] == 0 {
			delete(hub.airportClientCount, client.airport)
		}
	}
	hub.airportClientsMu.Unlock()

	server := hub.server
	controllerRepo := server.GetControllerRepository()
	count, err := controllerRepo.SetCid(context.Background(), client.session, client.callsign, nil)

	if err != nil || count != 1 {
		slog.Error("Failed to remove CID for client", slog.String("callsign", client.callsign), slog.String("cid", client.GetCid()), slog.Any("error", err))
	}

	if master, ok := hub.master[client.session]; !ok || master != client {
		return
	}

	// No clients, no master can be assigned
	if len(hub.clients) == 0 {
		delete(hub.master, client.session)
		return
	}

	// TODO better master selection. For now just use the next available client
	for newMaster := range hub.clients {
		hub.master[client.session] = newMaster
		newMaster.send <- euroscope.SessionInfoEvent{Role: euroscope.SessionInfoMaster}
		break
	}
}

func (hub *Hub) SendGenerateSquawk(session int32, cid string, callsign string) {
	event := euroscope.GenerateSquawkEvent{
		Callsign: callsign,
	}
	hub.Send(session, cid, event)
}

func (hub *Hub) SendCdmReadyRequest(session int32, cid string, callsign string) {
	hub.Send(session, cid, euroscope.CdmReadyRequestEvent{Callsign: callsign})
}

func (hub *Hub) SendGroundState(session int32, cid string, callsign string, state string) {
	event := euroscope.GroundStateEvent{
		Callsign:    callsign,
		GroundState: state,
	}

	hub.Send(session, cid, event)
}

func (hub *Hub) SendClearedFlag(session int32, cid string, callsign string, flag bool) {
	event := euroscope.ClearedFlagEvent{
		Callsign: callsign,
		Cleared:  flag,
	}

	hub.Send(session, cid, event)
}

func (hub *Hub) SendStand(session int32, cid string, callsign string, stand string) {
	event := euroscope.StandEvent{
		Callsign: callsign,
		Stand:    stand,
	}

	hub.Send(session, cid, event)
}

func (hub *Hub) SendRoute(session int32, cid string, callsign string, route string) {
	event := euroscope.RouteEvent{
		Callsign: callsign,
		Route:    route,
	}

	hub.Send(session, cid, event)
}

func (hub *Hub) SendRemarks(session int32, cid string, callsign string, remarks string) {
	event := euroscope.RemarksEvent{
		Callsign: callsign,
		Remarks:  remarks,
	}

	hub.Send(session, cid, event)
}

func (hub *Hub) SendSid(session int32, cid string, callsign string, sid string) {
	event := euroscope.SidEvent{
		Callsign: callsign,
		Sid:      sid,
	}

	hub.Send(session, cid, event)
}

func (hub *Hub) SendAssignedSquawk(session int32, cid string, callsign string, squawk string) {
	event := euroscope.AssignedSquawkEvent{
		Callsign: callsign,
		Squawk:   squawk,
	}

	hub.Send(session, cid, event)
}

func (hub *Hub) SendRunway(session int32, cid string, callsign string, runway string) {
	event := euroscope.AircraftRunwayEvent{
		Callsign: callsign,
		Runway:   runway,
	}

	hub.Send(session, cid, event)
}

func (hub *Hub) SendClearedAltitude(session int32, cid string, callsign string, altitude int32) {
	event := euroscope.ClearedAltitudeEvent{
		Callsign: callsign,
		Altitude: altitude,
	}

	hub.Send(session, cid, event)
}

func (hub *Hub) SendHeading(session int32, cid string, callsign string, heading int32) {
	event := euroscope.HeadingEvent{
		Callsign: callsign,
		Heading:  heading,
	}

	hub.Send(session, cid, event)
}

func (hub *Hub) SendAssumeAndDrop(session int32, cid string, callsign string) {
	hub.Send(session, cid, euroscope.AssumeAndDropEvent{Callsign: callsign})
}

// HasActiveClientForAirport returns true if at least one ES client is currently
// connected for the given airport. This is safe to call from any goroutine.
func (hub *Hub) HasActiveClientForAirport(airport string) bool {
	hub.airportClientsMu.RLock()
	defer hub.airportClientsMu.RUnlock()
	return hub.airportClientCount[airport] > 0
}

func (hub *Hub) SendCoordinationHandover(session int32, cid string, callsign string, targetCallsign string) {
	event := euroscope.CoordinationHandoverEvent{
		Callsign:       callsign,
		TargetCallsign: targetCallsign,
	}

	hub.Send(session, cid, event)
}

func (hub *Hub) SendCreateFPL(session int32, cid string, event euroscope.CreateFPLEvent) {
	hub.Send(session, cid, event)
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
				hub.server.GetFrontendHub().CidDisconnect(client.GetCid())
			}
			hub.OnUnregister(client)
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

// scheduleOfflineActions starts a goroutine that, after a 15-second grace period,
// deletes the controller from the database, notifies all frontend clients that the
// controller is offline, recalculates sector ownership, and — 5 seconds later —
// broadcasts a specific sector-change notification.
//
// If cancelOfflineTimer is called for the same position key before the 15-second
// window elapses, the goroutine exits cleanly and none of the above happens.
func (hub *Hub) scheduleOfflineActions(session int32, callsign, positionFreq, positionName string) {
	key := fmt.Sprintf("%d:%s", session, positionName)

	ctx, cancel := context.WithCancel(context.Background())

	hub.offlineMu.Lock()
	if existing, ok := hub.offlineTimers[key]; ok {
		existing()
	}
	hub.offlineTimers[key] = cancel
	hub.offlineMu.Unlock()

	go func() {
		// Phase 1: 15-second grace period
		select {
		case <-ctx.Done():
			slog.Debug("Controller offline timer cancelled (position came back online)",
				slog.String("position", positionName),
				slog.String("callsign", callsign),
				slog.Int("session", int(session)))
			return
		case <-time.After(15 * time.Second):
		}

		slog.Debug("Controller offline timer fired: processing offline",
			slog.String("position", positionName),
			slog.String("callsign", callsign),
			slog.Int("session", int(session)))

		s := hub.server
		bgCtx := context.Background()

		controllerRepo := s.GetControllerRepository()
		if err := controllerRepo.Delete(bgCtx, session, callsign); err != nil {
			slog.Error("Failed to delete controller record in offline timer",
				slog.String("callsign", callsign),
				slog.Any("error", err))
		}

		s.GetFrontendHub().SendControllerOffline(session, callsign, positionFreq, "")

		var changes []shared.SectorChange
		if _, configErr := config.GetPositionBasedOnFrequency(positionFreq); configErr == nil {
			var err error
			changes, err = s.UpdateSectors(session)
			if err != nil {
				slog.Error("Failed to update sectors in offline timer",
					slog.String("callsign", callsign),
					slog.Any("error", err))
			}
			if err := s.UpdateLayouts(session); err != nil {
				slog.Error("Failed to update layouts in offline timer",
					slog.String("callsign", callsign),
					slog.Any("error", err))
			}
			if err := s.UpdateRoutesForSession(session, true); err != nil {
				slog.Error("Failed to update routes in offline timer",
					slog.String("callsign", callsign),
					slog.Any("error", err))
			}
		}

		hub.offlineMu.Lock()
		delete(hub.offlineTimers, key)
		hub.offlineMu.Unlock()

		// Phase 2: 5-second delay before broadcast notification
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		msg := buildOfflineBroadcastMessage(positionName, changes)
		slog.Info("Sending offline broadcast message",
			slog.String("position", positionName),
			slog.String("callsign", callsign),
			slog.String("message", msg),
			slog.Int("session", int(session)))
		s.GetFrontendHub().SendBroadcast(session, msg, "SYSTEM")
	}()
}

// cancelOfflineTimer cancels a pending offline timer for the given position.
// Returns true if a timer was found and cancelled, false if none was pending.
func (hub *Hub) cancelOfflineTimer(session int32, positionName string) bool {
	key := fmt.Sprintf("%d:%s", session, positionName)
	hub.offlineMu.Lock()
	defer hub.offlineMu.Unlock()
	if cancel, ok := hub.offlineTimers[key]; ok {
		cancel()
		delete(hub.offlineTimers, key)
		slog.Debug("Offline timer cancelled — position came back online",
			slog.String("position", positionName),
			slog.Int("session", int(session)))
		return true
	}
	return false
}

// scheduleOnlineBroadcast fires a broadcast notification 5 seconds after a position
// first comes online, giving the sector update enough time to propagate to all
// clients before the message arrives.
func (hub *Hub) scheduleOnlineBroadcast(session int32, positionName string, changes []shared.SectorChange) {
	go func() {
		time.Sleep(5 * time.Second)
		msg := buildOnlineBroadcastMessage(positionName, changes)
		slog.Info("Sending online broadcast message",
			slog.String("position", positionName),
			slog.String("message", msg),
			slog.Int("session", int(session)))
		hub.server.GetFrontendHub().SendBroadcast(session, msg, "SYSTEM")
	}()
}

// buildOnlineBroadcastMessage constructs the human-readable broadcast message for a
// position coming online, listing each sector that transferred responsibility.
func buildOnlineBroadcastMessage(positionName string, changes []shared.SectorChange) string {
	if len(changes) == 0 {
		return fmt.Sprintf("%s is now online.", positionName)
	}
	if len(changes) == 1 {
		c := changes[0]
		if c.FromPosition == "" {
			return fmt.Sprintf("%s is now online. Sector %s now has coverage.", positionName, c.SectorName)
		}
		return fmt.Sprintf("%s is now online. Sector %s transferred from %s.", positionName, c.SectorName, c.FromPosition)
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		if c.FromPosition == "" {
			parts[i] = fmt.Sprintf("%s (no previous coverage)", c.SectorName)
		} else {
			parts[i] = fmt.Sprintf("%s (from %s)", c.SectorName, c.FromPosition)
		}
	}
	return fmt.Sprintf("%s is now online. Sectors: %s.", positionName, strings.Join(parts, ", "))
}

// buildOfflineBroadcastMessage constructs the human-readable broadcast message for a
// position going offline, listing each sector that transferred responsibility.
func buildOfflineBroadcastMessage(positionName string, changes []shared.SectorChange) string {
	if len(changes) == 0 {
		return fmt.Sprintf("%s went offline.", positionName)
	}
	if len(changes) == 1 {
		c := changes[0]
		if c.ToPosition == "" {
			return fmt.Sprintf("%s went offline. Sector %s has no coverage.", positionName, c.SectorName)
		}
		return fmt.Sprintf("%s went offline. Sector %s transferred to %s.", positionName, c.SectorName, c.ToPosition)
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		if c.ToPosition == "" {
			parts[i] = fmt.Sprintf("%s (no coverage)", c.SectorName)
		} else {
			parts[i] = fmt.Sprintf("%s (to %s)", c.SectorName, c.ToPosition)
		}
	}
	return fmt.Sprintf("%s went offline. Sectors: %s.", positionName, strings.Join(parts, ", "))
}

// RecordEvent records an event if recording is enabled for the session
func (hub *Hub) RecordEvent(sessionID int32, eventType string, payload interface{}) error {
	if rec, ok := hub.recorders[sessionID]; ok {
		return rec.RecordEvent(eventType, payload)
	}
	return nil // Not recording, no error
}

// StartRecording starts recording for a session
func (hub *Hub) StartRecording(sessionID int32, airport, connection, description string) error {
	if _, ok := hub.recorders[sessionID]; ok {
		return fmt.Errorf("recording already active for session %d", sessionID)
	}

	rec := recorder.NewRecorder(airport, connection, description)
	rec.Start()
	hub.recorders[sessionID] = rec

	slog.Info("Started recording", slog.Int("session", int(sessionID)), slog.String("airport", airport))
	return nil
}

// StopRecording stops recording for a session
func (hub *Hub) StopRecording(sessionID int32) error {
	rec, ok := hub.recorders[sessionID]
	if !ok {
		return fmt.Errorf("no active recording for session %d", sessionID)
	}

	if err := rec.Stop(); err != nil {
		return fmt.Errorf("failed to stop recording: %w", err)
	}

	delete(hub.recorders, sessionID)
	slog.Info("Stopped recording", slog.Int("session", int(sessionID)), slog.String("path", rec.GetOutputPath()))
	return nil
}

// IsRecording returns true if the session is being recorded
func (hub *Hub) IsRecording(sessionID int32) bool {
	_, ok := hub.recorders[sessionID]
	return ok
}
