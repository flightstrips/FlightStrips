package vatsim

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/metrics"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const defaultObservationStaleAfter = time.Minute

// ObservationWorkerDependencies are deliberately separate from Reconciler.
// AMAN consumes source facts before a strip, Stand Assignment, session, or
// EuroScope controller exists.
type ObservationWorkerDependencies struct {
	Cache           SnapshotSource
	Identities      aman.VATSIMFlightIdentityBinder
	Sink            aman.ObservationSink
	EnabledAirports []string
	StaleAfter      time.Duration
	Now             func() time.Time
}

// ObservationWorker maps immutable VATSIM cache snapshots into the neutral
// AMAN observation contract. It owns no prediction, sequencing, strip, or SAT
// policy.
type ObservationWorker struct {
	cache      SnapshotSource
	identities aman.VATSIMFlightIdentityBinder
	sink       aman.ObservationSink
	airports   map[string]struct{}
	staleAfter time.Duration
	now        func() time.Time
	known      map[string]aman.FlightObservation
}

func NewObservationWorker(deps ObservationWorkerDependencies) (*ObservationWorker, error) {
	if deps.Cache == nil {
		return nil, fmt.Errorf("AMAN observation worker requires VATSIM cache")
	}
	if deps.Identities == nil {
		return nil, fmt.Errorf("AMAN observation worker requires flight identity binder")
	}
	if deps.Sink == nil {
		return nil, fmt.Errorf("AMAN observation worker requires observation sink")
	}
	airports := make(map[string]struct{}, len(deps.EnabledAirports))
	for _, value := range deps.EnabledAirports {
		airport := strings.ToUpper(strings.TrimSpace(value))
		if airport != "" {
			airports[airport] = struct{}{}
		}
	}
	if len(airports) == 0 {
		return nil, fmt.Errorf("AMAN observation worker requires enabled airports")
	}
	if deps.StaleAfter <= 0 {
		deps.StaleAfter = defaultObservationStaleAfter
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &ObservationWorker{
		cache: deps.Cache, identities: deps.Identities, sink: deps.Sink, airports: airports,
		staleAfter: deps.StaleAfter, now: deps.Now, known: make(map[string]aman.FlightObservation),
	}, nil
}

// Run is intentionally independent of the strip reconciler loop. Delivery
// errors are logged and retried on the next source interval; they cannot stop
// Stand Assignment or strip reconciliation.
func (w *ObservationWorker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultRefreshInterval
	}
	if err := w.Publish(ctx); err != nil {
		slog.WarnContext(ctx, "AMAN VATSIM observation publication failed", slog.Any("error", err))
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.Publish(ctx); err != nil {
				slog.WarnContext(ctx, "AMAN VATSIM observation publication failed", slog.Any("error", err))
			}
		}
	}
}

// Publish sends changes from one cache generation. Repeated unchanged source
// health states are not re-emitted, while fresh, stale, disconnected, and
// restored transitions are all delivered with their latest source facts.
func (w *ObservationWorker) Publish(ctx context.Context) error {
	snapshot := w.cache.Snapshot()
	now := w.now().UTC()
	status := observationSourceStatus(snapshot, now, w.staleAfter)
	metrics.RecordAMANObservation(ctx, now.Sub(snapshot.Timestamp), string(status))
	metrics.RecordAMANSourceRefresh(ctx, "vatsim", string(status))
	current := make(map[string]aman.FlightObservation)
	failed := make(map[string]struct{})
	var publishErrors []error
	if healthSink, ok := w.sink.(aman.ObservationSourceHealthSink); ok {
		if err := healthSink.ObserveSourceHealth(ctx, status, now); err != nil {
			publishErrors = append(publishErrors, fmt.Errorf("publish VATSIM source health: %w", err))
		}
	}
	if status != aman.DataDisconnected && !snapshot.Timestamp.IsZero() {
		for _, flight := range snapshot.Flights() {
			if _, enabled := w.airports[strings.ToUpper(strings.TrimSpace(flight.FlightPlan.Destination))]; !enabled {
				continue
			}
			previous, known := w.known[flight.CID]
			observation, err := w.mapFlight(ctx, flight, snapshot.Timestamp, status, now, optionalObservation(previous, known))
			if err != nil {
				failed[flight.CID] = struct{}{}
				publishErrors = append(publishErrors, fmt.Errorf("map VATSIM observation for CID %s: %w", flight.CID, err))
				continue
			}
			if known {
				observation = preserveNewerObservationFacts(previous, observation)
			}
			current[flight.CID] = observation
		}
	}
	if status == aman.DataDisconnected {
		for cid, observation := range w.known {
			observation.SourceStatus = status
			observation.ReconciledAt = now
			current[cid] = observation
		}
	}

	for cid, observation := range current {
		if previous, exists := w.known[cid]; exists && sameObservation(previous, observation) {
			continue
		}
		if err := w.sink.Observe(ctx, observation); err != nil {
			publishErrors = append(publishErrors, fmt.Errorf("publish VATSIM observation for CID %s: %w", cid, err))
			continue
		}
		w.known[cid] = observation
	}
	if status != aman.DataDisconnected {
		for cid := range w.known {
			if _, mappingFailed := failed[cid]; mappingFailed {
				continue
			}
			if _, present := current[cid]; !present {
				missing := w.known[cid]
				missing.Missing = true
				missing.SourceStatus = status
				missing.ReconciledAt = now
				if err := w.sink.Observe(ctx, missing); err != nil {
					publishErrors = append(publishErrors, fmt.Errorf("publish missing VATSIM observation for CID %s: %w", cid, err))
					continue
				}
				delete(w.known, cid)
			}
		}
	}
	return errors.Join(publishErrors...)
}

func (w *ObservationWorker) mapFlight(ctx context.Context, flight Flight, snapshotAt time.Time, status aman.DataStatus, reconciledAt time.Time, previous *aman.FlightObservation) (aman.FlightObservation, error) {
	identity := aman.VATSIMFlightIdentity{VATSIMCID: strings.TrimSpace(flight.CID), CurrentCallsign: strings.TrimSpace(flight.Callsign)}
	flightID := aman.FlightID("")
	if previous != nil && previous.Callsign == identity.CurrentCallsign {
		flightID = previous.FlightID
	} else {
		var err error
		flightID, err = w.identities.BindVATSIMFlight(ctx, identity)
		if err != nil {
			return aman.FlightObservation{}, fmt.Errorf("bind VATSIM flight identity: %w", err)
		}
	}
	observedAt := flight.LastUpdated.UTC()
	if observedAt.IsZero() {
		observedAt = snapshotAt.UTC()
	}
	observation := aman.FlightObservation{
		FlightID: flightID, VATSIMCID: identity.VATSIMCID, Callsign: identity.CurrentCallsign,
		Origin: strings.ToUpper(strings.TrimSpace(flight.FlightPlan.Origin)), Destination: strings.ToUpper(strings.TrimSpace(flight.FlightPlan.Destination)),
		AircraftType: optionalString(flight.FlightPlan.AircraftShort), WakeCategory: wakeCategory(flight.FlightPlan.Aircraft),
		FiledRoute: optionalString(flight.FlightPlan.Route), RequestedLevel: requestedLevelFeet(flight.FlightPlan.RequestedLevel),
		PlannedTiming: plannedTiming(observedAt, flight.FlightPlan), FlightPlan: flightPlanFact(flight.FlightPlan.Revision, observedAt),
		Surveillance: surveillanceFact(flight, observedAt, previous), TakeoffDetected: takeoffDetected(flight, observedAt),
		ReconciledAt: reconciledAt, SourceStatus: status,
	}
	if err := observation.Validate(); err != nil {
		return aman.FlightObservation{}, fmt.Errorf("map VATSIM observation: %w", err)
	}
	return observation, nil
}

func observationSourceStatus(snapshot Snapshot, now time.Time, staleAfter time.Duration) aman.DataStatus {
	if snapshot.Timestamp.IsZero() || snapshot.LastRefreshError != nil {
		return aman.DataDisconnected
	}
	if now.Sub(snapshot.Timestamp) > staleAfter {
		return aman.DataStale
	}
	return aman.DataFresh
}

func flightPlanFact(revision int64, observedAt time.Time) aman.FlightPlanFact {
	var sourceRevision *uint64
	if revision >= 0 {
		value := uint64(revision)
		sourceRevision = &value
	}
	return aman.FlightPlanFact{Revision: sourceRevision, ObservedAt: &observedAt}
}

func surveillanceFact(flight Flight, observedAt time.Time, previous *aman.FlightObservation) *aman.SurveillanceFact {
	if !flight.Online() || !validCoordinates(flight.Latitude, flight.Longitude) {
		return nil
	}
	altitude := flight.Altitude
	groundspeed := float64(flight.Groundspeed)
	sequence := uint64(observedAt.UnixMilli())
	fact := &aman.SurveillanceFact{
		LatitudeDegrees: flight.Latitude, LongitudeDegrees: flight.Longitude, AltitudeFeet: &altitude,
		GroundspeedKnots: &groundspeed, Sequence: &sequence, ObservedAt: &observedAt,
	}
	if track, ok := derivedGroundTrack(previous, fact); ok {
		fact.TrackTrueDegrees = &track
	}
	return fact
}

func derivedGroundTrack(previous *aman.FlightObservation, current *aman.SurveillanceFact) (float64, bool) {
	if previous == nil || previous.Surveillance == nil || previous.Surveillance.ObservedAt == nil || current == nil || current.ObservedAt == nil || !current.ObservedAt.After(*previous.Surveillance.ObservedAt) {
		return 0, false
	}
	from := previous.Surveillance
	if from.LatitudeDegrees == current.LatitudeDegrees && from.LongitudeDegrees == current.LongitudeDegrees {
		return 0, false
	}
	lat1, lat2 := from.LatitudeDegrees*math.Pi/180, current.LatitudeDegrees*math.Pi/180
	dLon := (current.LongitudeDegrees - from.LongitudeDegrees) * math.Pi / 180
	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360), true
}

func takeoffDetected(flight Flight, observedAt time.Time) *time.Time {
	if flight.Online() && flight.Altitude >= minimumLiveAltitude && flight.Groundspeed >= minimumLiveGroundspeed {
		return &observedAt
	}
	return nil
}

func plannedTiming(now time.Time, plan FlightPlan) *aman.PlannedTiming {
	departure, ok := plannedOffBlockTime(now, plan.EOBT)
	if !ok {
		return nil
	}
	duration, err := parseDuration(plan.EnrouteDuration)
	if err != nil {
		return &aman.PlannedTiming{EstimatedOffBlockTime: &departure}
	}
	return &aman.PlannedTiming{EstimatedOffBlockTime: &departure, EstimatedEnrouteTime: &duration}
}

func plannedOffBlockTime(now time.Time, eobt string) (time.Time, bool) {
	hour, minute, err := parseClock(eobt)
	if err != nil {
		return time.Time{}, false
	}
	now = now.UTC()
	candidates := []time.Time{
		time.Date(now.Year(), now.Month(), now.Day()-1, hour, minute, 0, 0, time.UTC),
		time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC),
		time.Date(now.Year(), now.Month(), now.Day()+1, hour, minute, 0, 0, time.UTC),
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if absoluteDuration(candidate.Sub(now)) < absoluteDuration(best.Sub(now)) {
			best = candidate
		}
	}
	return best, true
}

func wakeCategory(aircraft string) *string {
	_, equipment, found := strings.Cut(strings.ToUpper(strings.TrimSpace(aircraft)), "/")
	if !found || equipment == "" {
		return nil
	}
	value := equipment[:1]
	switch value {
	case "L", "M", "H", "J":
		return &value
	default:
		return nil
	}
}

func requestedLevelFeet(value string) *int {
	value = strings.ToUpper(strings.TrimSpace(value))
	multiplier := 1
	switch {
	case strings.HasPrefix(value, "FL"):
		value, multiplier = strings.TrimPrefix(value, "FL"), 100
	case strings.HasPrefix(value, "F"):
		value, multiplier = strings.TrimPrefix(value, "F"), 100
	}
	level, err := strconv.Atoi(value)
	if err != nil || level <= 0 {
		return nil
	}
	feet := level * multiplier
	if feet < 1000 || feet > 60000 {
		return nil
	}
	return &feet
}

func preserveNewerObservationFacts(previous, next aman.FlightObservation) aman.FlightObservation {
	if flightPlanIsOlder(previous.FlightPlan, next.FlightPlan) {
		next.Origin, next.Destination = previous.Origin, previous.Destination
		next.AircraftType, next.WakeCategory, next.FiledRoute = previous.AircraftType, previous.WakeCategory, previous.FiledRoute
		next.RequestedLevel, next.PlannedTiming, next.FlightPlan = previous.RequestedLevel, previous.PlannedTiming, previous.FlightPlan
	}
	if surveillanceIsOlder(previous.Surveillance, next.Surveillance) {
		next.Surveillance, next.TakeoffDetected = previous.Surveillance, previous.TakeoffDetected
	}
	if previous.TakeoffDetected != nil && (next.TakeoffDetected == nil || previous.TakeoffDetected.Before(*next.TakeoffDetected)) {
		takeoff := *previous.TakeoffDetected
		next.TakeoffDetected = &takeoff
	}
	return next
}

func flightPlanIsOlder(previous, next aman.FlightPlanFact) bool {
	if previous.Revision != nil && next.Revision != nil {
		if *next.Revision != *previous.Revision {
			return *next.Revision < *previous.Revision
		}
	}
	return previous.ObservedAt != nil && next.ObservedAt != nil && next.ObservedAt.Before(*previous.ObservedAt)
}

func surveillanceIsOlder(previous, next *aman.SurveillanceFact) bool {
	if previous == nil || next == nil {
		return false
	}
	if previous.Sequence != nil && next.Sequence != nil && *next.Sequence < *previous.Sequence {
		return true
	}
	return previous.ObservedAt != nil && next.ObservedAt != nil && next.ObservedAt.Before(*previous.ObservedAt)
}

func sameObservation(previous, next aman.FlightObservation) bool {
	previous.ReconciledAt = time.Time{}
	next.ReconciledAt = time.Time{}
	return reflect.DeepEqual(previous, next)
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalObservation(value aman.FlightObservation, ok bool) *aman.FlightObservation {
	if !ok {
		return nil
	}
	return &value
}
