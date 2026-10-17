package operational

import (
	"FlightStrips/internal/aman"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestConfirmedShorterDirectPromotesEligibleSlotWithoutReleasingTMAFreeze(t *testing.T) {
	for _, freeze := range []aman.FreezeReason{aman.FreezeNone, aman.FreezeTMA} {
		t.Run(string(freeze), func(t *testing.T) {
			svc, state, at := recomputeFlightFixture(t)
			geometry := svc.deps.Geometry.(terminalIdentityGeometry)
			// The published path has a dogleg. The direct skips it using real geometry.
			geometry.fixes[1].Position.LatitudeDeg = 55.3
			geometry.fixes[1].Position.LongitudeDeg = 11.9
			svc.deps.Geometry = geometry
			f := state.Flights[0]
			f.State = aman.StateStable
			f.Prediction.OperationalTETA = at.Add(20 * time.Minute)
			f.Slot.Time = at.Add(22 * time.Minute)
			oldSlot := f.Slot.Time
			if freeze != aman.FreezeNone {
				f.FreezeReason = freeze
				f.FrozenAt = &at
				eta, slot := f.Prediction.OperationalTETA, *f.Slot
				f.FrozenOperationalTETA = &eta
				f.FrozenSlot = &slot
			}
			f.ActiveRouteFact = &aman.RouteFact{ID: "shorter-direct", Callsign: f.Callsign, Fix: "TNO", State: aman.RouteFactActive, DatasetVersion: navigationDatasetID(geometry.version), ObservedAt: at, ReceivedAt: at}
			observation := *f.LatestObservation
			position := *observation.Surveillance
			position.ObservedAt = &at
			observation.Surveillance = &position
			observation.ReconciledAt = at
			updated, err := svc.reconcileFlight(context.Background(), state, f, observation, at)
			require.NoError(t, err)
			if freeze == aman.FreezeNone {
				require.Equal(t, updated.Prediction.RawTETA, updated.Prediction.OperationalTETA, "confirmed routing bypasses routine smoothing")
				require.True(t, updated.Prediction.RawTETA.Before(at.Add(20*time.Minute)))
			} else {
				require.Equal(t, f.Prediction.OperationalTETA, updated.Prediction.OperationalTETA)
			}
			state.Flights = []aman.AMANFlight{updated}
			svc.resequence(&state, at)
			if freeze == aman.FreezeNone {
				require.True(t, state.Flights[0].Slot.Time.Before(oldSlot))
			} else {
				require.Equal(t, oldSlot, state.Flights[0].Slot.Time)
			}
		})
	}
}

func TestUnresolvedDirectDoesNotBypassPredictionAcceptance(t *testing.T) {
	svc, state, at := recomputeFlightFixture(t)
	f := state.Flights[0]
	f.Prediction.OperationalTETA = at.Add(20 * time.Minute)
	f.ActiveRouteFact = &aman.RouteFact{ID: "unresolved", Fix: "UNKNOWN", State: aman.RouteFactActive, ObservedAt: at, ReceivedAt: at}
	observation := *f.LatestObservation
	position := *observation.Surveillance
	position.ObservedAt = &at
	observation.Surveillance = &position
	observation.ReconciledAt = at
	updated, err := svc.reconcileFlight(context.Background(), state, f, observation, at)
	require.NoError(t, err)
	require.NotEqual(t, updated.Prediction.RawTETA, updated.Prediction.OperationalTETA)
}
