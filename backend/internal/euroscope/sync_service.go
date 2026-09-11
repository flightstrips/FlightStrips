package euroscope

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/metrics"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/models"
	"context"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type EuroscopeSyncRequest struct {
	Session     int32
	SessionName string
	Airport     string
	CID         string
	Callsign    string
	Position    string
	Version     string
	IsMaster    bool
	HasMaster   bool
	Event       euroscope.SyncEvent
}

type EuroscopeSyncMetrics struct {
	SessionName        string
	Airport            string
	StripCount         int
	ControllerCount    int
	ChangedStrips      int
	ChangedControllers int
	DBOperations       int
}

type EuroscopeSyncResult struct {
	Metrics           EuroscopeSyncMetrics
	MarkSessionSynced bool
	WakeFrontendCID   string
}

type euroscopeSyncRuntime interface {
	CancelOfflineTimer(session int32, positionName string)
	CancelAircraftDisconnect(session int32, callsign string) bool
	EvaluateClientRunwayState(session int32, cid, callsign string, current, master models.ActiveRunways, isMaster bool) runwayClientEvaluation
	ResyncSessionRunwayMismatchTargets(session int32, masterCID string, master models.ActiveRunways)
	CurrentMasterStatus(session int32, cid, callsign string) (hasMaster bool, isMaster bool)
	Send(session int32, cid string, message euroscope.OutgoingMessage)
	ScheduleOfflineActions(session int32, callsign, positionFreq, positionName string, delay time.Duration)
	ScheduleAircraftDisconnect(session int32, callsign string, delay time.Duration)
}

type euroscopeSyncHubRuntime struct {
	hub *Hub
}

func (r euroscopeSyncHubRuntime) CancelOfflineTimer(session int32, positionName string) {
	if r.hub != nil {
		r.hub.cancelOfflineTimer(session, positionName)
	}
}

func (r euroscopeSyncHubRuntime) CancelAircraftDisconnect(session int32, callsign string) bool {
	if r.hub != nil {
		return r.hub.cancelAircraftDisconnect(session, callsign)
	}
	return false
}

func (r euroscopeSyncHubRuntime) EvaluateClientRunwayState(session int32, cid, callsign string, current, master models.ActiveRunways, isMaster bool) runwayClientEvaluation {
	if r.hub == nil {
		return runwayClientEvaluation{CID: cid}
	}
	return r.hub.evaluateClientRunwayState(session, cid, callsign, current, master, isMaster)
}

func (r euroscopeSyncHubRuntime) ResyncSessionRunwayMismatchTargets(session int32, masterCID string, master models.ActiveRunways) {
	if r.hub != nil {
		r.hub.resyncSessionRunwayMismatchTargets(session, masterCID, master)
	}
}

func (r euroscopeSyncHubRuntime) CurrentMasterStatus(session int32, cid, callsign string) (hasMaster bool, isMaster bool) {
	if r.hub == nil {
		return false, false
	}

	master := r.hub.getMasterClient(session)
	if master == nil {
		return false, false
	}

	hasMaster = true
	isMaster = master.GetCid() == cid && strings.EqualFold(strings.TrimSpace(master.callsign), strings.TrimSpace(callsign))
	return hasMaster, isMaster
}

func (r euroscopeSyncHubRuntime) Send(session int32, cid string, message euroscope.OutgoingMessage) {
	if r.hub != nil {
		r.hub.Send(session, cid, message)
	}
}

func (r euroscopeSyncHubRuntime) ScheduleOfflineActions(session int32, callsign, positionFreq, positionName string, delay time.Duration) {
	if r.hub != nil {
		r.hub.scheduleOfflineActions(session, callsign, positionFreq, positionName, delay)
	}
}

func (r euroscopeSyncHubRuntime) ScheduleAircraftDisconnect(session int32, callsign string, delay time.Duration) {
	if r.hub != nil {
		r.hub.scheduleAircraftDisconnect(session, callsign, delay)
	}
}

type EuroscopeSyncService struct {
	server            shared.Server
	controllerService shared.ControllerService
	stripService      shared.StripService
	runtime           euroscopeSyncRuntime
}

func newEuroscopeSyncService(server shared.Server, controllerService shared.ControllerService, stripService shared.StripService, runtime euroscopeSyncRuntime) *EuroscopeSyncService {
	return &EuroscopeSyncService{
		server:            server,
		controllerService: controllerService,
		stripService:      stripService,
		runtime:           runtime,
	}
}

func newEuroscopeSyncServiceForClient(client *Client) *EuroscopeSyncService {
	if client == nil || client.hub == nil {
		return newEuroscopeSyncService(nil, nil, nil, nil)
	}

	return newEuroscopeSyncService(
		client.hub.server,
		client.hub.controllerService,
		client.hub.stripService,
		euroscopeSyncHubRuntime{hub: client.hub},
	)
}

func newEuroscopeSyncRequest(client *Client, event euroscope.SyncEvent) EuroscopeSyncRequest {
	request := EuroscopeSyncRequest{Event: event}
	if client == nil {
		return request
	}

	request.Session = client.session
	request.SessionName = client.sessionName
	request.Airport = client.airport
	request.CID = client.GetCid()
	request.Callsign = client.callsign
	request.Position = client.position
	request.Version = client.version

	if client.hub != nil {
		master := client.hub.getMasterClient(client.session)
		request.HasMaster = master != nil
		request.IsMaster = master == client
	}

	return request
}

func (s *EuroscopeSyncService) ApplySync(ctx context.Context, request EuroscopeSyncRequest) (EuroscopeSyncResult, error) {
	slog.DebugContext(ctx, "Received sync event", slog.Int("session", int(request.Session)), slog.String("client", request.Callsign))

	ctx, span := otel.Tracer("euroscope").Start(ctx, "euroscope.sync",
		trace.WithAttributes(
			attribute.Int("session", int(request.Session)),
			attribute.Int("sync.input.strips", len(request.Event.Strips)),
			attribute.Int("sync.input.controllers", len(request.Event.Controllers)),
			attribute.Int("sync.input.runways", len(request.Event.Runways)),
			attribute.Int("sync.input.sids", len(request.Event.Sids)),
		),
	)
	defer span.End()

	timings := &syncPhaseTimings{}

	controllers := make([]syncController, len(request.Event.Controllers))
	for i, controller := range request.Event.Controllers {
		controllers[i] = syncController{Position: controller.Position, Callsign: controller.Callsign}
	}

	phase := timings.begin()
	syncState, err := buildSyncState(ctx, s.server, request.Session)
	timings.observe(metrics.SyncPhaseBuildState, phase)
	if err != nil {
		return EuroscopeSyncResult{}, syncFailure(span, err)
	}

	ctx = shared.WithSyncState(ctx, syncState)

	phase = timings.begin()
	controllerPositionsChanged, err := s.syncControllersFromEvent(ctx, request.Session, controllers)
	timings.observe(metrics.SyncPhaseControllers, phase)
	if err != nil {
		return EuroscopeSyncResult{}, syncFailure(span, err)
	}
	syncState.GndOnline = hasGroundController(syncState.ExistingControllers)

	runwaysChanged := false
	if len(request.Event.Runways) > 0 {
		phase = timings.begin()
		runwaysChanged, err = s.applyOrValidateRunways(ctx, request, runwayValues(request.Event.Runways))
		timings.observe(metrics.SyncPhaseRunways, phase)
		if err != nil {
			return EuroscopeSyncResult{}, syncFailure(span, err)
		}
	}

	if runwaysChanged {
		syncState.Session = nil
	}

	if syncState.Session == nil {
		phase = timings.begin()
		syncState.Session, err = s.server.GetSessionRepository().GetByID(ctx, request.Session)
		timings.observe(metrics.SyncPhaseSession, phase)
		if err != nil {
			return EuroscopeSyncResult{}, syncFailure(span, err)
		}
		syncState.AddDBOperations(1)
	}

	if syncState.ChangedControllers > 0 || runwaysChanged {
		phase = timings.begin()
		_, err = updateSectorsForSync(ctx, s.server, request.Session)
		if err == nil {
			syncState.SectorOwners = nil
			err = updateLayoutsForSync(ctx, s.server, request.Session)
		}
		timings.observe(metrics.SyncPhaseSectors, phase)
		if err != nil {
			return EuroscopeSyncResult{}, syncFailure(span, err)
		}
	}

	phase = timings.begin()
	err = s.syncStripsFromEvent(ctx, request, stripValues(request.Event.Strips))
	timings.observe(metrics.SyncPhaseStrips, phase)
	if err != nil {
		return EuroscopeSyncResult{}, syncFailure(span, err)
	}

	// Captured before finalization so the counts describe the work this sync
	// scheduled. Finalization itself marks further strips for update, and folding
	// those back in would make the metric measure its own side effects.
	followUpWork := syncFollowUpWork(syncState)

	phase = timings.begin()
	err = s.finalizeSyncStripChanges(ctx, request.Session, syncState)
	timings.observe(metrics.SyncPhaseFinalize, phase)
	if err != nil {
		return EuroscopeSyncResult{}, syncFailure(span, err)
	}

	if syncState.ChangedControllers > 0 || syncState.ChangedStrips > 0 {
		phase = timings.begin()
		s.autoAssumeForSync(ctx, request, controllers, controllerPositionsChanged)
		timings.observe(metrics.SyncPhaseAutoAssume, phase)
	}

	_, isMaster := s.currentMasterStatus(request)
	if isMaster {
		phase = timings.begin()
		s.reconcileDBState(ctx, request, syncState)
		timings.observe(metrics.SyncPhaseReconcile, phase)
	}

	sidsChanged := false
	if len(request.Event.Sids) > 0 {
		phase = timings.begin()
		sidsChanged = s.persistSIDs(ctx, request.Session, syncState, sidValues(request.Event.Sids))
		timings.observe(metrics.SyncPhaseSids, phase)
	}

	sessionName := request.SessionName
	airport := request.Airport
	if syncState.Session != nil {
		sessionName = syncState.Session.Name
		airport = syncState.Session.Airport
	}

	changed := syncState.ChangedStrips > 0 || syncState.ChangedControllers > 0 || runwaysChanged || sidsChanged
	timings.publish(ctx, span, sessionName, airport)
	publishSyncFollowUpWork(ctx, span, sessionName, airport, followUpWork)
	metrics.RecordEuroscopeSyncOutcome(ctx, sessionName, airport, changed)
	span.SetAttributes(
		attribute.Bool("sync.changed", changed),
		attribute.Int("sync.changed.strips", syncState.ChangedStrips),
		attribute.Int("sync.changed.controllers", syncState.ChangedControllers),
		attribute.Int("sync.db_operations", syncState.DBOperations),
	)
	span.SetStatus(codes.Ok, "")

	return EuroscopeSyncResult{
		Metrics: EuroscopeSyncMetrics{
			SessionName:        sessionName,
			Airport:            airport,
			StripCount:         len(request.Event.Strips),
			ControllerCount:    len(request.Event.Controllers),
			ChangedStrips:      syncState.ChangedStrips,
			ChangedControllers: syncState.ChangedControllers,
			DBOperations:       syncState.DBOperations,
		},
		MarkSessionSynced: true,
		WakeFrontendCID:   request.CID,
	}, nil
}

// syncPhaseTimings accumulates how long each stage of a sync took. Timings are
// published once at the end rather than as they are measured, because the
// session name and airport that label them are only final after the session has
// been loaded.
type syncPhaseTimings struct {
	entries []syncPhaseTiming
}

type syncPhaseTiming struct {
	phase    string
	duration time.Duration
}

func (t *syncPhaseTimings) begin() time.Time {
	return time.Now()
}

func (t *syncPhaseTimings) observe(phase string, started time.Time) {
	t.entries = append(t.entries, syncPhaseTiming{phase: phase, duration: time.Since(started)})
}

// publish emits every measured phase as a metric and mirrors it onto the sync
// span, so the same breakdown is available from a dashboard and from a trace.
func (t *syncPhaseTimings) publish(ctx context.Context, span trace.Span, sessionName, airport string) {
	for _, entry := range t.entries {
		metrics.RecordEuroscopeSyncPhase(ctx, sessionName, airport, entry.phase, entry.duration)
		span.SetAttributes(attribute.Float64("sync.phase."+entry.phase+".seconds", entry.duration.Seconds()))
	}
}

// syncFollowUpWork counts the work a sync has scheduled for finalization. A sync
// that reports no changes but keeps scheduling follow-up work is repeating work
// that its inputs did not ask for.
func syncFollowUpWork(state *shared.SyncState) map[string]int {
	if state == nil {
		return nil
	}

	work := map[string]int{
		metrics.SyncWorkRouteRecalc:   len(state.RouteRecalcStrips),
		metrics.SyncWorkBayUpdate:     len(state.BayUpdates),
		metrics.SyncWorkPdcValidation: len(state.PdcValidationStrips),
		metrics.SyncWorkStripUpdate:   len(state.StripUpdates),
	}
	work[metrics.SyncWorkSquawkValidation] = boolToCount(state.SquawkValidation)
	work[metrics.SyncWorkLandingValidation] = boolToCount(state.LandingValidation)
	work[metrics.SyncWorkCdmRecalculation] = boolToCount(state.CdmRecalculation)
	return work
}

func publishSyncFollowUpWork(ctx context.Context, span trace.Span, sessionName, airport string, work map[string]int) {
	for _, kind := range []string{
		metrics.SyncWorkRouteRecalc, metrics.SyncWorkBayUpdate, metrics.SyncWorkPdcValidation,
		metrics.SyncWorkStripUpdate, metrics.SyncWorkSquawkValidation,
		metrics.SyncWorkLandingValidation, metrics.SyncWorkCdmRecalculation,
	} {
		count := work[kind]
		if count == 0 {
			continue
		}
		metrics.RecordEuroscopeSyncFollowUpWork(ctx, sessionName, airport, kind, count)
		span.SetAttributes(attribute.Int("sync.work."+kind, count))
	}
}

func boolToCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func syncFailure(span trace.Span, err error) error {
	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
	return err
}

// syncController is the value form used by the sync service after protobuf decoding.
type syncController struct {
	Position string
	Callsign string
}

func runwayValues(values []*euroscope.Runway) []euroscope.SyncRunway {
	result := make([]euroscope.SyncRunway, 0, len(values))
	for _, value := range values {
		if value != nil {
			result = append(result, *value)
		}
	}
	return result
}

func stripValues(values []*euroscope.Strip) []euroscope.Strip {
	result := make([]euroscope.Strip, 0, len(values))
	for _, value := range values {
		if value != nil {
			result = append(result, *value)
		}
	}
	return result
}

func sidValues(values []*euroscope.SidEntry) models.AvailableSids {
	result := make(models.AvailableSids, 0, len(values))
	for _, value := range values {
		if value != nil {
			result = append(result, models.SidInfo{Name: value.Name, Runway: value.Runway})
		}
	}
	return result
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
func (s *EuroscopeSyncService) syncControllersFromEvent(ctx context.Context, session int32, controllers []syncController) (map[string]struct{}, error) {
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

		if err := s.controllerService.UpsertController(ctx, session, controller.Callsign, controller.Position); err != nil {
			return nil, err
		}
		if pos, err := config.GetPositionBasedOnFrequency(controller.Position); err == nil && s.runtime != nil {
			s.runtime.CancelOfflineTimer(session, pos.Name)
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
func (s *EuroscopeSyncService) syncStripsFromEvent(ctx context.Context, request EuroscopeSyncRequest, strips []euroscope.Strip) error {
	for _, strip := range strips {
		recoveredFromPendingDisconnect := s.runtime != nil && s.runtime.CancelAircraftDisconnect(request.Session, strip.Callsign)
		if err := s.stripService.SyncStrip(ctx, request.Session, request.CID, strip, request.Airport); err != nil {
			return err
		}
		if recoveredFromPendingDisconnect {
			// A frontend may have refreshed while the strip was hidden by the
			// disconnect grace period. Republish it after sync finalization even
			// when none of its persisted fields changed.
			if syncState := shared.GetSyncState(ctx); syncState != nil {
				syncState.MarkStripUpdate(strip.Callsign)
			}
		}
	}
	return nil
}

func (s *EuroscopeSyncService) finalizeSyncStripChanges(ctx context.Context, session int32, syncState *shared.SyncState) error {
	if syncState == nil {
		return nil
	}

	stripService, ok := s.stripService.(syncStripFinalizer)
	if !ok {
		return nil
	}

	for _, callsign := range syncState.SortedRouteRecalcStrips() {
		if err := s.server.UpdateRouteForStripContext(ctx, callsign, session, false); err != nil {
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
		if cdmService := s.server.GetCdmService(); cdmService != nil && syncState.Session != nil {
			cdmService.TriggerRecalculate(ctx, session, syncState.Session.Airport)
		}
	}

	for _, callsign := range syncState.SortedStripUpdates() {
		s.server.GetFrontendHub().SendStripUpdate(session, callsign)
	}

	return nil
}

// autoAssumeForSync triggers AutoAssumeForControllerOnline for every position seen in
// the sync event plus the master's own position. Errors are logged but not returned
// because a failing auto-assume must not abort the sync.
func (s *EuroscopeSyncService) autoAssumeForSync(ctx context.Context, request EuroscopeSyncRequest, controllers []syncController, changedPositions map[string]struct{}) {
	positions := make(map[string]bool)
	for position := range changedPositions {
		if position != "" {
			positions[position] = true
		}
	}
	if request.Position != "" {
		positions[request.Position] = true
	}
	if len(positions) == 0 {
		for _, controller := range controllers {
			if controller.Position != "" {
				positions[controller.Position] = true
			}
		}
	}
	for position := range positions {
		if err := s.stripService.AutoAssumeForControllerOnline(ctx, request.Session, position); err != nil {
			slog.ErrorContext(ctx, "AutoAssumeForControllerOnline failed during sync",
				slog.String("position", position), slog.Any("error", err))
		}
	}
}

// reconcileDBState compares the DB against the sync event and schedules grace-period
// timers for any stale controllers or strips the master did not report as live.
func (s *EuroscopeSyncService) reconcileDBState(ctx context.Context, request EuroscopeSyncRequest, syncState *shared.SyncState) {
	knownControllers := make(map[string]bool, len(request.Event.Controllers)+1)
	for _, controller := range request.Event.Controllers {
		knownControllers[controller.Callsign] = true
	}
	knownControllers[request.Callsign] = true

	s.reconcileStaleControllers(ctx, request.Session, knownControllers, syncState)

	knownStrips := make(map[string]bool, len(request.Event.Strips))
	for _, strip := range request.Event.Strips {
		knownStrips[strip.Callsign] = true
	}

	s.reconcileStaleStrips(ctx, request.Session, knownStrips, syncState)
}

// reconcileStaleControllers schedules offline timers for any DB controllers whose
// callsign is absent from knownCallsigns. Errors fetching the DB list are logged only.
func (s *EuroscopeSyncService) reconcileStaleControllers(ctx context.Context, session int32, knownCallsigns map[string]bool, syncState *shared.SyncState) {
	if syncState != nil && syncState.ExistingControllers != nil {
		for _, dbCtrl := range syncState.ExistingControllers {
			s.reconcileStaleController(session, knownCallsigns, dbCtrl)
		}
		return
	}

	dbControllers, err := s.server.GetControllerRepository().List(ctx, session)
	if err != nil {
		slog.ErrorContext(ctx, "Sync reconciliation: failed to list controllers", slog.Any("error", err))
		return
	}
	for _, dbCtrl := range dbControllers {
		s.reconcileStaleController(session, knownCallsigns, dbCtrl)
	}
}

// reconcileStaleStrips schedules aircraft-disconnect timers for any DB strips whose
// callsign is absent from knownCallsigns. Errors fetching the DB list are logged only.
func (s *EuroscopeSyncService) reconcileStaleStrips(ctx context.Context, session int32, knownCallsigns map[string]bool, syncState *shared.SyncState) {
	if syncState != nil && syncState.ExistingStrips != nil {
		for _, dbStrip := range syncState.ExistingStrips {
			s.reconcileStaleStrip(session, knownCallsigns, dbStrip)
		}
		return
	}

	dbStrips, err := s.server.GetStripRepository().List(ctx, session)
	if err != nil {
		slog.ErrorContext(ctx, "Sync reconciliation: failed to list strips", slog.Any("error", err))
		return
	}
	for _, dbStrip := range dbStrips {
		s.reconcileStaleStrip(session, knownCallsigns, dbStrip)
	}
}

// persistSIDs saves the available SIDs from the sync event and broadcasts to the frontend.
// Errors are logged only because SID persistence should not abort the sync.
func (s *EuroscopeSyncService) persistSIDs(ctx context.Context, session int32, syncState *shared.SyncState, sids models.AvailableSids) bool {
	availSids := sids
	if syncState != nil && syncState.Session != nil && reflect.DeepEqual(syncState.Session.AvailableSids, availSids) {
		return false
	}
	changed := false
	if err := s.server.GetSessionRepository().UpdateSessionSids(ctx, session, availSids); err != nil {
		slog.ErrorContext(ctx, "Failed to persist available SIDs", slog.Any("error", err))
	} else {
		changed = true
		if syncState != nil {
			syncState.AddDBOperations(1)
			if syncState.Session != nil {
				syncState.Session.AvailableSids = availSids
			}
		}
	}
	s.server.GetFrontendHub().SendAvailableSids(session, availSids)
	return changed
}

// applyOrValidateRunways applies the runway configuration when the client is master,
// or compares it against the session's current runways and logs a warning if they differ
// (conflict detection for slave clients).
func (s *EuroscopeSyncService) applyOrValidateRunways(ctx context.Context, request EuroscopeSyncRequest, runways []euroscope.SyncRunway) (bool, error) {
	sessionRepo := s.server.GetSessionRepository()
	hasMaster, isMaster := s.currentMasterStatus(request)

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

	if !isMaster {
		currentSession, err := sessionRepo.GetByID(ctx, request.Session)
		if err != nil {
			return false, err
		}
		if !hasMaster {
			if s.runtime != nil {
				s.runtime.EvaluateClientRunwayState(
					request.Session,
					request.CID,
					request.Callsign,
					activeRunways,
					activeRunways,
					true,
				)
			}
			return false, nil
		}

		evaluation := runwayClientEvaluation{CID: request.CID}
		if s.runtime != nil {
			evaluation = s.runtime.EvaluateClientRunwayState(
				request.Session,
				request.CID,
				request.Callsign,
				activeRunways,
				currentSession.ActiveRunways,
				false,
			)
		}

		masterDep := currentSession.ActiveRunways.DepartureRunways
		masterArr := currentSession.ActiveRunways.ArrivalRunways
		if evaluation.DepartureMismatch || evaluation.ArrivalMismatch {
			slog.WarnContext(ctx, "Slave ES client has different runway configuration than master",
				slog.Int("session", int(request.Session)),
				slog.String("client", request.Callsign),
				slog.Any("slave_departure", departure),
				slog.Any("slave_arrival", arrival),
				slog.Any("master_departure", masterDep),
				slog.Any("master_arrival", masterArr),
			)
		}
		if evaluation.Changed {
			s.server.GetFrontendHub().Send(request.Session, request.CID, frontendEvents.RunwayConfigurationEvent{
				RunwaySetup: buildFrontendRunwayConfiguration(currentSession.ActiveRunways, evaluation.DepartureMismatch, evaluation.ArrivalMismatch),
			})
		}
		if evaluation.Alert != nil && s.runtime != nil {
			s.runtime.Send(request.Session, request.CID, *evaluation.Alert)
		}
		return false, nil
	}

	if s.runtime != nil {
		s.runtime.EvaluateClientRunwayState(request.Session, request.CID, request.Callsign, activeRunways, activeRunways, true)
	}

	currentSession, err := sessionRepo.GetByID(ctx, request.Session)
	if err != nil {
		return false, err
	}
	oldActiveRunways := currentSession.ActiveRunways

	activeRunways.RunwayStatus = currentSession.ActiveRunways.RunwayStatus
	if reflect.DeepEqual(normalizeActiveRunways(oldActiveRunways), normalizeActiveRunways(activeRunways)) {
		if s.runtime != nil {
			s.runtime.ResyncSessionRunwayMismatchTargets(request.Session, request.CID, activeRunways)
		}
		return false, nil
	}

	slog.InfoContext(ctx, "Runway change received",
		slog.Int("session", int(request.Session)),
		slog.Any("departure", departure),
		slog.Any("arrival", arrival),
	)

	if err = sessionRepo.UpdateActiveRunways(ctx, request.Session, activeRunways); err != nil {
		return false, err
	}

	if err := s.stripService.PropagateRunwayChange(ctx, request.Session, currentSession.Airport, oldActiveRunways, activeRunways); err != nil {
		slog.ErrorContext(ctx, "Failed to propagate runway change to strips", slog.Int("session", int(request.Session)), slog.Any("error", err))
	}

	s.server.GetFrontendHub().SendRunwayConfiguration(request.Session, departure, arrival, activeRunways.RunwayStatus)
	if s.runtime != nil {
		s.runtime.ResyncSessionRunwayMismatchTargets(request.Session, request.CID, activeRunways)
	}

	if _, err = s.server.RecalculateSessionContext(ctx, request.Session, true); err != nil {
		slog.ErrorContext(ctx, "Session recalculation failed after runway change", slog.Int("session", int(request.Session)), slog.Any("error", err))
		return false, err
	}
	slog.DebugContext(ctx, "Session recalculation completed", slog.Int("session", int(request.Session)))

	return !reflect.DeepEqual(oldActiveRunways, activeRunways), nil
}

func (s *EuroscopeSyncService) currentMasterStatus(request EuroscopeSyncRequest) (hasMaster bool, isMaster bool) {
	if s.runtime == nil {
		return request.HasMaster, request.IsMaster
	}
	return s.runtime.CurrentMasterStatus(request.Session, request.CID, request.Callsign)
}

func buildSyncState(ctx context.Context, server shared.Server, session int32) (*shared.SyncState, error) {
	controllerRepo := server.GetControllerRepository()
	stripRepo := server.GetStripRepository()
	sessionRepo := server.GetSessionRepository()

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

func (s *EuroscopeSyncService) reconcileStaleController(session int32, knownCallsigns map[string]bool, dbCtrl *internalModels.Controller) {
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
	if s.runtime != nil {
		s.runtime.ScheduleOfflineActions(session, dbCtrl.Callsign, posFreq, posName, offlineGracePeriod)
	}
}

func (s *EuroscopeSyncService) reconcileStaleStrip(session int32, knownCallsigns map[string]bool, dbStrip *internalModels.Strip) {
	if knownCallsigns[dbStrip.Callsign] {
		return
	}
	slog.Info("Sync reconciliation: scheduling disconnect for missing strip",
		slog.String("callsign", dbStrip.Callsign),
		slog.Int("session", int(session)))
	if s.runtime != nil {
		s.runtime.ScheduleAircraftDisconnect(session, dbStrip.Callsign, offlineGracePeriod)
	}
}

func applyOrValidateRunways(ctx context.Context, client *Client, runways []euroscope.SyncRunway) (bool, error) {
	service := newEuroscopeSyncServiceForClient(client)
	return service.applyOrValidateRunways(ctx, newEuroscopeSyncRequest(client, euroscope.SyncEvent{}), runways)
}

func autoAssumeForSync(ctx context.Context, client *Client, session int32, controllers []syncController, changedPositions map[string]struct{}) {
	service := newEuroscopeSyncServiceForClient(client)
	request := newEuroscopeSyncRequest(client, euroscope.SyncEvent{})
	request.Session = session
	service.autoAssumeForSync(ctx, request, controllers, changedPositions)
}

func reconcileStaleControllers(ctx context.Context, client *Client, session int32, knownCallsigns map[string]bool, syncState *shared.SyncState) {
	service := newEuroscopeSyncServiceForClient(client)
	service.reconcileStaleControllers(ctx, session, knownCallsigns, syncState)
}

func reconcileStaleStrips(ctx context.Context, client *Client, session int32, knownCallsigns map[string]bool, syncState *shared.SyncState) {
	service := newEuroscopeSyncServiceForClient(client)
	service.reconcileStaleStrips(ctx, session, knownCallsigns, syncState)
}
