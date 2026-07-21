package predictor

import (
	"FlightStrips/internal/aman"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var baselineNow = time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)

func TestPlannedTimingPrecedenceTable(t *testing.T) {
	eobt := baselineNow.Add(time.Hour)
	estimatedDeparture := baselineNow.Add(2 * time.Hour)
	filed := 90 * time.Minute
	api := 2 * time.Hour
	reducer := testReducer(t)

	for _, test := range []struct {
		name    string
		timing  Timing
		arrival time.Time
		source  aman.BaselineSource
		status  Status
		reason  Reason
	}{
		{"EOBT and filed EET", Timing{EOBT: &eobt, EstimatedDeparture: &estimatedDeparture, FiledEET: &filed, APIEstimatedFlightTime: &api}, eobt.Add(DefaultEXOT + filed), aman.BaselineSourcePlannedEOBTFiledEET, StatusAvailable, ReasonNone},
		{"estimated departure and filed EET", Timing{EstimatedDeparture: &estimatedDeparture, FiledEET: &filed, APIEstimatedFlightTime: &api}, estimatedDeparture.Add(filed), aman.BaselineSourcePlannedEstimatedDepartureFiledEET, StatusAvailable, ReasonNone},
		{"EOBT and API estimate", Timing{EOBT: &eobt, APIEstimatedFlightTime: &api}, eobt.Add(DefaultEXOT + api), aman.BaselineSourcePlannedEOBTAPIEstimatedFlightTime, StatusDegraded, ReasonFiledEETMissingAPIUsed},
		{"estimated departure and API estimate", Timing{EstimatedDeparture: &estimatedDeparture, APIEstimatedFlightTime: &api}, estimatedDeparture.Add(api), aman.BaselineSourcePlannedEstimatedDepartureAPIEstimatedFlightTime, StatusDegraded, ReasonFiledEETMissingAPIUsed},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := reducer.Reduce(plannedInput(test.timing), nil)
			require.Equal(t, test.status, result.Status)
			require.Equal(t, test.reason, result.Reason)
			require.Equal(t, test.source, result.Source)
			require.Equal(t, test.arrival, *result.ArrivalAt)
		})
	}
}

func TestBaselineRejectsTimestampDurationAndDestinationBoundaries(t *testing.T) {
	reducer := testReducer(t)
	eobt := baselineNow.Add(time.Hour)
	filed := time.Hour
	observed := baselineNow.Add(-time.Minute)
	flightPlanObserved := baselineNow.Add(-2 * time.Minute)

	for _, test := range []struct {
		name   string
		input  Input
		reason Reason
	}{
		{"destination mismatch", Input{Now: baselineNow, ExpectedDestination: "EKCH", Destination: "EKRN", Timing: Timing{EOBT: &eobt, FiledEET: &filed}}, ReasonDestinationMismatch},
		{"non UTC EOBT", plannedInput(Timing{EOBT: timePointer(eobt.In(time.FixedZone("CEST", 7200))), FiledEET: &filed}), ReasonInvalidTimestamp},
		{"negative filed EET", plannedInput(Timing{EOBT: &eobt, FiledEET: durationPointer(-time.Minute)}), ReasonNegativeFlightDuration},
		{"past planned arrival", plannedInput(Timing{EOBT: timePointer(baselineNow.Add(-2 * time.Hour)), FiledEET: &filed}), ReasonArrivalInPast},
		{"future observation", airborneInput(baselineNow.Add(time.Second), &filed, &flightPlanObserved), ReasonObservationInFuture},
		{"over age observation", airborneInput(baselineNow.Add(-3*time.Minute), &filed, &flightPlanObserved), ReasonObservationTooOld},
		{"past airborne arrival", airborneInput(observed, durationPointer(30*time.Second), &flightPlanObserved), ReasonArrivalInPast},
		{"no duration or geometry", airborneInput(observed, nil, &flightPlanObserved), ReasonMissingFlightDurationAndGeometry},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := reducer.Reduce(test.input, nil)
			require.Equal(t, StatusUnavailable, result.Status)
			require.Equal(t, test.reason, result.Reason)
			require.Nil(t, result.ArrivalAt)
			require.Nil(t, result.State)
		})
	}
}

func TestAirborneAPIFallbackPersistsTypedDegradation(t *testing.T) {
	reducer := testReducer(t)
	api := 90 * time.Minute
	observed := baselineNow.Add(-time.Minute)
	input := airborneInput(observed, nil, &observed)
	input.Timing.APIEstimatedFlightTime = &api
	result := reducer.Reduce(input, nil)
	require.Equal(t, StatusDegraded, result.Status)
	require.Equal(t, ReasonFiledEETMissingAPIUsed, result.Reason)
	require.Equal(t, aman.BaselineSourceAirborneAPIEstimatedFlightTime, result.Source)
	require.NotNil(t, result.State.DegradationReason)
	require.Equal(t, aman.BaselineDegradationFiledEETMissingAPIUsed, *result.State.DegradationReason)
	require.NoError(t, result.State.Validate())
}

func TestFirstAirborneHoldsAcrossFreshReducerUntilExplicitReset(t *testing.T) {
	reducer := testReducer(t)
	filed := 90 * time.Minute
	flightPlanObserved := baselineNow.Add(-time.Minute)
	input := airborneInput(baselineNow.Add(-time.Minute), &filed, &flightPlanObserved)

	first := reducer.Reduce(input, nil)
	require.Equal(t, StatusAvailable, first.Status)
	require.Equal(t, aman.BaselineSourceAirborneFiledEET, first.Source)
	require.NotNil(t, first.State)
	require.Equal(t, (*input.Airborne.SensedAt).Add(filed), *first.ArrivalAt)

	// This mimics a restarted process receiving a later source observation and
	// the state reconstructed by the #306 aggregate repository.
	freshReducer := testReducer(t)
	later := input
	later.Now = baselineNow.Add(time.Minute)
	later.Airborne.SensedAt = timePointer(baselineNow)
	later.Timing.FiledEET = durationPointer(3 * time.Hour)
	held := freshReducer.Reduce(later, first.State)
	require.Equal(t, ReasonHeldFirstAirborneBaseline, held.Reason)
	require.Equal(t, *first.State, *held.State)
	require.Equal(t, *first.ArrivalAt, *held.ArrivalAt)

	later.ResetHeldAirborne = true
	reset := freshReducer.Reduce(later, first.State)
	require.Equal(t, StatusUnavailable, reset.Status)
	require.Equal(t, ReasonUnstableReviewReset, reset.Reason)
}

func TestFirstSeenAirborneRequiresSuddenAppearancePolicy(t *testing.T) {
	reducer := testReducer(t)
	filed := time.Hour
	flightPlanObserved := baselineNow.Add(-time.Minute)
	input := airborneInput(baselineNow.Add(-time.Minute), &filed, &flightPlanObserved)
	input.Airborne.PreviouslyObserved = false

	result := reducer.Reduce(input, nil)
	require.Equal(t, StatusPolicyRequired, result.Status)
	require.Equal(t, ReasonSuddenAppearancePolicyRequired, result.Reason)
	require.Nil(t, result.ArrivalAt)
	require.Nil(t, result.State)
}

func TestRouteAwareCannotBypassBaselineObservationGate(t *testing.T) {
	reducer := testReducer(t)
	filed := time.Hour
	observed := baselineNow.Add(-time.Minute)
	route := &RouteAwareEstimate{ArrivalAt: baselineNow.Add(20 * time.Minute), Confidence: aman.ConfidenceHigh}
	prior := heldBaseline(t, reducer, observed, filed)

	for _, test := range []struct {
		name   string
		input  Input
		prior  *aman.BaselineState
		status Status
		reason Reason
		source aman.BaselineSource
	}{
		{
			name: "planned input remains planned", input: Input{Now: baselineNow, ExpectedDestination: "EKCH", Destination: "EKCH", Timing: Timing{EOBT: timePointer(baselineNow.Add(time.Hour)), FiledEET: &filed}, RouteAware: route},
			status: StatusAvailable, reason: ReasonNone, source: aman.BaselineSourcePlannedEOBTFiledEET,
		},
		{
			name: "normal first airborne remains filed baseline", input: Input{Now: baselineNow, ExpectedDestination: "EKCH", Destination: "EKCH", Timing: Timing{FiledEET: &filed}, RouteAware: route, Airborne: AirborneObservation{SensedAt: &observed, PreviouslyObserved: true}, FlightPlanObservedAt: &observed},
			status: StatusAvailable, reason: ReasonNone, source: aman.BaselineSourceAirborneFiledEET,
		},
		{
			name: "future airborne observation", input: routeInput(baselineNow.Add(time.Second), true, route),
			status: StatusUnavailable, reason: ReasonObservationInFuture, source: aman.BaselineSourceNone,
		},
		{
			name: "stale airborne observation", input: routeInput(baselineNow.Add(-3*time.Minute), true, route),
			status: StatusUnavailable, reason: ReasonObservationTooOld, source: aman.BaselineSourceNone,
		},
		{
			name: "sudden appearance", input: routeInput(observed, false, route),
			status: StatusPolicyRequired, reason: ReasonSuddenAppearancePolicyRequired, source: aman.BaselineSourceNone,
		},
		{
			name: "prior cannot cross destination", input: Input{Now: baselineNow, ExpectedDestination: "EKCH", Destination: "EKRN"}, prior: prior,
			status: StatusUnavailable, reason: ReasonDestinationMismatch, source: aman.BaselineSourceNone,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := reducer.Reduce(test.input, test.prior)
			require.Equal(t, test.status, result.Status)
			require.Equal(t, test.reason, result.Reason)
			require.Equal(t, test.source, result.Source)
		})
	}
}

func TestDurationAndTimestampBoundsAreTypedAndCheckedBeforeArrivalArithmetic(t *testing.T) {
	reducer, err := NewReducer(Config{MaxObservationAge: 2 * time.Minute, MaxFlightDuration: 2 * time.Hour, MaxArrivalHorizon: 3 * time.Hour})
	require.NoError(t, err)
	flightPlanObserved := baselineNow.Add(-time.Minute)
	departure := baselineNow.Add(time.Hour)

	for _, test := range []struct {
		name   string
		input  Input
		reason Reason
	}{
		{"zero duration", plannedInput(Timing{EOBT: &departure, FiledEET: durationPointer(0)}), ReasonNegativeFlightDuration},
		{"negative duration", plannedInput(Timing{EOBT: &departure, FiledEET: durationPointer(-time.Minute)}), ReasonNegativeFlightDuration},
		{"maximum duration edge", plannedInput(Timing{EOBT: &departure, FiledEET: durationPointer(2 * time.Hour)}), ReasonArrivalBeyondHorizon},
		{"over maximum duration", plannedInput(Timing{EOBT: &departure, FiledEET: durationPointer(2*time.Hour + time.Second)}), ReasonFlightDurationTooLong},
		{"far future departure", plannedInput(Timing{EOBT: timePointer(baselineNow.Add(4 * time.Hour)), FiledEET: durationPointer(time.Hour)}), ReasonTimestampBeyondHorizon},
		{"far future route result", resetRouteInput(baselineNow.Add(-time.Minute), &RouteAwareEstimate{ArrivalAt: baselineNow.Add(4 * time.Hour), Confidence: aman.ConfidenceHigh}), ReasonArrivalBeyondHorizon},
		{"future flight plan observed", airborneInput(baselineNow.Add(-time.Minute), durationPointer(time.Hour), timePointer(baselineNow.Add(time.Second))), ReasonFlightPlanObservedInFuture},
		{"reset without current airborne route", Input{Now: baselineNow, ExpectedDestination: "EKCH", Destination: "EKCH", ResetHeldAirborne: true, FlightPlanObservedAt: &flightPlanObserved}, ReasonUnstableReviewReset},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := reducer.Reduce(test.input, nil)
			if test.name == "maximum duration edge" {
				require.Equal(t, StatusUnavailable, result.Status)
			} else {
				require.Equal(t, StatusUnavailable, result.Status)
			}
			require.Equal(t, test.reason, result.Reason)
		})
	}

	// Exactly at the max duration remains valid when it is within the horizon.
	withinHorizon, err := NewReducer(Config{MaxObservationAge: 2 * time.Minute, MaxFlightDuration: 2 * time.Hour, MaxArrivalHorizon: 4 * time.Hour})
	require.NoError(t, err)
	result := withinHorizon.Reduce(plannedInput(Timing{EOBT: &departure, FiledEET: durationPointer(2 * time.Hour)}), nil)
	require.Equal(t, StatusAvailable, result.Status)
}

func TestBaselineStateDoesNotAliasInputOrPriorPointers(t *testing.T) {
	reducer := testReducer(t)
	filed := time.Hour
	observed := baselineNow.Add(-time.Minute)
	revision := uint64(4)
	input := airborneInput(observed, &filed, timePointer(observed))
	input.FlightPlanRevision = &revision
	first := reducer.Reduce(input, nil)
	require.NotNil(t, first.State)

	revision = 9
	require.Equal(t, uint64(4), *first.State.FlightPlanRevision)
	degradation := aman.BaselineDegradationGreatCircleUsed
	prior := *first.State
	prior.DegradationReason = &degradation
	prior.Source = aman.BaselineSourceAirborneGreatCircle
	prior.SpeedDefaultsVersion = "speed-v1"
	held := reducer.Reduce(Input{Now: baselineNow, ExpectedDestination: "EKCH", Destination: "EKCH"}, &prior)
	require.NotNil(t, held.State)
	*held.State.FlightPlanRevision = 12
	*held.State.DegradationReason = aman.BaselineDegradationFiledEETMissingAPIUsed
	require.Equal(t, uint64(4), *prior.FlightPlanRevision)
	require.Equal(t, aman.BaselineDegradationGreatCircleUsed, *prior.DegradationReason)
}

func TestGreatCircleFallbackUsesVersionedCategorySpeedAndRouteAwareSupersedesIt(t *testing.T) {
	reducer := testReducer(t)
	flightPlanObserved := baselineNow.Add(-time.Minute)
	input := airborneInput(baselineNow.Add(-time.Minute), nil, &flightPlanObserved)
	input.GreatCircle = &GreatCircleInput{
		LatitudeDegrees: 55.0, LongitudeDegrees: 11.0,
		DestinationLatitudeDegrees: 55.6, DestinationLongitudeDegrees: 12.6,
		AircraftCategory: CategoryMedium,
	}

	fallback := reducer.Reduce(input, nil)
	require.Equal(t, StatusDegraded, fallback.Status)
	require.Equal(t, aman.BaselineSourceAirborneGreatCircle, fallback.Source)
	require.Equal(t, ReasonGreatCircleUsed, fallback.Reason)
	require.Equal(t, "aircraft-category-speeds-v1", reducer.config.SpeedDefaults.Version)
	require.NotNil(t, fallback.State)

	input.RouteAware = &RouteAwareEstimate{ArrivalAt: baselineNow.Add(20 * time.Minute), Confidence: aman.ConfidenceHigh}
	routeAware := reducer.Reduce(input, fallback.State)
	require.Equal(t, ReasonHeldFirstAirborneBaseline, routeAware.Reason, "a held first-airborne baseline is not silently replaced")

	input.ResetHeldAirborne = true
	routeAware = reducer.Reduce(input, fallback.State)
	require.Equal(t, StatusAvailable, routeAware.Status)
	require.Equal(t, ReasonRouteAwareSupersedesGreatCircle, routeAware.Reason)
	require.Equal(t, aman.BaselineSourceRouteAware, routeAware.Source)
}

func TestGreatCircleAndAircraftCategoryDefaultsUseDocumentedUnits(t *testing.T) {
	// One degree of latitude is about 60 NM. The calculation uses WGS84-style
	// degrees at its boundary and returns nautical miles.
	distance := greatCircleDistanceNM(55, 12, 56, 12)
	require.InDelta(t, 60.04, distance, 0.05)
	defaults := SpeedDefaultsV1()
	require.Equal(t, map[AircraftCategory]float64{
		CategoryLight: 180, CategoryMedium: 420, CategoryHeavy: 440, CategorySuper: 460,
	}, defaults.Knots)

	flightPlanObserved := baselineNow.Add(-time.Minute)
	for category, speed := range defaults.Knots {
		t.Run(string(category), func(t *testing.T) {
			reducer := testReducer(t)
			input := airborneInput(baselineNow.Add(-time.Minute), nil, &flightPlanObserved)
			input.GreatCircle = &GreatCircleInput{LatitudeDegrees: 55, LongitudeDegrees: 12, DestinationLatitudeDegrees: 56, DestinationLongitudeDegrees: 12, AircraftCategory: category}
			result := reducer.Reduce(input, nil)
			require.Equal(t, StatusDegraded, result.Status)
			require.Equal(t, ReasonGreatCircleUsed, result.Reason)
			require.Equal(t, aman.BaselineSourceAirborneGreatCircle, result.Source)
			expected := time.Duration(float64(time.Hour) * distance / speed)
			require.InDelta(t, expected, result.State.ArrivalAt.Sub(*input.Airborne.SensedAt), float64(time.Second))
			require.Equal(t, defaults.Version, result.State.SpeedDefaultsVersion)
		})
	}
}

func TestGreatCirclePersistsCustomSpeedDefaultVersion(t *testing.T) {
	defaults := SpeedDefaultsV1()
	defaults.Version = "aircraft-category-speeds-v2"
	defaults.Knots[CategoryMedium] = 400
	reducer, err := NewReducer(Config{MaxObservationAge: 2 * time.Minute, SpeedDefaults: defaults, ConfigVersion: "configuration-v2"})
	require.NoError(t, err)
	flightPlanObserved := baselineNow.Add(-time.Minute)
	input := airborneInput(baselineNow.Add(-time.Minute), nil, &flightPlanObserved)
	input.GreatCircle = &GreatCircleInput{LatitudeDegrees: 55, LongitudeDegrees: 12, DestinationLatitudeDegrees: 56, DestinationLongitudeDegrees: 12, AircraftCategory: CategoryMedium}
	result := reducer.Reduce(input, nil)
	require.Equal(t, "aircraft-category-speeds-v2", result.State.SpeedDefaultsVersion)
	require.NoError(t, result.State.Validate())
}

func TestPredictorPackageDoesNotDependOnSourceOrEuroScopeAdapters(t *testing.T) {
	command := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", ".")
	output, err := command.Output()
	require.NoError(t, err)
	for _, importPath := range strings.Fields(string(output)) {
		require.NotContains(t, importPath, "vatsim")
		require.NotContains(t, importPath, "euroscope")
		require.NotContains(t, importPath, "navdata/airacnet")
	}
}

func testReducer(t *testing.T) Reducer {
	t.Helper()
	reducer, err := NewReducer(Config{MaxObservationAge: 2 * time.Minute})
	require.NoError(t, err)
	return reducer
}

func plannedInput(timing Timing) Input {
	return Input{Now: baselineNow, ExpectedDestination: "EKCH", Destination: "EKCH", Timing: timing}
}

func airborneInput(sensed time.Time, filed *time.Duration, flightPlanObserved *time.Time) Input {
	return Input{
		Now: baselineNow, ExpectedDestination: "EKCH", Destination: "EKCH",
		Timing:               Timing{FiledEET: filed},
		Airborne:             AirborneObservation{SensedAt: &sensed, PreviouslyObserved: true},
		FlightPlanObservedAt: flightPlanObserved,
	}
}

func routeInput(sensed time.Time, previouslyObserved bool, route *RouteAwareEstimate) Input {
	return Input{
		Now: baselineNow, ExpectedDestination: "EKCH", Destination: "EKCH", RouteAware: route,
		Airborne: AirborneObservation{SensedAt: &sensed, PreviouslyObserved: previouslyObserved},
	}
}

func resetRouteInput(sensed time.Time, route *RouteAwareEstimate) Input {
	input := routeInput(sensed, true, route)
	input.ResetHeldAirborne = true
	return input
}

func heldBaseline(t *testing.T, reducer Reducer, sensed time.Time, filed time.Duration) *aman.BaselineState {
	t.Helper()
	observed := sensed
	result := reducer.Reduce(airborneInput(sensed, &filed, &observed), nil)
	require.NotNil(t, result.State)
	return result.State
}

func timePointer(value time.Time) *time.Time             { return &value }
func durationPointer(value time.Duration) *time.Duration { return &value }
