package predictor

import (
	"context"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestSelectProfileMapsICAOAndFallsBackDeterministically(t *testing.T) {
	now := performanceNow()
	repo := profilesRepository{profiles: []PerformanceProfile{
		{ID: "medium-z", Version: "v1", WakeTurbulenceCategory: CategoryMedium, CruiseTrueAirspeedKnots: 400},
		{ID: "a320", Version: "v2", AircraftICAOTypes: []string{"A320"}, WakeTurbulenceCategory: CategoryMedium, CruiseTrueAirspeedKnots: 430},
		{ID: "heavy", Version: "v1", WakeTurbulenceCategory: CategoryHeavy, CruiseTrueAirspeedKnots: 450},
		{ID: "medium-a", Version: "v1", WakeTurbulenceCategory: CategoryMedium, CruiseTrueAirspeedKnots: 410},
	}}
	for _, test := range []struct {
		name, icao string
		category   AircraftCategory
		want       string
	}{
		{"known ICAO", "a320", CategoryHeavy, "a320"},
		{"known WTC", "unknown", CategoryHeavy, "heavy"},
		{"unknown uses stable medium fallback", "unknown", "unknown", "medium-a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, _, _ := selectProfile(context.Background(), repo, PerformanceWindInput{PredictionAt: now, AircraftICAO: test.icao, WakeTurbulenceCategory: test.category})
			require.Equal(t, test.want, got.ID)
		})
	}
}

func TestEstimatePerformanceWindProjectsInterpolatedWindAndBoundsCorrection(t *testing.T) {
	now, input := performanceNow(), performanceInput()
	profile := profilesRepository{profiles: []PerformanceProfile{{ID: "A320", Version: "v1", AircraftICAOTypes: []string{"A320"}, WakeTurbulenceCategory: CategoryMedium, CruiseTrueAirspeedKnots: 400}}}
	for _, test := range []struct {
		name    string
		east    float64
		config  PerformanceWindConfig
		compare func(*testing.T, PerformanceWindResult)
	}{
		{"tailwind decreases arrival", 40, PerformanceWindConfig{}, func(t *testing.T, got PerformanceWindResult) {
			require.Less(t, got.Duration, got.NoWindDuration)
			require.Equal(t, aman.ConfidenceHigh, got.Confidence)
		}},
		{"headwind increases arrival", -40, PerformanceWindConfig{}, func(t *testing.T, got PerformanceWindResult) { require.Greater(t, got.Duration, got.NoWindDuration) }},
		{"cap limits extreme headwind", -1000, PerformanceWindConfig{MinimumGroundspeedKnots: 20, MaximumGroundspeedKnots: 900, MaxWindCorrectionPercent: .1}, func(t *testing.T, got PerformanceWindResult) {
			require.Equal(t, time.Duration(float64(got.NoWindDuration)*1.1), got.Duration)
		}},
		{"groundspeed clamp limits extreme tailwind", 1000, PerformanceWindConfig{MinimumGroundspeedKnots: 120, MaximumGroundspeedKnots: 500, MaxWindCorrectionPercent: .9}, func(t *testing.T, got PerformanceWindResult) {
			require.Equal(t, time.Duration(float64(time.Hour)*100/500), got.Duration)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			wind := fixedWind{east: test.east, now: now}
			got, err := EstimatePerformanceWind(context.Background(), profile, wind, input, test.config)
			require.NoError(t, err)
			require.Equal(t, now.Add(got.Duration), got.RawTETA)
			test.compare(t, got)
		})
	}
}

func TestEstimatePerformanceWindPreservesGeometryForRepositoryAndWeatherDegradation(t *testing.T) {
	now, input := performanceNow(), performanceInput()
	for _, test := range []struct {
		name        string
		performance AircraftPerformanceRepository
		wind        WindProfileReader
		want        string
	}{
		{"performance repository failure", failingProfiles{}, fixedWind{now: now}, "PERFORMANCE_PROFILE_UNAVAILABLE"},
		{"missing weather", nil, nil, "WEATHER_UNAVAILABLE"},
		{"provider failure", profilesRepository{}, failingWind{}, "WEATHER_UNAVAILABLE"},
		{"stale cached weather", profilesRepository{}, fixedWind{now: now.Add(-2 * time.Hour), expires: now.Add(-time.Minute)}, "WEATHER_UNAVAILABLE"},
		{"incomplete weather", profilesRepository{}, incompleteWind{now: now}, "WEATHER_INCOMPLETE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := EstimatePerformanceWind(context.Background(), test.performance, test.wind, input, PerformanceWindConfig{})
			require.NoError(t, err)
			require.False(t, got.RawTETA.IsZero())
			require.Equal(t, now.Add(got.NoWindDuration), got.RawTETA)
			require.Equal(t, aman.ConfidenceMedium, got.Confidence)
			require.Contains(t, got.DegradationReasons, test.want)
			require.NotNil(t, got.PerformanceProfileID)
		})
	}
}

func TestInterpolateWind(t *testing.T) {
	east, north, ok := interpolateWind([]WindLevel{{AltitudeFeet: 10000, EastKnots: 40, NorthKnots: 20}, {AltitudeFeet: 0, EastKnots: 0, NorthKnots: 10}}, 5000)
	require.True(t, ok)
	require.InDelta(t, 20, east, .001)
	require.InDelta(t, 15, north, .001)
	_, _, ok = interpolateWind([]WindLevel{{AltitudeFeet: 0}}, 5000)
	require.False(t, ok)
}

func TestWindRequestsUseLegMidpointsAndEstimatedTraversalTimes(t *testing.T) {
	now := performanceNow()
	input := PerformanceWindInput{PredictionAt: now, AltitudeFeet: 10000, Remaining: []RouteLeg{
		{ID: "one", DistanceNM: 100, CourseTrueDegrees: 90, Start: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 10}, End: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 12}},
		{ID: "two", DistanceNM: 100, CourseTrueDegrees: 90, Start: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 12}, End: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 14}},
	}}
	requests := windRequests(input, 400, PerformanceWindConfig{MinimumGroundspeedKnots: 120, MaximumGroundspeedKnots: 600})
	require.Len(t, requests, 2)
	require.InDelta(t, 11, requests[0].Position.LongitudeDegrees, .01)
	require.InDelta(t, 13, requests[1].Position.LongitudeDegrees, .01)
	require.Equal(t, now.Add(7*time.Minute+30*time.Second), requests[0].At)
	require.Equal(t, now.Add(22*time.Minute+30*time.Second), requests[1].At)
}

func TestEstimatePerformanceWindClampsNoWindTAS(t *testing.T) {
	now, input := performanceNow(), performanceInput()
	for _, test := range []struct {
		name          string
		tas, min, max float64
		want          time.Duration
	}{
		{"minimum", 20, 120, 600, time.Duration(float64(time.Hour) * 100 / 120)},
		{"maximum", 900, 120, 600, time.Duration(float64(time.Hour) * 100 / 600)},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := profilesRepository{profiles: []PerformanceProfile{{ID: "profile", Version: "v1", AircraftICAOTypes: []string{"A320"}, CruiseTrueAirspeedKnots: test.tas}}}
			got, err := EstimatePerformanceWind(context.Background(), repository, nil, input, PerformanceWindConfig{MinimumGroundspeedKnots: test.min, MaximumGroundspeedKnots: test.max, MaxWindCorrectionPercent: .2})
			require.NoError(t, err)
			require.Equal(t, test.want, got.NoWindDuration)
			require.Equal(t, now.Add(test.want), got.RawTETA)
		})
	}
}

func TestEstimatePerformanceWindAllowsFutureRouteSamplesWhileWeatherIsFreshAtPrediction(t *testing.T) {
	now, input := performanceNow(), performanceInput()
	input.Remaining[0].DistanceNM = 500 // midpoint traversal is 37m30s ahead at 400 kt.
	repository := profilesRepository{profiles: []PerformanceProfile{{ID: "profile", Version: "v1", AircraftICAOTypes: []string{"A320"}, CruiseTrueAirspeedKnots: 400}}}
	weather := fixedWind{now: now, expires: now.Add(30 * time.Minute)}
	got, err := EstimatePerformanceWind(context.Background(), repository, weather, input, PerformanceWindConfig{})
	require.NoError(t, err)
	require.NotContains(t, got.DegradationReasons, "WEATHER_UNAVAILABLE")
	require.NotNil(t, got.WeatherSource)
	require.Equal(t, "test-v1", *got.WeatherSourceRevision)
}

func TestEstimatePerformanceWindLabelsDeterministicProfileFallback(t *testing.T) {
	input := performanceInput()
	input.AircraftICAO = "UNKNOWN"
	input.WakeTurbulenceCategory = CategoryHeavy
	repository := profilesRepository{profiles: []PerformanceProfile{{ID: "heavy-default", Version: "v1", WakeTurbulenceCategory: CategoryHeavy, CruiseTrueAirspeedKnots: 440}}}
	got, err := EstimatePerformanceWind(context.Background(), repository, nil, input, PerformanceWindConfig{})
	require.NoError(t, err)
	require.Contains(t, got.DegradationReasons, "PERFORMANCE_PROFILE_FALLBACK")
	require.NotContains(t, got.DegradationReasons, "PERFORMANCE_PROFILE_UNAVAILABLE")
}

func TestValidWindProfileFreshnessBoundaries(t *testing.T) {
	now := performanceNow()
	requests := []WindSampleRequest{{Position: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 12}, At: now, AltitudeFeet: 10000}}
	profile, err := (fixedWind{now: now}).WindProfile(context.Background(), WindProfileRequest{Samples: requests})
	require.NoError(t, err)
	require.True(t, validWindProfile(profile, requests, now), "observation at the prediction instant is valid")
	profile.ObservedAt = now.Add(time.Second)
	require.False(t, validWindProfile(profile, requests, now), "future observation is not valid for this prediction")
	profile.ObservedAt, profile.ExpiresAt = now, now.Add(-time.Second)
	require.False(t, validWindProfile(profile, requests, now), "expiry must not precede observation")
}

type profilesRepository struct {
	profiles []PerformanceProfile
	err      error
}

func (p profilesRepository) PerformanceProfiles(context.Context, time.Time) ([]PerformanceProfile, error) {
	return p.profiles, p.err
}

type failingProfiles struct{}

func (failingProfiles) PerformanceProfiles(context.Context, time.Time) ([]PerformanceProfile, error) {
	return nil, errors.New("down")
}

type failingWind struct{}

func (failingWind) WindProfile(context.Context, WindProfileRequest) (WindProfile, error) {
	return WindProfile{}, errors.New("down")
}

type incompleteWind struct{ now time.Time }

func (w incompleteWind) WindProfile(_ context.Context, request WindProfileRequest) (WindProfile, error) {
	samples := make([]WindSample, len(request.Samples))
	for i, sample := range request.Samples {
		samples[i] = WindSample{Position: sample.Position, At: sample.At}
	}
	return WindProfile{SourceID: "fixture", SourceRevision: "test-v1", ObservedAt: w.now, ExpiresAt: w.now.Add(time.Hour), Samples: samples}, nil
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
		samples[i] = WindSample{Position: sample.Position, At: sample.At, Levels: []WindLevel{{AltitudeFeet: 0, EastKnots: w.east}, {AltitudeFeet: 20000, EastKnots: w.east}}}
	}
	return WindProfile{SourceID: "fixture", SourceRevision: "test-v1", ObservedAt: w.now, ExpiresAt: expires, Samples: samples}, nil
}
func performanceNow() time.Time { return time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC) }
func performanceInput() PerformanceWindInput {
	return PerformanceWindInput{PredictionAt: performanceNow(), AircraftICAO: "A320", WakeTurbulenceCategory: CategoryMedium, AltitudeFeet: 10000, Remaining: []RouteLeg{{ID: "EAST", DistanceNM: 100, CourseTrueDegrees: 90, Start: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 12}, End: WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 14}}}}
}
