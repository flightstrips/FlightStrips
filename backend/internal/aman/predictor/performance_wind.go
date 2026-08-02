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
	defaultPerformanceVersion      = "aman-performance-defaults-v1"
	amanCPHModelVersion            = "aman-cph-teta-v3"
	descentFeetPerNM               = 318.4
	terminalWeatherHorizonNM       = 180.0
	surveillanceWindHorizonNM      = 120.0
	weatherNotRequiredSource       = "not-required-outside-terminal-horizon"
	surveillanceWindFallbackSource = "surveillance-groundspeed-fallback"
	maxSurveillanceWindKnots       = 150.0
	minSurveillanceWindAltitudeFt  = 5000.0
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
}

func (c PerformanceWindConfig) normalized() (PerformanceWindConfig, error) {
	if c.MinimumGroundspeedKnots == 0 {
		c.MinimumGroundspeedKnots = defaultMinimumGroundspeedKnots
	}
	if c.MaximumGroundspeedKnots == 0 {
		c.MaximumGroundspeedKnots = defaultMaximumGroundspeedKnots
	}
	if !finite(c.MinimumGroundspeedKnots) || !finite(c.MaximumGroundspeedKnots) || c.MinimumGroundspeedKnots <= 0 || c.MaximumGroundspeedKnots < c.MinimumGroundspeedKnots {
		return c, errPerformanceWindConfig
	}
	return c, nil
}

type PerformanceWindInput struct {
	PredictionAt           time.Time
	AircraftICAO           string
	WakeTurbulenceCategory AircraftCategory
	AltitudeFeet           float64
	// CruiseAltitudeFeet is the planned cruise level used to place TOD. A zero
	// value retains the observed altitude for callers that have no higher filed
	// level. When it is higher than AltitudeFeet, the PPOS-to-TOD profile
	// includes the climb before level cruise.
	CruiseAltitudeFeet      float64
	CurrentGroundspeedKnots float64
	// CurrentTrackTrueDegrees permits a conservative along-track wind estimate
	// when the weather provider is unavailable. It is ignored whenever a valid
	// provider profile exists.
	CurrentTrackTrueDegrees *float64
	// UseObservedGroundspeedBeforeTOD is true only when surveillance confirms
	// the aircraft is established at its cruise level. In that case, observed
	// GS is the best speed source for the remaining cruise distance.
	UseObservedGroundspeedBeforeTOD bool
	// DescentConfirmed anchors the remaining vertical profile at the observed
	// altitude instead of allowing nominal TOD geometry to replace it.
	DescentConfirmed bool
	Remaining        []RouteLeg
}

type PerformanceWindResult struct {
	RawTETA                                         time.Time
	RawRETA                                         time.Time
	NoWindDuration, Duration                        time.Duration
	DistanceToGoNM                                  float64
	Confidence                                      aman.Confidence
	ModelVersion                                    string
	PerformanceProfileID, PerformanceProfileVersion *string
	WeatherSource, WeatherSourceRevision            *string
	DegradationReasons                              []string
	NoWindLegDurations, LegDurations                []time.Duration
	Segments                                        []DescentSegmentCalculation
}

// DescentSegmentCalculation records the physical inputs used for one
// sub-section of the arrival prediction. Tailwind is positive; a nil value
// means that wind was not applied to that sub-section.
type DescentSegmentCalculation struct {
	RouteLegIndex                                    int
	PreTOD                                           bool
	PhaseID, PhaseName, PhaseFormula                 string
	DistanceNM, CourseTrueDegrees                    float64
	StartAltitudeFeet, EndAltitudeFeet, AltitudeFeet float64
	IndicatedAirspeedKnots                           *float64
	NoWindGroundspeedKnots, GroundspeedKnots         float64
	TailwindKnots                                    *float64
	NoWindDuration, Duration                         time.Duration
}

// EstimatePerformanceWind calculates only the latest physical/raw route
// arrival. It intentionally does not persist, smooth, freeze, or project an
// operational TETA; those behaviors belong to #314.
func EstimatePerformanceWind(ctx context.Context, performance AircraftPerformanceRepository, wind WindProfileReader, input PerformanceWindInput, config PerformanceWindConfig) (PerformanceWindResult, error) {
	config, err := config.normalized()
	if err != nil {
		return PerformanceWindResult{}, err
	}
	if !validPredictionInstant(input.PredictionAt) || !finite(input.AltitudeFeet) || input.AltitudeFeet < 0 || !finite(input.CruiseAltitudeFeet) || input.CruiseAltitudeFeet < 0 || !finite(input.CurrentGroundspeedKnots) || input.CurrentGroundspeedKnots <= 0 || len(input.Remaining) == 0 ||
		(input.CurrentTrackTrueDegrees != nil && (!finite(*input.CurrentTrackTrueDegrees) || *input.CurrentTrackTrueDegrees < 0 || *input.CurrentTrackTrueDegrees >= 360)) {
		return PerformanceWindResult{}, errPerformanceWindInput
	}
	for _, leg := range input.Remaining {
		if !validRouteLeg(leg) {
			return PerformanceWindResult{}, errPerformanceWindInput
		}
	}

	_ = performance // AMAN-CPH uses fixed category bands, not external cruise profiles.
	segments := buildDescentSegments(input)
	if len(segments) == 0 {
		return PerformanceWindResult{}, errPerformanceWindInput
	}
	distance := routeDistance(input.Remaining)
	result := PerformanceWindResult{
		Confidence: aman.ConfidenceHigh, ModelVersion: amanCPHModelVersion, DistanceToGoNM: distance,
		PerformanceProfileID: pointerString("aman-cph-speed-bands"), PerformanceProfileVersion: pointerString(amanCPHModelVersion),
	}
	result.RawRETA = input.PredictionAt.Add(durationForDistance(distance, input.CurrentGroundspeedKnots, config))

	base, baseLegDurations, baseSegments := durationBreakdownForSegments(segments, input, nil, config)
	result.NoWindDuration, result.Duration = base, base
	result.NoWindLegDurations, result.LegDurations = baseLegDurations, baseLegDurations
	result.Segments = baseSegments

	if wind == nil {
		if estimated, ok := applySurveillanceWindFallback(result, input, segments, config); ok {
			return estimated, nil
		}
		result = degradeWind(result, "WEATHER_UNAVAILABLE")
		result.RawTETA = input.PredictionAt.Add(result.Duration)
		return result, nil
	}
	requests, weatherSegments := windRequestsForSegments(input, segments, config)
	if len(requests) == 0 {
		result.WeatherSource = pointerString(weatherNotRequiredSource)
		result.RawTETA = input.PredictionAt.Add(result.Duration)
		return result, nil
	}
	weather, err := wind.WindProfile(ctx, WindProfileRequest{Samples: requests})
	if err != nil || !validWindProfile(weather, requests, input.PredictionAt) {
		if estimated, ok := applySurveillanceWindFallback(result, input, segments, config); ok {
			return estimated, nil
		}
		result = degradeWind(result, "WEATHER_UNAVAILABLE")
		result.RawTETA = input.PredictionAt.Add(result.Duration)
		return result, nil
	}
	result.WeatherSource = pointerString(weather.SourceID)
	result.WeatherSourceRevision = pointerString(weather.SourceRevision)
	windDuration, windLegDurations, windSegments, ok := durationForWeather(segments, input, weather, weatherSegments, config)
	if !ok {
		result = degradeWind(result, "WEATHER_INCOMPLETE")
		result.RawTETA = input.PredictionAt.Add(result.Duration)
		return result, nil
	}
	result.Duration = windDuration
	result.LegDurations = windLegDurations
	result.Segments = withNoWindBreakdown(windSegments, baseSegments)
	result.RawTETA = input.PredictionAt.Add(result.Duration)
	return result, nil
}

func applySurveillanceWindFallback(result PerformanceWindResult, input PerformanceWindInput, segments []descentSegment, config PerformanceWindConfig) (PerformanceWindResult, bool) {
	if input.CurrentTrackTrueDegrees == nil || input.AltitudeFeet < minSurveillanceWindAltitudeFt ||
		(!input.UseObservedGroundspeedBeforeTOD && input.CruiseAltitudeFeet > input.AltitudeFeet+100) {
		return result, false
	}
	noWindGroundspeed := iasToTAS(currentModelIAS(input), input.AltitudeFeet)
	alongTrackWind := input.CurrentGroundspeedKnots - noWindGroundspeed
	if !finite(alongTrackWind) || math.Abs(alongTrackWind) > maxSurveillanceWindKnots {
		return result, false
	}
	trackRadians := *input.CurrentTrackTrueDegrees * math.Pi / 180
	east := alongTrackWind * math.Sin(trackRadians)
	north := alongTrackWind * math.Cos(trackRadians)
	weather := make(map[int]WindSample)
	for index, segment := range segments {
		if !segment.surveillanceWindEligible || (segment.preTOD && input.UseObservedGroundspeedBeforeTOD) {
			continue
		}
		weather[index] = WindSample{
			Position: segment.position,
			At:       input.PredictionAt,
			Levels:   []WindLevel{{AltitudeFeet: segment.altitudeFeet, EastKnots: east, NorthKnots: north}},
		}
	}
	if len(weather) == 0 {
		return result, false
	}
	duration, legDurations, windSegments := durationBreakdownForSegments(segments, input, weather, config)
	if duration <= 0 || len(legDurations) != len(input.Remaining) {
		return result, false
	}
	result.Duration = duration
	result.LegDurations = legDurations
	result.Segments = withNoWindBreakdown(windSegments, result.Segments)
	result.WeatherSource = pointerString(surveillanceWindFallbackSource)
	result.WeatherSourceRevision = pointerString(amanCPHModelVersion)
	result = degradeWind(result, "WEATHER_ESTIMATED_FROM_SURVEILLANCE")
	result.RawTETA = input.PredictionAt.Add(result.Duration)
	return result, true
}

func currentModelIAS(input PerformanceWindInput) float64 {
	if !input.DescentConfirmed && routeDistance(input.Remaining) > predictionCruiseAltitude(input)/descentFeetPerNM {
		return cruiseIAS(input.WakeTurbulenceCategory)
	}
	return descentIASForAircraft(input.AircraftICAO, input.WakeTurbulenceCategory, input.AltitudeFeet, cruiseIAS(input.WakeTurbulenceCategory))
}

// withNoWindBreakdown keeps the no-wind side of the explanation tied to the
// no-wind total. Wind sampling may adjust the inferred high-altitude IAS, but
// that must not silently replace the persisted no-wind reference calculation.
func withNoWindBreakdown(wind, noWind []DescentSegmentCalculation) []DescentSegmentCalculation {
	if len(wind) != len(noWind) {
		return wind
	}
	for i := range wind {
		wind[i].NoWindGroundspeedKnots = noWind[i].NoWindGroundspeedKnots
		wind[i].NoWindDuration = noWind[i].NoWindDuration
	}
	return wind
}

type descentSegment struct {
	distanceNM, courseTrueDegrees, startAltitudeFeet, endAltitudeFeet, altitudeFeet float64
	position                                                                        WindCoordinate
	legIndex                                                                        int
	preTOD, weatherEligible, surveillanceWindEligible                               bool
}

type modelPhase struct{ id, name, formula string }

func phaseForSegment(segment descentSegment, input PerformanceWindInput) modelPhase {
	if segment.preTOD {
		if input.UseObservedGroundspeedBeforeTOD {
			return modelPhase{id: "ppos_to_tod", name: "Segment 1 · PPOS → TOD", formula: "time = distance ÷ observed groundspeed"}
		}
		return modelPhase{id: "ppos_to_tod", name: "Segment 1 · PPOS → TOD", formula: "time = distance ÷ TAS from cruise IAS"}
	}
	switch {
	case segment.altitudeFeet > 27000:
		return modelPhase{id: "tod_to_fl270", name: "Segment 2 · TOD → FL270", formula: "time = distance ÷ TAS from high-altitude speed"}
	case segment.altitudeFeet > 10000:
		return modelPhase{id: "fl270_to_fl100", name: "Segment 3 · FL270 → FL100", formula: "time = distance ÷ (TAS from WTC IAS + wind)"}
	case segment.altitudeFeet > 5000:
		return modelPhase{id: "fl100_to_fl050", name: "Segment 4 · FL100 → FL050", formula: "time = distance ÷ (TAS from 250 kt IAS + wind)"}
	case segment.altitudeFeet > 3000:
		return modelPhase{id: "fl050_to_fl030", name: "Segment 5 · FL050 → FL030", formula: "time = distance ÷ (TAS from 210 kt IAS + wind)"}
	default:
		return modelPhase{id: "fl030_to_landing", name: "Segment 6 · FL030 → landing", formula: "time = distance ÷ (TAS from 140 kt IAS + wind)"}
	}
}

func buildDescentSegments(input PerformanceWindInput) []descentSegment {
	total := routeDistance(input.Remaining)
	cruiseAltitude := predictionCruiseAltitude(input)
	boundaries := []float64{0, total}
	if total > terminalWeatherHorizonNM {
		boundaries = append(boundaries, total-terminalWeatherHorizonNM)
	}
	if total > surveillanceWindHorizonNM {
		boundaries = append(boundaries, total-surveillanceWindHorizonNM)
	}
	altitudeAt := func(travelled float64) float64 {
		if input.DescentConfirmed {
			return confirmedDescentAltitudeAtDistance(total, input.AltitudeFeet, travelled)
		}
		return altitudeAtDistance(total, input.AltitudeFeet, cruiseAltitude, travelled)
	}
	if input.DescentConfirmed {
		for _, altitude := range []float64{27000, 10000, 5000, 3000} {
			if input.AltitudeFeet <= 0 || altitude >= input.AltitudeFeet {
				continue
			}
			travelled := total * (1 - altitude/input.AltitudeFeet)
			if travelled > 0 && travelled < total {
				boundaries = append(boundaries, travelled)
			}
		}
	} else {
		climbDistance := max(0, cruiseAltitude-input.AltitudeFeet) / descentFeetPerNM
		if climbDistance > 0 && climbDistance < total {
			boundaries = append(boundaries, climbDistance)
		}
		for _, altitude := range []float64{cruiseAltitude, 27000, 10000, 5000, 3000, 0} {
			travelled := total - min(cruiseAltitude, altitude)/descentFeetPerNM
			if travelled > 0 && travelled < total {
				boundaries = append(boundaries, travelled)
			}
		}
	}
	travelled := 0.0
	for _, leg := range input.Remaining {
		travelled += leg.DistanceNM
		if travelled > 0 && travelled < total {
			boundaries = append(boundaries, travelled)
		}
	}
	slices.Sort(boundaries)
	boundaries = slices.Compact(boundaries)
	segments := make([]descentSegment, 0, len(boundaries)-1)
	for i := 1; i < len(boundaries); i++ {
		from, to := boundaries[i-1], boundaries[i]
		if to <= from {
			continue
		}
		mid := (from + to) / 2
		leg, legIndex, fraction, ok := routePosition(input.Remaining, mid)
		if !ok {
			continue
		}
		remaining := total - mid
		descentDistance := cruiseAltitude / descentFeetPerNM
		preTOD := !input.DescentConfirmed && remaining > descentDistance
		altitude := altitudeAt(mid)
		segments = append(segments, descentSegment{
			distanceNM: to - from, courseTrueDegrees: leg.CourseTrueDegrees,
			startAltitudeFeet: altitudeAt(from), endAltitudeFeet: altitudeAt(to),
			altitudeFeet: altitude, position: interpolateCoordinate(leg.Start, leg.End, fraction),
			legIndex: legIndex, preTOD: preTOD,
			weatherEligible:          from >= total-terminalWeatherHorizonNM,
			surveillanceWindEligible: from >= total-surveillanceWindHorizonNM,
		})
	}
	return segments
}

func confirmedDescentAltitudeAtDistance(totalDistance, currentAltitude, travelled float64) float64 {
	if totalDistance <= 0 || currentAltitude <= 0 {
		return 0
	}
	remainingDistance := max(0, totalDistance-travelled)
	nominalAltitude := remainingDistance * descentFeetPerNM
	if currentAltitude <= totalDistance*descentFeetPerNM {
		// An aircraft already below the nominal three-degree path normally
		// levels until that path catches it. Starting a shallow descent
		// immediately would put every downstream speed band too low.
		return min(currentAltitude, nominalAltitude)
	}
	// Above the nominal path there is no level intercept available. Preserve
	// the observed altitude and distribute the required steeper descent over
	// the remaining route.
	return max(0, currentAltitude*(1-clamp(travelled/totalDistance, 0, 1)))
}

func predictionCruiseAltitude(input PerformanceWindInput) float64 {
	return max(input.AltitudeFeet, input.CruiseAltitudeFeet)
}

func altitudeAtDistance(totalDistance, currentAltitude, cruiseAltitude, travelled float64) float64 {
	climbDistance := max(0, cruiseAltitude-currentAltitude) / descentFeetPerNM
	descentStart := totalDistance - cruiseAltitude/descentFeetPerNM
	if climbDistance > 0 && travelled < climbDistance {
		return min(cruiseAltitude, currentAltitude+travelled*descentFeetPerNM)
	}
	if travelled < descentStart {
		return cruiseAltitude
	}
	return max(0, (totalDistance-travelled)*descentFeetPerNM)
}

func durationForSegments(segments []descentSegment, input PerformanceWindInput, weather map[int]WindSample, config PerformanceWindConfig) time.Duration {
	total, _, _ := durationBreakdownForSegments(segments, input, weather, config)
	return total
}

func durationBreakdownForSegments(segments []descentSegment, input PerformanceWindInput, weather map[int]WindSample, config PerformanceWindConfig) (time.Duration, []time.Duration, []DescentSegmentCalculation) {
	inferredIAS := highAltitudeIAS(input)
	total := time.Duration(0)
	legDurations := make([]time.Duration, len(input.Remaining))
	breakdown := make([]DescentSegmentCalculation, len(segments))
	for i, segment := range segments {
		groundspeed := input.CurrentGroundspeedKnots
		noWindGroundspeed := groundspeed
		var indicatedAirspeed, tailwindComponent *float64
		if segment.preTOD && input.UseObservedGroundspeedBeforeTOD {
			// The observed GS already includes the actual wind effect while the
			// aircraft is established at cruise.
		} else if segment.preTOD {
			ias := cruiseIAS(input.WakeTurbulenceCategory)
			indicatedAirspeed = &ias
			groundspeed = iasToTAS(ias, segment.altitudeFeet)
			noWindGroundspeed = groundspeed
			// A descent-confirmed aircraft may sit below the nominal
			// three-degree path, leaving a short level segment before the
			// calculated descent path catches up. It still needs wind.
			if sample, found := weather[i]; found {
				east, north, ok := interpolateWind(sample.Levels, segment.altitudeFeet)
				if !ok {
					return 0, nil, nil
				}
				component := tailwind(segment.courseTrueDegrees, east, north)
				tailwindComponent = &component
				groundspeed += component
			}
		} else {
			ias := descentIASForAircraft(input.AircraftICAO, input.WakeTurbulenceCategory, segment.altitudeFeet, inferredIAS)
			indicatedAirspeed = &ias
			groundspeed = iasToTAS(ias, segment.altitudeFeet)
			noWindGroundspeed = groundspeed
			if sample, found := weather[i]; found {
				east, north, ok := interpolateWind(sample.Levels, segment.altitudeFeet)
				if !ok {
					return 0, nil, nil
				}
				component := tailwind(segment.courseTrueDegrees, east, north)
				tailwindComponent = &component
				groundspeed += component
			}
		}
		noWindDuration := durationForDistance(segment.distanceNM, noWindGroundspeed, config)
		duration := durationForDistance(segment.distanceNM, groundspeed, config)
		phase := phaseForSegment(segment, input)
		total += duration
		legDurations[segment.legIndex] += duration
		breakdown[i] = DescentSegmentCalculation{RouteLegIndex: segment.legIndex, PreTOD: segment.preTOD, PhaseID: phase.id, PhaseName: phase.name, PhaseFormula: phase.formula, DistanceNM: segment.distanceNM, CourseTrueDegrees: segment.courseTrueDegrees, StartAltitudeFeet: segment.startAltitudeFeet, EndAltitudeFeet: segment.endAltitudeFeet, AltitudeFeet: segment.altitudeFeet, IndicatedAirspeedKnots: indicatedAirspeed, NoWindGroundspeedKnots: noWindGroundspeed, GroundspeedKnots: groundspeed, TailwindKnots: tailwindComponent, NoWindDuration: noWindDuration, Duration: duration}
	}
	return total, legDurations, breakdown
}

func durationForWeather(segments []descentSegment, input PerformanceWindInput, weather WindProfile, weatherSegments []int, config PerformanceWindConfig) (time.Duration, []time.Duration, []DescentSegmentCalculation, bool) {
	if len(weather.Samples) != len(weatherSegments) {
		return 0, nil, nil, false
	}
	samples := make(map[int]WindSample, len(weatherSegments))
	for i, segmentIndex := range weatherSegments {
		samples[segmentIndex] = weather.Samples[i]
	}
	duration, legs, breakdown := durationBreakdownForSegments(segments, input, samples, config)
	return duration, legs, breakdown, duration > 0
}

func windRequestsForSegments(input PerformanceWindInput, segments []descentSegment, config PerformanceWindConfig) ([]WindSampleRequest, []int) {
	requests := make([]WindSampleRequest, 0, len(segments))
	segmentIndexes := make([]int, 0, len(segments))
	elapsed := time.Duration(0)
	inferredIAS := highAltitudeIAS(input)
	for index, segment := range segments {
		speed := input.CurrentGroundspeedKnots
		if segment.preTOD && input.UseObservedGroundspeedBeforeTOD {
			// The same observed GS places later weather samples in time.
		} else if segment.preTOD {
			speed = iasToTAS(cruiseIAS(input.WakeTurbulenceCategory), segment.altitudeFeet)
		} else {
			speed = iasToTAS(descentIASForAircraft(input.AircraftICAO, input.WakeTurbulenceCategory, segment.altitudeFeet, inferredIAS), segment.altitudeFeet)
		}
		duration := durationForDistance(segment.distanceNM, speed, config)
		if segment.weatherEligible && !segment.preTOD {
			requests = append(requests, WindSampleRequest{Position: segment.position, At: input.PredictionAt.Add(elapsed + duration/2), AltitudeFeet: segment.altitudeFeet})
			segmentIndexes = append(segmentIndexes, index)
		}
		elapsed += duration
	}
	return requests, segmentIndexes
}

func descentIAS(category AircraftCategory, altitude, inferredHighIAS float64) float64 {
	return descentIASForAircraft("", category, altitude, inferredHighIAS)
}

// descentIASForAircraft keeps the CPH category defaults while giving common
// turboprops their own slower descent schedule. ICAO designators are used so
// the profile follows the filed aircraft type rather than a callsign pattern.
func descentIASForAircraft(aircraftICAO string, category AircraftCategory, altitude, inferredHighIAS float64) float64 {
	if profile, ok := turbopropDescentProfile(aircraftICAO); ok {
		switch {
		case altitude > 27000:
			return max(inferredHighIAS, profile.high)
		case altitude > 10000:
			return profile.upper
		case altitude > 5000:
			return profile.mid
		case altitude > 3000:
			return profile.low
		default:
			return profile.final
		}
	}
	switch {
	case altitude > 27000:
		return max(inferredHighIAS, 150)
	case altitude > 10000:
		if category == CategoryHeavy || category == CategorySuper {
			return 300
		}
		return 280
	case altitude > 5000:
		return 250
	case altitude > 3000:
		return 210
	default:
		return 140
	}
}

type turbopropProfile struct{ high, upper, mid, low, final float64 }

func turbopropDescentProfile(aircraftICAO string) (turbopropProfile, bool) {
	switch strings.ToUpper(strings.TrimSpace(aircraftICAO)) {
	case "AT43", "AT44", "AT45", "AT46", "AT72", "AT73", "AT75", "AT76", "DH8A", "DH8B", "DH8C", "DH8D":
		return turbopropProfile{high: 230, upper: 220, mid: 200, low: 180, final: 130}, true
	case "TBM7", "TBM8", "TBM9", "TBM":
		return turbopropProfile{high: 220, upper: 210, mid: 190, low: 170, final: 120}, true
	default:
		return turbopropProfile{}, false
	}
}

// highAltitudeIAS preserves a real cruise-speed observation only after the
// aircraft is established at cruise. While climbing, its current groundspeed
// is not a valid input for Segment 2 (TOD to FL270), so that segment starts
// from the filed cruise altitude and category cruise speed instead.
func highAltitudeIAS(input PerformanceWindInput) float64 {
	if !input.UseObservedGroundspeedBeforeTOD {
		return cruiseIAS(input.WakeTurbulenceCategory)
	}
	return tasToIAS(input.CurrentGroundspeedKnots, predictionCruiseAltitude(input))
}

func cruiseIAS(category AircraftCategory) float64 {
	if category == CategoryHeavy || category == CategorySuper {
		return 300
	}
	return 280
}

func densityRatio(altitudeFeet float64) float64 {
	altitudeFeet = max(0, altitudeFeet)
	if altitudeFeet <= 36089 {
		return math.Pow(1-6.87535e-6*altitudeFeet, 4.2561)
	}
	return 0.2971 * math.Exp(-(altitudeFeet-36089)/20806.7)
}

func iasToTAS(ias, altitudeFeet float64) float64 { return ias / math.Sqrt(densityRatio(altitudeFeet)) }
func tasToIAS(tas, altitudeFeet float64) float64 {
	return max(1, tas*math.Sqrt(densityRatio(altitudeFeet)))
}
func tailwind(course, east, north float64) float64 {
	radians := course * math.Pi / 180
	return east*math.Sin(radians) + north*math.Cos(radians)
}
func routeDistance(legs []RouteLeg) float64 {
	total := 0.0
	for _, leg := range legs {
		total += leg.DistanceNM
	}
	return total
}
func routePosition(legs []RouteLeg, travelled float64) (RouteLeg, int, float64, bool) {
	start := 0.0
	for index, leg := range legs {
		if travelled <= start+leg.DistanceNM {
			return leg, index, clamp((travelled-start)/leg.DistanceNM, 0, 1), true
		}
		start += leg.DistanceNM
	}
	return RouteLeg{}, 0, 0, false
}
func interpolateCoordinate(a, b WindCoordinate, fraction float64) WindCoordinate {
	return WindCoordinate{LatitudeDegrees: a.LatitudeDegrees + (b.LatitudeDegrees-a.LatitudeDegrees)*fraction, LongitudeDegrees: a.LongitudeDegrees + (b.LongitudeDegrees-a.LongitudeDegrees)*fraction}
}
func durationForDistance(distance, speed float64, config PerformanceWindConfig) time.Duration {
	return time.Duration(float64(time.Hour) * distance / clamp(speed, config.MinimumGroundspeedKnots, config.MaximumGroundspeedKnots))
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
