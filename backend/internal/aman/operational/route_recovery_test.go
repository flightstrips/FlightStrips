package operational

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestRouteRecoveryRefreshesLegacyGeometryTimingAndAutomaticSlot(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	geometry := service.deps.Geometry.(terminalIdentityGeometry)
	geometry.fixes[1].Position.LatitudeDeg, geometry.fixes[1].Position.LongitudeDeg = 55, 14
	geometry.fixes[2].Position.LatitudeDeg, geometry.fixes[2].Position.LongitudeDeg = 55, 16
	service.deps.Geometry, service.deps.Wind = geometry, unavailableWind{}
	initialAt := now.Add(-time.Minute)
	observation := *state.Flights[0].LatestObservation
	position := *observation.Surveillance
	position.LatitudeDegrees, position.LongitudeDegrees = 55, 12.01
	position.ObservedAt = &initialAt
	observation.Surveillance, observation.ReconciledAt = &position, initialAt
	observation.WakeCategory = stringPointer("M")
	initial, err := service.reconcileFlight(t.Context(), state, newFlight(observation, initialAt), observation, initialAt)
	require.NoError(t, err)
	require.NotNil(t, initial.Prediction.Calculation)
	require.Equal(t, "TESPI", initial.Prediction.Calculation.Legs[0].To)
	initial.RouteProgress.ObservedAt = nil // An aggregate persisted before this fix.
	initial.State = aman.StateStable
	initial.Prediction.OperationalTETA = now.Add(90 * time.Minute)
	initial.RawTETASamples = []aman.RawTETASample{{TETA: initial.Prediction.OperationalTETA, GeneratedAt: initialAt}}
	initial.Slot = &aman.Slot{Time: now.Add(92 * time.Minute), RunwayGroupID: *initial.SelectedRunwayGroup, Sequence: 1, Reason: "rate_wtc"}
	oldSlot, oldDistance := initial.Slot.Time, *initial.Prediction.DistanceToGoNM

	at := initialAt.Add(20 * time.Minute)
	position.LatitudeDegrees, position.LongitudeDegrees, position.ObservedAt = 55, 14.5, &at
	observation.ReconciledAt = at
	updated, err := service.reconcileFlight(t.Context(), state, initial, observation, at)
	require.NoError(t, err)
	require.Equal(t, "TNO", updated.Prediction.Calculation.Legs[0].To)
	require.Less(t, *updated.Prediction.DistanceToGoNM, oldDistance-50)
	require.Equal(t, at, updated.Prediction.InputObservedAt)
	require.Equal(t, at, *updated.RouteProgress.ObservedAt)
	require.Equal(t, aman.OperationalReasonRouteRecovery, updated.Prediction.OperationalReason)
	require.Equal(t, updated.Prediction.RawTETA, updated.Prediction.OperationalTETA)
	state.Flights = []aman.AMANFlight{updated}
	service.resequence(&state, at)
	require.True(t, state.Flights[0].Slot.Time.Before(oldSlot))
}
