// Package predictor contains deterministic AMAN prediction inputs. It is
// independent of source adapters, persistence, EuroScope, and navigation
// acquisition. Route-aware calculation will replace the great-circle fallback
// supplied here.
package predictor

import (
	"FlightStrips/internal/aman"
	"math"
	"strings"
	"time"
)

const (
	// DefaultEXOT is the planning-only estimated taxi-out duration added to a
	// filed EOBT. It is never an operational arrival timeline value.
	DefaultEXOT = 15 * time.Minute
	// DefaultMaxFlightDuration accepts an aviation-long filed duration while
	// bounding malformed input before time arithmetic.
	DefaultMaxFlightDuration = 24 * time.Hour
	// DefaultMaxArrivalHorizon bounds planning and route-aware arrival instants
	// relative to the injected clock. It is intentionally longer than the
	// maximum duration to allow a next-service-day EOBT.
	DefaultMaxArrivalHorizon = 36 * time.Hour
	defaultModelVersion      = "aman-baseline-v1"
	defaultConfigVersion     = "aman-baseline-defaults-v1"
)

// Status reports whether an estimate may be consumed, is usable with an
// explicitly degraded source, or needs policy/input before it can be used.
type Status string

const (
	StatusAvailable      Status = "available"
	StatusDegraded       Status = "degraded"
	StatusUnavailable    Status = "unavailable"
	StatusPolicyRequired Status = "policy_required"
)

// Reason is stable caller-visible context for a non-normal baseline result.
// A zero Result never represents an unknown arrival timestamp.
type Reason string

const (
	ReasonNone                             Reason = "none"
	ReasonMissingDepartureTime             Reason = "missing_departure_time"
	ReasonMissingFlightDuration            Reason = "missing_flight_duration"
	ReasonMissingFlightDurationAndGeometry Reason = "missing_flight_duration_and_great_circle_input"
	ReasonNegativeFlightDuration           Reason = "negative_flight_duration"
	ReasonFlightDurationTooLong            Reason = "flight_duration_too_long"
	ReasonInvalidTimestamp                 Reason = "invalid_timestamp"
	ReasonDestinationMismatch              Reason = "destination_mismatch"
	ReasonObservationInFuture              Reason = "observation_in_future"
	ReasonFlightPlanObservedInFuture       Reason = "flight_plan_observed_in_future"
	ReasonObservationTooOld                Reason = "observation_too_old"
	ReasonArrivalInPast                    Reason = "arrival_in_past"
	ReasonTimestampBeyondHorizon           Reason = "timestamp_beyond_arrival_horizon"
	ReasonArrivalBeyondHorizon             Reason = "arrival_beyond_horizon"
	ReasonSuddenAppearancePolicyRequired   Reason = "sudden_appearance_policy_required"
	ReasonMissingGreatCircleInput          Reason = "missing_great_circle_input"
	ReasonMissingAircraftSpeed             Reason = "missing_aircraft_speed"
	ReasonFiledEETMissingAPIUsed           Reason = "filed_eet_missing_api_estimated_flight_time_used"
	ReasonGreatCircleUsed                  Reason = "route_geometry_or_duration_unavailable_great_circle_used"
	ReasonHeldFirstAirborneBaseline        Reason = "held_first_airborne_baseline"
	ReasonUnstableReviewReset              Reason = "unstable_review_reset"
	ReasonRouteAwareSupersedesGreatCircle  Reason = "route_aware_supersedes_great_circle"
)

// AircraftCategory selects a documented default speed for a degraded
// great-circle estimate. Speeds are knots and distances are nautical miles.
type AircraftCategory string

const (
	CategoryLight  AircraftCategory = "light"
	CategoryMedium AircraftCategory = "medium"
	CategoryHeavy  AircraftCategory = "heavy"
	CategorySuper  AircraftCategory = "super"
)

// SpeedDefaults is versioned performance configuration, not handler policy.
// Callers should persist the selected version in ConfigVersion when changing
// these values operationally.
type SpeedDefaults struct {
	Version string
	Knots   map[AircraftCategory]float64
}

// SpeedDefaultsV1 returns the initial explicit ground-speed defaults used only
// for a great-circle fallback. Values are knots: L 180, M 420, H 440, J 460.
func SpeedDefaultsV1() SpeedDefaults {
	return SpeedDefaults{
		Version: "aircraft-category-speeds-v1",
		Knots: map[AircraftCategory]float64{
			CategoryLight:  180,
			CategoryMedium: 420,
			CategoryHeavy:  440,
			CategorySuper:  460,
		},
	}
}

// Config controls the pure reducer. Now is always supplied with Input, making
// observation-age evaluation deterministic and independently testable.
type Config struct {
	EXOT              time.Duration
	MaxObservationAge time.Duration
	// MaxFlightDuration bounds positive filed/API and great-circle durations;
	// zero selects DefaultMaxFlightDuration.
	MaxFlightDuration time.Duration
	// MaxArrivalHorizon bounds accepted departure and arrival instants from Now;
	// zero selects DefaultMaxArrivalHorizon.
	MaxArrivalHorizon time.Duration
	SpeedDefaults     SpeedDefaults
	ModelVersion      string
	ConfigVersion     string
}

// Reducer calculates and holds only the baseline layer. It has no clock,
// repository, source, navigation reader, or mutable process state.
type Reducer struct {
	config Config
}

// NewReducer validates and snapshots its versioned configuration.
func NewReducer(config Config) (Reducer, error) {
	if config.EXOT == 0 {
		config.EXOT = DefaultEXOT
	}
	if config.MaxFlightDuration == 0 {
		config.MaxFlightDuration = DefaultMaxFlightDuration
	}
	if config.MaxArrivalHorizon == 0 {
		config.MaxArrivalHorizon = DefaultMaxArrivalHorizon
	}
	if config.EXOT < 0 || config.MaxObservationAge <= 0 || config.MaxFlightDuration <= 0 || config.MaxArrivalHorizon <= 0 {
		return Reducer{}, errInvalidConfig
	}
	if config.SpeedDefaults.Version == "" {
		config.SpeedDefaults = SpeedDefaultsV1()
	}
	if !validSpeedDefaults(config.SpeedDefaults) {
		return Reducer{}, errInvalidConfig
	}
	if config.ModelVersion == "" {
		config.ModelVersion = defaultModelVersion
	}
	if config.ConfigVersion == "" {
		config.ConfigVersion = defaultConfigVersion
	}
	config.SpeedDefaults.Knots = cloneSpeeds(config.SpeedDefaults.Knots)
	return Reducer{config: config}, nil
}

// Timing exposes each source fact independently so the precedence is visible:
// EOBT is preferred over EstimatedDeparture, and FiledEET is preferred over
// APIEstimatedFlightTime. EOBT receives EXOT; EstimatedDeparture already
// represents departure and does not receive EXOT.
type Timing struct {
	EOBT                   *time.Time
	EstimatedDeparture     *time.Time
	FiledEET               *time.Duration
	APIEstimatedFlightTime *time.Duration
}

// AirborneObservation carries the result of the existing source movement
// classifier. The predictor deliberately does not classify positions, altitude
// or groundspeed itself.
type AirborneObservation struct {
	SensedAt           *time.Time
	PreviouslyObserved bool
}

// GreatCircleInput is a cache-/adapter-supplied position pair. It has no route
// geometry: DistanceNM is calculated with the haversine formula as a degraded
// remaining-distance estimate.
type GreatCircleInput struct {
	LatitudeDegrees             float64
	LongitudeDegrees            float64
	DestinationLatitudeDegrees  float64
	DestinationLongitudeDegrees float64
	AircraftCategory            AircraftCategory
}

// RouteAwareEstimate is supplied by the later route-aware layer. Its presence
// explicitly supersedes a great-circle fallback after an Unstable reset.
type RouteAwareEstimate struct {
	ArrivalAt  time.Time
	Confidence aman.Confidence
}

// Input is package-owned and contains no vendor DTOs. ExpectedDestination is
// the configured AMAN airport that must match Destination case-insensitively.
type Input struct {
	Now                  time.Time
	ExpectedDestination  string
	Destination          string
	Timing               Timing
	Airborne             AirborneObservation
	GreatCircle          *GreatCircleInput
	RouteAware           *RouteAwareEstimate
	FlightPlanRevision   *uint64
	FlightPlanObservedAt *time.Time
	ResetHeldAirborne    bool
}

// Result always labels its source and reason. ArrivalAt and State are nil when
// unavailable or when #320 sudden-appearance policy must decide what happens.
type Result struct {
	Status     Status
	Reason     Reason
	ArrivalAt  *time.Time
	Source     aman.BaselineSource
	Confidence aman.Confidence
	State      *aman.BaselineState
}

// Reduce produces a deterministic planned or first-airborne baseline. A prior
// held baseline wins unchanged until ResetHeldAirborne is explicitly true; the
// caller, not this package, owns the Unstable-review transition and commit.
func (r Reducer) Reduce(input Input, prior *aman.BaselineState) Result {
	// Invalid current facts must never let a persisted baseline cross an
	// airport boundary or make the injected clock irrelevant.
	if !validUTC(input.Now) {
		return unavailable(ReasonInvalidTimestamp)
	}
	if !destinationMatches(input.ExpectedDestination, input.Destination) {
		return unavailable(ReasonDestinationMismatch)
	}
	if prior != nil && !input.ResetHeldAirborne {
		if prior.Validate() != nil {
			return unavailable(ReasonInvalidTimestamp)
		}
		return held(*prior)
	}
	if input.Airborne.SensedAt == nil {
		if input.ResetHeldAirborne {
			return unavailable(ReasonUnstableReviewReset)
		}
		return r.planned(input)
	}
	return r.airborne(input)
}

func (r Reducer) routeAware(input Input) Result {
	if input.RouteAware == nil {
		return unavailable(ReasonUnstableReviewReset)
	}
	if !validUTC(input.RouteAware.ArrivalAt) {
		return unavailable(ReasonInvalidTimestamp)
	}
	if !input.RouteAware.ArrivalAt.After(input.Now) {
		return unavailable(ReasonArrivalInPast)
	}
	if beyondHorizon(input.Now, input.RouteAware.ArrivalAt, r.config.MaxArrivalHorizon) {
		return unavailable(ReasonArrivalBeyondHorizon)
	}
	confidence := input.RouteAware.Confidence
	if !confidence.Valid() || confidence == aman.ConfidenceUnknown {
		confidence = aman.ConfidenceMedium
	}
	arrival := input.RouteAware.ArrivalAt
	return Result{Status: StatusAvailable, Reason: ReasonRouteAwareSupersedesGreatCircle, ArrivalAt: &arrival, Source: aman.BaselineSourceRouteAware, Confidence: confidence}
}

func (r Reducer) airborne(input Input) Result {
	sensed := *input.Airborne.SensedAt
	if !validUTC(sensed) {
		return unavailable(ReasonInvalidTimestamp)
	}
	if sensed.After(input.Now) {
		return unavailable(ReasonObservationInFuture)
	}
	if input.Now.Sub(sensed) > r.config.MaxObservationAge {
		return unavailable(ReasonObservationTooOld)
	}
	if !input.Airborne.PreviouslyObserved {
		return Result{Status: StatusPolicyRequired, Reason: ReasonSuddenAppearancePolicyRequired, Source: aman.BaselineSourceNone, Confidence: aman.ConfidenceUnknown}
	}
	if input.ResetHeldAirborne {
		// The next model layer owns recalculation after an explicit review.
		// Do not infer that review from time, position, or a lifecycle enum.
		return r.routeAware(input)
	}

	duration, source, reason := preferredDuration(input.Timing)
	if duration != nil {
		return r.hold(input, sensed, *duration, source, reason)
	}
	if reason == ReasonNegativeFlightDuration {
		return unavailable(reason)
	}
	return r.greatCircle(input, sensed, reason)
}

func (r Reducer) planned(input Input) Result {
	departure, source, reason := r.departure(input, input.Timing)
	if departure == nil {
		return unavailable(reason)
	}
	duration, durationSource, durationReason := preferredDuration(input.Timing)
	if duration == nil {
		return unavailable(durationReason)
	}
	if *duration > r.config.MaxFlightDuration {
		return unavailable(ReasonFlightDurationTooLong)
	}
	if beyondHorizon(input.Now, *departure, r.config.MaxArrivalHorizon) {
		return unavailable(ReasonTimestampBeyondHorizon)
	}
	arrival := departure.Add(*duration)
	if !arrival.After(input.Now) {
		return unavailable(ReasonArrivalInPast)
	}
	if beyondHorizon(input.Now, arrival, r.config.MaxArrivalHorizon) {
		return unavailable(ReasonArrivalBeyondHorizon)
	}
	return Result{Status: statusFor(durationReason), Reason: durationReason, ArrivalAt: pointer(arrival), Source: plannedSource(source, durationSource), Confidence: confidenceFor(durationReason)}
}

func (r Reducer) departure(input Input, timing Timing) (*time.Time, aman.BaselineSource, Reason) {
	if timing.EOBT != nil {
		if !validUTC(*timing.EOBT) {
			return nil, aman.BaselineSourceNone, ReasonInvalidTimestamp
		}
		if beyondHorizon(input.Now, *timing.EOBT, r.config.MaxArrivalHorizon) {
			return nil, aman.BaselineSourceNone, ReasonTimestampBeyondHorizon
		}
		value := timing.EOBT.Add(r.config.EXOT)
		return &value, aman.BaselineSourcePlannedEOBTFiledEET, ReasonNone
	}
	if timing.EstimatedDeparture != nil {
		if !validUTC(*timing.EstimatedDeparture) {
			return nil, aman.BaselineSourceNone, ReasonInvalidTimestamp
		}
		if beyondHorizon(input.Now, *timing.EstimatedDeparture, r.config.MaxArrivalHorizon) {
			return nil, aman.BaselineSourceNone, ReasonTimestampBeyondHorizon
		}
		value := *timing.EstimatedDeparture
		return &value, aman.BaselineSourcePlannedEstimatedDepartureFiledEET, ReasonNone
	}
	return nil, aman.BaselineSourceNone, ReasonMissingDepartureTime
}

func (r Reducer) hold(input Input, sensed time.Time, duration time.Duration, source aman.BaselineSource, reason Reason) Result {
	if duration <= 0 {
		return unavailable(ReasonNegativeFlightDuration)
	}
	if duration > r.config.MaxFlightDuration {
		return unavailable(ReasonFlightDurationTooLong)
	}
	arrival := sensed.Add(duration)
	if !arrival.After(input.Now) {
		return unavailable(ReasonArrivalInPast)
	}
	if beyondHorizon(input.Now, arrival, r.config.MaxArrivalHorizon) {
		return unavailable(ReasonArrivalBeyondHorizon)
	}
	if input.FlightPlanObservedAt == nil || !validUTC(*input.FlightPlanObservedAt) {
		return unavailable(ReasonInvalidTimestamp)
	}
	if input.FlightPlanObservedAt.After(input.Now) {
		return unavailable(ReasonFlightPlanObservedInFuture)
	}
	state := aman.BaselineState{
		ArrivalAt: arrival, AirborneSensedAt: sensed, Source: source, Confidence: confidenceFor(reason),
		FlightPlanRevision: cloneUint64(input.FlightPlanRevision), FlightPlanObservedAt: *input.FlightPlanObservedAt,
		ModelVersion: r.config.ModelVersion, ConfigVersion: r.config.ConfigVersion,
	}
	if degradation := degradationFor(reason); degradation != nil {
		state.DegradationReason = degradation
	}
	if source == aman.BaselineSourceAirborneGreatCircle {
		state.SpeedDefaultsVersion = r.config.SpeedDefaults.Version
	}
	return Result{Status: statusFor(reason), Reason: reason, ArrivalAt: pointer(arrival), Source: source, Confidence: state.Confidence, State: cloneBaselineState(&state)}
}

func (r Reducer) greatCircle(input Input, sensed time.Time, durationReason Reason) Result {
	if input.GreatCircle == nil || !validCoordinates(input.GreatCircle.LatitudeDegrees, input.GreatCircle.LongitudeDegrees) || !validCoordinates(input.GreatCircle.DestinationLatitudeDegrees, input.GreatCircle.DestinationLongitudeDegrees) {
		if durationReason == ReasonMissingFlightDuration {
			return unavailable(ReasonMissingFlightDurationAndGeometry)
		}
		return unavailable(ReasonMissingGreatCircleInput)
	}
	speed, found := r.config.SpeedDefaults.Knots[input.GreatCircle.AircraftCategory]
	if !found || speed <= 0 {
		return unavailable(ReasonMissingAircraftSpeed)
	}
	distance := greatCircleDistanceNM(input.GreatCircle.LatitudeDegrees, input.GreatCircle.LongitudeDegrees, input.GreatCircle.DestinationLatitudeDegrees, input.GreatCircle.DestinationLongitudeDegrees)
	if distance <= 0 || math.IsNaN(distance) || math.IsInf(distance, 0) {
		return unavailable(ReasonMissingGreatCircleInput)
	}
	durationHours := distance / speed
	if durationHours > float64(r.config.MaxFlightDuration)/float64(time.Hour) {
		return unavailable(ReasonFlightDurationTooLong)
	}
	duration := time.Duration(float64(time.Hour) * durationHours)
	return r.hold(input, sensed, duration, aman.BaselineSourceAirborneGreatCircle, ReasonGreatCircleUsed)
}

func preferredDuration(timing Timing) (*time.Duration, aman.BaselineSource, Reason) {
	if timing.FiledEET != nil {
		if *timing.FiledEET <= 0 {
			return nil, aman.BaselineSourceNone, ReasonNegativeFlightDuration
		}
		value := *timing.FiledEET
		return &value, aman.BaselineSourceAirborneFiledEET, ReasonNone
	}
	if timing.APIEstimatedFlightTime != nil {
		if *timing.APIEstimatedFlightTime <= 0 {
			return nil, aman.BaselineSourceNone, ReasonNegativeFlightDuration
		}
		value := *timing.APIEstimatedFlightTime
		return &value, aman.BaselineSourceAirborneAPIEstimatedFlightTime, ReasonFiledEETMissingAPIUsed
	}
	return nil, aman.BaselineSourceNone, ReasonMissingFlightDuration
}

func plannedSource(departure, duration aman.BaselineSource) aman.BaselineSource {
	if departure == aman.BaselineSourcePlannedEOBTFiledEET {
		if duration == aman.BaselineSourceAirborneFiledEET {
			return aman.BaselineSourcePlannedEOBTFiledEET
		}
		return aman.BaselineSourcePlannedEOBTAPIEstimatedFlightTime
	}
	if duration == aman.BaselineSourceAirborneFiledEET {
		return aman.BaselineSourcePlannedEstimatedDepartureFiledEET
	}
	return aman.BaselineSourcePlannedEstimatedDepartureAPIEstimatedFlightTime
}

func unavailable(reason Reason) Result {
	return Result{Status: StatusUnavailable, Reason: reason, Source: aman.BaselineSourceNone, Confidence: aman.ConfidenceUnknown}
}

func held(state aman.BaselineState) Result {
	copy := cloneBaselineState(&state)
	arrival := copy.ArrivalAt
	return Result{Status: statusForBaselineDegradation(copy.DegradationReason), Reason: ReasonHeldFirstAirborneBaseline, ArrivalAt: &arrival, Source: copy.Source, Confidence: copy.Confidence, State: copy}
}

func statusFor(reason Reason) Status {
	if reason == ReasonNone {
		return StatusAvailable
	}
	return StatusDegraded
}

func statusForBaselineDegradation(reason *aman.BaselineDegradationReason) Status {
	if reason == nil {
		return StatusAvailable
	}
	return StatusDegraded
}

func degradationFor(reason Reason) *aman.BaselineDegradationReason {
	var value aman.BaselineDegradationReason
	switch reason {
	case ReasonFiledEETMissingAPIUsed:
		value = aman.BaselineDegradationFiledEETMissingAPIUsed
	case ReasonGreatCircleUsed:
		value = aman.BaselineDegradationGreatCircleUsed
	default:
		return nil
	}
	return &value
}

func confidenceFor(reason Reason) aman.Confidence {
	if reason == ReasonNone {
		return aman.ConfidenceMedium
	}
	return aman.ConfidenceLow
}

func destinationMatches(expected, actual string) bool {
	expected, actual = strings.ToUpper(strings.TrimSpace(expected)), strings.ToUpper(strings.TrimSpace(actual))
	return expected != "" && expected == actual
}

func validUTC(value time.Time) bool { return !value.IsZero() && value.Location() == time.UTC }

func beyondHorizon(now, value time.Time, horizon time.Duration) bool {
	return value.After(now.Add(horizon))
}

func validCoordinates(latitude, longitude float64) bool {
	return latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180 && (latitude != 0 || longitude != 0)
}

func greatCircleDistanceNM(latitudeA, longitudeA, latitudeB, longitudeB float64) float64 {
	const earthRadiusNM = 3440.065
	latA, latB := latitudeA*math.Pi/180, latitudeB*math.Pi/180
	deltaLat, deltaLon := (latitudeB-latitudeA)*math.Pi/180, (longitudeB-longitudeA)*math.Pi/180
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) + math.Cos(latA)*math.Cos(latB)*math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	return earthRadiusNM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func validSpeedDefaults(defaults SpeedDefaults) bool {
	if strings.TrimSpace(defaults.Version) == "" {
		return false
	}
	for _, category := range []AircraftCategory{CategoryLight, CategoryMedium, CategoryHeavy, CategorySuper} {
		if defaults.Knots[category] <= 0 || math.IsNaN(defaults.Knots[category]) || math.IsInf(defaults.Knots[category], 0) {
			return false
		}
	}
	return true
}

func cloneSpeeds(values map[AircraftCategory]float64) map[AircraftCategory]float64 {
	cloned := make(map[AircraftCategory]float64, len(values))
	for category, speed := range values {
		cloned[category] = speed
	}
	return cloned
}

func pointer(value time.Time) *time.Time { return &value }

func cloneUint64(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneBaselineState(value *aman.BaselineState) *aman.BaselineState {
	if value == nil {
		return nil
	}
	copy := *value
	copy.FlightPlanRevision = cloneUint64(value.FlightPlanRevision)
	if value.DegradationReason != nil {
		degradation := *value.DegradationReason
		copy.DegradationReason = &degradation
	}
	return &copy
}

var errInvalidConfig = &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "baseline predictor configuration is invalid"}
