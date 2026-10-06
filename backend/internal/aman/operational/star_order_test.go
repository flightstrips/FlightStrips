package operational

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestSequenceSTARProgressUsesFreshRouteEvidence(t *testing.T) {
	start := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
	family, feeder := "MONAK", "FEEDER"
	flight := aman.AMANFlight{
		DataStatus: aman.DataFresh, SelectedSTARFamily: &family, SelectedFeederFix: &feeder,
		RouteProgress:    &aman.RouteProgress{TerminalDigest: "terminal"},
		Prediction:       &aman.Prediction{Publishable: true},
		DerivedFeederETA: &aman.FeederETAState{Source: aman.FeederETASourceRoute, ETA: &start},
		FeederETA:        &aman.FeederETAState{Source: aman.FeederETASourceManual, ETA: &start},
	}
	progress := sequenceSTARProgress(flight)
	require.NotNil(t, progress)
	require.Equal(t, feeder, progress.Fix)
	require.Equal(t, start, *progress.ETA)
	require.NotSame(t, flight.DerivedFeederETA.ETA, progress.ETA)

	flight.DerivedFeederETA = nil
	require.Nil(t, sequenceSTARProgress(flight), "a manual feeder ETA is not physical order evidence")
	flight.FeederETA = &aman.FeederETAState{Source: aman.FeederETASourceHolding, ETA: &start}
	require.Nil(t, sequenceSTARProgress(flight), "a holding release is not free-flight order evidence")
	flight.FeederETA = &aman.FeederETAState{Source: aman.FeederETASourceRoute, ETA: &start}
	flight.Prediction.Publishable = false
	require.Nil(t, sequenceSTARProgress(flight))
	flight.Prediction.Publishable = true
	flight.DataStatus = aman.DataStale
	require.Nil(t, sequenceSTARProgress(flight))
}

func TestSequenceSTARProgressAfterPassingFeeder(t *testing.T) {
	family, feeder := "MONAK", "FEEDER"
	flight := aman.AMANFlight{
		DataStatus: aman.DataFresh, SelectedSTARFamily: &family, SelectedFeederFix: &feeder,
		RouteProgress:    &aman.RouteProgress{TerminalDigest: "terminal"},
		DerivedFeederETA: &aman.FeederETAState{Source: aman.FeederETASourcePassed, Passed: true},
		Prediction: &aman.Prediction{Publishable: true, Calculation: &aman.PredictionCalculation{Legs: []aman.PredictionLeg{
			{From: "FEEDER", To: "FINAL", DistanceNM: 20, EndLatitude: 55, EndLongitude: 12},
			{From: "FINAL", To: "RWY", DistanceNM: 5, EndLatitude: 55.1, EndLongitude: 12.1},
		}}},
	}
	progress := sequenceSTARProgress(flight)
	require.NotNil(t, progress)
	require.True(t, progress.Passed)
	require.Nil(t, progress.ETA)
	require.Equal(t, float64(25), progress.DistanceToGoNM)
	require.Equal(t, []string{"FEEDER>FINAL:55:12", "FINAL>RWY:55.1:12.1"}, progress.RemainingFixes)
	flight.Prediction.Calculation = nil
	require.Nil(t, sequenceSTARProgress(flight), "passed progress needs a comparable remaining route")
}
