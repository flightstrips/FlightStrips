package frontend

import (
	"FlightStrips/internal/clx"
	"FlightStrips/internal/config"
	"FlightStrips/internal/dependencies"
	"FlightStrips/internal/metrics"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/rnav"
	internalServer "FlightStrips/internal/server"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/helpers"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
)

type internalMessage struct {
	session int32
	message frontend.OutgoingMessage
	cid     *string
}

const (
	hubSendQueueSize    = 256
	clientSendQueueSize = 256
)

type layoutUpdateMessage struct {
	session   int32
	layoutMap map[string]string
}

type cidOnlineMessage struct {
	session int32
	cid     string
}

type cidDisconnectMessage struct {
	cid string
}

type Hub struct {
	server                shared.Server
	stripService          shared.StripService
	validationService     validationStatusAcknowledger
	stripUpdateService    frontendStripUpdateUseCase
	pdcService            shared.PdcService
	authenticationService shared.AuthenticationService
	clients               map[*Client]bool

	send          chan internalMessage
	layoutUpdates chan layoutUpdateMessage

	register      chan *Client
	unregister    chan *Client
	cidOnline     chan cidOnlineMessage
	cidDisconnect chan cidDisconnectMessage

	handlers shared.MessageHandlers[frontend.EventType, *Client]

	msgMu      sync.Mutex
	msgCounter int64 // accessed atomically
	messages   map[int32][]frontend.MessageReceivedEvent

	metarMu          sync.RWMutex
	metarCache       map[int32]string // session ID → latest METAR string
	arrAtisCodeCache map[int32]string // session ID → latest arrival ATIS code
	depAtisCodeCache map[int32]string // session ID → latest departure ATIS code

	clxMu        sync.RWMutex
	clxOverrides map[int32]map[string]bool

	snapshotBuilder    *SnapshotBuilder
	standActionService *services.StandActionService
}

type nextDisplayComputer interface {
	ComputeNextDisplayForStripContext(ctx context.Context, strip *internalModels.Strip, sessionId int32) (*internalModels.NextDisplay, error)
}

type nextDisplayBatchComputer interface {
	ComputeNextDisplaysForStripsContext(ctx context.Context, strips []*internalModels.Strip, sessionId int32) error
}

type validationStatusAcknowledger interface {
	AcknowledgeValidationStatus(ctx context.Context, session int32, callsign string, activationKey string, requestingPosition string) error
	ReconcileStandAssignmentValidation(ctx context.Context, session int32, callsign string, blockedBy []string, conflictReason string) error
}

type HubDependencies struct {
	Strips         shared.StripService
	Authentication shared.AuthenticationService
}

func NewHub(deps HubDependencies) (*Hub, error) {
	if dependencies.IsNil(deps.Strips) {
		return nil, errors.New("frontend hub requires strip service")
	}
	if dependencies.IsNil(deps.Authentication) {
		return nil, errors.New("frontend hub requires authentication service")
	}

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
	handlers.Add(frontend.CoordinationForceAssumeRequestType, handleCoordinationForceAssumeRequest)
	handlers.Add(frontend.CoordinationTagRequestType, handleCoordinationTagRequest)
	handlers.Add(frontend.CoordinationAcceptTagRequestType, handleCoordinationAcceptTagRequest)
	handlers.Add(frontend.UpdateOrder, handleUpdateOrder)
	handlers.Add(frontend.SendMessage, handleSendMessage)
	handlers.Add(frontend.CdmReady, handleCdmReady)
	handlers.Add(frontend.ReleasePoint, handleReleasePoint)
	handlers.Add(frontend.StartReq, handleStartReq)
	handlers.Add(frontend.Marked, handleMarked)
	handlers.Add(frontend.RunwayClearance, handleRunwayClearance)
	handlers.Add(frontend.RunwayConfirmation, handleRunwayConfirmation)
	handlers.Add(frontend.AcknowledgeUnexpectedChange, handleAcknowledgeUnexpectedChange)
	handlers.Add(frontend.ActionCreateTacticalStrip, handleCreateTacticalStrip)
	handlers.Add(frontend.ActionDeleteTacticalStrip, handleDeleteTacticalStrip)
	handlers.Add(frontend.ActionConfirmTacticalStrip, handleConfirmTacticalStrip)
	handlers.Add(frontend.ActionStartTacticalTimer, handleStartTacticalTimer)
	handlers.Add(frontend.ActionMoveTacticalStrip, handleMoveTacticalStrip)
	handlers.Add(frontend.MissedApproachRequestType, handleMissedApproach)
	handlers.Add(frontend.ActionCreateManualFPL, handleCreateManualFPL)
	handlers.Add(frontend.SendPrivateMessage, handleSendPrivateMessage)
	handlers.Add(frontend.ActionCreateVFRFPL, handleCreateVFRFPL)
	handlers.Add(frontend.UpdateRunwayStatus, handleUpdateRunwayStatus)
	handlers.Add(frontend.AcknowledgeValidationStatus, handleAcknowledgeValidationStatus)
	handlers.Add(frontend.ClxOverrideValidation, handleClxOverrideValidation)
	handlers.Add(frontend.ClxUpdateTobt, handleClxUpdateTobt)

	hub := &Hub{
		send:                  make(chan internalMessage, hubSendQueueSize),
		layoutUpdates:         make(chan layoutUpdateMessage),
		register:              make(chan *Client),
		unregister:            make(chan *Client),
		cidOnline:             make(chan cidOnlineMessage),
		cidDisconnect:         make(chan cidDisconnectMessage),
		clients:               make(map[*Client]bool),
		handlers:              handlers,
		stripService:          deps.Strips,
		authenticationService: deps.Authentication,
		messages:              make(map[int32][]frontend.MessageReceivedEvent),
		metarCache:            make(map[int32]string),
		arrAtisCodeCache:      make(map[int32]string),
		depAtisCodeCache:      make(map[int32]string),
		clxOverrides:          make(map[int32]map[string]bool),
	}

	return hub, nil
}

func (hub *Hub) RegisterPDCHandlers(service shared.PdcService) error {
	if dependencies.IsNil(service) {
		return errors.New("frontend hub requires PDC service when registering PDC handlers")
	}
	hub.pdcService = service
	hub.handlers.Add(frontend.IssuePdcClearance, handleIssuePdcClearance)
	hub.handlers.Add(frontend.PdcManualStateChange, handlePdcManualStateChange)
	hub.handlers.Add(frontend.RevertToVoice, handleRevertToVoice)
	return nil
}

// SetStandActionService enables SAT commands only after SAT readiness has been
// established during application startup.
func (hub *Hub) SetStandActionService(service *services.StandActionService) {
	hub.standActionService = service
	if service == nil {
		return
	}
	hub.handlers.Add(frontend.ActionStandAutomaticRequest, handleStandAutomaticRequest)
	hub.handlers.Add(frontend.ActionStandManualRequest, handleStandManualRequest)
	hub.handlers.Add(frontend.ActionStandConfirmedOverride, handleStandConfirmedOverride)
	hub.handlers.Add(frontend.ActionStandAcknowledge, handleStandAcknowledge)
	hub.handlers.Add(frontend.ActionStandOccupy, handleStandOccupy)
	hub.handlers.Add(frontend.ActionStandVacate, handleStandVacate)
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
	hub.snapshotBuilder = nil
	hub.stripUpdateService = hub.newStripUpdateService()
}

func (hub *Hub) SetValidationService(validationService validationStatusAcknowledger) {
	hub.validationService = validationService
}

func (hub *Hub) getStripUpdateService() frontendStripUpdateUseCase {
	if hub.stripUpdateService != nil {
		return hub.stripUpdateService
	}
	hub.stripUpdateService = hub.newStripUpdateService()
	return hub.stripUpdateService
}

func (hub *Hub) newStripUpdateService() frontendStripUpdateUseCase {
	if hub.server == nil {
		return nil
	}

	var standUpdater frontendStripUpdateStandUpdater
	if hub.stripService != nil {
		standUpdater = hub.stripService
	}

	var pdcReevaluator pdcInvalidValidationStripReevaluator
	if reevaluator, ok := hub.stripService.(pdcInvalidValidationStripReevaluator); ok {
		pdcReevaluator = reevaluator
	}

	var departureReevaluator departureValidationStripReevaluator
	if reevaluator, ok := hub.stripService.(departureValidationStripReevaluator); ok {
		departureReevaluator = reevaluator
	}

	return NewFrontendStripUpdateService(
		hub.server.GetStripRepository(),
		hub.server.GetSessionRepository(),
		hub.server.GetEuroscopeHub(),
		hub.server.GetCdmService(),
		standUpdater,
		pdcReevaluator,
		departureReevaluator,
		hub,
	)
}

func (hub *Hub) HandleNewConnection(conn *gorilla.Conn, user shared.AuthenticatedUser, authenticationEvent events.AuthenticationEvent) (*Client, error) {
	controllerRepo := hub.server.GetControllerRepository()
	sessionRepo := hub.server.GetSessionRepository()

	controller, err := controllerRepo.GetByCid(context.Background(), user.GetCid())

	var session int32
	var sessionName, position, airport, callsign string
	readOnly := false
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}

		session = WaitingForEuroscopeConnectionSessionId
		sessionName = ""
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
		sessionName = dbSession.Name
		position = controller.Position
		airport = dbSession.Airport
		callsign = controller.Callsign
		if esHub := hub.server.GetEuroscopeHub(); esHub != nil {
			readOnly = esHub.IsObserverCid(user.GetCid())
		}
	}

	// If no EuroScope client is currently online for this airport, or the session has not yet
	// completed its first full sync, put the frontend client into the waiting state.
	// CidOnline will associate it with the session when ES connects and syncs.
	if airport != WaitingForEuroscopeConnectionAirport {
		esHub := hub.server.GetEuroscopeHub()
		if esHub != nil && (!esHub.HasActiveClientForAirport(airport) || !esHub.IsSessionSynced(session)) {
			slog.Info("No EuroScope client online or session not yet synced; frontend will wait",
				slog.String("cid", user.GetCid()),
				slog.String("airport", airport),
			)
			session = WaitingForEuroscopeConnectionSessionId
			sessionName = ""
			position = WaitingForEuroscopeConnectionPosition
			airport = WaitingForEuroscopeConnectionAirport
			callsign = WaitingForEuroscopeConnectionCallsign
		}
	}

	// Create and return the client
	client := &Client{
		conn:        conn,
		user:        user,
		session:     session,
		sessionName: sessionName,
		send:        make(chan events.OutgoingMessage, clientSendQueueSize),
		closed:      make(chan struct{}),
		hub:         hub,
		position:    position,
		airport:     airport,
		callsign:    callsign,
		version:     strings.TrimSpace(authenticationEvent.Version),
		readOnly:    readOnly,
	}

	hub.register <- client

	return client, nil
}

func (hub *Hub) sendInitialEvent(ctx context.Context, client *Client) {
	builder := hub.getSnapshotBuilder()

	event, cachedAtis, err := builder.Build(ctx, InitialSnapshotRequest{
		SessionID: client.session,
		Position:  client.position,
		Airport:   client.airport,
		Callsign:  client.callsign,
		UserCID:   client.user.GetCid(),
		ReadOnly:  client.readOnly,
	})
	if err != nil {
		slog.Error("Failed to build initial frontend snapshot", slog.Any("error", err), slog.Int("session", int(client.session)))
		return
	}

	client.Enqueue(event)
	if cachedAtis != nil {
		client.Enqueue(*cachedAtis)
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
	return MapStripToFrontendModelWithClx(strip, clx.Context{})
}

func MapStripToFrontendModelWithClx(strip *internalModels.Strip, clxContext clx.Context) frontend.Strip {
	cdm := strip.CdmData
	if cdm == nil {
		cdm = (&internalModels.CdmData{}).Normalize()
	}
	cdmEvent := shared.BuildFrontendCdmDataEvent(strip.Callsign, cdm)

	return frontend.Strip{
		Callsign:                 strip.Callsign,
		Origin:                   strip.Origin,
		Destination:              strip.Destination,
		Alternate:                helpers.ValueOrDefault(strip.Alternative),
		Route:                    helpers.ValueOrDefault(strip.Route),
		Remarks:                  helpers.ValueOrDefault(strip.Remarks),
		Runway:                   helpers.ValueOrDefault(strip.Runway),
		Squawk:                   helpers.ValueOrDefault(strip.Squawk),
		AssignedSquawk:           helpers.ValueOrDefault(strip.AssignedSquawk),
		Sid:                      helpers.ValueOrDefault(strip.Sid),
		Star:                     helpers.ValueOrDefault(strip.Star),
		ClearedAltitude:          helpers.ValueOrDefault(strip.ClearedAltitude),
		RequestedAltitude:        helpers.ValueOrDefault(strip.RequestedAltitude),
		Heading:                  helpers.ValueOrDefault(strip.Heading),
		PositionAltitude:         helpers.ValueOrDefault(strip.PositionAltitude),
		AircraftType:             helpers.ValueOrDefault(strip.AircraftType),
		AircraftCategory:         helpers.ValueOrDefault(strip.AircraftCategory),
		SpokenCallsign:           helpers.ValueOrDefault(strip.SpokenCallsign),
		Stand:                    helpers.ValueOrDefault(strip.Stand),
		Capabilities:             rnav.DeriveCapability(helpers.ValueOrDefault(strip.AircraftType), helpers.ValueOrDefault(strip.Remarks)),
		CommunicationType:        helpers.ValueOrDefault(strip.CommunicationType),
		Bay:                      strip.Bay,
		ReleasePoint:             helpers.ValueOrDefault(strip.ReleasePoint),
		Version:                  strip.Version,
		Sequence:                 helpers.ValueOrDefault(strip.Sequence),
		NextControllers:          strip.NextOwners,
		PreviousControllers:      strip.PreviousOwners,
		NextDisplay:              mapNextDisplayToDTO(strip.NextDisplay),
		Owner:                    helpers.ValueOrDefault(strip.Owner),
		Eobt:                     cdmEvent.Eobt,
		Tobt:                     cdmEvent.Tobt,
		ReqTobt:                  cdmEvent.ReqTobt,
		ReqTobtType:              cdmEvent.ReqTobtType,
		TobtSetBy:                cdmEvent.TobtSetBy,
		Tsat:                     cdmEvent.Tsat,
		Ttot:                     cdmEvent.Ttot,
		Ctot:                     cdmEvent.Ctot,
		Aobt:                     cdmEvent.Aobt,
		Asat:                     cdmEvent.Asat,
		Asrt:                     cdmEvent.Asrt,
		Tsac:                     cdmEvent.Tsac,
		Status:                   cdmEvent.Status,
		MostPenalizingAirspace:   cdmEvent.MostPenalizingAirspace,
		EcfmpID:                  cdmEvent.EcfmpID,
		CtotSource:               cdmEvent.CtotSource,
		Phase:                    cdmEvent.Phase,
		EcfmpRestrictions:        cdmEvent.EcfmpRestrictions,
		PdcState:                 strip.PdcState,
		PdcRequestRemarks:        helpers.ValueOrDefault(strip.PdcRequestRemarks),
		StartReq:                 strip.StartReq,
		Marked:                   strip.Marked,
		Registration:             helpers.ValueOrDefault(strip.Registration),
		TrackingController:       strip.TrackingController,
		RunwayCleared:            strip.RunwayCleared,
		RunwayConfirmed:          strip.RunwayConfirmed,
		Aldt:                     truncateFrontendClockValue(helpers.ValueOrDefault(strip.EffectiveAldt())),
		UnexpectedChangeFields:   strip.UnexpectedChangeFields,
		ControllerModifiedFields: strip.ControllerModifiedFields,
		IsManual:                 strip.IsManual,
		PersonsOnBoard:           helpers.ValueOrDefault(strip.PersonsOnBoard),
		FplType:                  helpers.ValueOrDefault(strip.FplType),
		Language:                 helpers.ValueOrDefault(strip.Language),
		HasFP:                    strip.HasFP,
		ValidationStatus:         mapValidationStatusToDTO(strip.ValidationStatus),
		ClxValidation:            mapClxValidationToDTO(clx.Validate(strip, clxContext)),
	}
}

func mapClxValidationToDTO(validation *clx.Validation) *frontend.ClxValidation {
	if validation == nil || len(validation.Faults) == 0 {
		return nil
	}
	faults := make([]frontend.ClxValidationFault, 0, len(validation.Faults))
	for _, fault := range validation.Faults {
		faults = append(faults, frontend.ClxValidationFault{
			Code:        fault.Code,
			Message:     fault.Message,
			NitosRemark: fault.NitosRemark,
			Fields:      append([]string(nil), fault.Fields...),
			OverrideKey: fault.OverrideKey,
		})
	}
	return &frontend.ClxValidation{Faults: faults}
}

func mapValidationStatusToDTO(vs *internalModels.ValidationStatus) *frontend.ValidationStatus {
	if vs == nil {
		return nil
	}
	dto := &frontend.ValidationStatus{
		IssueType:      vs.IssueType,
		Message:        vs.Message,
		OwningPosition: vs.OwningPosition,
		Active:         vs.Active,
		ActivationKey:  vs.ActivationKey,
	}
	if vs.CustomAction != nil {
		dto.CustomAction = &frontend.ValidationAction{
			Label:      vs.CustomAction.Label,
			ActionKind: vs.CustomAction.ActionKind,
			Payload:    vs.CustomAction.Payload,
		}
	}
	return dto
}

func truncateFrontendClockValue(value string) string {
	if len(value) > 4 {
		return value[:4]
	}
	return value
}

func (hub *Hub) CidOnline(session int32, cid string) {
	hub.cidOnline <- cidOnlineMessage{session: session, cid: cid}
}

func (hub *Hub) CidDisconnect(cid string) {
	hub.cidDisconnect <- cidDisconnectMessage{cid: cid}
}

func (hub *Hub) associateCidOnlineClients(msg cidOnlineMessage) []*Client {
	controllerRepo := hub.server.GetControllerRepository()
	sessionRepo := hub.server.GetSessionRepository()

	var controller *internalModels.Controller
	var dbSession *internalModels.Session

	if loadedController, err := controllerRepo.GetByCid(context.Background(), msg.cid); err == nil {
		controller = loadedController
		if loadedSession, err := sessionRepo.GetByID(context.Background(), controller.Session); err == nil {
			dbSession = loadedSession
		} else {
			slog.Error("Failed to get session for CID online client",
				slog.String("cid", msg.cid), slog.Any("error", err))
		}
	} else {
		slog.Error("Failed to get controller for CID online client",
			slog.String("cid", msg.cid), slog.Any("error", err))
	}

	readOnly := false
	if esHub := hub.server.GetEuroscopeHub(); esHub != nil {
		readOnly = esHub.IsObserverCid(msg.cid)
	}

	initialClients := make([]*Client, 0)
	for client := range hub.clients {
		if client.user.GetCid() != msg.cid {
			continue
		}

		oldSession := client.session
		oldSessionName := client.sessionName
		oldAirport := client.airport
		oldCallsign := client.callsign
		wasWaiting := oldSession == WaitingForEuroscopeConnectionSessionId

		slog.Debug("Associating frontend client with session",
			slog.String("cid", msg.cid),
			slog.Int("session", int(msg.session)))
		client.session = msg.session
		client.readOnly = readOnly

		// Always refresh callsign, position, and airport from DB so that
		// sendInitialEvent and LayoutUpdateEvent routing always use the most
		// current values. Without this, a controller who changed position in
		// EuroScope while their browser tab was open would receive layout
		// updates keyed to their old position.
		if controller != nil && dbSession != nil {
			client.callsign = controller.Callsign
			client.position = controller.Position
			client.airport = dbSession.Airport
			client.sessionName = dbSession.Name
		}

		switch {
		case oldSession == WaitingForEuroscopeConnectionSessionId && client.sessionName != "":
			metrics.ConnectionClosed(context.Background(), "", "", "frontend", "", client.version)
			metrics.ConnectionOpened(context.Background(), client.sessionName, client.airport, "frontend", client.callsign, client.version)
		case oldSession != WaitingForEuroscopeConnectionSessionId &&
			(oldSessionName != client.sessionName || oldAirport != client.airport || oldCallsign != client.callsign):
			metrics.ConnectionClosed(context.Background(), oldSessionName, oldAirport, "frontend", oldCallsign, client.version)
			metrics.ConnectionOpened(context.Background(), client.sessionName, client.airport, "frontend", client.callsign, client.version)
		}

		if wasWaiting || client.readOnly {
			initialClients = append(initialClients, client)
		}
	}

	return initialClients
}

func (hub *Hub) handleCidOnline(msg cidOnlineMessage) {
	for _, client := range hub.associateCidOnlineClients(msg) {
		hub.sendInitialEvent(context.Background(), client)
	}
}

func (hub *Hub) handleCidDisconnect(cid string) {
	for client := range hub.clients {
		if client.user.GetCid() == cid {
			readOnly := false
			if esHub := hub.server.GetEuroscopeHub(); esHub != nil {
				readOnly = esHub.IsObserverCid(cid)
			}
			if client.session != WaitingForEuroscopeConnectionSessionId &&
				(client.sessionName != "" || client.callsign != "" || client.airport != "") {
				metrics.ConnectionClosed(context.Background(), client.sessionName, client.airport, "frontend", client.callsign, client.version)
				metrics.ConnectionOpened(context.Background(), "", "", "frontend", "", client.version)
			}
			client.session = WaitingForEuroscopeConnectionSessionId
			client.sessionName = ""
			client.position = WaitingForEuroscopeConnectionPosition
			client.airport = WaitingForEuroscopeConnectionAirport
			client.callsign = WaitingForEuroscopeConnectionCallsign
			client.Enqueue(frontend.DisconnectEvent{ReadOnly: readOnly})
		}
	}
}

func (hub *Hub) SendStripUpdate(session int32, callsign string) {
	stripRepo := hub.server.GetStripRepository()
	strip, err := stripRepo.GetByCallsign(context.Background(), session, callsign)
	if err != nil {
		return
	}

	hub.populateNextDisplay(strip, session)
	model := MapStripToFrontendModelWithClx(strip, hub.makeClxValidationContext(session))
	if repo := hub.server.GetStandAssignmentRepository(); repo != nil {
		if assignment, assignmentErr := repo.GetAssignment(context.Background(), session, callsign); assignmentErr == nil && assignment != nil {
			entry := mapStandAssignmentEntry(assignment)
			all, _ := repo.ListAssignments(context.Background(), session)
			entries := make([]frontend.StandAssignmentEntry, 0, len(all))
			for _, item := range all {
				if item != nil {
					entries = append(entries, mapStandAssignmentEntry(item))
				}
			}
			enrichStandAssignmentBlocking(entries, clientAirport(strip, assignment.Direction))
			for _, candidate := range entries {
				if candidate.Callsign == callsign {
					entry = candidate
					break
				}
			}
			model.StandAssignment = &entry
		}
	}

	event := frontend.StripUpdateEvent{
		Strip: model,
	}

	hub.Broadcast(session, event)
}

func (hub *Hub) makeClxValidationContext(session int32) clx.Context {
	return clx.Context{
		Now:       time.Now().UTC(),
		Overrides: hub.clxOverrideSnapshot(session),
		Rules:     config.GetClxValidationConfig(),
	}
}

func (hub *Hub) clxOverrideSnapshot(session int32) map[string]bool {
	hub.clxMu.RLock()
	defer hub.clxMu.RUnlock()

	source := hub.clxOverrides[session]
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]bool, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (hub *Hub) setClxOverride(session int32, overrideKey string) {
	if overrideKey == "" {
		return
	}
	hub.clxMu.Lock()
	defer hub.clxMu.Unlock()

	if hub.clxOverrides[session] == nil {
		hub.clxOverrides[session] = make(map[string]bool)
	}
	hub.clxOverrides[session][overrideKey] = true
}

func (hub *Hub) SendControllerOnline(session int32, callsign string, position string, identifier string, ownedSectors []string) {
	hub.broadcastController(session, frontend.ControllerOnlineEvent{
		Controller: hub.controllerPayload(session, callsign, position, identifier, ownedSectors),
	})
}

func (hub *Hub) SendControllerUpdate(session int32, callsign string, position string, identifier string, ownedSectors []string) {
	hub.broadcastController(session, frontend.ControllerUpdateEvent{
		Controller: hub.controllerPayload(session, callsign, position, identifier, ownedSectors),
	})
}

func (hub *Hub) controllerPayload(session int32, callsign string, position string, identifier string, ownedSectors []string) frontend.Controller {
	payload := frontend.Controller{
		Callsign:     callsign,
		Position:     position,
		Identifier:   identifier,
		OwnedSectors: slices.Clone(ownedSectors),
	}

	if pos, err := config.GetPositionBasedOnFrequency(position); err == nil {
		payload.Section = pos.Section
	}

	if (payload.Identifier != "" || len(payload.OwnedSectors) > 0) || hub.server == nil {
		return payload
	}

	sectorRepo := hub.server.GetSectorOwnerRepository()
	if sectorRepo == nil {
		return payload
	}

	sectors, err := sectorRepo.ListBySession(context.Background(), session)
	if err != nil {
		slog.Warn("Failed to load sector ownership for controller payload",
			slog.String("callsign", callsign),
			slog.Int("session", int(session)),
			slog.Any("error", err))
		return payload
	}

	sectorsMap := make(map[string]*internalModels.SectorOwner, len(sectors))
	for _, sector := range sectors {
		if sector == nil {
			continue
		}
		sectorsMap[sector.Position] = sector
	}

	enriched := buildFrontendController(callsign, position, sectorsMap)
	if payload.Identifier == "" {
		payload.Identifier = enriched.Identifier
	}
	if len(payload.OwnedSectors) == 0 {
		payload.OwnedSectors = enriched.OwnedSectors
	}
	if payload.Section == "" {
		payload.Section = enriched.Section
	}

	return payload
}

func (hub *Hub) broadcastController(session int32, event frontend.OutgoingMessage) {
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

func (hub *Hub) SendBulkBayEvent(session int32, bay string, strips []frontend.BulkBayEntry) {
	hub.Broadcast(session, frontend.BulkBayEvent{Bay: bay, Strips: strips})
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

func (hub *Hub) SendStandAssignmentBroadcast(session int32, entry frontend.StandAssignmentEntry) {
	hub.Broadcast(session, frontend.StandAssignmentUpdateEvent{
		Assignment: entry,
	})
}

func (hub *Hub) SendStandAssignmentRemoved(session int32, callsign string) {
	hub.Broadcast(session, frontend.StandAssignmentRemovedEvent{Callsign: callsign})
}

func (hub *Hub) enrichedStandAssignmentEntry(sessionID int32, assignment *internalModels.StandAssignment) frontend.StandAssignmentEntry {
	entry := mapStandAssignmentEntry(assignment)
	session, err := hub.server.GetSessionRepository().GetByID(context.Background(), sessionID)
	if err != nil {
		return entry
	}
	all, err := hub.server.GetStandAssignmentRepository().ListAssignments(context.Background(), sessionID)
	if err != nil {
		return entry
	}
	entries := make([]frontend.StandAssignmentEntry, 0, len(all))
	for _, item := range all {
		if item != nil {
			entries = append(entries, mapStandAssignmentEntry(item))
		}
	}
	enrichStandAssignmentBlocking(entries, session.Airport)
	for _, candidate := range entries {
		if strings.EqualFold(candidate.Callsign, assignment.Callsign) {
			return candidate
		}
	}
	return entry
}

func (hub *Hub) PublishStandAllocation(ctx context.Context, result services.StandAllocationResult) error {
	sessionID := result.Assignment.SessionID
	publishEntries := []frontend.StandAssignmentEntry{}
	if !result.Removed {
		publishEntries = append(publishEntries, mapStandAssignmentEntry(&result.Assignment))
	}
	blockEntries := []frontend.StandBlockEntry{}
	assignmentStatusReady := false
	snapshotReady := false

	session, err := hub.server.GetSessionRepository().GetByID(ctx, sessionID)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to enrich committed stand allocation publication", slog.Int("session", int(sessionID)), slog.Any("error", err))
	} else if repo := hub.server.GetStandAssignmentRepository(); repo != nil {
		all, assignmentsErr := repo.ListAssignments(ctx, sessionID)
		if assignmentsErr != nil {
			slog.ErrorContext(ctx, "Failed to list committed stand assignments for publication", slog.Int("session", int(sessionID)), slog.Any("error", assignmentsErr))
		} else {
			publishEntries = make([]frontend.StandAssignmentEntry, 0, len(all))
			for _, item := range all {
				if item != nil {
					publishEntries = append(publishEntries, mapStandAssignmentEntry(item))
				}
			}
			enrichStandAssignmentBlocking(publishEntries, session.Airport)
			assignmentStatusReady = true
		}

		blocks, blocksErr := repo.ListBlocks(ctx, sessionID)
		if blocksErr != nil {
			slog.ErrorContext(ctx, "Failed to list committed stand blocks for publication", slog.Int("session", int(sessionID)), slog.Any("error", blocksErr))
		} else {
			blockEntries = make([]frontend.StandBlockEntry, 0, len(blocks))
			for _, block := range blocks {
				if block != nil {
					blockEntries = append(blockEntries, mapStandBlockEntry(block, session.Airport))
				}
			}
			snapshotReady = assignmentStatusReady
		}
	}

	for _, published := range publishEntries {
		conflictReason := ""
		if published.ConflictReason != nil {
			conflictReason = *published.ConflictReason
		}
		if assignmentStatusReady && hub.validationService != nil {
			if err := hub.validationService.ReconcileStandAssignmentValidation(ctx, result.Assignment.SessionID, published.Callsign, published.BlockedBy, conflictReason); err != nil {
				slog.ErrorContext(ctx, "Failed to reconcile stand assignment validation", slog.String("callsign", published.Callsign), slog.Any("error", err))
			}
		}
		hub.SendStandAssignmentBroadcast(sessionID, published)
	}

	removed := make([]internalModels.StandAssignment, 0, len(result.RemovedAssignments)+1)
	seenRemoved := map[string]struct{}{}
	addRemoved := func(assignment internalModels.StandAssignment) {
		key := strings.ToUpper(strings.TrimSpace(assignment.Callsign))
		if key == "" {
			return
		}
		if _, exists := seenRemoved[key]; exists {
			return
		}
		seenRemoved[key] = struct{}{}
		removed = append(removed, assignment)
	}
	if result.Removed {
		addRemoved(result.Assignment)
	}
	for _, assignment := range result.RemovedAssignments {
		addRemoved(assignment)
	}
	for _, assignment := range removed {
		if hub.validationService != nil {
			if err := hub.validationService.ReconcileStandAssignmentValidation(ctx, sessionID, assignment.Callsign, nil, ""); err != nil {
				slog.ErrorContext(ctx, "Failed to clear removed stand assignment validation", slog.String("callsign", assignment.Callsign), slog.Any("error", err))
			}
		}
		hub.SendStandAssignmentRemoved(sessionID, assignment.Callsign)
	}

	if snapshotReady {
		hub.SendStandStatusSnapshot(sessionID, publishEntries, blockEntries)
	}

	euroscopeHub := hub.server.GetEuroscopeHub()
	for _, assignment := range removed {
		hub.SendStandEvent(sessionID, assignment.Callsign, "")
		if euroscopeHub != nil {
			euroscopeHub.Broadcast(sessionID, euroscopeEvents.StandEvent{Callsign: assignment.Callsign, Stand: ""})
		}
	}
	if !result.Removed {
		hub.SendStandEvent(sessionID, result.Assignment.Callsign, result.Assignment.Stand)
		if euroscopeHub != nil {
			euroscopeHub.Broadcast(sessionID, euroscopeEvents.StandEvent{Callsign: result.Assignment.Callsign, Stand: result.Assignment.Stand})
		}
	}
	return nil
}

func clientAirport(strip *internalModels.Strip, direction string) string {
	if direction == "DEPARTURE" {
		return strip.Origin
	}
	return strip.Destination
}

func (hub *Hub) SendStandBlockBroadcast(session int32, stand string, block *frontend.StandBlockEntry, blockID ...int64) {
	id := int64(0)
	if len(blockID) > 0 {
		id = blockID[0]
	}
	hub.Broadcast(session, frontend.StandBlockUpdateEvent{
		Stand:   stand,
		Block:   block,
		BlockID: id,
	})
}

func (hub *Hub) SendStandStatusSnapshot(session int32, assignments []frontend.StandAssignmentEntry, blocks []frontend.StandBlockEntry) {
	hub.Broadcast(session, frontend.StandStatusSnapshotEvent{
		Assignments: assignments,
		Blocks:      blocks,
	})
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

func (hub *Hub) SendOwnersUpdate(session int32, callsign, owner string, nextOwners []string, previousOwners []string, nextDisplay *internalModels.NextDisplay) {
	event := frontend.OwnersUpdateEvent{
		Callsign:       callsign,
		Owner:          owner,
		NextOwners:     nextOwners,
		PreviousOwners: previousOwners,
		NextDisplay:    mapNextDisplayToDTO(hub.resolveOwnersUpdateNextDisplay(session, callsign, nextDisplay)),
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) resolveOwnersUpdateNextDisplay(session int32, callsign string, nextDisplay *internalModels.NextDisplay) *internalModels.NextDisplay {
	if nextDisplay != nil || hub.server == nil {
		return nextDisplay
	}

	stripRepo := hub.server.GetStripRepository()
	if stripRepo == nil {
		return nil
	}

	strip, err := stripRepo.GetByCallsign(context.Background(), session, callsign)
	if err != nil {
		slog.Warn("Failed to reload strip for owners update next display",
			slog.String("callsign", callsign),
			slog.Int("session", int(session)),
			slog.Any("error", err))
		return nil
	}

	hub.populateNextDisplay(strip, session)
	return strip.NextDisplay
}

func (hub *Hub) populateNextDisplay(strip *internalModels.Strip, session int32) {
	hub.populateNextDisplayContext(context.Background(), strip, session)
}

func (hub *Hub) populateNextDisplayContext(ctx context.Context, strip *internalModels.Strip, session int32) {
	if strip == nil {
		return
	}

	computer, ok := hub.server.(nextDisplayComputer)
	if !ok {
		return
	}

	nextDisplay, err := computer.ComputeNextDisplayForStripContext(ctx, strip, session)
	if err != nil {
		slog.Warn("Failed to compute next display data for strip",
			slog.String("callsign", strip.Callsign),
			slog.Int("session", int(session)),
			slog.Any("error", err))
		return
	}

	strip.NextDisplay = nextDisplay
}

func (hub *Hub) populateNextDisplays(strips []*internalModels.Strip, session int32) {
	hub.populateNextDisplaysContext(context.Background(), strips, session)
}

func (hub *Hub) populateNextDisplaysContext(ctx context.Context, strips []*internalModels.Strip, session int32) {
	if len(strips) == 0 {
		return
	}

	if computer, ok := hub.server.(nextDisplayBatchComputer); ok {
		if err := computer.ComputeNextDisplaysForStripsContext(ctx, strips, session); err != nil {
			slog.Warn("Failed to compute next display data for strip batch",
				slog.Int("session", int(session)),
				slog.Int("strip_count", len(strips)),
				slog.Any("error", err))
			return
		}
		return
	}

	for _, strip := range strips {
		hub.populateNextDisplayContext(ctx, strip, session)
	}
}

var _ nextDisplayComputer = (*internalServer.Server)(nil)
var _ nextDisplayBatchComputer = (*internalServer.Server)(nil)

func mapNextDisplayToDTO(nextDisplay *internalModels.NextDisplay) *frontend.NextDisplay {
	if nextDisplay == nil {
		return nil
	}

	return &frontend.NextDisplay{
		Label:     nextDisplay.Label,
		Frequency: nextDisplay.Frequency,
	}
}

func (hub *Hub) SendLayoutUpdates(session int32, layoutMap map[string]string) {
	hub.layoutUpdates <- layoutUpdateMessage{session: session, layoutMap: layoutMap}
}

func (hub *Hub) SendCdmUpdate(session int32, event frontend.CdmDataEvent) {
	hub.Broadcast(session, event)
}

func (hub *Hub) SendCdmUpdates(session int32, events []frontend.CdmDataEvent) {
	switch len(events) {
	case 0:
		return
	case 1:
		hub.Broadcast(session, events[0])
	default:
		hub.Broadcast(session, frontend.CdmDataBatchEvent{Updates: events})
	}
}

func (hub *Hub) SendCdmWait(session int32, callsign string) {
	event := frontend.CdmWaitEvent{
		Callsign: callsign,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendPdcStateChange(session int32, callsign, state, remarks string) {
	event := frontend.PdcStateChangeEvent{
		Callsign:          callsign,
		State:             state,
		PdcRequestRemarks: remarks,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendRunwayConfiguration(session int32, departure, arrival []string, status map[string]string) {
	event := frontend.RunwayConfigurationEvent{
		RunwaySetup: frontend.RunwayConfiguration{
			Departure:    departure,
			Arrival:      arrival,
			RunwayStatus: status,
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

func (hub *Hub) SendMessage(session int32, sender, text string, recipients []string) {
	if recipients == nil {
		recipients = []string{}
	}

	msg := frontend.MessageReceivedEvent{
		ID:          hub.NextMessageID(),
		Sender:      sender,
		Text:        text,
		IsBroadcast: len(recipients) == 0,
		Recipients:  recipients,
	}

	hub.storeMessage(session, msg)
	hub.dispatchMessage(session, msg, "")
}

func (hub *Hub) SendGoAround(session int32, callsign string) {
	hub.Broadcast(session, frontend.GoAroundEvent{
		Callsign: callsign,
	})
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
			client.Enqueue(message)
		}
	}
}
func (hub *Hub) SendCoordinationTagRequest(session int32, callsign, from, to string) {
	event := frontend.CoordinationTagRequestBroadcastEvent{
		Callsign: callsign,
		From:     from,
		To:       to,
	}
	hub.Broadcast(session, event)
}

func (hub *Hub) SendAvailableSids(session int32, sids pkgModels.AvailableSids) {
	hub.Broadcast(session, frontend.AvailableSidsEvent{Sids: sids})
}

func (hub *Hub) OnRegister(client *Client) {
	slog.Debug("Client registered", slog.String("cid", client.user.GetCid()))
	metrics.ConnectionOpened(context.Background(), client.sessionName, client.airport, "frontend", client.callsign, client.version)
	if client.session != WaitingForEuroscopeConnectionSessionId {
		hub.sendInitialEvent(context.Background(), client)
		return
	}

	client.Enqueue(frontend.DisconnectEvent{ReadOnly: client.readOnly})
}

func (hub *Hub) OnUnregister(client *Client) {
	slog.Debug("Client unregistered", slog.String("cid", client.user.GetCid()))
	metrics.ConnectionClosed(context.Background(), client.sessionName, client.airport, "frontend", client.callsign, client.version)
}

func (hub *Hub) getSnapshotBuilder() *SnapshotBuilder {
	if hub.snapshotBuilder != nil {
		return hub.snapshotBuilder
	}

	hub.snapshotBuilder = NewSnapshotBuilder(SnapshotBuilderDependencies{
		ControllerRepo:         hub.server.GetControllerRepository(),
		StripRepo:              hub.server.GetStripRepository(),
		SectorRepo:             hub.server.GetSectorOwnerRepository(),
		SessionRepo:            hub.server.GetSessionRepository(),
		CoordinationRepo:       hub.server.GetCoordinationRepository(),
		TacticalStripRepo:      hub.server.GetTacticalStripRepository(),
		StandAssignmentRepo:    hub.server.GetStandAssignmentRepository(),
		StandAssignmentEnabled: hub.server.GetStandAssignmentRepository() != nil,
		EuroscopeHub:           hub.server.GetEuroscopeHub(),
		BuildClxContext:        hub.makeClxValidationContext,
		PopulateNextStrips:     hub.populateNextDisplaysContext,
		LoadMessages:           hub.snapshotMessages,
		LoadCachedAtis:         hub.cachedAtisEvent,
	})

	return hub.snapshotBuilder
}

func (hub *Hub) snapshotMessages(session int32) []frontend.MessageReceivedEvent {
	hub.msgMu.Lock()
	defer hub.msgMu.Unlock()

	storedMsgs := make([]frontend.MessageReceivedEvent, len(hub.messages[session]))
	copy(storedMsgs, hub.messages[session])
	return storedMsgs
}

func (hub *Hub) cachedAtisEvent(session int32) *frontend.AtisUpdateEvent {
	hub.metarMu.RLock()
	defer hub.metarMu.RUnlock()

	metar := hub.metarCache[session]
	if metar == "" {
		return nil
	}

	return &frontend.AtisUpdateEvent{
		Metar:       metar,
		ArrAtisCode: hub.arrAtisCodeCache[session],
		DepAtisCode: hub.depAtisCodeCache[session],
	}
}

func (hub *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case client := <-hub.register:
			hub.clients[client] = true
			hub.OnRegister(client)
		case client := <-hub.unregister:
			if _, ok := hub.clients[client]; ok {
				delete(hub.clients, client)
				client.Close()
				hub.OnUnregister(client)
			}
		case msg := <-hub.cidOnline:
			hub.handleCidOnline(msg)
		case msg := <-hub.cidDisconnect:
			hub.handleCidDisconnect(msg.cid)
		case message := <-hub.send:
			if message.cid != nil {
				for client := range hub.clients {
					if message.session == client.session && *message.cid == client.GetCid() {
						client.Enqueue(message.message)
					}
				}
			} else {
				for client := range hub.clients {
					if message.session == client.session {
						client.Enqueue(message.message)
					}
				}
			}
		case msg := <-hub.layoutUpdates:
			for client := range hub.clients {
				if layout, ok := msg.layoutMap[client.position]; client.session == msg.session && ok {
					client.Enqueue(frontend.LayoutUpdateEvent{Layout: layout})
				}
			}
		}
	}
}

func buildFrontendController(callsign string, position string, sectorsMap map[string]*internalModels.SectorOwner) frontend.Controller {
	identifier := ""
	if sector, ok := sectorsMap[position]; ok {
		identifier = sector.Identifier
	}

	section := ""
	if pos, err := config.GetPositionBasedOnFrequency(position); err == nil {
		section = pos.Section
	}

	ownedSectors := []string{}
	if sector, ok := sectorsMap[position]; ok {
		ownedSectors = slices.Clone(sector.Sector)
	}

	return frontend.Controller{
		Callsign:     callsign,
		Position:     position,
		Identifier:   identifier,
		Section:      section,
		OwnedSectors: ownedSectors,
	}
}

func isObserverController(controller *internalModels.Controller, _ shared.EuroscopeHub) bool {
	return controller != nil && controller.Observer
}
