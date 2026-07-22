package lifecycle_test

import (
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestGoAroundDetectorConfirmsClimbOnceAndTransitionsThroughLifecycle(t *testing.T) {
	detector := newDetector(t, detectorConfig())
	base := lifecycleTime()
	state := aman.GoAroundDetectionState{}
	var beforeConfirmation aman.GoAroundDetectionState
	var confirmed *lifecycle.GoAroundConfirmed

	for index, sample := range []struct {
		latitude float64
		altitude int
	}{
		{-0.030, 1200},
		{-0.020, 1100},
		{-0.010, 1250},
		{-0.005, 1400},
	} {
		if index == 3 {
			beforeConfirmation = state
		}
		at := base.Add(time.Duration(index+1) * time.Second)
		result, err := detector.Detect(detectorInput(state, observation(uint64(index+1), at, sample.latitude, 0, sample.altitude, 160, floatPointer(0)), at))
		require.NoError(t, err)
		state = result.State
		if result.Confirmed != nil {
			confirmed = result.Confirmed
		}
	}

	require.NotNil(t, confirmed)
	require.Equal(t, lifecycle.GoAroundReasonClimb, confirmed.Reason)
	require.Equal(t, "flight-1/go-around/1", confirmed.EpisodeID)
	require.Equal(t, confirmed.EpisodeID+"/confirmed", confirmed.ID)
	require.Equal(t, []time.Time{base.Add(3 * time.Second), base.Add(4 * time.Second)}, confirmed.SupportingObservationTimes)
	require.Equal(t, uint64(1), state.LastEmittedEpisode)
	require.False(t, state.Armed)

	flight := lifecycleFlight(base, aman.StateStable)
	transition, err := lifecycle.Reduce(lifecycle.DefaultConfig(), flight, confirmed.LifecycleEvent())
	require.NoError(t, err)
	require.Equal(t, aman.StateGoAround, transition.Flight.State)
	require.Equal(t, confirmed.ID, transition.Transition.EventID)

	lastAt := base.Add(4 * time.Second)
	duplicate, err := detector.Detect(detectorInput(state, observation(4, lastAt, -0.005, 0, 1400, 160, floatPointer(0)), lastAt))
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	require.Nil(t, duplicate.Confirmed)
	require.Equal(t, state, duplicate.State)

	replayed, err := detector.Detect(detectorInput(beforeConfirmation, observation(4, lastAt, -0.005, 0, 1400, 160, floatPointer(0)), lastAt))
	require.NoError(t, err)
	require.Equal(t, confirmed, replayed.Confirmed)
	require.Equal(t, state, replayed.State)
}

func TestGoAroundDetectorConfirmsDerivedTrackAwayAndRunwayExit(t *testing.T) {
	for _, test := range []struct {
		name   string
		reason lifecycle.GoAroundReason
		points []detectorPoint
	}{
		{
			name: "derived track away", reason: lifecycle.GoAroundReasonTrackAway,
			points: []detectorPoint{
				{latitude: -0.030, altitude: 1200, track: floatPointer(0)},
				{latitude: -0.020, altitude: 1100, track: floatPointer(0)},
				{latitude: -0.020, longitude: 0.004, altitude: 1100},
				{latitude: -0.020, longitude: 0.008, altitude: 1100},
			},
		},
		{
			name: "runway exit without landing", reason: lifecycle.GoAroundReasonRunwayExit,
			points: []detectorPoint{
				{latitude: -0.030, altitude: 900, track: floatPointer(0)},
				{latitude: -0.020, altitude: 800, track: floatPointer(0)},
				{latitude: 0.002, altitude: 500, track: floatPointer(0)},
				{latitude: 0.004, altitude: 500, track: floatPointer(0)},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			detector := newDetector(t, detectorConfig())
			state := aman.GoAroundDetectionState{}
			var confirmed *lifecycle.GoAroundConfirmed
			for index, point := range test.points {
				at := lifecycleTime().Add(time.Duration(index+1) * time.Second)
				result, err := detector.Detect(detectorInput(state, observation(uint64(index+1), at, point.latitude, point.longitude, point.altitude, 160, point.track), at))
				require.NoError(t, err)
				state, confirmed = result.State, result.Confirmed
			}
			require.NotNil(t, confirmed)
			require.Equal(t, test.reason, confirmed.Reason)
			require.Len(t, confirmed.SupportingObservationTimes, 2)
		})
	}
}

func TestGoAroundDetectorRejectsLandingVectorNoiseAndStaleTracks(t *testing.T) {
	detector := newDetector(t, detectorConfig())
	base := lifecycleTime()
	state := armDetector(t, detector, base)

	// A single climb point is insufficient and the next descending point
	// resets the consecutive evidence counter.
	climb, err := detector.Detect(detectorInput(state, observation(3, base.Add(3*time.Second), -0.010, 0, 1000, 160, floatPointer(0)), base.Add(3*time.Second)))
	require.NoError(t, err)
	require.Nil(t, climb.Confirmed)
	descent, err := detector.Detect(detectorInput(climb.State, observation(4, base.Add(4*time.Second), -0.005, 0, 900, 160, floatPointer(0)), base.Add(4*time.Second)))
	require.NoError(t, err)
	require.Nil(t, descent.Confirmed)
	require.Zero(t, descent.State.ClimbCount)

	// A normal landing beyond the threshold is too low and slow to count as
	// runway-exit-without-landing evidence.
	landing, err := detector.Detect(detectorInput(descent.State, observation(5, base.Add(5*time.Second), 0.002, 0, 50, 25, floatPointer(0)), base.Add(5*time.Second)))
	require.NoError(t, err)
	require.Nil(t, landing.Confirmed)
	require.Zero(t, landing.State.RunwayExitCount)

	// Stale source state disarms and clears evidence without emitting.
	staleObservation := observation(6, base.Add(6*time.Second), 0.004, 0, 600, 160, floatPointer(180))
	staleObservation.SourceStatus = aman.DataStale
	stale, err := detector.Detect(detectorInput(landing.State, staleObservation, base.Add(6*time.Second)))
	require.NoError(t, err)
	require.Nil(t, stale.Confirmed)
	require.False(t, stale.State.Armed)
	require.Empty(t, stale.State.Evidence)

	// Vectoring before arming never satisfies the inbound final-path gate.
	vectorState := aman.GoAroundDetectionState{}
	for index := 0; index < 4; index++ {
		at := base.Add(time.Duration(index+10) * time.Second)
		result, err := detector.Detect(detectorInput(vectorState, observation(uint64(index+10), at, -0.03+float64(index)*0.002, 0, 1200, 160, floatPointer(90)), at))
		require.NoError(t, err)
		vectorState = result.State
		require.Nil(t, result.Confirmed)
	}
	require.False(t, vectorState.Armed)
	require.Zero(t, vectorState.ArmCount)
}

func TestGoAroundDetectorUsesExactBoundariesAndConsecutiveArming(t *testing.T) {
	config := detectorConfig()
	detector := newDetector(t, config)
	base := lifecycleTime()
	state := aman.GoAroundDetectionState{}

	first, err := detector.Detect(detectorInput(state, observation(1, base.Add(time.Second), -0.03, 0, config.ArmBelowAltitudeFeet, 160, floatPointer(config.InboundToleranceDegrees)), base.Add(time.Second)))
	require.NoError(t, err)
	require.Equal(t, 1, first.State.ArmCount, "exact altitude and inbound-angle boundaries arm")

	outside, err := detector.Detect(detectorInput(first.State, observation(2, base.Add(2*time.Second), -0.02, 0, config.ArmBelowAltitudeFeet+1, 160, floatPointer(config.InboundToleranceDegrees)), base.Add(2*time.Second)))
	require.NoError(t, err)
	require.Zero(t, outside.State.ArmCount)
	require.False(t, outside.State.Armed)

	second, err := detector.Detect(detectorInput(outside.State, observation(3, base.Add(3*time.Second), -0.02, 0, 1000, 160, floatPointer(0)), base.Add(3*time.Second)))
	require.NoError(t, err)
	third, err := detector.Detect(detectorInput(second.State, observation(4, base.Add(4*time.Second), -0.01, 0, 900, 160, floatPointer(0)), base.Add(4*time.Second)))
	require.NoError(t, err)
	require.True(t, third.State.Armed)
}

func TestGoAroundDetectorPersistsBoundedStateAndHandlesOrderingAndPolicyChange(t *testing.T) {
	config := detectorConfig()
	config.EvidenceLimit = 4
	config.MinimumClimbFeet = 1000
	detector := newDetector(t, config)
	base := lifecycleTime()
	state := aman.GoAroundDetectionState{}
	for index := 0; index < 8; index++ {
		at := base.Add(time.Duration(index+1) * time.Second)
		result, err := detector.Detect(detectorInput(state, observation(uint64(index+1), at, -0.03+float64(index)*0.001, 0, 1200, 160, floatPointer(0)), at))
		require.NoError(t, err)
		state = result.State
	}
	require.Len(t, state.Evidence, 4)
	require.Equal(t, base.Add(5*time.Second), state.Evidence[0].ObservedAt)

	persisted, err := json.Marshal(state)
	require.NoError(t, err)
	var restored aman.GoAroundDetectionState
	require.NoError(t, json.Unmarshal(persisted, &restored))
	require.NoError(t, restored.Validate())
	require.Equal(t, state, restored)

	old, err := detector.Detect(detectorInput(restored, observation(7, base.Add(7*time.Second), -0.02, 0, 1200, 160, floatPointer(0)), base.Add(9*time.Second)))
	require.NoError(t, err)
	require.True(t, old.Duplicate)
	require.Equal(t, restored, old.State)

	changedInput := detectorInput(restored, observation(9, base.Add(9*time.Second), -0.02, 0, 1200, 160, floatPointer(0)), base.Add(9*time.Second))
	changedInput.PolicyVersion = "go-around-v2"
	changed, err := detector.Detect(changedInput)
	require.NoError(t, err)
	require.Equal(t, "go-around-v2", changed.State.PolicyVersion)
	require.False(t, changed.State.Armed)
	require.Len(t, changed.State.Evidence, 1, "policy change disarms before accepting current evidence")
	require.Equal(t, restored.Episode, changed.State.Episode)
}

func TestGoAroundDetectorRestartAtEveryObservationIsEquivalent(t *testing.T) {
	detector := newDetector(t, detectorConfig())
	base := lifecycleTime()
	inputs := []lifecycle.GoAroundInput{}
	state := aman.GoAroundDetectionState{}
	for index, point := range []detectorPoint{
		{latitude: -0.030, altitude: 1200, track: floatPointer(0)},
		{latitude: -0.020, altitude: 1100, track: floatPointer(0)},
		{latitude: -0.010, altitude: 1250, track: floatPointer(0)},
		{latitude: -0.005, altitude: 1400, track: floatPointer(0)},
	} {
		at := base.Add(time.Duration(index+1) * time.Second)
		inputs = append(inputs, detectorInput(state, observation(uint64(index+1), at, point.latitude, point.longitude, point.altitude, 160, point.track), at))
		result, err := detector.Detect(inputs[index])
		require.NoError(t, err)
		state = result.State
	}
	wantState := state
	wantEventID := "flight-1/go-around/1/confirmed"

	for checkpoint := range inputs {
		checkpointState := aman.GoAroundDetectionState{}
		var eventID string
		for index := 0; index <= checkpoint; index++ {
			input := inputs[index]
			input.Previous = checkpointState
			result, err := detector.Detect(input)
			require.NoError(t, err)
			checkpointState = result.State
			if result.Confirmed != nil {
				eventID = result.Confirmed.ID
			}
		}
		encoded, err := json.Marshal(checkpointState)
		require.NoError(t, err)
		var restored aman.GoAroundDetectionState
		require.NoError(t, json.Unmarshal(encoded, &restored))
		for index := checkpoint + 1; index < len(inputs); index++ {
			input := inputs[index]
			input.Previous = restored
			result, err := detector.Detect(input)
			require.NoError(t, err)
			restored = result.State
			if result.Confirmed != nil {
				eventID = result.Confirmed.ID
			}
		}
		require.Equal(t, wantEventID, eventID, "checkpoint %d", checkpoint)
		require.Equal(t, wantState, restored, "checkpoint %d", checkpoint)
	}
}

func TestGoAroundDetectorDisarmsOnRouteRunwayAndScopeChanges(t *testing.T) {
	base := lifecycleTime()
	for _, change := range []struct {
		name  string
		apply func(*lifecycle.GoAroundInput)
	}{
		{"route", func(input *lifecycle.GoAroundInput) { input.RouteChanged = true }},
		{"runway group", func(input *lifecycle.GoAroundInput) { input.RunwayGroupChanged = true }},
	} {
		t.Run(change.name, func(t *testing.T) {
			detector := newDetector(t, detectorConfig())
			armed := armDetector(t, detector, base)
			input := detectorInput(armed, observation(3, base.Add(3*time.Second), -0.01, 0, 900, 160, floatPointer(0)), base.Add(3*time.Second))
			change.apply(&input)
			result, err := detector.Detect(input)
			require.NoError(t, err)
			require.False(t, result.State.Armed)
			require.Equal(t, 1, result.State.ArmCount, "current evidence starts a new arming run after disarm")
			require.Len(t, result.State.Evidence, 1)
		})
	}

	detector := newDetector(t, detectorConfig())
	first, err := detector.Detect(detectorInput(aman.GoAroundDetectionState{}, observation(1, base.Add(time.Second), -0.03, 0, 900, 160, floatPointer(0)), base.Add(time.Second)))
	require.NoError(t, err)
	require.Equal(t, 1, first.State.ArmCount)
	leaving := detectorInput(first.State, observation(2, base.Add(2*time.Second), -0.02, 0, 900, 160, floatPointer(0)), base.Add(2*time.Second))
	leaving.InScope = false
	left, err := detector.Detect(leaving)
	require.NoError(t, err)
	require.False(t, left.State.Armed)
	require.Zero(t, left.State.ArmCount)
	require.Empty(t, left.State.Evidence)
}

type detectorPoint struct {
	latitude, longitude float64
	altitude            int
	track               *float64
}

func detectorConfig() lifecycle.GoAroundConfig {
	return lifecycle.GoAroundConfig{
		EvidenceLimit: 8, ArmSamples: 2, ConfirmSamples: 2, ArmBelowAltitudeFeet: 2000,
		InboundToleranceDegrees: 20, MinimumClimbFeet: 100, TrackAwayDegrees: 60,
		RunwayExitAfterThresholdNM: 0.1, LandingAltitudeToleranceFeet: 200, MinimumAirborneGroundspeedKnots: 80,
	}
}

func finalCorridor() lifecycle.FinalPathCorridor {
	return lifecycle.FinalPathCorridor{
		ID: "EKCH-22L", ThresholdLatitude: 0, ThresholdLongitude: 0, ThresholdElevationFeet: 0,
		InboundCourseDegrees: 0, LengthNM: 3, HalfWidthNM: 0.5,
	}
}

func newDetector(t *testing.T, config lifecycle.GoAroundConfig) lifecycle.GoAroundDetector {
	t.Helper()
	detector, err := lifecycle.NewGoAroundDetector(config)
	require.NoError(t, err)
	return detector
}

func detectorInput(state aman.GoAroundDetectionState, current aman.FlightObservation, now time.Time) lifecycle.GoAroundInput {
	return lifecycle.GoAroundInput{
		FlightID: "flight-1", Observation: current, Corridor: finalCorridor(), Previous: state,
		PolicyVersion: "go-around-v1", Now: now, InScope: true,
	}
}

func observation(sequence uint64, at time.Time, latitude, longitude float64, altitude int, groundspeed float64, track *float64) aman.FlightObservation {
	return aman.FlightObservation{
		FlightID: "flight-1", VATSIMCID: "1234567", Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH",
		ReconciledAt: at, SourceStatus: aman.DataFresh,
		Surveillance: &aman.SurveillanceFact{
			LatitudeDegrees: latitude, LongitudeDegrees: longitude, AltitudeFeet: &altitude,
			GroundspeedKnots: &groundspeed, TrackTrueDegrees: track, Sequence: &sequence, ObservedAt: &at,
		},
	}
}

func armDetector(t *testing.T, detector lifecycle.GoAroundDetector, base time.Time) aman.GoAroundDetectionState {
	t.Helper()
	state := aman.GoAroundDetectionState{}
	for index, latitude := range []float64{-0.03, -0.02} {
		at := base.Add(time.Duration(index+1) * time.Second)
		result, err := detector.Detect(detectorInput(state, observation(uint64(index+1), at, latitude, 0, 900, 160, floatPointer(0)), at))
		require.NoError(t, err)
		state = result.State
	}
	require.True(t, state.Armed)
	return state
}

func floatPointer(value float64) *float64 { return &value }
