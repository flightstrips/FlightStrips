package euroscope

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/metrics"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/models"
	"context"
	"log/slog"
	"reflect"
	"regexp"
	"time"

	gorilla "github.com/gorilla/websocket"
)

type Message = shared.Message[euroscope.EventType]

func handleLoginEvent(ctx context.Context, client *Client, message Message) error {
	previousPosition := client.position
	previousCallsign := client.callsign
	previousAirport := client.airport

	event, _, err := client.hub.handleLogin(message.Message, client.user)
	if err != nil {
		return err
	}

	client.position = event.Position
	client.callsign = event.Callsign
	client.observer = event.Observer
	client.localIP = event.LocalIP
	client.hub.setObserverCid(client.GetCid(), event.Observer)
	client.hub.setClientLocalIP(client.session, client.GetCid(), event.LocalIP)
	if master, ok := client.hub.master[client.session]; ok && master == client && previousCallsign != client.callsign {
		client.hub.setMasterClient(client)
	}

	if !event.Observer {
		client.hub.markPendingOnlineOrchestration(client.session, client.callsign)
		if layoutErr := client.hub.server.UpdateLayouts(client.session); layoutErr != nil {
			slog.ErrorContext(ctx, "Failed to update layouts after ES re-login", slog.String("cid", client.GetCid()), slog.Any("error", layoutErr))
		}
	} else if previousPosition != client.position || previousCallsign != client.callsign || previousAirport != client.airport {
		client.hub.server.GetFrontendHub().CidOnline(client.session, client.GetCid())
	}

	return nil
}

var hhmmPattern = regexp.MustCompile(`^(?:[01]\d|2[0-3])[0-5]\d$`)

func handleTokenEvent(ctx context.Context, client *Client, message Message) error {
	var event events.AuthenticationEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	user, err := client.hub.authenticationService.Validate(event.Token)
	if err != nil {
		slog.InfoContext(ctx, "Token re-validation failed, disconnecting client", slog.String("cid", client.GetCid()), slog.Any("error", err))
		_ = client.GetConnection().WriteMessage(gorilla.CloseMessage,
			gorilla.FormatCloseMessage(gorilla.CloseNormalClosure, "token invalid"))
		client.GetConnection().Close()
		return err
	}

	client.SetUser(user)
	return nil
}

func handleControllerOnline(ctx context.Context, client *Client, message Message) error {
	var event euroscope.ControllerOnlineEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	session := client.session

	// Resolve the position name for the timer key.
	positionName := ""
	if posConfig, configErr := config.GetPositionBasedOnFrequency(event.Position); configErr == nil {
		positionName = posConfig.Name
	}

	// Cancel any pending offline timer for this position.
	if positionName != "" {
		client.hub.cancelOfflineTimer(session, positionName)
	}

	result, err := client.hub.controllerService.ControllerOnlineWithOptions(
		ctx,
		session,
		event.Callsign,
		event.Position,
		positionName,
		shared.ControllerOnlineOptions{
			ForceOrchestration: client.hub.consumePendingOnlineOrchestration(session, event.Callsign),
		},
	)
	if err != nil {
		return err
	}

	slog.DebugContext(ctx, "Controller online result",
		slog.String("callsign", event.Callsign),
		slog.String("position", event.Position),
		slog.String("positionName", positionName),
		slog.Bool("notifyOnline", result.NotifyOnline),
		slog.Bool("singleOnPosition", result.SingleOnPosition),
		slog.Int("sectorChanges", len(result.SectorChanges)))

	if result.NotifyOnline {
		client.hub.server.GetFrontendHub().SendControllerOnline(session, event.Callsign, event.Position, "", nil)
	}

	if result.SingleOnPosition && positionName != "" {
		slog.InfoContext(ctx, "Scheduling online broadcast",
			slog.String("position", positionName),
			slog.String("callsign", event.Callsign),
			slog.Int("session", int(session)))
		client.hub.scheduleOnlineBroadcast(session, positionName, result.SectorChanges)
	}

	return nil
}

func handleControllerOffline(ctx context.Context, client *Client, message Message) error {
	var event euroscope.ControllerOfflineEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	session := client.session

	result, err := client.hub.controllerService.ControllerOffline(ctx, session, event.Callsign)
	if err != nil {
		return err
	}

	slog.DebugContext(ctx, "Controller offline result",
		slog.String("callsign", event.Callsign),
		slog.Bool("shouldScheduleTimer", result.ShouldScheduleTimer),
		slog.String("positionName", result.PositionName),
		slog.Int("session", int(session)))

	if result.ShouldScheduleTimer {
		slog.InfoContext(ctx, "Scheduling offline grace period timer",
			slog.String("callsign", event.Callsign),
			slog.String("position", result.PositionName),
			slog.Int("session", int(session)))
		client.hub.scheduleOfflineActions(session, event.Callsign, result.PositionFrequency, result.PositionName, offlineGracePeriod)
	}

	return nil
}

func handleAssignedSquawk(ctx context.Context, client *Client, message Message) error {
	var event euroscope.AssignedSquawkEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateAssignedSquawk(ctx, client.session, event.Callsign, event.Squawk)
}

func handleSquawk(ctx context.Context, client *Client, message Message) error {
	var event euroscope.SquawkEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateSquawk(ctx, client.session, event.Callsign, event.Squawk)
}

func handleRequestedAltitude(ctx context.Context, client *Client, message Message) error {
	var event euroscope.RequestedAltitudeEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateRequestedAltitude(ctx, client.session, event.Callsign, event.Altitude)
}

func handleClearedAltitude(ctx context.Context, client *Client, message Message) error {
	var event euroscope.ClearedAltitudeEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateClearedAltitude(ctx, client.session, event.Callsign, event.Altitude)
}

func handleCommunicationType(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CommunicationTypeEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateCommunicationType(ctx, client.session, event.Callsign, event.CommunicationType)
}

func handleGroundState(ctx context.Context, client *Client, message Message) error {
	var event euroscope.GroundStateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateGroundState(ctx, client.session, event.Callsign, event.GroundState, client.airport)
}

func handleClearedFlag(ctx context.Context, client *Client, message Message) error {
	var event euroscope.ClearedFlagEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateClearedFlag(ctx, client.session, event.Callsign, event.Cleared)
}

func handleSetHeading(ctx context.Context, client *Client, message Message) error {
	var event euroscope.HeadingEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateHeading(ctx, client.session, event.Callsign, event.Heading)
}

func handleAircraftDisconnected(ctx context.Context, client *Client, message Message) error {
	var event euroscope.AircraftDisconnectEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	client.hub.scheduleAircraftDisconnect(client.session, event.Callsign, offlineGracePeriod)
	return nil
}

func handleStand(ctx context.Context, client *Client, message Message) error {
	var event euroscope.StandEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateStand(ctx, client.session, event.Callsign, event.Stand)
}

func handleCdmTobtUpdate(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmTobtUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	if !hhmmPattern.MatchString(event.Tobt) {
		return nil
	}
	return client.hub.server.GetCdmService().HandleTobtUpdate(ctx, client.session, event.Callsign, event.Tobt, client.callsign, clientRole(client))
}

func handleCdmDeiceUpdate(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmDeiceUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	switch event.DeiceType {
	case "", "L", "M", "H", "J":
	default:
		return nil
	}
	return client.hub.server.GetCdmService().HandleDeiceUpdate(ctx, client.session, event.Callsign, event.DeiceType)
}

func handleCdmAsrtToggle(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmAsrtToggleEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.server.GetCdmService().HandleAsrtToggle(ctx, client.session, event.Callsign, event.Asrt)
}

func handleCdmTsacUpdate(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmTsacUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.server.GetCdmService().HandleTsacUpdate(ctx, client.session, event.Callsign, event.Tsac)
}

func handleCdmManualCtot(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmManualCtotEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	if !hhmmPattern.MatchString(event.Ctot) {
		return nil
	}
	return client.hub.server.GetCdmService().HandleManualCtot(ctx, client.session, event.Callsign, event.Ctot)
}

func handleCdmCtotRemove(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmCtotRemoveEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.server.GetCdmService().HandleCtotRemove(ctx, client.session, event.Callsign)
}

func handleCdmApproveReqTobt(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmApproveReqTobtEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.server.GetCdmService().HandleApproveReqTobt(ctx, client.session, event.Callsign, client.callsign, clientRole(client))
}

func handlePositionUpdate(ctx context.Context, client *Client, message Message) error {
	var event euroscope.AircraftPositionUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	client.hub.cancelAircraftDisconnect(client.session, event.Callsign)
	return client.hub.stripService.UpdateAircraftPosition(ctx, client.session, event.Callsign, event.Lat, event.Lon, int32(event.Altitude), client.airport)
}

func handleTrackingControllerChanged(ctx context.Context, client *Client, message Message) error {
	var event euroscope.TrackingControllerChangedEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.HandleTrackingControllerChanged(ctx, client.session, event.Callsign, event.TrackingController)
}

func handleCoordinationReceived(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CoordinationReceivedEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.HandleCoordinationReceived(
		ctx,
		client.session,
		event.Callsign,
		event.SourceControllerCallsign,
		event.ControllerCallsign,
	)
}

func handleSync(ctx context.Context, client *Client, message Message) error {
	startedAt := time.Now()

	var event euroscope.SyncEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	s := client.hub.server
	session := client.session

	slog.DebugContext(ctx, "Received sync event", slog.Int("session", int(session)), slog.String("client", client.callsign))

	// Convert the anonymous struct slice to the named helper type.
	controllers := make([]syncController, len(event.Controllers))
	for i, c := range event.Controllers {
		controllers[i] = syncController{Position: c.Position, Callsign: c.Callsign}
	}

	syncState, err := buildSyncState(ctx, client, session)
	if err != nil {
		return err
	}
	ctx = shared.WithSyncState(ctx, syncState)

	controllerPositionsChanged, err := syncControllersFromEvent(ctx, client, session, controllers)
	if err != nil {
		return err
	}
	syncState.GndOnline = hasGroundController(syncState.ExistingControllers)

	runwaysChanged := false
	if len(event.Runways) > 0 {
		if runwaysChanged, err = applyOrValidateRunways(ctx, client, event.Runways); err != nil {
			return err
		}
	}

	if runwaysChanged {
		syncState.Session = nil
	}

	if syncState.Session == nil {
		syncState.Session, err = s.GetSessionRepository().GetByID(ctx, session)
		if err != nil {
			return err
		}
		syncState.AddDBOperations(1)
	}

	if syncState.ChangedControllers > 0 || runwaysChanged {
		if _, err := updateSectorsForSync(ctx, s, session); err != nil {
			return err
		}
		syncState.SectorOwners = nil
		if err := updateLayoutsForSync(ctx, s, session); err != nil {
			return err
		}
	}

	if err := syncStripsFromEvent(ctx, client, session, event.Strips); err != nil {
		return err
	}

	if err := finalizeSyncStripChanges(ctx, client, session, syncState); err != nil {
		return err
	}

	if syncState.ChangedControllers > 0 || syncState.ChangedStrips > 0 {
		autoAssumeForSync(ctx, client, session, controllers, controllerPositionsChanged)
	}

	// Mark the session as fully synced before waking waiting frontends so that
	// any frontend connecting at this exact moment gets a real session immediately
	// rather than falling into the waiting state.
	client.hub.markSessionSynced(session)

	s.GetFrontendHub().CidOnline(session, client.user.GetCid())

	// Only the master client can authoritatively declare what is live.
	if master, ok := client.hub.master[client.session]; ok && master == client {
		reconcileDBState(ctx, client, session, event, syncState)
	}

	if len(event.Sids) > 0 {
		persistSIDs(ctx, client, session, syncState, models.AvailableSids(event.Sids))
	}

	sessionName := client.sessionName
	airport := client.airport
	if syncState.Session != nil {
		sessionName = syncState.Session.Name
		airport = syncState.Session.Airport
	}
	metrics.RecordEuroscopeSync(
		ctx,
		sessionName,
		airport,
		len(event.Strips),
		len(event.Controllers),
		syncState.ChangedStrips,
		syncState.ChangedControllers,
		syncState.DBOperations,
		time.Since(startedAt),
	)

	return nil
}

// syncController mirrors the anonymous struct inside euroscope.SyncEvent.Controllers.
type syncController struct {
	Position string `json:"position"`
	Callsign string `json:"callsign"`
}

type syncStripFinalizer interface {
	MoveToBay(ctx context.Context, session int32, callsign string, bay string, sendNotification bool) error
	ReevaluatePdcRequestValidationsForStrip(ctx context.Context, session int32, strip *internalModels.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error
	ReevaluateSquawkValidationsForSession(ctx context.Context, session int32, publish bool) error
	ReevaluateLandingClearanceValidationsForSession(ctx context.Context, session int32, publish bool, forceReactivate bool) error
}

type syncContextServer interface {
	UpdateSectorsContext(ctx context.Context, sessionId int32) ([]shared.SectorChange, error)
	UpdateLayoutsContext(ctx context.Context, sessionId int32) error
}

// syncControllersFromEvent upserts each controller from the sync and cancels any
// pending offline timer for its position.
func syncControllersFromEvent(ctx context.Context, client *Client, session int32, controllers []syncController) (map[string]struct{}, error) {
	changedPositions := make(map[string]struct{})
	syncState := shared.GetSyncState(ctx)

	for _, controller := range controllers {
		if syncState != nil && syncState.ExistingControllers != nil {
			if existing := syncState.ExistingControllers[controller.Callsign]; existing == nil || existing.Position != controller.Position {
				changedPositions[controller.Position] = struct{}{}
				if existing != nil && existing.Position != "" {
					changedPositions[existing.Position] = struct{}{}
				}
			}
		}
		if err := client.hub.controllerService.UpsertController(ctx, session, controller.Callsign, controller.Position); err != nil {
			return nil, err
		}
		if pos, err := config.GetPositionBasedOnFrequency(controller.Position); err == nil {
			client.hub.cancelOfflineTimer(session, pos.Name)
		}
	}
	return changedPositions, nil
}

func updateSectorsForSync(ctx context.Context, server shared.Server, session int32) ([]shared.SectorChange, error) {
	if syncServer, ok := server.(syncContextServer); ok {
		return syncServer.UpdateSectorsContext(ctx, session)
	}
	return server.UpdateSectors(session)
}

func updateLayoutsForSync(ctx context.Context, server shared.Server, session int32) error {
	if syncServer, ok := server.(syncContextServer); ok {
		return syncServer.UpdateLayoutsContext(ctx, session)
	}
	return server.UpdateLayouts(session)
}

// syncStripsFromEvent syncs each strip to the DB and cancels any pending aircraft-disconnect timer.
func syncStripsFromEvent(ctx context.Context, client *Client, session int32, strips []euroscope.Strip) error {
	for _, strip := range strips {
		if err := client.hub.stripService.SyncStrip(ctx, session, client.GetCid(), strip, client.airport); err != nil {
			return err
		}
		client.hub.cancelAircraftDisconnect(session, strip.Callsign)
	}
	return nil
}

func finalizeSyncStripChanges(ctx context.Context, client *Client, session int32, syncState *shared.SyncState) error {
	if syncState == nil {
		return nil
	}

	server := client.hub.server
	stripService, ok := client.hub.stripService.(syncStripFinalizer)
	if !ok {
		return nil
	}
	for _, callsign := range syncState.SortedRouteRecalcStrips() {
		if err := server.UpdateRouteForStripContext(ctx, callsign, session, false); err != nil {
			slog.ErrorContext(ctx, "Error updating route for strip during sync finalization",
				slog.String("callsign", callsign),
				slog.Any("error", err))
		}
	}

	for _, callsign := range syncState.SortedBayUpdateCallsigns() {
		if err := stripService.MoveToBay(ctx, session, callsign, syncState.BayUpdates[callsign], false); err != nil {
			slog.ErrorContext(ctx, "Error moving bay for strip during sync finalization",
				slog.String("callsign", callsign),
				slog.Any("error", err))
		}
	}

	for _, callsign := range syncState.SortedPdcValidationStrips() {
		strip := syncState.ExistingStrips[callsign]
		if strip == nil || syncState.Session == nil {
			continue
		}
		if err := stripService.ReevaluatePdcRequestValidationsForStrip(
			ctx,
			session,
			strip,
			syncState.Session.ActiveRunways.DepartureRunways,
			false,
			false,
		); err != nil {
			return err
		}
	}

	if syncState.SquawkValidation {
		if err := stripService.ReevaluateSquawkValidationsForSession(ctx, session, false); err != nil {
			return err
		}
	}

	if syncState.LandingValidation {
		if err := stripService.ReevaluateLandingClearanceValidationsForSession(ctx, session, false, false); err != nil {
			return err
		}
	}

	if syncState.CdmRecalculation {
		if cdmService := server.GetCdmService(); cdmService != nil && syncState.Session != nil {
			cdmService.TriggerRecalculate(ctx, session, syncState.Session.Airport)
		}
	}

	for _, callsign := range syncState.SortedStripUpdates() {
		server.GetFrontendHub().SendStripUpdate(session, callsign)
	}

	return nil
}

// autoAssumeForSync triggers AutoAssumeForControllerOnline for every position seen in
// the sync event plus the master's own position. Errors are logged but not returned
// because a failing auto-assume must not abort the sync.
func autoAssumeForSync(ctx context.Context, client *Client, session int32, controllers []syncController, changedPositions map[string]struct{}) {
	positions := make(map[string]bool)
	for position := range changedPositions {
		if position != "" {
			positions[position] = true
		}
	}
	if client.position != "" {
		positions[client.position] = true
	}
	if len(positions) == 0 {
		for _, controller := range controllers {
			if controller.Position != "" {
				positions[controller.Position] = true
			}
		}
	}
	for position := range positions {
		if err := client.hub.stripService.AutoAssumeForControllerOnline(ctx, session, position); err != nil {
			slog.ErrorContext(ctx, "AutoAssumeForControllerOnline failed during sync",
				slog.String("position", position), slog.Any("error", err))
		}
	}
}

// reconcileDBState compares the DB against the sync event and schedules grace-period
// timers for any stale controllers or strips the master did not report as live.
func reconcileDBState(ctx context.Context, client *Client, session int32, event euroscope.SyncEvent, syncState *shared.SyncState) {
	// Build the set of known-live controller callsigns. The master's own callsign is
	// never in event.Controllers (remote-only list) so we add it explicitly.
	knownControllers := make(map[string]bool, len(event.Controllers)+1)
	for _, c := range event.Controllers {
		knownControllers[c.Callsign] = true
	}
	knownControllers[client.callsign] = true

	reconcileStaleControllers(ctx, client, session, knownControllers, syncState)

	knownStrips := make(map[string]bool, len(event.Strips))
	for _, s := range event.Strips {
		knownStrips[s.Callsign] = true
	}

	reconcileStaleStrips(ctx, client, session, knownStrips, syncState)
}

// reconcileStaleControllers schedules offline timers for any DB controllers whose
// callsign is absent from knownCallsigns. Errors fetching the DB list are logged only.
func reconcileStaleControllers(ctx context.Context, client *Client, session int32, knownCallsigns map[string]bool, syncState *shared.SyncState) {
	if syncState != nil && syncState.ExistingControllers != nil {
		for _, dbCtrl := range syncState.ExistingControllers {
			reconcileStaleController(client, session, knownCallsigns, dbCtrl)
		}
		return
	}

	dbControllers, err := client.hub.server.GetControllerRepository().List(ctx, session)
	if err != nil {
		slog.ErrorContext(ctx, "Sync reconciliation: failed to list controllers", slog.Any("error", err))
		return
	}
	for _, dbCtrl := range dbControllers {
		reconcileStaleController(client, session, knownCallsigns, dbCtrl)
	}
}

// reconcileStaleStrips schedules aircraft-disconnect timers for any DB strips whose
// callsign is absent from knownCallsigns. Errors fetching the DB list are logged only.
func reconcileStaleStrips(ctx context.Context, client *Client, session int32, knownCallsigns map[string]bool, syncState *shared.SyncState) {
	if syncState != nil && syncState.ExistingStrips != nil {
		for _, dbStrip := range syncState.ExistingStrips {
			reconcileStaleStrip(client, session, knownCallsigns, dbStrip)
		}
		return
	}

	dbStrips, err := client.hub.server.GetStripRepository().List(ctx, session)
	if err != nil {
		slog.ErrorContext(ctx, "Sync reconciliation: failed to list strips", slog.Any("error", err))
		return
	}
	for _, dbStrip := range dbStrips {
		reconcileStaleStrip(client, session, knownCallsigns, dbStrip)
	}
}

// persistSIDs saves the available SIDs from the sync event and broadcasts to the frontend.
// Errors are logged only — a SID persistence failure must not abort the sync.
func persistSIDs(ctx context.Context, client *Client, session int32, syncState *shared.SyncState, sids models.AvailableSids) {
	s := client.hub.server
	availSids := sids
	if syncState != nil && syncState.Session != nil && reflect.DeepEqual(syncState.Session.AvailableSids, availSids) {
		return
	}
	if err := s.GetSessionRepository().UpdateSessionSids(ctx, session, availSids); err != nil {
		slog.ErrorContext(ctx, "Failed to persist available SIDs", slog.Any("error", err))
	} else if syncState != nil {
		syncState.AddDBOperations(1)
		if syncState.Session != nil {
			syncState.Session.AvailableSids = availSids
		}
	}
	s.GetFrontendHub().SendAvailableSids(session, availSids)
}

func handleStripUpdateEvent(ctx context.Context, client *Client, message Message) error {
	var event euroscope.StripUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	client.hub.cancelAircraftDisconnect(client.session, event.Callsign)
	return client.hub.stripService.SyncStrip(ctx, client.session, client.GetCid(), event.Strip, client.airport)
}

func handleRunways(ctx context.Context, client *Client, message Message) error {
	var event euroscope.RunwayEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	slog.DebugContext(ctx, "Received runway configuration change", slog.Int("session", int(client.session)), slog.Any("event", event))

	_, err := applyOrValidateRunways(ctx, client, event.Runways)
	return err
}

// applyOrValidateRunways applies the runway configuration when the client is master,
// or compares it against the session's current runways and logs a warning if they differ
// (conflict detection for slave clients).
func applyOrValidateRunways(ctx context.Context, client *Client, runways []euroscope.SyncRunway) (bool, error) {
	s := client.hub.server
	sessionRepo := s.GetSessionRepository()

	departure := make([]string, 0)
	arrival := make([]string, 0)
	for _, runway := range runways {
		if runway.Arrival {
			arrival = append(arrival, runway.Name)
		}
		if runway.Departure {
			departure = append(departure, runway.Name)
		}
	}

	activeRunways := models.ActiveRunways{
		DepartureRunways: departure,
		ArrivalRunways:   arrival,
	}

	isMaster := false
	if master, ok := client.hub.master[client.session]; ok && master == client {
		isMaster = true
	}

	if !isMaster {
		currentSession, err := sessionRepo.GetByID(ctx, client.session)
		if err != nil {
			return false, err
		}
		if _, hasMaster := client.hub.master[client.session]; !hasMaster {
			client.hub.evaluateClientRunwayState(
				client.session,
				client.GetCid(),
				client.callsign,
				activeRunways,
				activeRunways,
				true,
			)
			return false, nil
		}
		evaluation := client.hub.evaluateClientRunwayState(
			client.session,
			client.GetCid(),
			client.callsign,
			activeRunways,
			currentSession.ActiveRunways,
			false,
		)
		masterDep := currentSession.ActiveRunways.DepartureRunways
		masterArr := currentSession.ActiveRunways.ArrivalRunways
		if evaluation.DepartureMismatch || evaluation.ArrivalMismatch {
			slog.WarnContext(ctx, "Slave ES client has different runway configuration than master",
				slog.Int("session", int(client.session)),
				slog.String("client", client.callsign),
				slog.Any("slave_departure", departure),
				slog.Any("slave_arrival", arrival),
				slog.Any("master_departure", masterDep),
				slog.Any("master_arrival", masterArr),
			)
		}
		if evaluation.Changed {
			s.GetFrontendHub().Send(client.session, client.GetCid(), frontendEvents.RunwayConfigurationEvent{
				RunwaySetup: buildFrontendRunwayConfiguration(currentSession.ActiveRunways, evaluation.DepartureMismatch, evaluation.ArrivalMismatch),
			})
		}
		if evaluation.Alert != nil {
			client.hub.Send(client.session, client.GetCid(), *evaluation.Alert)
		}
		return false, nil
	}

	client.hub.evaluateClientRunwayState(client.session, client.GetCid(), client.callsign, activeRunways, activeRunways, true)

	slog.InfoContext(ctx, "Runway change received",
		slog.Int("session", int(client.session)),
		slog.Any("departure", departure),
		slog.Any("arrival", arrival),
	)

	currentSession, err := sessionRepo.GetByID(ctx, client.session)
	if err != nil {
		return false, err
	}
	oldActiveRunways := currentSession.ActiveRunways

	// Preserve any frontend-set runway status when EuroScope pushes a runway change.
	activeRunways.RunwayStatus = currentSession.ActiveRunways.RunwayStatus

	if err = sessionRepo.UpdateActiveRunways(ctx, client.session, activeRunways); err != nil {
		return false, err
	}

	if err := client.hub.stripService.PropagateRunwayChange(ctx, client.session, currentSession.Airport, oldActiveRunways, activeRunways); err != nil {
		slog.ErrorContext(ctx, "Failed to propagate runway change to strips", slog.Int("session", int(client.session)), slog.Any("error", err))
	}

	s.GetFrontendHub().SendRunwayConfiguration(client.session, departure, arrival, activeRunways.RunwayStatus)
	client.hub.resyncSessionRunwayMismatchTargets(client.session, client.GetCid(), activeRunways)

	if _, err = s.UpdateSectors(client.session); err != nil {
		slog.ErrorContext(ctx, "UpdateSectors failed after runway change", slog.Int("session", int(client.session)), slog.Any("error", err))
		return false, err
	}
	slog.DebugContext(ctx, "UpdateSectors completed", slog.Int("session", int(client.session)))

	if err = s.UpdateRoutesForSession(client.session, true); err != nil {
		slog.ErrorContext(ctx, "UpdateRoutesForSession failed after runway change", slog.Int("session", int(client.session)), slog.Any("error", err))
		return false, err
	}
	slog.DebugContext(ctx, "UpdateRoutesForSession completed", slog.Int("session", int(client.session)))

	// Recalculate and broadcast per-controller layouts after runway change.
	// Do not return on failure — a layout error must not block the runway change.
	if err = s.UpdateLayouts(client.session); err != nil {
		slog.ErrorContext(ctx, "Failed to update layouts after runway change",
			slog.Int("session", int(client.session)),
			slog.Any("error", err))
	}

	return !reflect.DeepEqual(oldActiveRunways, activeRunways), nil
}

func buildSyncState(ctx context.Context, client *Client, session int32) (*shared.SyncState, error) {
	controllerRepo := client.hub.server.GetControllerRepository()
	stripRepo := client.hub.server.GetStripRepository()
	sessionRepo := client.hub.server.GetSessionRepository()

	controllers, err := controllerRepo.List(ctx, session)
	if err != nil {
		return nil, err
	}
	strips, err := stripRepo.List(ctx, session)
	if err != nil {
		return nil, err
	}
	sessionModel, err := sessionRepo.GetByID(ctx, session)
	if err != nil {
		return nil, err
	}

	state := &shared.SyncState{
		Session:             sessionModel,
		ExistingControllers: make(map[string]*internalModels.Controller, len(controllers)),
		ExistingStrips:      make(map[string]*internalModels.Strip, len(strips)),
		BayMaxSequence:      make(map[string]int32),
		DBOperations:        3,
	}
	for _, controller := range controllers {
		state.ExistingControllers[controller.Callsign] = controller
	}
	for _, strip := range strips {
		state.ExistingStrips[strip.Callsign] = strip
		if strip.Sequence != nil && *strip.Sequence > state.BayMaxSequence[strip.Bay] {
			state.BayMaxSequence[strip.Bay] = *strip.Sequence
		}
	}
	state.GndOnline = hasGroundController(state.ExistingControllers)

	return state, nil
}

func hasGroundController(controllers map[string]*internalModels.Controller) bool {
	for _, controller := range controllers {
		if pos, err := config.GetPositionBasedOnFrequency(controller.Position); err == nil && pos.Section == "GND" {
			return true
		}
	}
	return false
}

func reconcileStaleController(client *Client, session int32, knownCallsigns map[string]bool, dbCtrl *internalModels.Controller) {
	if knownCallsigns[dbCtrl.Callsign] {
		return
	}
	posFreq := dbCtrl.Position
	posName := ""
	if pos, posErr := config.GetPositionBasedOnFrequency(dbCtrl.Position); posErr == nil {
		posFreq = pos.Frequency
		posName = pos.Name
	}
	slog.Info("Sync reconciliation: scheduling offline for missing controller",
		slog.String("callsign", dbCtrl.Callsign),
		slog.Int("session", int(session)))
	client.hub.scheduleOfflineActions(session, dbCtrl.Callsign, posFreq, posName, offlineGracePeriod)
}

func reconcileStaleStrip(client *Client, session int32, knownCallsigns map[string]bool, dbStrip *internalModels.Strip) {
	if knownCallsigns[dbStrip.Callsign] {
		return
	}
	slog.Info("Sync reconciliation: scheduling disconnect for missing strip",
		slog.String("callsign", dbStrip.Callsign),
		slog.Int("session", int(session)))
	client.hub.scheduleAircraftDisconnect(session, dbStrip.Callsign, offlineGracePeriod)
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func clientRole(client *Client) string {
	if master, ok := client.hub.master[client.session]; ok && master == client {
		return "master"
	}
	return "slave"
}

func handleCdmMasterToggle(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmMasterToggleEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.server.GetCdmService().SetSessionCdmMaster(ctx, client.session, event.Master)
}

func handleIssuePdcClearance(ctx context.Context, client *Client, message Message) error {
	var event euroscope.IssuePdcClearanceEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	pdcService := client.hub.server.GetPdcService()
	if pdcService == nil {
		return nil
	}
	return pdcService.IssueClearance(ctx, event.Callsign, event.Remarks, client.GetCid(), client.session)
}

func handlePdcRevertToVoice(ctx context.Context, client *Client, message Message) error {
	var event euroscope.PdcRevertToVoiceEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	pdcService := client.hub.server.GetPdcService()
	if pdcService == nil {
		return nil
	}
	return pdcService.RevertToVoice(ctx, event.Callsign, client.session, client.GetCid())
}
