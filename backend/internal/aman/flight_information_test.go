package aman

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func informationFlight() AMANFlight {
	now := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	feeder, eta, reta := "ROSBI", now.Add(10*time.Minute), now.Add(20*time.Minute)
	return AMANFlight{
		State: StateStable, SelectedFeederFix: &feeder,
		Prediction: &Prediction{Publishable: true, RawRETA: &reta, Calculation: &PredictionCalculation{Legs: []PredictionLeg{
			{To: "ROSBI", Duration: 10 * time.Minute}, {To: "CH626", Duration: 3 * time.Minute}, {To: "EKCH", Duration: 7 * time.Minute},
		}}},
		FeederETA: &FeederETAState{ETA: &eta, Source: FeederETASourceRoute},
		Slot:      &Slot{Time: now.Add(25 * time.Minute)},
	}
}

func TestScheduledFeederTimeUsesSlotAndAcceptedTransit(t *testing.T) {
	flight := informationFlight()
	want := flight.Slot.Time.Add(-10 * time.Minute)
	require.Equal(t, &want, ScheduledFeederTime(flight))
	manual := flight.Slot.Time.Add(time.Hour)
	flight.FeederETA = &FeederETAState{ETA: &manual, Source: FeederETASourceManual}
	require.Equal(t, &want, ScheduledFeederTime(flight), "a manual estimate cannot substitute for the scheduled time")
	holding := flight.Slot.Time.Add(-8 * time.Minute)
	flight.DerivedFeederETA = &FeederETAState{ETA: &holding, Source: FeederETASourceHolding}
	require.Equal(t, &holding, ScheduledFeederTime(flight))
	flight.DerivedFeederETA = &FeederETAState{Passed: true, Source: FeederETASourcePassed}
	require.Nil(t, ScheduledFeederTime(flight))
	flight.DerivedFeederETA = nil
	*flight.SelectedFeederFix = "MISSING"
	require.Nil(t, ScheduledFeederTime(flight), "missing geometry must not manufacture a time")
}

func TestInitialTimingSurvivesRecalculationAndPersistence(t *testing.T) {
	flight := informationFlight()
	CaptureInitialTiming(&flight)
	require.NotNil(t, flight.InitialTiming)
	first := *flight.InitialTiming
	payload, err := json.Marshal(flight)
	require.NoError(t, err)
	var reloaded AMANFlight
	require.NoError(t, json.Unmarshal(payload, &reloaded))
	require.Equal(t, flight.InitialTiming, reloaded.InitialTiming)
	*reloaded.FeederETA.ETA = reloaded.FeederETA.ETA.Add(time.Minute)
	*reloaded.Prediction.RawRETA = reloaded.Prediction.RawRETA.Add(2 * time.Minute)
	reloaded.Slot.Time = reloaded.Slot.Time.Add(3 * time.Minute)
	CaptureInitialTiming(&reloaded)
	require.Equal(t, first, *reloaded.InitialTiming)
	require.Equal(t, first, *flight.InitialTiming)
}

func TestCaptureInitialTimingFillsLateSlotWithoutMutatingPreviousState(t *testing.T) {
	flight := informationFlight()
	slot := flight.Slot
	flight.Slot = nil
	CaptureInitialTiming(&flight)
	previous := flight.InitialTiming
	require.Nil(t, previous.RunwaySTA)
	require.Nil(t, previous.FeederSTA)
	flight.Slot = slot
	CaptureInitialTiming(&flight)
	require.NotNil(t, flight.InitialTiming.RunwaySTA)
	require.NotNil(t, flight.InitialTiming.FeederSTA)
	require.Nil(t, previous.RunwaySTA)
	require.Nil(t, previous.FeederSTA)
	flight.State, flight.InitialTiming = StatePlanned, nil
	CaptureInitialTiming(&flight)
	require.Nil(t, flight.InitialTiming)
}
