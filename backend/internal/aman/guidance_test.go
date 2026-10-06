package aman

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGainLossGuidance(t *testing.T) {
	target := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		drift time.Duration
		want  int64
	}{{90 * time.Second, 90}, {2 * time.Minute, 120}, {45 * time.Minute, 120}, {-100 * time.Minute, -6000}, {29900 * time.Millisecond, 30}} {
		seconds, err := GainLossGuidance(target.Add(test.drift), target)
		require.NoError(t, err)
		require.Equal(t, test.want, seconds)
	}
}

func TestGainLossPredictionFiltersOutliersWithoutFreezingLiveGuidance(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	target := now.Add(20 * time.Minute)
	flight := AMANFlight{State: StateStable, FreezeReason: FreezeSuperstable, Slot: &Slot{Time: target},
		Prediction: &Prediction{RawTETA: target.Add(-15 * time.Minute), OperationalTETA: target.Add(-10 * time.Minute), GeneratedAt: now, OperationalReason: OperationalReasonSuperstableFreeze},
		RawTETASamples: []RawTETASample{
			{TETA: target.Add(-time.Minute), GeneratedAt: now.Add(-20 * time.Second)},
			{TETA: target.Add(-50 * time.Second), GeneratedAt: now.Add(-10 * time.Second)},
			{TETA: target.Add(-15 * time.Minute), GeneratedAt: now},
		}}
	predicted, available := GainLossPrediction(flight)
	require.True(t, available)
	require.Equal(t, target.Add(-time.Minute), predicted, "one outlier cannot replace the established live estimate")
	flight.RawTETASamples[1].TETA = flight.Prediction.RawTETA
	predicted, available = GainLossPrediction(flight)
	require.True(t, available)
	require.Equal(t, flight.Prediction.RawTETA, predicted, "confirmed changes must flow through even during an operational freeze")
	flight.Prediction.Publishable = false
	predicted, available = GainLossPrediction(flight)
	require.True(t, available)
	require.Equal(t, flight.Prediction.RawTETA, predicted, "temporary data loss retains the latest estimate")
	flight.State = StateLanded
	_, available = GainLossPrediction(flight)
	require.False(t, available)
	flight.State, flight.Slot = StateStable, nil
	_, available = GainLossPrediction(flight)
	require.False(t, available)
}

func TestGainLossPredictionAcceptsExplicitRouteChangeImmediately(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	flight := AMANFlight{Slot: &Slot{Time: now.Add(20 * time.Minute)},
		Prediction: &Prediction{RawTETA: now.Add(5 * time.Minute), GeneratedAt: now, OperationalReason: OperationalReasonRouteRevision},
		RawTETASamples: []RawTETASample{
			{TETA: now.Add(20 * time.Minute), GeneratedAt: now.Add(-20 * time.Second)},
			{TETA: now.Add(20 * time.Minute), GeneratedAt: now.Add(-10 * time.Second)},
			{TETA: now.Add(5 * time.Minute), GeneratedAt: now},
		}}
	predicted, available := GainLossPrediction(flight)
	require.True(t, available)
	require.Equal(t, flight.Prediction.RawTETA, predicted)
}
