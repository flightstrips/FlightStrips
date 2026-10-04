package operational

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/trajectory"
	"github.com/stretchr/testify/require"
)

func TestClearedHoldingKeepsQueueAdmissionAcrossRacetrackLaps(t *testing.T) {
	base := time.Date(2026, time.October, 4, 19, 0, 0, 0, time.UTC)
	id := navdata.HoldingID("LUGAS-HOLD")
	flight := aman.AMANFlight{
		HoldingClearance: &aman.HoldingClearance{Hold: "LUGAS", HoldType: aman.HoldingClearanceEnroute},
		HoldingStack: &aman.HoldingStackState{HoldingID: string(id), FirstObservedAt: base,
			CandidateObservedAt: base, Confirmed: true, ConsecutiveObservations: 2},
	}
	projection := trajectory.Result{SelectedHolding: &navdata.HoldingPattern{ID: id, Fix: "LUGAS"}}
	retained := updateFlightHoldingStack(flight, projection, base.Add(4*time.Minute))
	require.Equal(t, flight.HoldingStack, retained)
	require.NotSame(t, flight.HoldingStack, retained)
	flight.HoldingStack = retained
	projection.HoldingCandidate = &trajectory.HoldingCandidate{HoldingID: id}
	resumed := updateFlightHoldingStack(flight, projection, base.Add(5*time.Minute))
	require.True(t, resumed.Confirmed)
	require.Equal(t, base, resumed.FirstObservedAt, "returning from a cleared outbound leg is not a new holding arrival")
	projection.HoldingCandidate = nil
	flight.HoldingStack = resumed
	flight.HoldingClearance = nil
	require.Nil(t, updateFlightHoldingStack(flight, projection, base.Add(6*time.Minute)))
	flight.HoldingClearance = &aman.HoldingClearance{Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute}
	require.Nil(t, updateFlightHoldingStack(flight, projection, base.Add(6*time.Minute)))
	flight.HoldingClearance.Hold = "LUGAS"
	flight.HoldingStack = nil
	require.Nil(t, updateFlightHoldingStack(flight, projection, base), "a clearance alone must not create confirmed physical stack traffic")
}
