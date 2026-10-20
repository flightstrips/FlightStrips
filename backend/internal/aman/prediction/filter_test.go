package prediction

import (
	"FlightStrips/internal/aman"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestFilterRejectsSpikeAndCountsDistinctObservations(t *testing.T) {
	now := predictionTime()
	eta := now.Add(40 * time.Minute)
	flight := aman.AMANFlight{Prediction: &aman.Prediction{RawTETA: eta, InputObservedAt: now}}
	candidate := aman.Prediction{RawTETA: eta.Add(32 * time.Minute), InputObservedAt: now.Add(time.Minute), GeneratedAt: now.Add(time.Minute)}
	flight, accepted, outcome := Filter(flight, candidate, nil, false, false)
	require.False(t, accepted)
	require.Equal(t, "filtered", outcome)
	require.Equal(t, eta, flight.Prediction.RawTETA)
	// Reprocessing cannot turn one radar observation into sustained evidence.
	candidate.GeneratedAt = now.Add(80 * time.Second)
	flight, accepted, outcome = Filter(flight, candidate, nil, false, false)
	require.False(t, accepted)
	require.Equal(t, "duplicate", outcome)
	require.EqualValues(t, 1, flight.PredictionFilter.Pending.Samples)
	// An isolated spike disappears without touching accepted timing.
	candidate.InputObservedAt = now.Add(85 * time.Second)
	candidate.GeneratedAt = candidate.InputObservedAt
	candidate.RawTETA = eta.Add(5 * time.Second)
	flight, accepted, _ = Filter(flight, candidate, nil, false, false)
	require.True(t, accepted)
	require.Nil(t, flight.PredictionFilter.Pending)
}

func TestFilterSustainedChangeSurvivesRestartAndNeedsThirtySeconds(t *testing.T) {
	now := predictionTime()
	eta := now.Add(time.Hour)
	flight := aman.AMANFlight{Prediction: &aman.Prediction{RawTETA: eta}}
	raw := aman.Prediction{RawTETA: eta.Add(10 * time.Minute), InputObservedAt: now, GeneratedAt: now}
	flight, accepted, _ := Filter(flight, raw, nil, false, false)
	require.False(t, accepted)
	encoded, err := json.Marshal(flight)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &flight))
	raw.InputObservedAt, raw.GeneratedAt = now.Add(10*time.Second), now.Add(10*time.Second)
	flight, accepted, _ = Filter(flight, raw, nil, false, false)
	require.False(t, accepted)
	raw.InputObservedAt, raw.GeneratedAt = now.Add(30*time.Second), now.Add(30*time.Second)
	flight, accepted, outcome := Filter(flight, raw, nil, false, false)
	require.True(t, accepted)
	require.Equal(t, "sustained_change", outcome)
	require.Nil(t, flight.PredictionFilter.Pending)
}

func TestFilterFeederSpikesExpireAndRecoverAutomatically(t *testing.T) {
	now := predictionTime()
	eta := now.Add(time.Hour)
	feeder := eta.Add(-10 * time.Minute)
	flight := aman.AMANFlight{Prediction: &aman.Prediction{RawTETA: eta, Publishable: true}, DerivedFeederETA: &aman.FeederETAState{Source: aman.FeederETASourceRoute, ETA: &feeder}}
	changed := feeder.Add(10 * time.Minute)
	raw := aman.Prediction{RawTETA: eta, InputObservedAt: now, GeneratedAt: now}
	flight, accepted, _ := Filter(flight, raw, &changed, false, false)
	require.False(t, accepted)
	// Continued recalculation on a stale observation expires guidance, never
	// fabricates new observations or releases the runway reservation.
	raw.GeneratedAt = now.Add(2 * time.Minute)
	flight, accepted, _ = Filter(flight, raw, &changed, false, false)
	require.False(t, accepted)
	require.True(t, flight.PredictionFilter.Unreliable)
	require.False(t, flight.Prediction.Publishable)
	raw.InputObservedAt, raw.GeneratedAt = now.Add(3*time.Minute), now.Add(3*time.Minute)
	flight, accepted, _ = Filter(flight, raw, &feeder, false, false)
	require.True(t, accepted)
	require.False(t, flight.PredictionFilter.Unreliable)
}

func TestFilterExplicitChangesBypassButSourceAndWeatherDoNot(t *testing.T) {
	now := predictionTime()
	eta := now.Add(time.Hour)
	for _, bypass := range []bool{false, true} {
		flight := aman.AMANFlight{Prediction: &aman.Prediction{RawTETA: eta}}
		raw := aman.Prediction{RawTETA: eta.Add(-20 * time.Minute), InputObservedAt: now, GeneratedAt: now}
		_, accepted, _ := Filter(flight, raw, nil, bypass, false)
		require.Equal(t, bypass, accepted)
		_, accepted, _ = Filter(flight, raw, nil, false, true)
		require.True(t, accepted, "rollback flag disables automatic filtering")
	}
}

func TestReducerDoesNotResampleOneObservation(t *testing.T) {
	now := predictionTime()
	raw := rawPrediction(now, now.Add(time.Hour))
	raw.InputObservedAt = now
	first, err := Reduce(DefaultConfig(), predictionFlight(now), Input{Raw: raw, State: aman.StateStable})
	require.NoError(t, err)
	raw.GeneratedAt = now.Add(time.Minute)
	raw.RawTETA = raw.RawTETA.Add(10 * time.Minute)
	repeated, err := Reduce(DefaultConfig(), first.Flight, Input{Raw: raw, State: aman.StateStable})
	require.NoError(t, err)
	require.Equal(t, first.Flight.RawTETASamples, repeated.Flight.RawTETASamples)
	require.Equal(t, first.Flight.Prediction, repeated.Flight.Prediction)
}
