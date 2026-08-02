package predictor

import (
	"context"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestAMANCPHDescentSpeedBands(t *testing.T) {
	require.Equal(t, 275.0, descentIAS(CategoryMedium, 30000, 275))
	require.Equal(t, 300.0, descentIAS(CategoryHeavy, 20000, 275))
	require.Equal(t, 300.0, descentIAS(CategorySuper, 20000, 275))
	require.Equal(t, 280.0, descentIAS(CategoryMedium, 20000, 275))
	require.Equal(t, 280.0, descentIAS("unknown", 20000, 275))
	require.Equal(t, 250.0, descentIAS(CategoryHeavy, 8000, 275))
	require.Equal(t, 210.0, descentIAS(CategoryMedium, 4000, 275))
	require.Equal(t, 140.0, descentIAS(CategoryMedium, 2000, 275))
}

func TestAMANCPHBuildsThreeDegreeProfileAndUsesObservedSpeedAtCruise(t *testing.T) {
	input := performanceInput()
	input.AltitudeFeet = 10000
	input.CurrentGroundspeedKnots = 400
	input.UseObservedGroundspeedBeforeTOD = true
	input.Remaining[0].DistanceNM = 100
	segments := buildDescentSegments(input)
	require.NotEmpty(t, segments)
	require.True(t, segments[0].preTOD)
	require.InDelta(t, 10000/descentFeetPerNM, routeDistance(input.Remaining)-firstDescentDistance(segments), .01)

	result, err := EstimatePerformanceWind(context.Background(), nil, nil, input, PerformanceWindConfig{})
	require.NoError(t, err)
	require.Equal(t, amanCPHModelVersion, result.ModelVersion)
	require.Equal(t, input.PredictionAt.Add(15*time.Minute), result.RawRETA)
	require.Greater(t, result.Duration, 0*time.Second)
	require.Equal(t, result.NoWindDuration, result.Duration)
	require.Contains(t, result.DegradationReasons, "WEATHER_UNAVAILABLE")
	require.NotEmpty(t, result.Segments)
	require.Equal(t, "ppos_to_tod", result.Segments[0].PhaseID)
	require.Equal(t, "Segment 1 · PPOS → TOD", result.Segments[0].PhaseName)
	require.Nil(t, result.Segments[0].IndicatedAirspeedKnots)
	require.Equal(t, 400.0, result.Segments[0].GroundspeedKnots)
}

func TestAMANCPHUsesFiledCruiseLevelForClimbAndTOD(t *testing.T) {
	input := performanceInput()
	input.AltitudeFeet = 28_800
	input.CruiseAltitudeFeet = 35_000
	input.CurrentGroundspeedKnots = 429
	input.UseObservedGroundspeedBeforeTOD = false
	input.Remaining[0].DistanceNM = 600

	result, err := EstimatePerformanceWind(context.Background(), nil, nil, input, PerformanceWindConfig{})

	require.NoError(t, err)
	preTOD := make([]DescentSegmentCalculation, 0)
	for _, segment := range result.Segments {
		if segment.PreTOD {
			preTOD = append(preTOD, segment)
		}
	}
	require.NotEmpty(t, preTOD)
	require.Equal(t, 28_800.0, preTOD[0].StartAltitudeFeet)
	require.Equal(t, 35_000.0, preTOD[len(preTOD)-1].EndAltitudeFeet)
	require.NotNil(t, preTOD[0].IndicatedAirspeedKnots)
	require.Equal(t, 280.0, *preTOD[0].IndicatedAirspeedKnots)

	var segment2 *DescentSegmentCalculation
	for index := range result.Segments {
		if result.Segments[index].PhaseID == "tod_to_fl270" {
			segment2 = &result.Segments[index]
			break
		}
	}
	require.NotNil(t, segment2)
	require.Greater(t, segment2.StartAltitudeFeet, 27_000.0)
	require.NotNil(t, segment2.IndicatedAirspeedKnots)
	require.Equal(t, 280.0, *segment2.IndicatedAirspeedKnots)
	require.InDelta(t, iasToTAS(280, segment2.AltitudeFeet), segment2.NoWindGroundspeedKnots, 0.001)
}

func TestAMANCPHConfirmedDescentUsesEveryAltitudeSpeedBand(t *testing.T) {
	input := performanceInput()
	input.AltitudeFeet = 35_000
	input.CruiseAltitudeFeet = 35_000
	input.DescentConfirmed = true
	input.Remaining[0].DistanceNM = 100

	segments := buildDescentSegments(input)

	require.NotEmpty(t, segments)
	require.Equal(t, input.AltitudeFeet, segments[0].startAltitudeFeet)
	require.Equal(t, 0.0, segments[len(segments)-1].endAltitudeFeet)
	phases := map[string]bool{}
	for _, segment := range segments {
		require.False(t, segment.preTOD)
		phases[phaseForSegment(segment, input).id] = true
	}
	for _, phase := range []string{"tod_to_fl270", "fl270_to_fl100", "fl100_to_fl050", "fl050_to_fl030", "fl030_to_landing"} {
		require.True(t, phases[phase], phase)
	}
	require.False(t, phases["ppos_to_tod"])
}

func TestAMANCPHConfirmedDescentKeepsObservedAltitudeInsideNominalPath(t *testing.T) {
	input := performanceInput()
	input.AltitudeFeet = 20_000
	input.CruiseAltitudeFeet = 20_000
	input.DescentConfirmed = true
	input.Remaining[0].DistanceNM = 20

	segments := buildDescentSegments(input)

	require.NotEmpty(t, segments)
	require.Equal(t, 20_000.0, segments[0].startAltitudeFeet)
	require.Equal(t, "fl270_to_fl100", phaseForSegment(segments[0], input).id)
	require.Equal(t, 0.0, segments[len(segments)-1].endAltitudeFeet)
}

func TestAMANCPHConfirmedDescentLevelsUntilInterceptingNominalPath(t *testing.T) {
	const (
		altitudeFeet = 10_000.0
		distanceNM   = 100.0
	)
	require.Equal(t, altitudeFeet, confirmedDescentAltitudeAtDistance(distanceNM, altitudeFeet, 50))
	require.Equal(t, altitudeFeet, confirmedDescentAltitudeAtDistance(distanceNM, altitudeFeet, 68))
	require.InDelta(t, 9_552, confirmedDescentAltitudeAtDistance(distanceNM, altitudeFeet, 70), 0.001)
	require.Equal(t, 0.0, confirmedDescentAltitudeAtDistance(distanceNM, altitudeFeet, distanceNM))
}

func TestAMANCPHProjectsWindWithoutGlobalCorrectionCap(t *testing.T) {
	input := performanceInput()
	input.AltitudeFeet = 10000
	input.CurrentGroundspeedKnots = 300
	input.Remaining[0].DistanceNM = 10000 / descentFeetPerNM
	repository := profilesRepository{}

	noWind, err := EstimatePerformanceWind(context.Background(), repository, nil, input, PerformanceWindConfig{MinimumGroundspeedKnots: 20, MaximumGroundspeedKnots: 900})
	require.NoError(t, err)
	headwind, err := EstimatePerformanceWind(context.Background(), repository, fixedWind{east: -200, now: input.PredictionAt}, input, PerformanceWindConfig{MinimumGroundspeedKnots: 20, MaximumGroundspeedKnots: 900})
	require.NoError(t, err)
	tailwind, err := EstimatePerformanceWind(context.Background(), repository, fixedWind{east: 200, now: input.PredictionAt}, input, PerformanceWindConfig{MinimumGroundspeedKnots: 20, MaximumGroundspeedKnots: 900})
	require.NoError(t, err)
	require.Greater(t, headwind.Duration, noWind.Duration)
	require.Less(t, tailwind.Duration, noWind.Duration)
	require.Greater(t, headwind.Duration, time.Duration(float64(noWind.Duration)*1.2), "wind duration is not capped at the old 20 percent bound")
}

func TestAMANCPHRequestsWeatherOnlyForTerminalDescentSegments(t *testing.T) {
	input := performanceInput()
	input.Remaining = []RouteLeg{
		{ID: "ENROUTE", DistanceNM: 300, CourseTrueDegrees: 90, Start: WindCoordinate{LatitudeDegrees: 50, LongitudeDegrees: 0}, End: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 12}},
		{ID: "TERMINAL", DistanceNM: 30, CourseTrueDegrees: 45, Start: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 12}, End: WindCoordinate{LatitudeDegrees: 55.6, LongitudeDegrees: 12.7}},
	}
	segments := buildDescentSegments(input)

	requests, indexes := windRequestsForSegments(input, segments, PerformanceWindConfig{})

	require.NotEmpty(t, requests)
	require.Equal(t, len(requests), len(indexes))
	for _, index := range indexes {
		require.True(t, segments[index].weatherEligible)
		require.False(t, segments[index].preTOD)
	}
	result, err := EstimatePerformanceWind(context.Background(), nil, fixedWind{east: 80, now: input.PredictionAt}, input, PerformanceWindConfig{})
	require.NoError(t, err)
	require.Len(t, result.Segments, len(segments))
	for index, segment := range segments {
		if segment.weatherEligible && !segment.preTOD {
			require.NotNil(t, result.Segments[index].TailwindKnots)
		} else {
			require.Nil(t, result.Segments[index].TailwindKnots)
		}
	}
}

func TestAMANCPHWeatherHorizonCoversLongHighAltitudeDescent(t *testing.T) {
	input := performanceInput()
	input.AltitudeFeet = 35_000
	input.CruiseAltitudeFeet = 35_000
	input.DescentConfirmed = true
	input.Remaining[0].DistanceNM = 160

	segments := buildDescentSegments(input)

	require.NotEmpty(t, segments)
	for _, segment := range segments {
		require.True(t, segment.weatherEligible)
	}
	require.False(t, segments[0].surveillanceWindEligible)
	require.True(t, segments[len(segments)-1].surveillanceWindEligible)

	input.Remaining[0].DistanceNM = 200
	segments = buildDescentSegments(input)
	require.NotEmpty(t, segments)
	require.False(t, segments[0].weatherEligible)
	require.False(t, segments[0].surveillanceWindEligible)
	require.True(t, segments[len(segments)-1].weatherEligible)
	require.True(t, segments[len(segments)-1].surveillanceWindEligible)
}

func TestAMANCPHRETAAndLightAircraftBehavior(t *testing.T) {
	input := performanceInput()
	input.CurrentGroundspeedKnots = 200
	input.Remaining[0].DistanceNM = 100
	input.WakeTurbulenceCategory = CategoryHeavy
	heavy, err := EstimatePerformanceWind(context.Background(), nil, nil, input, PerformanceWindConfig{})
	require.NoError(t, err)
	require.Equal(t, input.PredictionAt.Add(30*time.Minute), heavy.RawRETA)

	input.WakeTurbulenceCategory = CategoryLight
	light, err := EstimatePerformanceWind(context.Background(), nil, fixedWind{east: 100, now: input.PredictionAt}, input, PerformanceWindConfig{})
	require.NoError(t, err)
	require.NotEqual(t, light.RawRETA, light.RawTETA)
	require.Less(t, light.Duration, 30*time.Minute)
	require.Equal(t, 280.0, descentIAS(CategoryLight, 20000, 250))
}

func TestAMANCPHUsesTurbopropDescentProfiles(t *testing.T) {
	require.Equal(t, 220.0, descentIASForAircraft("AT76", CategoryMedium, 20_000, 280))
	require.Equal(t, 200.0, descentIASForAircraft("DH8D", CategoryMedium, 8_000, 280))
	require.Equal(t, 120.0, descentIASForAircraft("TBM9", CategoryLight, 2_000, 280))
	require.Equal(t, 280.0, descentIASForAircraft("A320", CategoryMedium, 20_000, 280))

	input := performanceInput()
	input.AircraftICAO = "AT76"
	input.AltitudeFeet = 20_000
	input.CruiseAltitudeFeet = 20_000
	input.DescentConfirmed = true
	input.Remaining[0].DistanceNM = 40
	result, err := EstimatePerformanceWind(context.Background(), nil, nil, input, PerformanceWindConfig{})
	require.NoError(t, err)
	require.NotEmpty(t, result.Segments)
	require.NotNil(t, result.Segments[0].IndicatedAirspeedKnots)
	require.Equal(t, 220.0, *result.Segments[0].IndicatedAirspeedKnots)
}

func TestAMANCPHReturnsActualDurationForEachRouteLeg(t *testing.T) {
	input := performanceInput()
	input.Remaining = []RouteLeg{
		{ID: "FAST", DistanceNM: 40, CourseTrueDegrees: 90, Start: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 12}, End: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 13}},
		{ID: "SLOW", DistanceNM: 60, CourseTrueDegrees: 180, Start: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 13}, End: WindCoordinate{LatitudeDegrees: 54, LongitudeDegrees: 13}},
	}

	result, err := EstimatePerformanceWind(context.Background(), nil, fixedWind{east: 100, now: input.PredictionAt}, input, PerformanceWindConfig{})
	require.NoError(t, err)
	require.Len(t, result.LegDurations, len(input.Remaining))
	require.Equal(t, result.Duration, result.LegDurations[0]+result.LegDurations[1])
	require.NotEqual(t, result.LegDurations[0], result.LegDurations[1])
	require.NotEmpty(t, result.Segments)
	segmentDuration, noWindSegmentDuration := time.Duration(0), time.Duration(0)
	windApplied := false
	for _, segment := range result.Segments {
		segmentDuration += segment.Duration
		noWindSegmentDuration += segment.NoWindDuration
		windApplied = windApplied || segment.TailwindKnots != nil
		require.Greater(t, segment.DistanceNM, 0.0)
		require.Greater(t, segment.GroundspeedKnots, 0.0)
	}
	require.Equal(t, result.Duration, segmentDuration)
	require.Equal(t, result.NoWindDuration, noWindSegmentDuration)
	require.True(t, windApplied)
}

func TestAMANCPHWindBreakdownRetainsNoWindSegmentTotalAtHighAltitude(t *testing.T) {
	input := performanceInput()
	input.AltitudeFeet = 30000
	input.CurrentGroundspeedKnots = 500
	input.Remaining[0].DistanceNM = 120

	result, err := EstimatePerformanceWind(context.Background(), nil, fixedWind{east: 100, now: input.PredictionAt}, input, PerformanceWindConfig{})
	require.NoError(t, err)

	segmentNoWindDuration := time.Duration(0)
	for _, segment := range result.Segments {
		segmentNoWindDuration += segment.NoWindDuration
	}
	require.Equal(t, result.NoWindDuration, segmentNoWindDuration)
}

func TestAMANCPHMissingWeatherDegradesButMissingEssentialInputFails(t *testing.T) {
	input := performanceInput()
	result, err := EstimatePerformanceWind(context.Background(), nil, failingWind{}, input, PerformanceWindConfig{})
	require.NoError(t, err)
	require.False(t, result.RawTETA.IsZero())
	require.Equal(t, aman.ConfidenceMedium, result.Confidence)
	require.Contains(t, result.DegradationReasons, "WEATHER_UNAVAILABLE")

	input.CurrentGroundspeedKnots = 0
	_, err = EstimatePerformanceWind(context.Background(), nil, nil, input, PerformanceWindConfig{})
	require.ErrorIs(t, err, errPerformanceWindInput)
}

func TestAMANCPHUsesObservedTrackAsWindFallbackWhenWeatherIsMissing(t *testing.T) {
	input := performanceInput()
	input.AltitudeFeet = 20_000
	input.CruiseAltitudeFeet = 20_000
	input.Remaining[0].DistanceNM = 50
	track := 90.0
	input.CurrentTrackTrueDegrees = &track
	noWindGroundspeed := iasToTAS(280, input.AltitudeFeet)
	input.CurrentGroundspeedKnots = noWindGroundspeed - 60

	result, err := EstimatePerformanceWind(context.Background(), nil, failingWind{}, input, PerformanceWindConfig{})

	require.NoError(t, err)
	require.Equal(t, pointerString(surveillanceWindFallbackSource), result.WeatherSource)
	require.Contains(t, result.DegradationReasons, "WEATHER_ESTIMATED_FROM_SURVEILLANCE")
	require.NotContains(t, result.DegradationReasons, "WEATHER_UNAVAILABLE")
	require.Greater(t, result.Duration, result.NoWindDuration)
	foundHeadwind := false
	for _, segment := range result.Segments {
		foundHeadwind = foundHeadwind || (segment.TailwindKnots != nil && *segment.TailwindKnots < -50)
	}
	require.True(t, foundHeadwind)
	require.NotNil(t, result.Segments[0].TailwindKnots, "the current level-to-descent-path segment must use the fallback too")
	require.Less(t, result.Segments[0].GroundspeedKnots, result.Segments[0].NoWindGroundspeedKnots)
}

func TestAMANCPHDoesNotInferWindWhileAircraftIsStillClimbing(t *testing.T) {
	input := performanceInput()
	input.AltitudeFeet = 20_000
	input.CruiseAltitudeFeet = 35_000
	track := 90.0
	input.CurrentTrackTrueDegrees = &track

	result, err := EstimatePerformanceWind(context.Background(), nil, failingWind{}, input, PerformanceWindConfig{})

	require.NoError(t, err)
	require.Nil(t, result.WeatherSource)
	require.Contains(t, result.DegradationReasons, "WEATHER_UNAVAILABLE")
}

func TestInterpolateWindAndISAConversions(t *testing.T) {
	east, north, ok := interpolateWind([]WindLevel{{AltitudeFeet: 10000, EastKnots: 40, NorthKnots: 20}, {AltitudeFeet: 0, EastKnots: 0, NorthKnots: 10}}, 5000)
	require.True(t, ok)
	require.InDelta(t, 20, east, .001)
	require.InDelta(t, 15, north, .001)
	require.InDelta(t, 250, tasToIAS(iasToTAS(250, 10000), 10000), .001)
}

func TestSelectProfileRemainsDeterministicForProvenanceConsumers(t *testing.T) {
	now := performanceNow()
	repo := profilesRepository{profiles: []PerformanceProfile{
		{ID: "medium-z", Version: "v1", WakeTurbulenceCategory: CategoryMedium, CruiseTrueAirspeedKnots: 400},
		{ID: "a320", Version: "v2", AircraftICAOTypes: []string{"A320"}, WakeTurbulenceCategory: CategoryMedium, CruiseTrueAirspeedKnots: 430},
		{ID: "medium-a", Version: "v1", WakeTurbulenceCategory: CategoryMedium, CruiseTrueAirspeedKnots: 410},
	}}
	got, _, _ := selectProfile(context.Background(), repo, PerformanceWindInput{PredictionAt: now, AircraftICAO: "a320", WakeTurbulenceCategory: CategoryMedium})
	require.Equal(t, "a320", got.ID)
}

func TestValidWindProfileFreshnessBoundaries(t *testing.T) {
	now := performanceNow()
	requests := []WindSampleRequest{{Position: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 12}, At: now, AltitudeFeet: 10000}}
	profile, err := (fixedWind{now: now}).WindProfile(context.Background(), WindProfileRequest{Samples: requests})
	require.NoError(t, err)
	require.True(t, validWindProfile(profile, requests, now))
	profile.ObservedAt = now.Add(time.Second)
	require.False(t, validWindProfile(profile, requests, now))
}

func firstDescentDistance(segments []descentSegment) float64 {
	distance := 0.0
	for _, segment := range segments {
		if !segment.preTOD {
			break
		}
		distance += segment.distanceNM
	}
	return distance
}

type profilesRepository struct{ profiles []PerformanceProfile }

func (p profilesRepository) PerformanceProfiles(context.Context, time.Time) ([]PerformanceProfile, error) {
	return p.profiles, nil
}

type failingWind struct{}

func (failingWind) WindProfile(context.Context, WindProfileRequest) (WindProfile, error) {
	return WindProfile{}, errors.New("down")
}

type fixedWind struct {
	east         float64
	now, expires time.Time
}

func (w fixedWind) WindProfile(_ context.Context, request WindProfileRequest) (WindProfile, error) {
	expires := w.expires
	if expires.IsZero() {
		expires = w.now.Add(2 * time.Hour)
	}
	samples := make([]WindSample, len(request.Samples))
	for i, sample := range request.Samples {
		samples[i] = WindSample{Position: sample.Position, At: sample.At, Levels: []WindLevel{{AltitudeFeet: 0, EastKnots: w.east}, {AltitudeFeet: 60000, EastKnots: w.east}}}
	}
	return WindProfile{SourceID: "fixture", SourceRevision: "test-v1", ObservedAt: w.now, ExpiresAt: expires, Samples: samples}, nil
}

func performanceNow() time.Time { return time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC) }
func performanceInput() PerformanceWindInput {
	return PerformanceWindInput{
		PredictionAt: performanceNow(), AircraftICAO: "A320", WakeTurbulenceCategory: CategoryMedium,
		AltitudeFeet: 10000, CurrentGroundspeedKnots: 400,
		Remaining: []RouteLeg{{ID: "EAST", DistanceNM: 100, CourseTrueDegrees: 90, Start: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 12}, End: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 14}}},
	}
}
