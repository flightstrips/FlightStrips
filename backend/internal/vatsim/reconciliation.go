package vatsim

import (
	"FlightStrips/internal/models"
	"context"
	"strings"
	"time"
)

const reconciliationSequenceSpacing int32 = 1000

const (
	plannedDepartureBay = "NOT_CLEARED"
	hiddenArrivalBay    = "ARR_HIDDEN"
)

type reconciliationSessionStore interface {
	List(context.Context) ([]*models.Session, error)
}

type reconciliationStripStore interface {
	List(context.Context, int32) ([]*models.Strip, error)
	Create(context.Context, *models.Strip) error
	UpdateVatsimSource(context.Context, int32, string, models.VatsimStripSource) (int64, error)
	UpdateArrivalETA(context.Context, int32, string, models.ArrivalETA) (int64, error)
	Delete(context.Context, int32, string) error
}

type reconciliationAssignmentStore interface {
	GetAssignment(context.Context, int32, string) (*models.StandAssignment, error)
}

type reconciliationNotifier interface {
	SendStripUpdate(session int32, callsign string)
}

// Reconciler materializes relevant VATSIM records into each FlightStrips
// session. It deliberately owns only feed fields; operational state remains
// controller/EuroScope owned.
type Reconciler struct {
	cache              *Cache
	sessions           reconciliationSessionStore
	strips             reconciliationStripStore
	assignments        reconciliationAssignmentStore
	notifier           reconciliationNotifier
	interval           time.Duration
	airportCoordinates AirportCoordinates
	now                func() time.Time
}

func NewReconciler(cache *Cache, sessions reconciliationSessionStore, strips reconciliationStripStore, assignments reconciliationAssignmentStore, notifier reconciliationNotifier, interval time.Duration, options ...ArrivalETAOption) *Reconciler {
	if interval <= 0 {
		interval = defaultRefreshInterval
	}
	reconciler := &Reconciler{cache: cache, sessions: sessions, strips: strips, assignments: assignments, notifier: notifier, interval: interval, now: time.Now}
	for _, option := range options {
		option(reconciler)
	}
	return reconciler
}

func (r *Reconciler) Start(ctx context.Context) {
	if r == nil || r.cache == nil || r.sessions == nil || r.strips == nil {
		return
	}
	_ = r.Reconcile(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = r.Reconcile(ctx)
		}
	}
}

// Reconcile performs one source snapshot reconciliation. Sessions are listed
// directly instead of deriving them from EuroScope clients, so it also works
// before a tower or EuroScope master connects.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	if r == nil || r.cache == nil || r.sessions == nil || r.strips == nil {
		return nil
	}
	snapshot := r.cache.Snapshot()
	if snapshot.Timestamp.IsZero() {
		return nil
	}
	sessions, err := r.sessions.List(ctx)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session == nil || strings.TrimSpace(session.Airport) == "" {
			continue
		}
		if err := r.reconcileSession(ctx, snapshot, session); err != nil {
			return err
		}
	}
	return nil
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
	airport := strings.ToUpper(strings.TrimSpace(session.Airport))
	for _, flight := range snapshot.Flights() {
		if flight.Callsign == "" || (strings.ToUpper(strings.TrimSpace(flight.FlightPlan.Origin)) != airport && strings.ToUpper(strings.TrimSpace(flight.FlightPlan.Destination)) != airport) {
			continue
		}
		relevant[flight.Callsign] = flight
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
		if strings.EqualFold(strings.TrimSpace(flight.FlightPlan.Destination), airport) {
			etaChanged, err := r.updateArrivalETA(ctx, session.ID, strip, flight)
			if err != nil {
				return err
			}
			changed = etaChanged || changed
		}
		if changed {
			r.notify(session.ID, strip.Callsign)
		}
	}

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

func (r *Reconciler) updateArrivalETA(ctx context.Context, session int32, strip *models.Strip, flight Flight) (bool, error) {
	now := time.Now()
	if r.now != nil {
		now = r.now()
	}
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
		bay = plannedDepartureBay
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
	if r != nil && r.cache != nil {
		if _, ok := r.cache.Snapshot().FlightByCallsign(callsign); ok {
			return true
		}
	}
	return r != nil && r.isAssigned(ctx, session, callsign)
}

func (r *Reconciler) isAssigned(ctx context.Context, session int32, callsign string) bool {
	if r.assignments == nil {
		return false
	}
	assignment, err := r.assignments.GetAssignment(ctx, session, callsign)
	return err == nil && assignment != nil
}

func (r *Reconciler) notify(session int32, callsign string) {
	if r.notifier != nil {
		r.notifier.SendStripUpdate(session, callsign)
	}
}

func nextSequence(strips []*models.Strip, flight Flight, airport string) int32 {
	bay := hiddenArrivalBay
	if strings.EqualFold(strings.TrimSpace(flight.FlightPlan.Origin), airport) {
		bay = plannedDepartureBay
	}
	return nextSequenceForBay(strips, bay)
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
