package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"context"
	"strings"
	"time"
)

// EvaluateAccepted runs the production policy once from a complete accepted
// input set. The candidate constructs a new Service for each evaluation so
// observed maps and health timers are rebuildable caches, never authority.
func (s *Service) EvaluateAccepted(ctx context.Context, airport string) error {
	s.observeNavigationCache(ctx, airport)
	s.refreshWeather(ctx, airport, s.deps.Now().UTC())
	return s.reconcileAirportOnce(ctx, airport)
}

// HoldingEATs is shared by the production transport and durable candidate.
// It includes only outputs allowed by the operational hold/FIFO/health gates.
func HoldingEATs(state aman.AirportState, health aman.TechnicalHealth, enabled bool, snapshot navdata.ActiveGeometrySnapshot) map[string]aman.HoldingClearance {
	result := map[string]aman.HoldingClearance{}
	if !enabled || !state.Authoritative || !health.AuthorityAllowed {
		return result
	}
	fixes := map[navdata.HoldingID]navdata.FixID{}
	for _, h := range snapshot.Holdings {
		fixes[h.ID] = h.Fix
	}
	for _, f := range state.Flights {
		c, p, stack := f.HoldingClearance, f.Prediction, f.HoldingStack
		if c == nil || c.Hold == "" || c.HoldType != aman.HoldingClearanceEnroute || p == nil || p.HoldingPlan == nil || stack == nil || f.SelectedHolding == nil || stack.HoldingID != *f.SelectedHolding {
			continue
		}
		fix, found := fixes[navdata.HoldingID(*f.SelectedHolding)]
		if !found || !strings.EqualFold(strings.TrimSpace(c.Hold), string(fix)) || HoldingEATBlockedByEarlierEntrant(f, state.Flights) {
			continue
		}
		value := *c
		value.HoldEAT = p.HoldingPlan.ApproachReleaseTime.UTC().Format("1504")
		result[f.Callsign] = value
	}
	return result
}

func HoldingEATBlockedByEarlierEntrant(flight aman.AMANFlight, flights []aman.AMANFlight) bool {
	stack := flight.HoldingStack
	if stack == nil || stack.FirstObservedAt.IsZero() || flight.Prediction == nil || flight.Prediction.HoldingPlan == nil {
		return false
	}
	release := flight.Prediction.HoldingPlan.ApproachReleaseTime
	for _, older := range flights {
		if older.Callsign == flight.Callsign || older.HoldingStack == nil || older.HoldingStack.HoldingID != stack.HoldingID || older.State == aman.StateLanded || older.State == aman.StateRemoved {
			continue
		}
		entry := older.HoldingStack.FirstObservedAt
		if entry.IsZero() && !older.HoldingStack.Confirmed || !entry.IsZero() && !entry.Before(stack.FirstObservedAt) {
			continue
		}
		if older.Prediction == nil || older.Prediction.HoldingPlan == nil || !older.Prediction.HoldingPlan.ApproachReleaseTime.UTC().Truncate(time.Minute).Before(release.UTC().Truncate(time.Minute)) {
			return true
		}
	}
	return false
}
