package vatsim

import (
	"FlightStrips/internal/dependencies"
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/models"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const reconciliationSequenceSpacing int32 = 1000

// Arrival reconciliation is deliberately capped independently of traffic
// volume. Five passes allow several dependent lifecycle updates and a final
// stability check without turning a large arrival set into an unbounded number
// of database round trips.
const maxArrivalReconciliationPasses = 5

const (
	hiddenDepartureBay = "DEP_HIDDEN"
	hiddenArrivalBay   = "ARR_HIDDEN"
)

type reconciliationSessionStore interface {
	List(context.Context) ([]*models.Session, error)
}

type reconciliationStripStore interface {
	List(context.Context, int32) ([]*models.Strip, error)
	Create(context.Context, *models.Strip) error
	UpdateVatsimSource(context.Context, int32, string, models.VatsimStripSource) (int64, error)
	UpdateArrivalETA(context.Context, int32, string, models.ArrivalETA) (int64, error)
	UpdateBayAndSequence(context.Context, int32, string, string, int32) (int64, error)
	Delete(context.Context, int32, string) error
}

type reconciliationAssignmentStore interface {
	GetAssignment(context.Context, int32, string) (*models.StandAssignment, error)
}

type reconciliationAssignmentLister interface {
	ListAssignments(context.Context, int32) ([]*models.StandAssignment, error)
}

// DepartureFlightInfo is the minimal VATSIM view the departure lifecycle needs
// to reserve, renew, or block a departure stand. It is deliberately decoupled
// from the full Flight record so the lifecycle service can stay testable.
type DepartureFlightInfo struct {
	Callsign     string
	CID          string
	Online       bool
	Revision     int64
	Origin       string
	Destination  string
	AircraftType string
	Latitude     float64
	Longitude    float64
}

// ArrivalFlightInfo is the minimal VATSIM view the arrival lifecycle needs to
// estimate, assign, or confirm an arrival stand. It is deliberately decoupled
// from the full Flight record so the lifecycle service can stay testable.
type ArrivalFlightInfo struct {
	Callsign     string
	CID          string
	Online       bool
	Revision     int64
	Origin       string
	Destination  string
	AircraftType string
}

// DepartureLifecycle drives stand reservation and departure-block transitions
// for a departure the reconciler has just applied. The reconciler owns feed
// timing; the lifecycle owns allocation timing and persistence.
type DepartureLifecycle interface {
	ProcessDeparture(ctx context.Context, session int32, strip *models.Strip, flight DepartureFlightInfo) error
	CancelDeparture(ctx context.Context, session int32, callsign string) error
}

// ArrivalLifecycle drives the ESTIMATED → ASSIGNED → CONFIRMED transitions
// for an arrival the reconciler has just applied. The reconciler owns feed
// timing; the lifecycle owns allocation timing and persistence.
type ArrivalLifecycle interface {
	ProcessArrival(ctx context.Context, session int32, strip *models.Strip, flight ArrivalFlightInfo) error
	CancelArrival(ctx context.Context, session int32, callsign string) error
}

// ArrivalLifecycleOrderer lets a lifecycle expose the target-stage priority it
// will apply to a feed record. Higher-stage arrivals must establish their hard
// occupancy before lower-stage reservations are validated or selected.
type ArrivalLifecycleOrderer interface {
	ArrivalProcessingPriority(strip *models.Strip, flight ArrivalFlightInfo, assignment *models.StandAssignment) int
}

type reconciliationNotifier interface {
	SendStripUpdate(session int32, callsign string)
}

// Reconciler materializes relevant VATSIM records into each FlightStrips
// session. It deliberately owns only feed fields; operational state remains
// controller/EuroScope owned.
type Reconciler struct {
	cache                  SnapshotSource
	sessions               reconciliationSessionStore
	strips                 reconciliationStripStore
	assignments            reconciliationAssignmentStore
	lifecycle              DepartureLifecycle
	arrivalLifecycle       ArrivalLifecycle
	notifier               reconciliationNotifier
	interval               time.Duration
	airportCoordinates     AirportCoordinates
	legacyArrivalETAWriter bool
	now                    func() time.Time
}

type ReconcilerDependencies struct {
	Cache              SnapshotSource
	Sessions           reconciliationSessionStore
	Strips             reconciliationStripStore
	Assignments        reconciliationAssignmentStore
	DepartureLifecycle DepartureLifecycle
	ArrivalLifecycle   ArrivalLifecycle
	Notifier           reconciliationNotifier
}

func NewReconciler(deps ReconcilerDependencies, interval time.Duration, options ...ArrivalETAOption) (*Reconciler, error) {
	required := []struct {
		name  string
		value any
	}{
		{"VATSIM cache", deps.Cache},
		{"session store", deps.Sessions},
		{"strip store", deps.Strips},
		{"stand assignment store", deps.Assignments},
		{"departure lifecycle", deps.DepartureLifecycle},
		{"arrival lifecycle", deps.ArrivalLifecycle},
		{"strip notifier", deps.Notifier},
	}
	for _, dependency := range required {
		if dependencies.IsNil(dependency.value) {
			return nil, fmt.Errorf("VATSIM reconciler requires %s", dependency.name)
		}
	}

	return newReconciler(
		deps.Cache,
		deps.Sessions,
		deps.Strips,
		deps.Assignments,
		deps.DepartureLifecycle,
		deps.ArrivalLifecycle,
		deps.Notifier,
		interval,
		options...,
	), nil
}

func newReconciler(cache SnapshotSource, sessions reconciliationSessionStore, strips reconciliationStripStore, assignments reconciliationAssignmentStore, departureLifecycle DepartureLifecycle, arrivalLifecycle ArrivalLifecycle, notifier reconciliationNotifier, interval time.Duration, options ...ArrivalETAOption) *Reconciler {
	if interval <= 0 {
		interval = defaultRefreshInterval
	}
	reconciler := &Reconciler{
		cache: cache, sessions: sessions, strips: strips, assignments: assignments,
		lifecycle: departureLifecycle, arrivalLifecycle: arrivalLifecycle,
		notifier: notifier, interval: interval, now: time.Now,
		legacyArrivalETAWriter: true,
	}
	for _, option := range options {
		option(reconciler)
	}
	return reconciler
}

func (r *Reconciler) Start(ctx context.Context) {
	if err := r.Reconcile(ctx); err != nil {
		slog.ErrorContext(ctx, "SAT VATSIM reconciliation failed", slog.Any("error", err))
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.Reconcile(ctx); err != nil {
				slog.ErrorContext(ctx, "SAT VATSIM reconciliation failed", slog.Any("error", err))
			}
		}
	}
}

// Reconcile performs one source snapshot reconciliation. Sessions are listed
// directly instead of deriving them from EuroScope clients, so it also works
// before a tower or EuroScope master connects.
func (r *Reconciler) Reconcile(ctx context.Context) (resultErr error) {
	startedAt := time.Now()
	defer func() {
		outcome := "success"
		if resultErr != nil {
			outcome = "failure"
		}
		metrics.RecordSATReconciliation(ctx, time.Since(startedAt), outcome)
	}()
	snapshot := r.cache.Snapshot()
	if snapshot.Timestamp.IsZero() {
		return nil
	}
	sessions, err := r.sessions.List(ctx)
	if err != nil {
		return err
	}
	var sessionErrors []error
	for _, session := range sessions {
		if session == nil || strings.TrimSpace(session.Airport) == "" {
			continue
		}
		if err := r.reconcileSession(ctx, snapshot, session); err != nil {
			sessionErrors = append(sessionErrors, fmt.Errorf("reconcile session %d (%s): %w", session.ID, session.Name, err))
			continue
		}
	}
	return errors.Join(sessionErrors...)
}

// ReconcileSession applies the current source snapshot to one explicitly
// selected session. Test tools use this to avoid leaking synthetic flights
// into other sessions that happen to serve the same airport.
func (r *Reconciler) ReconcileSession(ctx context.Context, sessionID int32) error {
	snapshot := r.cache.Snapshot()
	if snapshot.Timestamp.IsZero() {
		return nil
	}
	sessions, err := r.sessions.List(ctx)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session != nil && session.ID == sessionID {
			if strings.TrimSpace(session.Airport) == "" {
				return fmt.Errorf("session %d has no airport", sessionID)
			}
			return r.reconcileSession(ctx, snapshot, session)
		}
	}
	return fmt.Errorf("session %d not found", sessionID)
}

func (r *Reconciler) reconcileSession(ctx context.Context, snapshot Snapshot, session *models.Session) error {
	strips, err := r.strips.List(ctx, session.ID)
	if err != nil {
		return err
	}
	existing := make(map[string]*models.Strip, len(strips))
	for _, strip := range strips {
		if strip != nil {
			existing[normalizeCallsign(strip.Callsign)] = strip
		}
	}

	relevant := make(map[string]Flight)
	relevantFlights := make([]Flight, 0)
	airport := strings.ToUpper(strings.TrimSpace(session.Airport))
	pilots, prefiles, changedCount := 0, 0, 0
	for _, flight := range snapshot.Flights() {
		if flight.Callsign == "" || (strings.ToUpper(strings.TrimSpace(flight.FlightPlan.Origin)) != airport && strings.ToUpper(strings.TrimSpace(flight.FlightPlan.Destination)) != airport) {
			continue
		}
		relevant[flight.Callsign] = flight
		relevantFlights = append(relevantFlights, flight)
		if flight.Prefile() {
			prefiles++
		} else {
			pilots++
		}
		strip := existing[flight.Callsign]
		created := false
		if strip == nil {
			strip = r.newStrip(session, flight, snapshot.Timestamp, nextSequence(strips, flight, airport))
			if err := r.strips.Create(ctx, strip); err != nil {
				return err
			}
			existing[flight.Callsign] = strip
			strips = append(strips, strip)
			created = true
		}
		changed := created
		if !created && r.applyFlight(strip, flight, snapshot.Timestamp) {
			if _, err := r.strips.UpdateVatsimSource(ctx, session.ID, strip.Callsign, vatsimSource(flight, snapshot.Timestamp)); err != nil {
				return err
			}
			changed = true
		}
		if !created {
			bay := apiDepartureBay(strip, flight, airport)
			if bay != "" && bay != strip.Bay {
				sequence := nextSequenceForBay(strips, bay)
				if _, err := r.strips.UpdateBayAndSequence(ctx, session.ID, strip.Callsign, bay, sequence); err != nil {
					return err
				}
				strip.Bay = bay
				strip.Sequence = &sequence
				changed = true
			}
		}
		if strings.EqualFold(strings.TrimSpace(flight.FlightPlan.Destination), airport) {
			if r.legacyArrivalETAWriter {
				etaChanged, err := r.updateArrivalETA(ctx, session.ID, strip, flight)
				if err != nil {
					return err
				}
				changed = etaChanged || changed
			}
		}
		if changed {
			changedCount++
			r.notify(session.ID, strip.Callsign)
		}
	}

	// Release lifecycle state for flights absent from this generation before
	// selecting stands for flights that are still present. Deferring cancellation
	// until the end makes newly freed capacity visible only on the next identical
	// poll, which can create a one-cycle stage-promotion move.
	for callsign, strip := range existing {
		if relevant[callsign].Callsign != "" || strip.EuroscopeSeenAt != nil {
			continue
		}
		if r.lifecycle != nil && strings.EqualFold(strings.TrimSpace(strip.Origin), airport) {
			if err := r.lifecycle.CancelDeparture(ctx, session.ID, callsign); err != nil {
				return err
			}
		}
		if r.arrivalLifecycle != nil && strings.EqualFold(strings.TrimSpace(strip.Destination), airport) {
			if err := r.arrivalLifecycle.CancelArrival(ctx, session.ID, callsign); err != nil {
				return err
			}
		}
	}

	// Apply every feed update before lifecycle processing, then establish
	// departure occupancy before selecting arrival stands. Otherwise a later
	// callsign's physical departure block is only visible to an earlier arrival
	// on the next pass over the identical snapshot, causing one-cycle churn.
	for _, flight := range relevantFlights {
		strip := existing[flight.Callsign]
		if r.lifecycle != nil && strings.EqualFold(strings.TrimSpace(flight.FlightPlan.Origin), airport) {
			// Prefiles may reserve a stand before EuroScope sees the flight, but
			// an online VATSIM record alone must not create a departure block.
			// EuroScope supplies the operational strip that makes the block
			// controller-visible and authoritative.
			prefileBeforeEuroscope := !flight.Online() && strip.EuroscopeSeenAt == nil
			onlineAfterEuroscope := flight.Online() && strip.EuroscopeSeenAt != nil
			if prefileBeforeEuroscope || onlineAfterEuroscope {
				if err := r.lifecycle.ProcessDeparture(ctx, session.ID, strip, departureFlightInfo(flight)); err != nil {
					return err
				}
			}
		}
	}
	arrivalFlights := make([]Flight, 0, len(relevantFlights))
	for _, flight := range relevantFlights {
		if strings.EqualFold(strings.TrimSpace(flight.FlightPlan.Destination), airport) {
			arrivalFlights = append(arrivalFlights, flight)
		}
	}
	orderer, orderedLifecycle := r.arrivalLifecycle.(ArrivalLifecycleOrderer)
	if orderedLifecycle {
		priorities := make(map[string]int, len(arrivalFlights))
		currentAssignments, err := r.loadArrivalAssignments(ctx, session.ID, arrivalFlights)
		if err != nil {
			return err
		}
		for _, flight := range arrivalFlights {
			priorities[flight.Callsign] = orderer.ArrivalProcessingPriority(existing[flight.Callsign], arrivalFlightInfo(flight), currentAssignments[flight.Callsign])
		}
		sort.SliceStable(arrivalFlights, func(i, j int) bool {
			left := priorities[arrivalFlights[i].Callsign]
			right := priorities[arrivalFlights[j].Callsign]
			if left != right {
				return left > right
			}
			// Refresh an existing operational commitment before introducing a
			// new reservation at the same lifecycle priority. Its latest timing
			// can make adjacent capacity unavailable in this snapshot.
			leftAssignment := currentAssignments[arrivalFlights[i].Callsign]
			rightAssignment := currentAssignments[arrivalFlights[j].Callsign]
			leftAssigned := leftAssignment != nil && strings.TrimSpace(leftAssignment.Stand) != ""
			rightAssigned := rightAssignment != nil && strings.TrimSpace(rightAssignment.Stand) != ""
			return leftAssigned && !rightAssigned
		})
	}
	if err := r.processArrivalFlights(ctx, session.ID, existing, arrivalFlights, orderedLifecycle); err != nil {
		return err
	}
	metrics.RecordSATRelevantFlights(ctx, session.Name, airport, pilots, prefiles)
	snapshotAge := time.Since(snapshot.Timestamp)
	if snapshotAge < 0 {
		snapshotAge = 0
	}
	slog.InfoContext(ctx, "SAT VATSIM reconciliation completed", slog.Int("session", int(session.ID)), slog.String("airport", airport), slog.Duration("snapshot_age", snapshotAge), slog.Int("pilots", pilots), slog.Int("prefiles", prefiles), slog.Int("changed", changedCount))

	for callsign, strip := range existing {
		if strip.VatsimCID == nil || relevant[callsign].Callsign != "" || strip.EuroscopeSeenAt != nil || r.isAssigned(ctx, session.ID, strip.Callsign) {
			continue
		}
		if err := r.strips.Delete(ctx, session.ID, strip.Callsign); err != nil {
			return err
		}
	}
	return nil
}

type arrivalAssignmentState struct {
	exists                         bool
	stand, stage, source, ruleID   string
	matchedVariant, eta, expiresAt string
	conflictReason, observedStand  string
	tier, version                  int32
	id                             int64
	hasTier, manual, acknowledged  bool
}

func (r *Reconciler) processArrivalFlights(ctx context.Context, sessionID int32, strips map[string]*models.Strip, flights []Flight, converge bool) error {
	if len(flights) == 0 {
		return nil
	}
	if !converge {
		return r.processArrivalPass(ctx, sessionID, strips, flights)
	}

	previous, err := r.arrivalAssignmentStates(ctx, sessionID, flights)
	if err != nil {
		return err
	}
	for pass := 1; pass <= maxArrivalReconciliationPasses; pass++ {
		previousStands := arrivalStripStandStates(strips, flights)
		if err := r.processArrivalPass(ctx, sessionID, strips, flights); err != nil {
			return err
		}
		current, err := r.arrivalAssignmentStates(ctx, sessionID, flights)
		if err != nil {
			return err
		}
		currentStands, err := r.reloadArrivalStripStands(ctx, sessionID, strips, flights)
		if err != nil {
			return err
		}
		if maps.Equal(previous, current) && maps.Equal(previousStands, currentStands) {
			metrics.RecordSATReconciliationPasses(ctx, pass, false)
			if pass > 1 {
				slog.DebugContext(ctx, "SAT arrival reconciliation converged",
					slog.Int("session", int(sessionID)), slog.Int("passes", pass), slog.Int("arrivals", len(flights)))
			}
			return nil
		}
		previous = current
	}
	metrics.RecordSATReconciliationPasses(ctx, maxArrivalReconciliationPasses, true)
	return fmt.Errorf("SAT arrival reconciliation reached the fixed cap of %d passes", maxArrivalReconciliationPasses)
}

func arrivalStripStandStates(strips map[string]*models.Strip, flights []Flight) map[string]string {
	states := make(map[string]string, len(flights))
	for _, flight := range flights {
		strip := strips[flight.Callsign]
		if strip != nil && strip.Stand != nil {
			states[flight.Callsign] = strings.TrimSpace(*strip.Stand)
		} else {
			states[flight.Callsign] = ""
		}
	}
	return states
}

func (r *Reconciler) reloadArrivalStripStands(ctx context.Context, sessionID int32, strips map[string]*models.Strip, flights []Flight) (map[string]string, error) {
	committed, err := r.strips.List(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("reload committed arrival strips: %w", err)
	}
	for _, strip := range committed {
		if strip != nil {
			strips[normalizeCallsign(strip.Callsign)] = strip
		}
	}
	return arrivalStripStandStates(strips, flights), nil
}

func (r *Reconciler) processArrivalPass(ctx context.Context, sessionID int32, strips map[string]*models.Strip, flights []Flight) error {
	for _, flight := range flights {
		strip := strips[flight.Callsign]
		if err := r.arrivalLifecycle.ProcessArrival(ctx, sessionID, strip, arrivalFlightInfo(flight)); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) arrivalAssignmentStates(ctx context.Context, sessionID int32, flights []Flight) (map[string]arrivalAssignmentState, error) {
	assignments, err := r.loadArrivalAssignments(ctx, sessionID, flights)
	if err != nil {
		return nil, err
	}
	states := make(map[string]arrivalAssignmentState, len(flights))
	for _, flight := range flights {
		states[flight.Callsign] = snapshotArrivalAssignment(assignments[flight.Callsign])
	}
	return states, nil
}

func (r *Reconciler) loadArrivalAssignments(ctx context.Context, sessionID int32, flights []Flight) (map[string]*models.StandAssignment, error) {
	result := make(map[string]*models.StandAssignment, len(flights))
	if lister, ok := r.assignments.(reconciliationAssignmentLister); ok {
		assignments, err := lister.ListAssignments(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("list arrival assignments: %w", err)
		}
		for _, assignment := range assignments {
			if assignment != nil {
				result[normalizeCallsign(assignment.Callsign)] = assignment
			}
		}
		return result, nil
	}
	for _, flight := range flights {
		assignment, err := r.assignments.GetAssignment(ctx, sessionID, flight.Callsign)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("load arrival assignment state for %s: %w", flight.Callsign, err)
		}
		if assignment != nil {
			result[normalizeCallsign(flight.Callsign)] = assignment
		}
	}
	return result, nil
}

func snapshotArrivalAssignment(assignment *models.StandAssignment) arrivalAssignmentState {
	if assignment == nil {
		return arrivalAssignmentState{}
	}
	state := arrivalAssignmentState{
		exists: assignment.ID != 0 || assignment.Stand != "" || assignment.Stage != "",
		id:     assignment.ID,
		stand:  assignment.Stand, stage: assignment.Stage, source: assignment.Source,
		ruleID: valueOrEmpty(assignment.RuleID), matchedVariant: valueOrEmpty(assignment.MatchedVariant),
		eta: timeOrEmpty(assignment.ETA), expiresAt: timeOrEmpty(assignment.ExpiresAt),
		conflictReason: valueOrEmpty(assignment.ConflictReason), observedStand: valueOrEmpty(assignment.ObservedStand),
		manual: assignment.Manual, acknowledged: assignment.Acknowledged, version: assignment.Version,
	}
	if assignment.Tier != nil {
		state.tier, state.hasTier = *assignment.Tier, true
	}
	return state
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func timeOrEmpty(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func (r *Reconciler) updateArrivalETA(ctx context.Context, session int32, strip *models.Strip, flight Flight) (bool, error) {
	now := r.now()
	candidate, ok := calculateArrivalETA(now, flight, r.airportCoordinates)
	if !ok {
		// VATSIM can temporarily omit timing or movement fields. Keep the last
		// accepted estimate rather than replacing a useful stable ETA with none.
		return false, nil
	}
	accepted, changed := acceptedArrivalETA(strip.ArrivalETA, candidate)
	if !changed {
		return false, nil
	}
	if _, err := r.strips.UpdateArrivalETA(ctx, session, strip.Callsign, accepted); err != nil {
		return false, err
	}
	strip.ArrivalETA = &accepted
	return true, nil
}

func vatsimSource(flight Flight, snapshotTime time.Time) models.VatsimStripSource {
	seenAt := flight.LastUpdated
	if seenAt.IsZero() {
		seenAt = snapshotTime
	}
	return models.VatsimStripSource{
		CID: flight.CID, Revision: flight.FlightPlan.Revision, SeenAt: seenAt.UTC(),
		Origin: flight.FlightPlan.Origin, Destination: flight.FlightPlan.Destination,
		Alternate: flight.FlightPlan.Alternate, Route: flight.FlightPlan.Route,
		Remarks: flight.FlightPlan.Remarks, AssignedSquawk: flight.FlightPlan.AssignedSquawk,
		AircraftType: flight.FlightPlan.AircraftShort, Online: flight.Online(),
		Latitude: flight.Latitude, Longitude: flight.Longitude, Altitude: int32(flight.Altitude),
	}
}

func (r *Reconciler) newStrip(session *models.Session, flight Flight, seenAt time.Time, sequence int32) *models.Strip {
	bay := hiddenArrivalBay
	if strings.EqualFold(strings.TrimSpace(flight.FlightPlan.Origin), strings.TrimSpace(session.Airport)) {
		bay = hiddenDepartureBay
	}
	strip := &models.Strip{Callsign: flight.Callsign, Session: session.ID, Bay: bay, Sequence: &sequence, HasFP: true}
	r.applyFlight(strip, flight, seenAt)
	return strip
}

func (r *Reconciler) applyFlight(strip *models.Strip, flight Flight, snapshotTime time.Time) bool {
	seenAt := flight.LastUpdated
	if seenAt.IsZero() {
		seenAt = snapshotTime
	}
	seenAt = seenAt.UTC()
	changed := false
	if strip.VatsimCID == nil || *strip.VatsimCID != flight.CID {
		strip.VatsimCID = ptr(flight.CID)
		changed = true
	}
	if strip.VatsimRevision == nil || *strip.VatsimRevision != flight.FlightPlan.Revision {
		strip.VatsimRevision = int64ptr(flight.FlightPlan.Revision)
		changed = true
	}
	if strip.VatsimSeenAt == nil || strip.VatsimSeenAt.Before(seenAt) {
		strip.VatsimSeenAt = timeptr(seenAt)
		changed = true
	}
	if strip.EuroscopeSeenAt != nil && strip.EuroscopeSeenAt.After(seenAt) {
		return changed
	}

	changed = setSourceString(strip, "origin", &strip.Origin, flight.FlightPlan.Origin) || changed
	changed = setSourceString(strip, "destination", &strip.Destination, flight.FlightPlan.Destination) || changed
	changed = setSourcePointer(strip, "alternative", &strip.Alternative, flight.FlightPlan.Alternate) || changed
	changed = setSourcePointer(strip, "route", &strip.Route, flight.FlightPlan.Route) || changed
	changed = setSourcePointer(strip, "remarks", &strip.Remarks, flight.FlightPlan.Remarks) || changed
	changed = setSourcePointer(strip, "assigned_squawk", &strip.AssignedSquawk, flight.FlightPlan.AssignedSquawk) || changed
	changed = setSourcePointer(strip, "aircraft_type", &strip.AircraftType, flight.FlightPlan.AircraftShort) || changed
	if flight.Online() {
		changed = setSourceFloat(strip, "position_latitude", &strip.PositionLatitude, flight.Latitude) || changed
		changed = setSourceFloat(strip, "position_longitude", &strip.PositionLongitude, flight.Longitude) || changed
		changed = setSourceInt(strip, "position_altitude", &strip.PositionAltitude, int32(flight.Altitude)) || changed
	}
	return changed
}

// RetainsStrip reports whether VATSIM or SAT is still responsible for keeping
// a strip alive after EuroScope disconnects it.
func (r *Reconciler) RetainsStrip(ctx context.Context, session int32, callsign string) bool {
	if _, ok := r.cache.Snapshot().FlightByCallsign(callsign); ok {
		return true
	}
	return r.isAssigned(ctx, session, callsign)
}

func (r *Reconciler) isAssigned(ctx context.Context, session int32, callsign string) bool {
	assignment, err := r.assignments.GetAssignment(ctx, session, callsign)
	if err != nil || assignment == nil {
		return false
	}
	// An automatic ESTIMATED arrival is advisory only. It must not retain an
	// API-created strip forever after the flight disappears.
	if !assignment.Manual &&
		strings.EqualFold(assignment.Direction, "ARRIVAL") &&
		strings.EqualFold(assignment.Stage, "ESTIMATED") {
		return false
	}
	if assignment.ExpiresAt == nil {
		return true
	}
	now := r.now()
	return assignment.ExpiresAt.After(now)
}

func departureFlightInfo(flight Flight) DepartureFlightInfo {
	return DepartureFlightInfo{
		Callsign:     flight.Callsign,
		CID:          flight.CID,
		Online:       flight.Online(),
		Revision:     flight.FlightPlan.Revision,
		Origin:       flight.FlightPlan.Origin,
		Destination:  flight.FlightPlan.Destination,
		AircraftType: flight.FlightPlan.AircraftShort,
		Latitude:     flight.Latitude,
		Longitude:    flight.Longitude,
	}
}

func arrivalFlightInfo(flight Flight) ArrivalFlightInfo {
	return ArrivalFlightInfo{
		Callsign:     flight.Callsign,
		CID:          flight.CID,
		Online:       flight.Online(),
		Revision:     flight.FlightPlan.Revision,
		Origin:       flight.FlightPlan.Origin,
		Destination:  flight.FlightPlan.Destination,
		AircraftType: flight.FlightPlan.AircraftShort,
	}
}

func (r *Reconciler) notify(session int32, callsign string) {
	r.notifier.SendStripUpdate(session, callsign)
}

func nextSequence(strips []*models.Strip, flight Flight, airport string) int32 {
	bay := hiddenArrivalBay
	if strings.EqualFold(strings.TrimSpace(flight.FlightPlan.Origin), airport) {
		bay = hiddenDepartureBay
	}
	return nextSequenceForBay(strips, bay)
}

// apiDepartureBay keeps departures known only through the VATSIM API out of all
// operational bays. Once EuroScope has supplied the strip, its operational bay
// wins and the reconciler no longer changes it.
func apiDepartureBay(strip *models.Strip, flight Flight, airport string) string {
	if strip == nil || strip.EuroscopeSeenAt != nil ||
		!strings.EqualFold(strings.TrimSpace(flight.FlightPlan.Origin), airport) {
		return ""
	}
	if strip.Bay != hiddenDepartureBay {
		return hiddenDepartureBay
	}
	return ""
}

func nextSequenceForBay(strips []*models.Strip, bay string) int32 {
	max := int32(0)
	for _, strip := range strips {
		if strip != nil && strip.Bay == bay && strip.Sequence != nil && *strip.Sequence > max {
			max = *strip.Sequence
		}
	}
	return max + reconciliationSequenceSpacing
}

func controllerModified(strip *models.Strip, field string) bool {
	for _, modified := range strip.ControllerModifiedFields {
		if strings.EqualFold(strings.TrimSpace(modified), field) {
			return true
		}
	}
	return false
}

func setSourceString(strip *models.Strip, field string, destination *string, value string) bool {
	if controllerModified(strip, field) || *destination == value {
		return false
	}
	*destination = value
	return true
}

func setSourcePointer(strip *models.Strip, field string, destination **string, value string) bool {
	if controllerModified(strip, field) || (*destination != nil && **destination == value) {
		return false
	}
	*destination = ptr(value)
	return true
}

func setSourceFloat(strip *models.Strip, field string, destination **float64, value float64) bool {
	if controllerModified(strip, field) || (*destination != nil && **destination == value) {
		return false
	}
	*destination = &value
	return true
}

func setSourceInt(strip *models.Strip, field string, destination **int32, value int32) bool {
	if controllerModified(strip, field) || (*destination != nil && **destination == value) {
		return false
	}
	*destination = &value
	return true
}

func ptr(value string) *string           { return &value }
func int64ptr(value int64) *int64        { return &value }
func timeptr(value time.Time) *time.Time { return &value }
