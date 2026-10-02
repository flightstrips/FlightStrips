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
	sink     aman.HoldingClearanceSink
	airports map[string]struct{}
	now      func() time.Time
}

type EuroScopeHoldingClearanceObserverDependencies struct {
	Sink            aman.HoldingClearanceSink
	EnabledAirports []string
	Now             func() time.Time
}

func NewEuroScopeHoldingClearanceObserver(deps EuroScopeHoldingClearanceObserverDependencies) (*EuroScopeHoldingClearanceObserver, error) {
	if deps.Sink == nil {
		return nil, fmt.Errorf("EuroScope AMAN holding-clearance observer requires sink")
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
	return &EuroScopeHoldingClearanceObserver{sink: deps.Sink, airports: airports, now: deps.Now}, nil
}

func (o *EuroScopeHoldingClearanceObserver) ObserveHoldingClearance(ctx context.Context, strip *models.Strip) error {
	return o.ObserveHoldingClearances(ctx, []shared.HoldingClearanceObservation{{Strip: strip, ObservedAt: o.now().UTC()}})
}

func (o *EuroScopeHoldingClearanceObserver) ObserveHoldingClearances(ctx context.Context, strips []shared.HoldingClearanceObservation) error {
	facts := make([]aman.HoldingClearanceFact, 0, len(strips))
	for _, observation := range strips {
		fact, err := o.projectStrip(observation.Strip, observation.ObservedAt)
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

func (o *EuroScopeHoldingClearanceObserver) projectStrip(strip *models.Strip, observedAt time.Time) (*aman.HoldingClearanceFact, error) {
	if strip == nil {
		return nil, nil
	}
	callsign := strings.ToUpper(strings.TrimSpace(strip.Callsign))
	destination := strings.ToUpper(strings.TrimSpace(strip.Destination))
	if callsign == "" || destination == "" {
		return nil, nil
	}
	if len(o.airports) > 0 {
		if _, enabled := o.airports[destination]; !enabled {
			return nil, nil
		}
	}
	fact := aman.HoldingClearanceFact{
		Callsign: callsign,
		Origin:   strings.ToUpper(strings.TrimSpace(strip.Origin)), Destination: strings.ToUpper(strings.TrimSpace(strip.Destination)),
		Hold: strip.Hold, HoldType: aman.HoldingClearanceType(strip.HoldType), HoldEAT: strip.HoldEat,
		ClearedAltitude: cloneInt32(strip.ClearedAltitude), ObservedAt: observedAt,
	}
	return &fact, nil
}

func normalizedStripHoldingClearance(strip *models.Strip, observedAt time.Time) *aman.HoldingClearance {
	hold := strings.ToUpper(strings.TrimSpace(strip.Hold))
	holdType := aman.HoldingClearanceType(strings.ToLower(strings.TrimSpace(strip.HoldType)))
	holdEAT := strings.TrimSpace(strip.HoldEat)
	altitude := cloneInt32(strip.ClearedAltitude)
	if hold == "" {
		holdType, holdEAT, altitude = "", "", nil
	}
	return &aman.HoldingClearance{Hold: hold, HoldType: holdType, HoldEAT: holdEAT, ClearedAltitude: altitude, ObservedAt: observedAt}
}

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
