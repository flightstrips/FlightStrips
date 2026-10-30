package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/prediction"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func actionAuditRecords(records []aman.AuditRecord) []aman.AuditRecord {
	var result []aman.AuditRecord
	for _, record := range records {
		if record.Category != "aman.sequence_operation" && record.Category != "aman.spacing_evaluated" {
			result = append(result, record)
		}
	}
	return result
}

func TestOutlierCannotChangeDemandFeederGuidanceOrSlot(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	flight := state.Flights[0]
	// Establish a coherent physical route first, then inject an isolated
	// speed collapse representing the large alternating calculations in Loki.
	beforeDemand := *aman.SequenceDemandArrivalAt(flight)
	beforeGuidance, ok := aman.GainLossPrediction(flight)
	require.True(t, ok)
	beforeFeeder, beforeSlot := flight.DerivedFeederETA, *flight.Slot
	observation := *flight.LatestObservation
	surveillance := *observation.Surveillance
	at := now.Add(time.Minute)
	surveillance.ObservedAt = &at
	speed := 60.0
	surveillance.GroundspeedKnots = &speed
	observation.Surveillance = &surveillance
	observation.ReconciledAt = at
	updated, err := service.reconcileFlight(context.Background(), state, flight, observation, at)
	require.NoError(t, err)
	require.NotNil(t, updated.PredictionFilter.Pending)
	require.Equal(t, beforeDemand, *aman.SequenceDemandArrivalAt(updated))
	require.Equal(t, beforeFeeder, updated.DerivedFeederETA)
	guidance, available := aman.GainLossPrediction(updated)
	require.True(t, available)
	require.Equal(t, beforeGuidance, guidance)
	state.Flights[0] = updated
	service.resequence(&state, at)
	require.Equal(t, beforeSlot.Time, state.Flights[0].Slot.Time)
	updated = prediction.ExpireFilter(updated, at.Add(2*time.Minute))
	_, available = aman.GainLossPrediction(updated)
	require.False(t, available)
	require.Equal(t, beforeDemand, *aman.SequenceDemandArrivalAt(updated), "unreliable traffic must not disappear from demand")
}

func TestFilteredPredictionsStillObserveHoldingFixCrossing(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	geometry := service.deps.Geometry.(terminalIdentityGeometry)
	hold := navdata.HoldingID("TESPI-HOLD")
	geometry.path.HoldingIDs = []navdata.HoldingID{hold}
	service.deps.Geometry = holdingReleaseGeometry{terminalIdentityGeometry: geometry,
		holding: navdata.HoldingPattern{ID: hold, Fix: "TESPI"}}
	service.deps.Terminal.Paths[0].SelectedHolding = hold
	flight := state.Flights[0]
	// Keep the accepted timing far from alternating candidates, so the
	// aircraft crosses the fix after more than two minutes of filtering.
	flight.Prediction.RawTETA = now.Add(time.Hour)
	accepted := flight.Prediction.RawTETA
	for i, offset := range []float64{0.04, 0.06, 0.07, 0.08, 0.12} {
		at := now.Add(time.Duration(i) * time.Minute)
		observation := *flight.LatestObservation
		surveillance := *observation.Surveillance
		speed := 120.0
		if i%2 != 0 {
			speed = 600
		}
		surveillance.LatitudeDegrees, surveillance.LongitudeDegrees = 55+offset, 12+offset
		surveillance.GroundspeedKnots, surveillance.ObservedAt = &speed, &at
		observation.Surveillance, observation.ReconciledAt = &surveillance, at
		observation.HoldingClearance = &aman.HoldingClearance{Hold: "TESPI", HoldType: aman.HoldingClearanceEnroute, ObservedAt: now}
		var err error
		flight, err = service.reconcileFlight(context.Background(), state, flight, observation, at)
		require.NoError(t, err)
		require.NotNil(t, flight.PredictionFilter.Pending, "sample %d must remain filtered", i)
		require.Equal(t, accepted, flight.Prediction.RawTETA)
		require.NotNil(t, flight.HoldingEpisode)
		require.Equal(t, at, *flight.HoldingEpisode.LastObservation.ObservedAt)
		if i < 4 {
			require.Nil(t, flight.HoldingEpisode.StartedAt, "approaching the fix must not start holding")
		}
	}
	require.NotNil(t, flight.HoldingEpisode.StartedAt)
	require.Equal(t, "fix_crossing", flight.HoldingEpisode.Source)
	require.True(t, flight.HoldingEpisode.StartedAt.After(now.Add(3*time.Minute)))
	require.True(t, flight.HoldingEpisode.StartedAt.Before(now.Add(4*time.Minute)))
	require.NoError(t, flight.HoldingEpisode.Validate())
}
