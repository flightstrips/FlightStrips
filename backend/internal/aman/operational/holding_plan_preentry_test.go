package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
)

func TestApproachingAircraftHasHoldingPlanBeforeClearanceOrPhysicalEntry(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	geometry := service.deps.Geometry.(terminalIdentityGeometry)
	hold := navdata.HoldingID("TESPI-HOLD")
	geometry.path.HoldingIDs = []navdata.HoldingID{hold}
	service.deps.Geometry = holdingReleaseGeometry{terminalIdentityGeometry: geometry,
		holding: navdata.HoldingPattern{ID: hold, Fix: "TESPI"}}
	service.deps.Terminal.Paths[0].SelectedHolding = hold
	flight := state.Flights[0]
	flight.RouteProgress = nil
	flight.Slot.Time = now.Add(30 * time.Minute)
	for _, cleared := range []bool{false, true} {
		observation := *flight.LatestObservation
		observation.ReconciledAt = now
		surveillance := *observation.Surveillance
		surveillance.LatitudeDegrees, surveillance.LongitudeDegrees = 55, 12
		surveillance.ObservedAt = &now
		observation.Surveillance = &surveillance
		if cleared {
			observation.HoldingClearance = &aman.HoldingClearance{Hold: "TESPI", HoldType: aman.HoldingClearanceEnroute, ObservedAt: now}
		}
		var err error
		flight, err = service.reconcileFlight(context.Background(), state, flight, observation, now)
		require.NoError(t, err)
		state.Flights = []aman.AMANFlight{flight}
		service.refreshHoldingPlans(&state)
		flight = state.Flights[0]
		require.Nil(t, flight.HoldingStack, "aircraft is still approaching the holding fix")
		require.Nil(t, flight.HoldingReleaseBasis)
		plan := flight.Prediction.HoldingPlan
		require.NotNil(t, plan, "slot delay must produce a plan before physical holding detection: cleared=%v prediction=%+v slot=%+v holding=%v", cleared, flight.Prediction, flight.Slot, flight.SelectedHolding)
		require.NoError(t, plan.Validate())
		require.Equal(t, *flight.Prediction.HoldingFixETA, plan.HoldingEntryTime)
		require.Equal(t, flight.Slot.Time.Add(-plan.PostHoldingTransit), plan.ApproachReleaseTime)
		require.Equal(t, flight.Slot.Time.Sub(flight.Prediction.RawTETA), plan.ExpectedHoldingDuration)
		require.Positive(t, plan.ExpectedHoldingDuration)
		now = now.Add(time.Minute)
	}
}
