package predictor

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

const (
	defaultMinimumGroundspeedKnots = 120.0
	defaultMaximumGroundspeedKnots = 600.0
	defaultMaxWindCorrection       = 0.20
	defaultPerformanceVersion      = "aman-performance-defaults-v1"
)

// AircraftPerformanceRepository supplies versioned, provider-neutral profile
// data. The predictor performs deterministic ICAO/WTC selection itself.
type AircraftPerformanceRepository interface {
	PerformanceProfiles(context.Context, time.Time) ([]PerformanceProfile, error)
}

// WindProfileReader supplies spatial and temporal upper-wind profiles. Wind
// components are knots towards east (U) and north (V); altitude is feet.
type WindProfileReader interface {
	WindProfile(context.Context, WindProfileRequest) (WindProfile, error)
}

type PerformanceProfile struct {
	ID, Version             string
	AircraftICAOTypes       []string
	WakeTurbulenceCategory  AircraftCategory
	CruiseTrueAirspeedKnots float64
	ValidFrom, ValidUntil   time.Time
}

type WindCoordinate struct{ LatitudeDegrees, LongitudeDegrees float64 }

type WindProfileRequest struct{ Samples []WindSampleRequest }
type WindSampleRequest struct {
	Position     WindCoordinate
	At           time.Time
	AltitudeFeet float64
}

// WindProfile is metadata plus one vertical profile per requested sample, in
// request order. ExpiresAt is an inclusive upper bound for prediction use.
type WindProfile struct {
	SourceID, SourceRevision string
	ObservedAt, ExpiresAt    time.Time
	Samples                  []WindSample
}
type WindSample struct {
	Position WindCoordinate
	At       time.Time
	Levels   []WindLevel
}
type WindLevel struct {
	AltitudeFeet, EastKnots, NorthKnots float64
}

// RouteLeg is the small trajectory seam consumed by the raw predictor.
// Course is true degrees, distance is NM, and WGS84 coordinates bound the leg.
type RouteLeg struct {
	ID                            string
	DistanceNM, CourseTrueDegrees float64
	Start, End                    WindCoordinate
}

type PerformanceWindConfig struct {
	MinimumGroundspeedKnots, MaximumGroundspeedKnots float64
	// MaxWindCorrectionPercent caps the total duration change around no-wind
	// geometry. 0 selects 20 percent; values must otherwise be in (0, 1).
	MaxWindCorrectionPercent float64
}

func (c PerformanceWindConfig) normalized() (PerformanceWindConfig, error) {
	if c.MinimumGroundspeedKnots == 0 {
		c.MinimumGroundspeedKnots = defaultMinimumGroundspeedKnots
	}
	if c.MaximumGroundspeedKnots == 0 {
		c.MaximumGroundspeedKnots = defaultMaximumGroundspeedKnots
	}
	if c.MaxWindCorrectionPercent == 0 {
		c.MaxWindCorrectionPercent = defaultMaxWindCorrection
	}
	if !finite(c.MinimumGroundspeedKnots) || !finite(c.MaximumGroundspeedKnots) || !finite(c.MaxWindCorrectionPercent) || c.MinimumGroundspeedKnots <= 0 || c.MaximumGroundspeedKnots < c.MinimumGroundspeedKnots || c.MaxWindCorrectionPercent <= 0 || c.MaxWindCorrectionPercent >= 1 {
		return c, errPerformanceWindConfig
	}
	return c, nil
}

type PerformanceWindInput struct {
	PredictionAt           time.Time
	AircraftICAO           string
	WakeTurbulenceCategory AircraftCategory
	AltitudeFeet           float64
	Remaining              []RouteLeg
}

type PerformanceWindResult struct {
	RawTETA                                         time.Time
	NoWindDuration, Duration                        time.Duration
	Confidence                                      aman.Confidence
	PerformanceProfileID, PerformanceProfileVersion *string
	WeatherSource, WeatherSourceRevision            *string
	DegradationReasons                              []string
}

// EstimatePerformanceWind calculates only the latest physical/raw route
// arrival. It intentionally does not persist, smooth, freeze, or project an
// operational TETA; those behaviors belong to #314.
func EstimatePerformanceWind(ctx context.Context, performance AircraftPerformanceRepository, wind WindProfileReader, input PerformanceWindInput, config PerformanceWindConfig) (PerformanceWindResult, error) {
	config, err := config.normalized()
	if err != nil {
		return PerformanceWindResult{}, err
	}
	if !validPredictionInstant(input.PredictionAt) || !finite(input.AltitudeFeet) || input.AltitudeFeet < 0 || len(input.Remaining) == 0 {
		return PerformanceWindResult{}, errPerformanceWindInput
	}
	for _, leg := range input.Remaining {
		if !validRouteLeg(leg) {
			return PerformanceWindResult{}, errPerformanceWindInput
		}
	}

	result := PerformanceWindResult{Confidence: aman.ConfidenceHigh}
	profile, exact, unavailable := selectProfile(ctx, performance, input)
	if unavailable {
		result.DegradationReasons = append(result.DegradationReasons, "PERFORMANCE_PROFILE_UNAVAILABLE")
		result.Confidence = aman.ConfidenceMedium
	} else if !exact {
		result.DegradationReasons = append(result.DegradationReasons, "PERFORMANCE_PROFILE_FALLBACK")
		result.Confidence = aman.ConfidenceMedium
	}
	result.PerformanceProfileID, result.PerformanceProfileVersion = pointerString(profile.ID), pointerString(profile.Version)
	base := durationFor(input.Remaining, profile.CruiseTrueAirspeedKnots, config)
	result.NoWindDuration, result.Duration = base, base

	if wind == nil {
		result = degradeWind(result, "WEATHER_UNAVAILABLE")
		result.RawTETA = input.PredictionAt.Add(result.Duration)
		return result, nil
	}
	requests := windRequests(input, profile.CruiseTrueAirspeedKnots, config)
	weather, err := wind.WindProfile(ctx, WindProfileRequest{Samples: requests})
	if err != nil || !validWindProfile(weather, requests, input.PredictionAt) {
		result = degradeWind(result, "WEATHER_UNAVAILABLE")
		result.RawTETA = input.PredictionAt.Add(result.Duration)
		return result, nil
	}
	result.WeatherSource = pointerString(weather.SourceID)
	result.WeatherSourceRevision = pointerString(weather.SourceRevision)
	windDuration, ok := durationWithWind(input.Remaining, profile.CruiseTrueAirspeedKnots, weather, input.AltitudeFeet, config)
	if !ok {
		result = degradeWind(result, "WEATHER_INCOMPLETE")
		result.RawTETA = input.PredictionAt.Add(result.Duration)
		return result, nil
	}
	low, high := durationBounds(base, config.MaxWindCorrectionPercent)
	result.Duration = maxDuration(low, minDuration(high, windDuration))
	result.RawTETA = input.PredictionAt.Add(result.Duration)
	return result, nil
}

func degradeWind(result PerformanceWindResult, reason string) PerformanceWindResult {
	result.DegradationReasons = append(result.DegradationReasons, reason)
	if result.Confidence == aman.ConfidenceHigh {
		result.Confidence = aman.ConfidenceMedium
	}
	result.RawTETA = time.Time{} // assigned by caller after known prediction instant
	return result
}

func selectProfile(ctx context.Context, repository AircraftPerformanceRepository, input PerformanceWindInput) (PerformanceProfile, bool, bool) {
	profiles := defaultPerformanceProfiles()
	unavailable := repository == nil
	if repository != nil {
		if supplied, err := repository.PerformanceProfiles(ctx, input.PredictionAt); err == nil {
			profiles = supplied
		} else {
			unavailable = true
		}
	}
	valid := make([]PerformanceProfile, 0, len(profiles))
	for _, profile := range profiles {
		if validPerformanceProfile(profile, input.PredictionAt) {
			valid = append(valid, cloneProfile(profile))
		}
	}
	if len(valid) == 0 {
		valid = defaultPerformanceProfiles()
		unavailable = true
	}
	slices.SortFunc(valid, func(a, b PerformanceProfile) int {
		if a.ID != b.ID {
			return strings.Compare(a.ID, b.ID)
		}
		return strings.Compare(a.Version, b.Version)
	})
	icao := strings.ToUpper(strings.TrimSpace(input.AircraftICAO))
	for _, profile := range valid {
		for _, candidate := range profile.AircraftICAOTypes {
			if icao != "" && icao == strings.ToUpper(strings.TrimSpace(candidate)) {
				return profile, true, unavailable
			}
		}
	}
	for _, profile := range valid {
		if len(profile.AircraftICAOTypes) == 0 && profile.WakeTurbulenceCategory == input.WakeTurbulenceCategory {
			return profile, false, unavailable
		}
	}
	// Medium is the deterministic final fallback for missing/unknown WTC.
	for _, profile := range valid {
		if len(profile.AircraftICAOTypes) == 0 && profile.WakeTurbulenceCategory == CategoryMedium {
			return profile, false, unavailable
		}
	}
	return defaultPerformanceProfiles()[1], false, true
}

func defaultPerformanceProfiles() []PerformanceProfile {
	return []PerformanceProfile{
		{ID: "fallback-light", Version: defaultPerformanceVersion, WakeTurbulenceCategory: CategoryLight, CruiseTrueAirspeedKnots: 180},
		{ID: "fallback-medium", Version: defaultPerformanceVersion, WakeTurbulenceCategory: CategoryMedium, CruiseTrueAirspeedKnots: 420},
		{ID: "fallback-heavy", Version: defaultPerformanceVersion, WakeTurbulenceCategory: CategoryHeavy, CruiseTrueAirspeedKnots: 440},
		{ID: "fallback-super", Version: defaultPerformanceVersion, WakeTurbulenceCategory: CategorySuper, CruiseTrueAirspeedKnots: 460},
	}
}

func windRequests(input PerformanceWindInput, tas float64, config PerformanceWindConfig) []WindSampleRequest {
	requests := make([]WindSampleRequest, len(input.Remaining))
	elapsed := time.Duration(0)
	for i, leg := range input.Remaining {
		half := durationFor([]RouteLeg{leg}, tas, config) / 2
		requests[i] = WindSampleRequest{Position: midpoint(leg.Start, leg.End), At: input.PredictionAt.Add(elapsed + half), AltitudeFeet: input.AltitudeFeet}
		elapsed += half * 2
	}
	return requests
}

func durationWithWind(legs []RouteLeg, tas float64, profile WindProfile, altitude float64, config PerformanceWindConfig) (time.Duration, bool) {
	total := time.Duration(0)
	for i, leg := range legs {
		east, north, ok := interpolateWind(profile.Samples[i].Levels, altitude)
		if !ok {
			return 0, false
		}
		radians := leg.CourseTrueDegrees * math.Pi / 180
		tailwind := east*math.Sin(radians) + north*math.Cos(radians)
		groundspeed := clamp(tas+tailwind, config.MinimumGroundspeedKnots, config.MaximumGroundspeedKnots)
		total += durationFor([]RouteLeg{leg}, groundspeed, config)
	}
	return total, true
}

// interpolateWind linearly interpolates U/V components at the requested
// altitude. Values outside the provider's vertical coverage are unusable.
func interpolateWind(levels []WindLevel, altitude float64) (float64, float64, bool) {
	if len(levels) == 0 || !finite(altitude) {
		return 0, 0, false
	}
	values := slices.Clone(levels)
	slices.SortFunc(values, func(a, b WindLevel) int { return cmp.Compare(a.AltitudeFeet, b.AltitudeFeet) })
	for _, level := range values {
		if !finite(level.AltitudeFeet) || !finite(level.EastKnots) || !finite(level.NorthKnots) {
			return 0, 0, false
		}
		if level.AltitudeFeet == altitude {
			return level.EastKnots, level.NorthKnots, true
		}
	}
	for i := 1; i < len(values); i++ {
		if values[i].AltitudeFeet <= values[i-1].AltitudeFeet {
			return 0, 0, false
		}
	}
	if altitude < values[0].AltitudeFeet || altitude > values[len(values)-1].AltitudeFeet {
		return 0, 0, false
	}
	for i := 1; i < len(values); i++ {
		if altitude < values[i].AltitudeFeet {
			low, high := values[i-1], values[i]
			fraction := (altitude - low.AltitudeFeet) / (high.AltitudeFeet - low.AltitudeFeet)
			return low.EastKnots + (high.EastKnots-low.EastKnots)*fraction, low.NorthKnots + (high.NorthKnots-low.NorthKnots)*fraction, true
		}
	}
	return 0, 0, false
}

func validWindProfile(profile WindProfile, requests []WindSampleRequest, at time.Time) bool {
	if strings.TrimSpace(profile.SourceID) == "" || strings.TrimSpace(profile.SourceRevision) == "" || !validPredictionInstant(profile.ObservedAt) || !validPredictionInstant(profile.ExpiresAt) || profile.ObservedAt.After(at) || profile.ExpiresAt.Before(profile.ObservedAt) || profile.ExpiresAt.Before(at) || len(profile.Samples) != len(requests) {
		return false
	}
	for i, sample := range profile.Samples {
		if !validPredictionInstant(sample.At) || !sample.At.Equal(requests[i].At) || math.Abs(sample.Position.LatitudeDegrees-requests[i].Position.LatitudeDegrees) > .000001 || math.Abs(sample.Position.LongitudeDegrees-requests[i].Position.LongitudeDegrees) > .000001 {
			return false
		}
	}
	return true
}
func validPerformanceProfile(p PerformanceProfile, at time.Time) bool {
	return strings.TrimSpace(p.ID) != "" && strings.TrimSpace(p.Version) != "" && finite(p.CruiseTrueAirspeedKnots) && p.CruiseTrueAirspeedKnots > 0 && (p.ValidFrom.IsZero() || !at.Before(p.ValidFrom)) && (p.ValidUntil.IsZero() || !at.After(p.ValidUntil))
}
func validRouteLeg(leg RouteLeg) bool {
	return strings.TrimSpace(leg.ID) != "" && finite(leg.DistanceNM) && leg.DistanceNM > 0 && finite(leg.CourseTrueDegrees) && leg.CourseTrueDegrees >= 0 && leg.CourseTrueDegrees < 360 && validWindCoordinate(leg.Start) && validWindCoordinate(leg.End)
}
func validPredictionInstant(v time.Time) bool { return !v.IsZero() && v.Location() == time.UTC }
func durationFor(legs []RouteLeg, speed float64, config PerformanceWindConfig) time.Duration {
	distance := 0.0
	for _, leg := range legs {
		distance += leg.DistanceNM
	}
	return time.Duration(float64(time.Hour) * distance / clamp(speed, config.MinimumGroundspeedKnots, config.MaximumGroundspeedKnots))
}
func durationBounds(base time.Duration, percent float64) (time.Duration, time.Duration) {
	return time.Duration(float64(base) * (1 - percent)), time.Duration(float64(base) * (1 + percent))
}
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
func clamp(v, low, high float64) float64 { return math.Max(low, math.Min(high, v)) }
func validWindCoordinate(value WindCoordinate) bool {
	return finite(value.LatitudeDegrees) && finite(value.LongitudeDegrees) && value.LatitudeDegrees >= -90 && value.LatitudeDegrees <= 90 && value.LongitudeDegrees >= -180 && value.LongitudeDegrees <= 180
}
func midpoint(a, b WindCoordinate) WindCoordinate {
	latA, lonA := a.LatitudeDegrees*math.Pi/180, a.LongitudeDegrees*math.Pi/180
	latB, lonB := b.LatitudeDegrees*math.Pi/180, b.LongitudeDegrees*math.Pi/180
	x, y, z := math.Cos(latA)*math.Cos(lonA)+math.Cos(latB)*math.Cos(lonB), math.Cos(latA)*math.Sin(lonA)+math.Cos(latB)*math.Sin(lonB), math.Sin(latA)+math.Sin(latB)
	return WindCoordinate{LatitudeDegrees: math.Atan2(z, math.Hypot(x, y)) * 180 / math.Pi, LongitudeDegrees: math.Atan2(y, x) * 180 / math.Pi}
}
func cloneProfile(p PerformanceProfile) PerformanceProfile {
	p.AircraftICAOTypes = slices.Clone(p.AircraftICAOTypes)
	return p
}
func pointerString(value string) *string { copy := value; return &copy }
func finite(value float64) bool          { return !math.IsNaN(value) && !math.IsInf(value, 0) }

var (
	errPerformanceWindConfig = errors.New("performance/wind predictor configuration is invalid")
	errPerformanceWindInput  = fmt.Errorf("performance/wind predictor input is invalid")
)
