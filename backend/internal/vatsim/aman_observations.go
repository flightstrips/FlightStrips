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
	Sessions        reconciliationSessionStore
	Sink            aman.ObservationSink
	EnabledAirports []string
	StaleAfter      time.Duration
	Now             func() time.Time
	OfflineReplay   bool
}

// ObservationWorker maps immutable VATSIM cache snapshots into the neutral
// AMAN observation contract. It owns no prediction, sequencing, strip, or SAT
// policy.
type ObservationWorker struct {
	cache         SnapshotSource
	sessions      reconciliationSessionStore
	sink          aman.ObservationSink
	airports      map[string]struct{}
	staleAfter    time.Duration
	now           func() time.Time
	offlineReplay bool
	known         map[string]aman.FlightObservation
}

func NewObservationWorker(deps ObservationWorkerDependencies) (*ObservationWorker, error) {
	if deps.Cache == nil {
		return nil, fmt.Errorf("AMAN observation worker requires VATSIM cache")
	}
	if deps.Sessions == nil {
		return nil, fmt.Errorf("AMAN observation worker requires session store")
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
		cache: deps.Cache, sessions: deps.Sessions, sink: deps.Sink, airports: airports,
		staleAfter: deps.StaleAfter, now: deps.Now, offlineReplay: deps.OfflineReplay, known: make(map[string]aman.FlightObservation),
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
	liveAirports, err := w.liveAirports(ctx)
	if err != nil {
		return fmt.Errorf("list live sessions for AMAN VATSIM observations: %w", err)
	}
	if len(liveAirports) == 0 {
		return w.retractKnown(ctx)
	}
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
			destination := strings.ToUpper(strings.TrimSpace(flight.FlightPlan.Destination))
			if _, enabled := liveAirports[destination]; !enabled {
				continue
			}
			callsign := strings.ToUpper(strings.TrimSpace(flight.Callsign))
			previous, known := w.known[callsign]
			if known && previous.Destination != destination {
				missing := previous
				missing.Missing = true
				missing.SourceStatus = status
				missing.ReconciledAt = now
				if err := w.sink.Observe(ctx, missing); err != nil {
					failed[callsign] = struct{}{}
					publishErrors = append(publishErrors, fmt.Errorf("retract VATSIM observation for callsign %s from %s: %w", callsign, previous.Destination, err))
					continue
				}
				delete(w.known, callsign)
				known = false
			}
			sameSource := known
			observation, err := w.mapFlight(ctx, flight, snapshot.Timestamp, status, now, optionalObservation(previous, sameSource))
			if err != nil {
				failed[callsign] = struct{}{}
				publishErrors = append(publishErrors, fmt.Errorf("map VATSIM observation for callsign %s: %w", callsign, err))
				continue
			}
			if sameSource {
				observation = preserveNewerObservationFacts(previous, observation)
			}
			current[callsign] = observation
		}
	}
	if status == aman.DataDisconnected {
		for callsign, observation := range w.known {
			observation.SourceStatus = status
			observation.ReconciledAt = now
			current[callsign] = observation
		}
	}

	for callsign, observation := range current {
		if previous, exists := w.known[callsign]; exists && sameObservation(previous, observation) {
			continue
		}
		if err := w.sink.Observe(ctx, observation); err != nil {
			publishErrors = append(publishErrors, fmt.Errorf("publish VATSIM observation for callsign %s: %w", observation.Callsign, err))
			continue
		}
		w.known[callsign] = observation
	}
	if status != aman.DataDisconnected {
		for callsign := range w.known {
			if _, stillLive := liveAirports[strings.ToUpper(strings.TrimSpace(w.known[callsign].Destination))]; !stillLive {
				missing := w.known[callsign]
				missing.Missing = true
				missing.SourceStatus = aman.DataFresh
				missing.ReconciledAt = now
				if err := w.sink.Observe(ctx, missing); err != nil {
					publishErrors = append(publishErrors, fmt.Errorf("retract VATSIM observation for callsign %s: %w", callsign, err))
					continue
				}
				delete(w.known, callsign)
				continue
			}
			if _, mappingFailed := failed[callsign]; mappingFailed {
				continue
			}
			if _, present := current[callsign]; !present {
				missing := w.known[callsign]
				missing.Missing = true
				missing.SourceStatus = status
				missing.ReconciledAt = now
				if err := w.sink.Observe(ctx, missing); err != nil {
					publishErrors = append(publishErrors, fmt.Errorf("publish missing VATSIM observation for callsign %s: %w", missing.Callsign, err))
					continue
				}
				delete(w.known, callsign)
			}
		}
	}
	return errors.Join(publishErrors...)
}

func (w *ObservationWorker) liveAirports(ctx context.Context) (map[string]struct{}, error) {
	if w.offlineReplay {
		result := make(map[string]struct{}, len(w.airports))
		for airport := range w.airports {
			result[airport] = struct{}{}
		}
		return result, nil
	}
	sessions, err := w.sessions.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{})
	for _, session := range sessions {
		if !isLiveSession(session) {
			continue
		}
		airport := strings.ToUpper(strings.TrimSpace(session.Airport))
		if _, enabled := w.airports[airport]; enabled {
			result[airport] = struct{}{}
		}
	}
	return result, nil
}

func (w *ObservationWorker) retractKnown(ctx context.Context) error {
	now := w.now().UTC()
	var publishErrors []error
	if healthSink, ok := w.sink.(aman.ObservationSourceHealthSink); ok {
		if err := healthSink.ObserveSourceHealth(ctx, aman.DataDisconnected, now); err != nil {
			publishErrors = append(publishErrors, fmt.Errorf("publish VATSIM source health: %w", err))
		}
	}
	for callsign, observation := range w.known {
		observation.Missing = true
		observation.SourceStatus = aman.DataDisconnected
		observation.ReconciledAt = now
		if err := w.sink.Observe(ctx, observation); err != nil {
			publishErrors = append(publishErrors, fmt.Errorf("retract VATSIM observation for callsign %s: %w", callsign, err))
			continue
		}
		delete(w.known, callsign)
	}
	return errors.Join(publishErrors...)
}

func (w *ObservationWorker) mapFlight(_ context.Context, flight Flight, snapshotAt time.Time, status aman.DataStatus, reconciledAt time.Time, previous *aman.FlightObservation) (aman.FlightObservation, error) {
	callsign := strings.ToUpper(strings.TrimSpace(flight.Callsign))
	observedAt := flight.LastUpdated.UTC()
	if observedAt.IsZero() {
		observedAt = snapshotAt.UTC()
	}
	observation := aman.FlightObservation{
		Callsign: callsign,
		Origin:   strings.ToUpper(strings.TrimSpace(flight.FlightPlan.Origin)), Destination: strings.ToUpper(strings.TrimSpace(flight.FlightPlan.Destination)),
		AircraftType: optionalString(flight.FlightPlan.AircraftShort), WakeCategory: wakeCategory(flight.FlightPlan.Aircraft),
		FiledRoute: optionalString(flight.FlightPlan.Route), RequestedLevel: requestedLevelFeet(flight.FlightPlan.RequestedLevel),
		PlannedTiming: plannedTiming(observedAt, flight.FlightPlan), FlightPlan: flightPlanFact(flight.FlightPlan.Revision, observedAt),
		Surveillance: surveillanceFact(flight, observedAt, previous), TakeoffDetected: takeoffDetected(flight, observedAt),
		SurveillanceSource: aman.SurveillanceSourceVATSIM, Provider: aman.ObservationProviderVATSIM,
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
	if err != nil || duration <= 0 {
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
