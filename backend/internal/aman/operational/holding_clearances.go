package operational

import (
	"context"
	"log/slog"

	"FlightStrips/internal/aman"
)

// RefreshHoldingClearances updates the route timing in the same revision as
// the accepted clearance. Waiting for the minute-driven reconciliation leaves
// newly cleared traffic without an EAT when its old route has passed the fix.
// Assigned slots and the normal release-order checks remain authoritative.
func (s *Service) RefreshHoldingClearances(ctx context.Context, state *aman.AirportState, callsigns []aman.Callsign) {
	now := state.GeneratedAt
	if s.deps.Now != nil {
		now = s.deps.Now().UTC()
	}
	for _, callsign := range callsigns {
		index := flightIndex(state.Flights, callsign)
		if index < 0 {
			continue
		}
		flight := state.Flights[index]
		if flight.State == aman.StateLanded || flight.State == aman.StateRemoved || flight.LatestObservation == nil {
			continue
		}
		observation := *flight.LatestObservation
		if observation.Missing || observation.SourceStatus != aman.DataFresh || observation.Surveillance == nil ||
			observation.Surveillance.ObservedAt == nil || observation.Surveillance.ObservedAt.After(now) ||
			now.Sub(*observation.Surveillance.ObservedAt) > tmaSurveillanceFresh {
			continue
		}
		observation.HoldingClearance = cloneHoldingClearance(flight.HoldingClearance)
		updated, err := s.reconcileFlight(ctx, *state, flight, observation, now)
		if err != nil {
			// A calculation failure must not discard the controller's fact.
			// The regular reconciliation will retry when inputs recover.
			slog.WarnContext(ctx, "AMAN holding clearance calculation unavailable", "callsign", callsign, "error", err)
			continue
		}
		state.Flights[index] = updated
	}
	s.refreshHoldingPlans(state)
	for _, flight := range state.Flights {
		if flight.Prediction != nil && flight.Prediction.HoldingPlanBlockedBy != nil {
			s.resequence(state, now)
			break
		}
	}
}
