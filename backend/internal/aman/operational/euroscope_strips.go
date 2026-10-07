package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/holdingclearance"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EuroScopeStripObserver maps complete persisted strip snapshots to AMAN
// flight-plan observations. Full syncs are replacement sets per ES session.
type EuroScopeStripObserver struct {
	sink     aman.ObservationSink
	airports map[string]struct{}
	now      func() time.Time

	mu    sync.Mutex
	known map[int32]map[aman.Callsign]aman.FlightObservation
}

type EuroScopeStripObserverDependencies struct {
	Sink            aman.ObservationSink
	EnabledAirports []string
	Now             func() time.Time
}

func NewEuroScopeStripObserver(deps EuroScopeStripObserverDependencies) (*EuroScopeStripObserver, error) {
	if deps.Sink == nil {
		return nil, fmt.Errorf("EuroScope AMAN strip observer requires observation sink")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	airports := make(map[string]struct{}, len(deps.EnabledAirports))
	for _, airport := range deps.EnabledAirports {
		if airport = strings.ToUpper(strings.TrimSpace(airport)); airport != "" {
			airports[airport] = struct{}{}
		}
	}
	return &EuroScopeStripObserver{sink: deps.Sink, airports: airports, now: deps.Now, known: make(map[int32]map[aman.Callsign]aman.FlightObservation)}, nil
}

func (o *EuroScopeStripObserver) ObserveEuroScopeStrip(ctx context.Context, strip *models.Strip) error {
	if strip == nil {
		return nil
	}
	observedAt := o.now().UTC()
	ctx = aman.WithSession(ctx, strip.Session)
	observation, err := o.project(strip, observedAt)
	if err != nil || observation == nil {
		return err
	}
	if err := o.sink.Observe(ctx, *observation); err != nil {
		return fmt.Errorf("publish EuroScope AMAN strip observation: %w", err)
	}
	o.mu.Lock()
	if o.known[strip.Session] == nil {
		o.known[strip.Session] = make(map[aman.Callsign]aman.FlightObservation)
	}
	o.known[strip.Session][observation.Callsign] = *observation
	o.mu.Unlock()
	return nil
}

func (o *EuroScopeStripObserver) ObserveEuroScopeStrips(ctx context.Context, session int32, strips []shared.EuroScopeStripObservation) error {
	ctx = aman.WithSession(ctx, session)
	current := make(map[aman.Callsign]aman.FlightObservation, len(strips))
	var projectionErrors []error
	for _, item := range strips {
		observation, err := o.project(item.Strip, item.ObservedAt)
		if err != nil {
			projectionErrors = append(projectionErrors, err)
			continue
		}
		if observation != nil {
			current[observation.Callsign] = *observation
		}
	}
	if err := errors.Join(projectionErrors...); err != nil {
		return err
	}

	o.mu.Lock()
	previous := cloneEuroScopeStripObservations(o.known[session])
	o.mu.Unlock()

	var publishErrors []error
	for callsign, observation := range current {
		if err := o.sink.Observe(ctx, observation); err != nil {
			publishErrors = append(publishErrors, fmt.Errorf("publish EuroScope AMAN strip observation for %s: %w", callsign, err))
		}
	}
	missingAt := o.now().UTC()
	for callsign, observation := range previous {
		if replacement, present := current[callsign]; present && replacement.Destination == observation.Destination {
			continue
		}
		observation.Missing = true
		observation.ReconciledAt = missingAt
		observation.SourceStatus = aman.DataFresh
		if err := o.sink.Observe(ctx, observation); err != nil {
			publishErrors = append(publishErrors, fmt.Errorf("publish missing EuroScope AMAN strip observation for %s: %w", callsign, err))
		}
	}
	if err := errors.Join(publishErrors...); err != nil {
		return err
	}
	o.mu.Lock()
	o.known[session] = current
	o.mu.Unlock()
	return nil
}

func (o *EuroScopeStripObserver) project(strip *models.Strip, observedAt time.Time) (*aman.FlightObservation, error) {
	if strip == nil {
		return nil, nil
	}
	callsign := strings.ToUpper(strings.TrimSpace(strip.Callsign))
	origin := strings.ToUpper(strings.TrimSpace(strip.Origin))
	destination := strings.ToUpper(strings.TrimSpace(strip.Destination))
	if callsign == "" || origin == "" || destination == "" {
		return nil, nil
	}
	aircraftType, wakeCategory := euroScopeAircraft(strip.AircraftType)
	observation := aman.FlightObservation{
		Callsign: callsign, Origin: origin, Destination: destination,
		AircraftType: aircraftType, WakeCategory: wakeCategory, FiledRoute: optionalStripString(strip.Route), RequestedLevel: requestedLevel(strip.RequestedAltitude),
		AssignedSTAR:     optionalStripString(strip.Star),
		FlightPlan:       aman.FlightPlanFact{Revision: vatsimRevision(strip.VatsimRevision), ObservedAt: &observedAt},
		HoldingClearance: normalizedStripHoldingClearance(strip, observedAt), Provider: aman.ObservationProviderEuroScope,
		PlannedTiming: euroScopePlannedTiming(strip.FlightPlanEOBT, strip.FlightPlanEET, observedAt),
		ReconciledAt:  observedAt, SourceStatus: aman.DataFresh,
	}
	if err := observation.Validate(); err != nil {
		return nil, fmt.Errorf("map EuroScope AMAN strip observation: %w", err)
	}
	return &observation, nil
}

func euroScopePlannedTiming(eobt, eet string, at time.Time) *aman.PlannedTiming {
	timing := &aman.PlannedTiming{}
	if departure, err := holdingclearance.ResolveEATUTC(strings.TrimSpace(eobt), at); err == nil {
		timing.EstimatedOffBlockTime = &departure
	}
	eet = strings.TrimSpace(eet)
	if len(eet) == 4 {
		valid := true
		for _, digit := range eet {
			valid = valid && digit >= '0' && digit <= '9'
		}
		hours, _ := strconv.Atoi(eet[:2])
		minutes, _ := strconv.Atoi(eet[2:])
		if valid && minutes < 60 && hours <= 23 && (hours > 0 || minutes > 0) {
			duration := time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute
			timing.EstimatedEnrouteTime = &duration
		}
	}
	if timing.EstimatedOffBlockTime == nil && timing.EstimatedEnrouteTime == nil {
		return nil
	}
	return timing
}

// RemoveEuroScopeStrip retracts one session's ownership of a callsign after an
// individual aircraft-disconnect deletion. Other sessions remain independent.
func (o *EuroScopeStripObserver) RemoveEuroScopeStrip(ctx context.Context, session int32, callsign string) error {
	ctx = aman.WithSession(ctx, session)
	normalized := aman.Callsign(strings.ToUpper(strings.TrimSpace(callsign)))
	if normalized == "" {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	observations := o.known[session]
	observation, exists := observations[normalized]
	if !exists {
		return nil
	}
	delete(observations, normalized)
	if len(observations) == 0 {
		delete(o.known, session)
	}
	observation.Missing = true
	observation.ReconciledAt = o.now().UTC()
	observation.SourceStatus = aman.DataFresh
	if err := o.sink.Observe(ctx, observation); err != nil {
		if o.known[session] == nil {
			o.known[session] = make(map[aman.Callsign]aman.FlightObservation)
		}
		o.known[session][normalized] = observation
		return fmt.Errorf("publish removed EuroScope AMAN strip observation for %s: %w", normalized, err)
	}
	return nil
}

func cloneEuroScopeStripObservations(source map[aman.Callsign]aman.FlightObservation) map[aman.Callsign]aman.FlightObservation {
	copy := make(map[aman.Callsign]aman.FlightObservation, len(source))
	for callsign, observation := range source {
		copy[callsign] = observation
	}
	return copy
}
