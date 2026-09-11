package sequence

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
)

func TestPrepareFlightsAssociatesExplicitSTARFamilyHoldingPolicy(t *testing.T) {
	start := time.Date(2026, time.September, 11, 20, 0, 0, 0, time.UTC)
	policies, err := prepareSTARFamilyPolicies([]STARFamilyPolicy{
		{STARFamily: "MONAK", HoldingSequencePolicy: navdata.HoldingSequenceDisabled},
		{STARFamily: "TESPI", HoldingSequencePolicy: navdata.HoldingSequenceLowestAltitudeFirst},
	})
	require.NoError(t, err)
	preparedPolicies, err := preparePoliciesWithSTARFamilies([]Policy{preparationPolicy(start)}, policies)
	require.NoError(t, err)

	flights, err := prepareFlights([]Flight{
		preparationFlight("enabled", start, "TESPI", "TESPI"),
		preparationFlight("disabled", start, "MONAK", "MONAK"),
		preparationFlight("missing-identity", start, "TESPI", ""),
		preparationFlight("unknown-family", start, "TESPI", "TUDLO"),
	}, preparedPolicies)
	require.NoError(t, err)

	byID := make(map[aman.FlightID]preparedFlight, len(flights["ARRIVAL-22"]))
	for _, flight := range flights["ARRIVAL-22"] {
		byID[flight.ID] = flight
	}
	require.Equal(t, navdata.HoldingSequenceLowestAltitudeFirst, byID["enabled"].holdingSequencePolicy)
	require.Equal(t, navdata.HoldingSequenceDisabled, byID["disabled"].holdingSequencePolicy)
	require.Equal(t, navdata.HoldingSequenceDisabled, byID["missing-identity"].holdingSequencePolicy,
		"legacy same-STAR identity must not select holding policy")
	require.Equal(t, navdata.HoldingSequenceDisabled, byID["unknown-family"].holdingSequencePolicy)
}

func TestPrepareFlightsHoldingPolicyLookupIsDeterministic(t *testing.T) {
	start := time.Date(2026, time.September, 11, 20, 0, 0, 0, time.UTC)
	familyPolicies := []STARFamilyPolicy{
		{STARFamily: "MONAK", HoldingSequencePolicy: navdata.HoldingSequenceDisabled},
		{STARFamily: "TESPI", HoldingSequencePolicy: navdata.HoldingSequenceLowestAltitudeFirst},
	}
	inputFlights := []Flight{
		preparationFlight("second", start, "MONAK", "MONAK"),
		preparationFlight("first", start, "TESPI", "TESPI"),
	}
	prepare := func(flights []Flight) map[aman.FlightID]navdata.HoldingSequencePolicy {
		families, err := prepareSTARFamilyPolicies(familyPolicies)
		require.NoError(t, err)
		policies, err := preparePoliciesWithSTARFamilies([]Policy{preparationPolicy(start)}, families)
		require.NoError(t, err)
		prepared, err := prepareFlights(flights, policies)
		require.NoError(t, err)
		result := map[aman.FlightID]navdata.HoldingSequencePolicy{}
		for _, flight := range prepared["ARRIVAL-22"] {
			result[flight.ID] = flight.holdingSequencePolicy
		}
		return result
	}

	first := prepare(inputFlights)
	reversed := slices.Clone(inputFlights)
	slices.Reverse(reversed)
	require.Equal(t, first, prepare(reversed))
}

func TestHoldingSequencePolicySurvivesInputCloneAndReplay(t *testing.T) {
	start := time.Date(2026, time.September, 11, 20, 0, 0, 0, time.UTC)
	input := Input{
		Policies: []Policy{preparationPolicy(start)},
		STARFamilyPolicies: []STARFamilyPolicy{{
			STARFamily: "TESPI", HoldingSequencePolicy: navdata.HoldingSequenceLowestAltitudeFirst,
		}},
		Flights: []Flight{preparationFlight("flight", start, "TESPI", "TESPI")},
	}
	cloned := cloneInput(input)
	require.Equal(t, input, cloned)

	payload, err := json.Marshal(cloned)
	require.NoError(t, err)
	var replayed Input
	require.NoError(t, json.Unmarshal(payload, &replayed))
	require.Equal(t, input, replayed)

	cloned.STARFamilyPolicies[0].HoldingSequencePolicy = navdata.HoldingSequenceDisabled
	cloned.Flights[0].SelectedSTARFamily = "MONAK"
	require.Equal(t, navdata.HoldingSequenceLowestAltitudeFirst, input.STARFamilyPolicies[0].HoldingSequencePolicy)
	require.Equal(t, "TESPI", input.Flights[0].SelectedSTARFamily)
}

func preparationPolicy(start time.Time) Policy {
	return Policy{
		RunwayGroupID:     "ARRIVAL-22",
		Rates:             []RatePoint{{EffectiveAt: start, ArrivalsPerHour: 20}},
		SeparationRules:   []SeparationRule{{Leading: "M", Trailing: "M"}},
		UnknownSeparation: time.Minute,
	}
}

func preparationFlight(id aman.FlightID, at time.Time, family, selectedFamily string) Flight {
	return Flight{
		ID: id, RunwayGroupID: "ARRIVAL-22", State: aman.StateAirborne,
		OperationalTETA: at, WakeCategory: "M", STARFamily: family, SelectedSTARFamily: selectedFamily,
		FreezeReason: aman.FreezeNone,
	}
}
