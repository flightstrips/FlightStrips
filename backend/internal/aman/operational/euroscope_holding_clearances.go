package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
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
	if strip == nil {
		return nil
	}
	cid := stripStringValue(strip.VatsimCID)
	callsign := strings.TrimSpace(strip.Callsign)
	if cid == "" || callsign == "" {
		return nil
	}
	flightID, err := o.identities.BindVATSIMFlight(ctx, aman.VATSIMFlightIdentity{
		VATSIMCID: cid, CurrentCallsign: callsign,
	})
	if err != nil {
		return fmt.Errorf("bind EuroScope AMAN holding-clearance identity: %w", err)
	}
	fact := aman.HoldingClearanceFact{
		FlightID: flightID, VATSIMCID: cid, Callsign: callsign,
		Origin: strings.ToUpper(strings.TrimSpace(strip.Origin)), Destination: strings.ToUpper(strings.TrimSpace(strip.Destination)),
		Hold: strip.Hold, HoldType: aman.HoldingClearanceType(strip.HoldType), HoldEAT: strip.HoldEat,
		ClearedAltitude: cloneInt32(strip.ClearedAltitude), ObservedAt: o.now().UTC(),
	}
	if err := o.sink.ObserveHoldingClearance(ctx, fact); err != nil {
		return fmt.Errorf("publish EuroScope AMAN holding clearance: %w", err)
	}
	return nil
}

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
