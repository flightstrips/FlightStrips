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
		{"EOBT and API estimate", Timing{EOBT: &eobt, APIEstimatedFlightTime: &api}, eobt.Add(DefaultEXOT + api), aman.BaselineSourcePlannedEOBTAPIEstimatedFlightTime, StatusDegraded, ReasonMissingFlightDuration},
		{"estimated departure and API estimate", Timing{EstimatedDeparture: &estimatedDeparture, APIEstimatedFlightTime: &api}, estimatedDeparture.Add(api), aman.BaselineSourcePlannedEstimatedDepartureAPIEstimatedFlightTime, StatusDegraded, ReasonMissingFlightDuration},
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
	require.Equal(t, ReasonMissingFlightDuration, fallback.Reason)
	require.Equal(t, "aircraft-category-speeds-v1", reducer.config.SpeedDefaults.Version)
	require.NotNil(t, fallback.State)

	input.RouteAware = &RouteAwareEstimate{ArrivalAt: baselineNow.Add(20 * time.Minute), Confidence: aman.ConfidenceHigh}
	routeAware := reducer.Reduce(input, &aman.BaselineState{
		ArrivalAt: fallback.State.ArrivalAt, AirborneSensedAt: fallback.State.AirborneSensedAt, Source: fallback.State.Source, Confidence: fallback.State.Confidence,
		DegradationReason: fallback.State.DegradationReason, FlightPlanObservedAt: fallback.State.FlightPlanObservedAt, ModelVersion: fallback.State.ModelVersion, ConfigVersion: fallback.State.ConfigVersion,
	})
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
	require.InDelta(t, 60.04, greatCircleDistanceNM(55, 12, 56, 12), 0.05)
	require.Equal(t, map[AircraftCategory]float64{
		CategoryLight: 180, CategoryMedium: 420, CategoryHeavy: 440, CategorySuper: 460,
	}, SpeedDefaultsV1().Knots)
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

func timePointer(value time.Time) *time.Time             { return &value }
func durationPointer(value time.Duration) *time.Duration { return &value }
