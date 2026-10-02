package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"fmt"
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
	observedAt := o.now().UTC()
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
	activeInOtherSessions := make(map[euroScopeStripKey]aman.FlightObservation)
	activeSession := make(map[euroScopeStripKey]int32)
	for otherSession, observations := range o.known {
		if otherSession == session {
			continue
		}
		for callsign, observation := range observations {
			key := euroScopeStripKey{airport: observation.Destination, callsign: callsign}
			selected, exists := activeInOtherSessions[key]
			if !exists || observation.ReconciledAt.After(selected.ReconciledAt) ||
				(observation.ReconciledAt.Equal(selected.ReconciledAt) && otherSession < activeSession[key]) {
				activeInOtherSessions[key] = observation
				activeSession[key] = otherSession
			}
		}
	}
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
		if surviving, presentElsewhere := activeInOtherSessions[euroScopeStripKey{airport: observation.Destination, callsign: callsign}]; presentElsewhere {
			if err := o.replaceObservationOwner(ctx, observation, surviving, missingAt); err != nil {
				publishErrors = append(publishErrors, fmt.Errorf("restore surviving EuroScope AMAN strip observation for %s: %w", callsign, err))
			}
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
	if len(o.airports) > 0 {
		if _, enabled := o.airports[destination]; !enabled {
			return nil, nil
		}
	}
	observation := aman.FlightObservation{
		Callsign: callsign, Origin: origin, Destination: destination,
		AircraftType: optionalStripString(strip.AircraftType), FiledRoute: optionalStripString(strip.Route), RequestedLevel: requestedLevel(strip.RequestedAltitude),
		FlightPlan:       aman.FlightPlanFact{Revision: vatsimRevision(strip.VatsimRevision), ObservedAt: &observedAt},
		HoldingClearance: normalizedStripHoldingClearance(strip, observedAt), Provider: aman.ObservationProviderEuroScope,
		ReconciledAt: observedAt, SourceStatus: aman.DataFresh,
	}
	if err := observation.Validate(); err != nil {
		return nil, fmt.Errorf("map EuroScope AMAN strip observation: %w", err)
	}
	return &observation, nil
}

// RemoveEuroScopeStrip retracts one session's ownership of a callsign after an
// individual aircraft-disconnect deletion. A callsign still present in another
// EuroScope session remains active.
func (o *EuroScopeStripObserver) RemoveEuroScopeStrip(ctx context.Context, session int32, callsign string) error {
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
	var surviving aman.FlightObservation
	var survivingSession int32
	foundSurvivor := false
	for otherSession, other := range o.known {
		if otherSession == session {
			continue
		}
		if otherObservation, present := other[normalized]; present && otherObservation.Destination == observation.Destination {
			if !foundSurvivor || otherObservation.ReconciledAt.After(surviving.ReconciledAt) ||
				(otherObservation.ReconciledAt.Equal(surviving.ReconciledAt) && otherSession < survivingSession) {
				surviving = otherObservation
				survivingSession = otherSession
				foundSurvivor = true
			}
		}
	}
	if foundSurvivor {
		if err := o.replaceObservationOwner(ctx, observation, surviving, o.now().UTC()); err != nil {
			if o.known[session] == nil {
				o.known[session] = make(map[aman.Callsign]aman.FlightObservation)
			}
			o.known[session][normalized] = observation
			return fmt.Errorf("restore surviving EuroScope AMAN strip observation for %s: %w", normalized, err)
		}
		return nil
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

// replaceObservationOwner clears the provider-wide merged view before
// restoring another session's snapshot. Without the retraction, facts omitted
// by the survivor (especially surveillance) would leak from the removed owner.
func (o *EuroScopeStripObserver) replaceObservationOwner(ctx context.Context, removed, surviving aman.FlightObservation, at time.Time) error {
	retracted := removed
	retracted.Missing = true
	retracted.ReconciledAt = at
	retracted.SourceStatus = aman.DataFresh
	if err := o.sink.Observe(ctx, retracted); err != nil {
		return fmt.Errorf("retract previous owner: %w", err)
	}
	if err := o.sink.Observe(ctx, surviving); err != nil {
		rollbackErr := o.sink.Observe(ctx, removed)
		if rollbackErr != nil {
			return errors.Join(fmt.Errorf("publish surviving owner: %w", err), fmt.Errorf("restore previous owner after failure: %w", rollbackErr))
		}
		return fmt.Errorf("publish surviving owner: %w", err)
	}
	return nil
}

type euroScopeStripKey struct {
	airport  string
	callsign aman.Callsign
}

func cloneEuroScopeStripObservations(source map[aman.Callsign]aman.FlightObservation) map[aman.Callsign]aman.FlightObservation {
	copy := make(map[aman.Callsign]aman.FlightObservation, len(source))
	for callsign, observation := range source {
		copy[callsign] = observation
	}
	return copy
}
