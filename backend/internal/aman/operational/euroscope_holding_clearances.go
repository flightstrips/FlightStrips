package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"fmt"
	"strings"
	"time"
)

// EuroScopeHoldingClearanceObserver maps the persisted authoritative strip
// fields to AMAN's policy-neutral holding fact boundary.
type EuroScopeHoldingClearanceObserver struct {
	sink       aman.HoldingClearanceSink
	identities aman.VATSIMFlightIdentityBinder
	now        func() time.Time
}

type EuroScopeHoldingClearanceObserverDependencies struct {
	Sink       aman.HoldingClearanceSink
	Identities aman.VATSIMFlightIdentityBinder
	Now        func() time.Time
}

func NewEuroScopeHoldingClearanceObserver(deps EuroScopeHoldingClearanceObserverDependencies) (*EuroScopeHoldingClearanceObserver, error) {
	if deps.Sink == nil {
		return nil, fmt.Errorf("EuroScope AMAN holding-clearance observer requires sink")
	}
	if deps.Identities == nil {
		return nil, fmt.Errorf("EuroScope AMAN holding-clearance observer requires VATSIM identity binder")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &EuroScopeHoldingClearanceObserver{sink: deps.Sink, identities: deps.Identities, now: deps.Now}, nil
}

func (o *EuroScopeHoldingClearanceObserver) ObserveHoldingClearance(ctx context.Context, strip *models.Strip) error {
	return o.ObserveHoldingClearances(ctx, []shared.HoldingClearanceObservation{{Strip: strip, ObservedAt: o.now().UTC()}})
}

func (o *EuroScopeHoldingClearanceObserver) ObserveHoldingClearances(ctx context.Context, strips []shared.HoldingClearanceObservation) error {
	facts := make([]aman.HoldingClearanceFact, 0, len(strips))
	for _, observation := range strips {
		fact, err := o.holdingClearanceFact(ctx, observation.Strip, observation.ObservedAt)
		if err != nil {
			return err
		}
		if fact != nil {
			facts = append(facts, *fact)
		}
	}
	if sink, ok := o.sink.(interface {
		ObserveHoldingClearances(context.Context, []aman.HoldingClearanceFact) error
	}); ok {
		return sink.ObserveHoldingClearances(ctx, facts)
	}
	for _, fact := range facts {
		if err := o.sink.ObserveHoldingClearance(ctx, fact); err != nil {
			return fmt.Errorf("publish EuroScope AMAN holding clearance: %w", err)
		}
	}
	return nil
}

func (o *EuroScopeHoldingClearanceObserver) holdingClearanceFact(ctx context.Context, strip *models.Strip, observedAt time.Time) (*aman.HoldingClearanceFact, error) {
	if strip == nil {
		return nil, nil
	}
	cid := stripStringValue(strip.VatsimCID)
	callsign := strings.TrimSpace(strip.Callsign)
	if cid == "" || callsign == "" {
		return nil, nil
	}
	flightID, err := o.identities.BindVATSIMFlight(ctx, aman.VATSIMFlightIdentity{
		VATSIMCID: cid, CurrentCallsign: callsign,
	})
	if err != nil {
		return nil, fmt.Errorf("bind EuroScope AMAN holding-clearance identity: %w", err)
	}
	fact := aman.HoldingClearanceFact{
		FlightID: flightID, VATSIMCID: cid, Callsign: callsign,
		Origin: strings.ToUpper(strings.TrimSpace(strip.Origin)), Destination: strings.ToUpper(strings.TrimSpace(strip.Destination)),
		Hold: strip.Hold, HoldType: aman.HoldingClearanceType(strip.HoldType), HoldEAT: strip.HoldEat,
		ClearedAltitude: cloneInt32(strip.ClearedAltitude), ObservedAt: observedAt,
	}
	return &fact, nil
}

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
