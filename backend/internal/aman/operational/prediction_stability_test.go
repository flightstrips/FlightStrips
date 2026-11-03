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

func TestInferredSTARForecastRespectsStabilityFilterThroughExpiryAndRecovery(t *testing.T) {
	now := time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)
	service, state, observation, _ := runwaySTARFixture(now)
	selectSTARTestRunway(&state, "ARRIVAL-04L")
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	require.Contains(t, *flight.Prediction.DegradationReason, aman.STARRunwayInferred)
	flight.Slot = &aman.Slot{RunwayGroupID: "ARRIVAL-04L", Time: flight.Prediction.OperationalTETA.Add(2 * time.Minute)}
	accepted := *flight.Prediction
	beforeDemand, beforeFeeder, beforeSlot := *aman.SequenceDemandArrivalAt(flight), flight.DerivedFeederETA, *flight.Slot
	_, available := aman.GainLossPrediction(flight)
	require.True(t, available)

	update := func(offset time.Duration, speed float64) {
		t.Helper()
		at := now.Add(offset)
		observation := *flight.LatestObservation
		surveillance := *observation.Surveillance
		surveillance.ObservedAt, surveillance.GroundspeedKnots = &at, &speed
		observation.Surveillance, observation.ReconciledAt = &surveillance, at
		var err error
		flight, err = service.reconcileFlight(t.Context(), state, flight, observation, at)
		require.NoError(t, err)
	}

	// Alternating candidates never agree. The third update expires the filter
	// before STAR resolution, replacing the inference diagnostic with unreliable.
	for index, speed := range []float64{120, 180, 120, 180} {
		update(time.Duration(10+index*60)*time.Second, speed)
		require.NotNil(t, flight.PredictionFilter.Pending, "sample %d must remain filtered", index)
		require.Equal(t, accepted.RawTETA, flight.Prediction.RawTETA)
		require.Equal(t, accepted.GeneratedAt, flight.Prediction.GeneratedAt)
		require.Equal(t, beforeDemand, *aman.SequenceDemandArrivalAt(flight))
		require.Equal(t, beforeFeeder, flight.DerivedFeederETA)
		require.Equal(t, beforeSlot, *flight.Slot)
		if index >= 2 {
			require.True(t, flight.PredictionFilter.Unreliable)
			require.False(t, flight.Prediction.Publishable)
			require.Equal(t, prediction.UnreliablePrediction, *flight.Prediction.DegradationReason)
			_, available = aman.GainLossPrediction(flight)
			require.False(t, available)
			// Persisted unreliable state must not masquerade as a new inference.
			flight = cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{flight}}).Flights[0]
		}
	}

	// A distinct new candidate still needs a consistent pair spanning 30s.
	update(220*time.Second, 150)
	require.True(t, flight.PredictionFilter.Unreliable)
	require.False(t, flight.Prediction.Publishable)
	require.Equal(t, accepted.RawTETA, flight.Prediction.RawTETA)
	update(250*time.Second, 150)
	require.Nil(t, flight.PredictionFilter.Pending)
	require.False(t, flight.PredictionFilter.Unreliable)
	require.True(t, flight.Prediction.Publishable)
	require.Equal(t, now.Add(250*time.Second), flight.Prediction.GeneratedAt)
	require.NotEqual(t, accepted.RawTETA, flight.Prediction.RawTETA)
	require.Contains(t, *flight.Prediction.DegradationReason, aman.STARRunwayInferred)
	_, available = aman.GainLossPrediction(flight)
	require.True(t, available)
	require.Equal(t, beforeSlot, *flight.Slot)
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
