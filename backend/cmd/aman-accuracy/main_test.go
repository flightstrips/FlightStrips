package main

import (
	"math"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestReplaySTARUsesLatestPhysicalClassification(t *testing.T) {
	points := []forecastPoint{
		{star: ""},
		{star: "TESPI"},
		{star: ""},
	}

	require.Equal(t, "TESPI", replaySTAR(points))
	require.Empty(t, replaySTAR([]forecastPoint{{star: ""}}))
}

func TestThresholdCrossingEventsSeparatesGoAroundAndFinalApproach(t *testing.T) {
	base := time.Date(2026, time.July, 28, 11, 40, 0, 0, time.UTC)
	corridor := lifecycle.FinalPathCorridor{
		ID: "EKCH-22L", ThresholdLatitude: 55.6254, ThresholdLongitude: 12.6676,
		InboundCourseDegrees: 221.2, LengthNM: 6, HalfWidthNM: 0.75,
	}
	point := func(at time.Duration, along float64, altitude int) trackPoint {
		course := corridor.InboundCourseDegrees * 3.141592653589793 / 180
		north := along * math.Cos(course)
		east := along * math.Sin(course)
		return trackPoint{
			at: base.Add(at), latitude: corridor.ThresholdLatitude + north/60,
			longitude:    corridor.ThresholdLongitude + east/(60*math.Cos(corridor.ThresholdLatitude*3.141592653589793/180)),
			altitudeFeet: altitude, groundspeedKnots: 130,
		}
	}
	track := []trackPoint{
		point(0, -0.2, 200), point(20*time.Second, 0.2, 400),
		point(time.Minute, -0.3, 300), point(90*time.Second, 0.3, 50),
	}

	events := thresholdCrossingEvents(track, corridor)

	require.Len(t, events, 2)
	require.Equal(t, 1, events[0].Sequence)
	require.Equal(t, base.Add(10*time.Second), events[0].At)
	require.Equal(t, 2, events[1].Sequence)
	require.Equal(t, base.Add(75*time.Second), events[1].At)
}

func TestModelSegmentFromCalculationRetainsInstantaneousSpeedEvidence(t *testing.T) {
	ias, tailwind := 250.0, -45.0
	segment := modelSegmentFromCalculation(aman.PredictionSegment{
		PhaseID: "fl270_to_fl100", PhaseName: "Segment 3", StartAltitudeFeet: 25_000, EndAltitudeFeet: 24_000, AltitudeFeet: 24_500,
		IndicatedAirspeedKnots: &ias, NoWindGroundspeedKnots: 390, GroundspeedKnots: 345, TailwindKnots: &tailwind,
		DistanceNM: 8, Duration: 83 * time.Second,
	})

	require.Equal(t, "fl270_to_fl100", segment.phaseID)
	require.Equal(t, 345.0, segment.groundspeedKnots)
	require.Equal(t, 390.0, segment.noWindGroundspeedKnots)
	require.InDelta(t, ias/math.Sqrt(replayDensityRatio(25_000)), segment.currentNoWindGroundspeedKnots, 0.001)
	require.InDelta(t, segment.currentNoWindGroundspeedKnots-45, segment.currentGroundspeedKnots, 0.001)
	require.Equal(t, -45.0, *segment.tailwindKnots)
	require.Equal(t, 83.0, segment.durationSec)
}
