package euroscope

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/metrics"
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
	"slices"
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

// offlineTimerEntry holds the cancel function and metadata for a pending position offline timer.
type offlineTimerEntry struct {
	cancel       context.CancelFunc
	session      int32
	callsign     string
	positionFreq string
	positionName string
}

// aircraftDisconnectEntry holds the cancel function and metadata for a pending aircraft disconnect timer.
type aircraftDisconnectEntry struct {
	cancel   context.CancelFunc
	session  int32
	callsign string
}

// sessionUpdatePending batches UpdateSectors/UpdateLayouts/UpdateRoutes calls
// that would otherwise fire concurrently from multiple offline timers.
type sessionUpdatePending struct {
	timer     *time.Timer
	positions []string // offline position names gathered in this debounce window
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
	offlineTimers map[string]*offlineTimerEntry // key: "<sessionID>:<positionName>"

	// Aircraft disconnect timer support — delays strip removal to survive master transitions
	aircraftDisconnectMu     sync.Mutex
	aircraftDisconnectTimers map[string]*aircraftDisconnectEntry // key: "<sessionID>:<callsign>"

	// Session update debouncer — batches UpdateSectors/UpdateLayouts/UpdateRoutes calls
	// so concurrent offline timers produce a single recalculation per session.
	sessionUpdateMu     sync.Mutex
	sessionUpdateTimers map[int32]*sessionUpdatePending

	squawkThrottle *squawkThrottle
}

func NewHub(stripService shared.StripService, controllerService shared.ControllerService, authenticationService shared.AuthenticationService) *Hub {
	handlers := shared.NewMessageHandlers[euroscope.EventType, *Client]()

	handlers.Add(euroscope.Login, handleLoginEvent)
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
	handlers.Add(euroscope.CdmTobtUpdate, handleCdmTobtUpdate)
	handlers.Add(euroscope.CdmDeiceUpdate, handleCdmDeiceUpdate)
	handlers.Add(euroscope.CdmManualCtot, handleCdmManualCtot)
	handlers.Add(euroscope.CdmCtotRemove, handleCdmCtotRemove)
	handlers.Add(euroscope.CdmApproveReqTobt, handleCdmApproveReqTobt)
	handlers.Add(euroscope.CdmAsrtToggle, handleCdmAsrtToggle)
	handlers.Add(euroscope.CdmTsacUpdate, handleCdmTsacUpdate)
	handlers.Add(euroscope.TrackingControllerChanged, handleTrackingControllerChanged)
	handlers.Add(euroscope.CoordinationReceived, handleCoordinationReceived)
	handlers.Add(euroscope.CdmMasterToggle, handleCdmMasterToggle)
	handlers.Add(euroscope.IssuePdcClearance, handleIssuePdcClearance)
	handlers.Add(euroscope.PdcRevertToVoice, handlePdcRevertToVoice)

	hub := &Hub{
		register:                 make(chan *Client),
		unregister:               make(chan *Client),
		clients:                  make(map[*Client]bool),
		send:                     make(chan internalMessage),
		master:                   make(map[int32]*Client),
		handlers:                 handlers,
		stripService:             stripService,
		controllerService:        controllerService,
		authenticationService:    authenticationService,
		recorders:                make(map[int32]*recorder.Recorder),
		offlineTimers:            make(map[string]*offlineTimerEntry),
		aircraftDisconnectTimers: make(map[string]*aircraftDisconnectEntry),
		sessionUpdateTimers:      make(map[int32]*sessionUpdatePending),
		airportClientCount:       make(map[string]int),
	}
	hub.squawkThrottle = newSquawkThrottle(defaultSquawkRequestInterval, hub.readAssignedSquawk, hub.dispatchGenerateSquawkRequest)

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
	metrics.ConnectionOpened(context.Background(), client.session, "euroscope")
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
		if strip.PdcRequestRemarks != nil {
			entry.PdcRequestRemarks = *strip.PdcRequestRemarks
		}
		if strip.CdmData != nil {
			entry.Cdm = euroscope.BackendSyncCdmData{
				Eobt:            valueOrEmpty(strip.CdmData.EffectiveEobt()),
				Tobt:            valueOrEmpty(strip.CdmData.EffectiveTobt()),
				TobtSetBy:       valueOrEmpty(strip.CdmData.TobtSetBy),
				TobtConfirmedBy: valueOrEmpty(strip.CdmData.TobtConfirmedBy),
				ReqTobt:         valueOrEmpty(strip.CdmData.EffectiveReqTobt()),
				Tsat:            truncateCDMClockValue(valueOrEmpty(strip.CdmData.EffectiveTsat())),
				Ttot:            truncateCDMClockValue(valueOrEmpty(strip.CdmData.EffectiveTtot())),
				Ctot:            valueOrEmpty(strip.CdmData.EffectiveCtot()),
				CtotSource:      valueOrEmpty(strip.CdmData.CtotSource),
				Asat:            valueOrEmpty(strip.CdmData.EffectiveAsat()),
				Asrt:            valueOrEmpty(strip.CdmData.Asrt),
				Tsac:            valueOrEmpty(strip.CdmData.Tsac),
				Status:          valueOrEmpty(strip.CdmData.EffectiveStatus()),
				EcfmpID:         valueOrEmpty(strip.CdmData.EcfmpID),
				Phase:           valueOrEmpty(strip.CdmData.EffectivePhase()),
			}
		}
		if strip.PdcState != "" {
			entry.PdcState = strip.PdcState
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

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func truncateCDMClockValue(value string) string {
	if len(value) > 4 {
		return value[:4]
	}
	return value
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

	// Recalculate layouts since the login is sent both on first logon and when a
	// position/frequency changes - the new position must be reflected immediately.
	if layoutErr := hub.server.UpdateLayouts(sessionID); layoutErr != nil {
		slog.Error("Failed to update layouts after ES login", slog.String("cid", user.GetCid()), slog.Any("error", layoutErr))
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

	sessionName := strings.ToUpper(strings.TrimSpace(event.Connection))
	if sessionName == "" {
		sessionName = "LIVE"
	}
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
	metrics.ConnectionClosed(context.Background(), client.session, "euroscope")
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

	// Extend pending offline and aircraft-disconnect timers so the new master has
	// time to send a SyncEvent that cancels them before they fire.
	hub.extendSessionTimers(client.session)
}

func (hub *Hub) SendGenerateSquawk(session int32, cid string, callsign string) {
	hub.squawkThrottle.Enqueue(session, queuedSquawkRequest{
		cid:      cid,
		callsign: callsign,
	})
}

func (hub *Hub) resolveGenerateSquawkCid(ctx context.Context, session int32) string {
	if hub.server != nil {
		sectorRepo := hub.server.GetSectorOwnerRepository()
		controllerRepo := hub.server.GetControllerRepository()
		if sectorRepo != nil && controllerRepo != nil {
			owners, err := sectorRepo.ListBySession(ctx, session)
			if err != nil {
				slog.Warn("Failed to load sector owners for squawk generation",
					slog.Int("session", int(session)),
					slog.Any("error", err),
				)
			} else {
				for _, owner := range owners {
					if !slices.Contains(owner.Sector, "DEL") {
						continue
					}

					controllers, err := controllerRepo.GetByPosition(ctx, session, owner.Position)
					if err != nil {
						slog.Warn("Failed to load DEL controllers for squawk generation",
							slog.Int("session", int(session)),
							slog.String("position", owner.Position),
							slog.Any("error", err),
						)
						continue
					}

					for _, controller := range controllers {
						if controller.Cid != nil && *controller.Cid != "" {
							return *controller.Cid
						}
					}
				}
			}
		}
	}

	if master, ok := hub.master[session]; ok && master != nil {
		return master.GetCid()
	}

	return ""
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

func (hub *Hub) SendAssumeOnly(session int32, cid string, callsign string) {
	hub.Send(session, cid, euroscope.AssumeOnlyEvent{Callsign: callsign})
}

func (hub *Hub) SendDropTracking(session int32, cid string, callsign string) {
	hub.Send(session, cid, euroscope.DropTrackingEvent{Callsign: callsign})
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

func (hub *Hub) SendPdcStateChange(session int32, callsign, state, remarks string) {
	hub.Broadcast(session, euroscope.PdcStateChangeEvent{
		Callsign:          callsign,
		State:             state,
		PdcRequestRemarks: remarks,
	})
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

const (
	offlineGracePeriod        = 60 * time.Second
	masterTransferGracePeriod = 45 * time.Second
)

// scheduleOfflineActions starts a goroutine that, after the given grace period,
// deletes the controller from the database, notifies all frontend clients that the
// controller is offline, recalculates sector ownership, and — 5 seconds later —
// broadcasts a specific sector-change notification.
//
// If cancelOfflineTimer is called for the same position key before the grace period
// elapses, the goroutine exits cleanly and none of the above happens.
func (hub *Hub) scheduleOfflineActions(session int32, callsign, positionFreq, positionName string, delay time.Duration) {
	key := fmt.Sprintf("%d:%s", session, positionName)

	ctx, cancel := context.WithCancel(context.Background())

	hub.offlineMu.Lock()
	if existing, ok := hub.offlineTimers[key]; ok {
		existing.cancel()
	}
	hub.offlineTimers[key] = &offlineTimerEntry{
		cancel:       cancel,
		session:      session,
		callsign:     callsign,
		positionFreq: positionFreq,
		positionName: positionName,
	}
	hub.offlineMu.Unlock()

	go func() {
		// Phase 1: grace period
		select {
		case <-ctx.Done():
			slog.Debug("Controller offline timer cancelled (position came back online)",
				slog.String("position", positionName),
				slog.String("callsign", callsign),
				slog.Int("session", int(session)))
			return
		case <-time.After(delay):
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

		hub.offlineMu.Lock()
		delete(hub.offlineTimers, key)
		hub.offlineMu.Unlock()

		// Signal the per-session debouncer to recalculate sectors/layouts/routes.
		// Multiple concurrent offline timers collapse into a single update run.
		hub.scheduleSessionUpdate(session, positionName)
	}()
}

// cancelOfflineTimer cancels a pending offline timer for the given position.
// Returns true if a timer was found and cancelled, false if none was pending.
func (hub *Hub) cancelOfflineTimer(session int32, positionName string) bool {
	key := fmt.Sprintf("%d:%s", session, positionName)
	hub.offlineMu.Lock()
	defer hub.offlineMu.Unlock()
	if entry, ok := hub.offlineTimers[key]; ok {
		entry.cancel()
		delete(hub.offlineTimers, key)
		slog.Debug("Offline timer cancelled — position came back online",
			slog.String("position", positionName),
			slog.Int("session", int(session)))
		return true
	}
	return false
}

// scheduleAircraftDisconnect starts a goroutine that removes the strip after the given
// delay, unless cancelAircraftDisconnect is called first (e.g. because the new master's
// SyncEvent confirms the aircraft is still alive).
func (hub *Hub) scheduleAircraftDisconnect(session int32, callsign string, delay time.Duration) {
	key := fmt.Sprintf("%d:%s", session, callsign)
	ctx, cancel := context.WithCancel(context.Background())

	hub.aircraftDisconnectMu.Lock()
	if existing, ok := hub.aircraftDisconnectTimers[key]; ok {
		existing.cancel()
	}
	hub.aircraftDisconnectTimers[key] = &aircraftDisconnectEntry{cancel: cancel, session: session, callsign: callsign}
	hub.aircraftDisconnectMu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			slog.Debug("Aircraft disconnect timer cancelled (aircraft still alive)",
				slog.String("callsign", callsign),
				slog.Int("session", int(session)))
			return
		case <-time.After(delay):
		}

		hub.aircraftDisconnectMu.Lock()
		delete(hub.aircraftDisconnectTimers, key)
		hub.aircraftDisconnectMu.Unlock()

		slog.Debug("Aircraft disconnected, removing strip",
			slog.String("callsign", callsign),
			slog.Int("session", int(session)))
		if err := hub.stripService.DeleteStrip(context.Background(), session, callsign); err != nil {
			slog.Error("Failed to delete strip in aircraft disconnect timer",
				slog.String("callsign", callsign),
				slog.Any("error", err))
		}
	}()
}

// cancelAircraftDisconnect cancels a pending aircraft disconnect timer.
func (hub *Hub) cancelAircraftDisconnect(session int32, callsign string) {
	key := fmt.Sprintf("%d:%s", session, callsign)
	hub.aircraftDisconnectMu.Lock()
	defer hub.aircraftDisconnectMu.Unlock()
	if entry, ok := hub.aircraftDisconnectTimers[key]; ok {
		entry.cancel()
		delete(hub.aircraftDisconnectTimers, key)
	}
}

// extendSessionTimers extends all pending position offline timers and aircraft disconnect
// timers for the session to masterTransferGracePeriod. Called when the master role is
// transferred so that slaves still on VATSIM have time to sync and cancel the timers.
func (hub *Hub) extendSessionTimers(session int32) {
	hub.offlineMu.Lock()
	posEntries := make([]*offlineTimerEntry, 0)
	for _, e := range hub.offlineTimers {
		if e.session == session {
			posEntries = append(posEntries, e)
		}
	}
	hub.offlineMu.Unlock()

	for _, e := range posEntries {
		hub.scheduleOfflineActions(e.session, e.callsign, e.positionFreq, e.positionName, masterTransferGracePeriod)
	}

	hub.aircraftDisconnectMu.Lock()
	acEntries := make([]*aircraftDisconnectEntry, 0)
	for _, e := range hub.aircraftDisconnectTimers {
		if e.session == session {
			acEntries = append(acEntries, e)
		}
	}
	hub.aircraftDisconnectMu.Unlock()

	for _, e := range acEntries {
		hub.scheduleAircraftDisconnect(e.session, e.callsign, masterTransferGracePeriod)
	}
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

// scheduleSessionUpdate debounces calls to UpdateSectors/UpdateLayouts/UpdateRoutes
// for a session. Multiple calls within the debounce window (300 ms) are collapsed
// into a single run. If new offline events arrive while the run is in progress, a
// second run is automatically scheduled.
//
// positionName is the human-readable position name that went offline; pass "" when
// triggering an update that is not tied to a specific position going offline.
func (hub *Hub) scheduleSessionUpdate(session int32, positionName string) {
	hub.sessionUpdateMu.Lock()
	defer hub.sessionUpdateMu.Unlock()

	if pending, ok := hub.sessionUpdateTimers[session]; ok {
		// Timer is still pending — append the position and reset the window.
		if positionName != "" {
			pending.positions = append(pending.positions, positionName)
		}
		pending.timer.Reset(300 * time.Millisecond)
		return
	}

	positions := make([]string, 0, 1)
	if positionName != "" {
		positions = append(positions, positionName)
	}
	pending := &sessionUpdatePending{positions: positions}
	pending.timer = time.AfterFunc(300*time.Millisecond, func() {
		hub.sessionUpdateMu.Lock()
		// Guard against the AfterFunc-Reset race: if this pending has already
		// been consumed or replaced, skip this run.
		cur, ok := hub.sessionUpdateTimers[session]
		if !ok || cur != pending {
			hub.sessionUpdateMu.Unlock()
			return
		}
		pos := pending.positions
		delete(hub.sessionUpdateTimers, session)
		hub.sessionUpdateMu.Unlock()

		hub.runSessionUpdate(session, pos)
	})
	hub.sessionUpdateTimers[session] = pending
}

// runSessionUpdate executes the combined UpdateSectors/UpdateLayouts/UpdateRoutes
// recalculation for a session and sends the broadcast notification.
// positions holds the names of any positions that went offline in this window.
func (hub *Hub) runSessionUpdate(session int32, positions []string) {
	s := hub.server
	changes, err := s.UpdateSectors(session)
	if err != nil {
		slog.Error("Failed to update sectors in session update",
			slog.Int("session", int(session)), slog.Any("error", err))
	}
	if err := s.UpdateLayouts(session); err != nil {
		slog.Error("Failed to update layouts in session update",
			slog.Int("session", int(session)), slog.Any("error", err))
	}
	if err := s.UpdateRoutesForSession(session, true); err != nil {
		slog.Error("Failed to update routes in session update",
			slog.Int("session", int(session)), slog.Any("error", err))
	}

	if len(positions) == 0 {
		return
	}

	// Broadcast after a short delay so clients have received the sector update first.
	go func() {
		time.Sleep(5 * time.Second)
		var msg string
		if len(positions) == 1 {
			msg = buildOfflineBroadcastMessage(positions[0], changes)
		} else {
			msg = buildMultipleOfflineBroadcastMessage(positions, changes)
		}
		slog.Info("Sending session update broadcast",
			slog.Int("session", int(session)),
			slog.String("message", msg))
		s.GetFrontendHub().SendBroadcast(session, msg, "SYSTEM")
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

// buildMultipleOfflineBroadcastMessage constructs a combined offline message when
// several positions went offline in the same debounce window.
func buildMultipleOfflineBroadcastMessage(positions []string, changes []shared.SectorChange) string {
	names := strings.Join(positions, ", ")
	if len(changes) == 0 {
		return fmt.Sprintf("%s went offline.", names)
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		if c.ToPosition == "" {
			parts[i] = fmt.Sprintf("%s (no coverage)", c.SectorName)
		} else {
			parts[i] = fmt.Sprintf("%s (to %s)", c.SectorName, c.ToPosition)
		}
	}
	return fmt.Sprintf("%s went offline. Sectors: %s.", names, strings.Join(parts, ", "))
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
