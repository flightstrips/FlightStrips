package sequence_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestRunwayGapsUseStartInclusiveEndExclusiveBoundaries(t *testing.T) {
	start := testTime()
	policy := simplePolicy("A", start.Add(-2*time.Minute), 60)
	policy.Gaps = []sequence.Gap{runwayGap(start, start.Add(2*time.Minute))}

	for _, test := range []struct {
		name string
		teta time.Time
		want time.Time
	}{
		{name: "before start remains available", teta: start.Add(-time.Minute), want: start.Add(-time.Minute)},
		{name: "start is blocked", teta: start, want: start.Add(2 * time.Minute)},
		{name: "inside is blocked", teta: start.Add(time.Minute), want: start.Add(2 * time.Minute)},
		{name: "end remains available", teta: start.Add(2 * time.Minute), want: start.Add(2 * time.Minute)},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{flight("F", "A", test.teta, "M")}})
			require.NoError(t, err)
			require.Equal(t, test.want, result.Entries[0].Time)
		})
	}
}

func TestMultipleCanonicalRunwayGapsAndRunwayIsolation(t *testing.T) {
	start := testTime()
	policyA := simplePolicy("A", start, 60)
	policyA.Gaps = []sequence.Gap{
		runwayGap(start, start.Add(2*time.Minute)),
		runwayGap(start.Add(3*time.Minute), start.Add(5*time.Minute)),
	}
	policyB := simplePolicy("B", start, 60)

	result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policyB, policyA}, Flights: []sequence.Flight{
		flight("A-FIRST", "A", start, "M"),
		flight("A-SECOND", "A", start.Add(3*time.Minute), "M"),
		flight("B", "B", start, "M"),
	}})
	require.NoError(t, err)
	require.Equal(t, start.Add(2*time.Minute), entryFor(t, result, "A-FIRST").Time)
	require.Equal(t, start.Add(5*time.Minute), entryFor(t, result, "A-SECOND").Time)
	require.Equal(t, start, entryFor(t, result, "B").Time)
}

func TestMergedRunwayGapBlocksTheEntirePersistedUnion(t *testing.T) {
	start := testTime()
	policy := simplePolicy("A", start, 60)
	policy.Gaps = []sequence.Gap{runwayGap(start, start.Add(5*time.Minute))}

	result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
		flight("AT-FORMER-SEAM", "A", start.Add(2*time.Minute), "M"),
	}})
	require.NoError(t, err)
	require.Equal(t, start.Add(5*time.Minute), result.Entries[0].Time)
}

func TestRunwayGapRetainsAbsoluteBoundsAcrossRateChange(t *testing.T) {
	start := testTime()
	policy := simplePolicy("A", start, 20)
	policy.Rates = append(policy.Rates, sequence.RatePoint{EffectiveAt: start.Add(3 * time.Minute), ArrivalsPerHour: 60})
	policy.Gaps = []sequence.Gap{runwayGap(start, start.Add(6*time.Minute))}

	result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{flight("F", "A", start, "M")}})
	require.NoError(t, err)
	require.Equal(t, start.Add(6*time.Minute), result.Entries[0].Time)
}

func TestRunwayGapContinuesWakeAndSTARSpacingAfterBlockedCapacity(t *testing.T) {
	start := testTime()
	policy := wtcPolicy("A", start, 30)
	policy.Gaps = []sequence.Gap{runwayGap(start, start.Add(4*time.Minute))}
	spacing := sequence.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	first := flight("FIRST", "A", start, "H")
	first.STARFamily = "MONAK"
	second := flight("SECOND", "A", start, "M")
	second.STARFamily = "MONAK"

	result, err := sequence.Generate(sequence.Input{
		Policies:           []sequence.Policy{policy},
		STARFamilyPolicies: []sequence.STARFamilyPolicy{{STARFamily: "MONAK", SameSTARSpacing: spacing, HoldingSequencePolicy: navdata.HoldingSequenceDisabled}},
		Flights:            []sequence.Flight{second, first},
	})
	require.NoError(t, err)
	require.Equal(t, start.Add(4*time.Minute), entryFor(t, result, "FIRST").Time)
	require.Equal(t, start.Add(8*time.Minute), entryFor(t, result, "SECOND").Time)
}

func TestRunwayGapPreservesLifecycleOrderAndDoesNotDisplaceProtectedSlots(t *testing.T) {
	start := testTime()
	policy := simplePolicy("A", start, 60)
	policy.Gaps = []sequence.Gap{runwayGap(start, start.Add(2*time.Minute))}
	stable := withState(flight("STABLE", "A", start, "M"), aman.StateStable)
	unstable := withState(flight("UNSTABLE", "A", start, "M"), aman.StateUnstable)
	airborne := flight("AIRBORNE", "A", start, "M")
	protected := protectedFlight("PROTECTED", "A", start, "M", start.Add(time.Minute), aman.FreezeSuperstable)

	result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
		airborne, unstable, stable, protected,
	}})
	require.NoError(t, err)
	require.Equal(t, start.Add(time.Minute), entryFor(t, result, "PROTECTED").Time, "protected displacement belongs to the later GAP mutation policy")
	require.Equal(t, []aman.FlightID{"PROTECTED", "STABLE", "UNSTABLE", "AIRBORNE"}, entryIDs(result))
	require.Equal(t, start.Add(2*time.Minute), entryFor(t, result, "STABLE").Time)
}

func TestRunwayGapAllocationReplayIsDeterministic(t *testing.T) {
	start := testTime()
	policy := simplePolicy("A", start, 60)
	policy.Gaps = []sequence.Gap{
		runwayGap(start.Add(4*time.Minute), start.Add(6*time.Minute)),
		runwayGap(start, start.Add(2*time.Minute)),
	}
	flights := []sequence.Flight{flight("Z", "A", start, "M"), flight("A", "A", start, "M")}

	first, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: flights})
	require.NoError(t, err)
	reversed := policy
	reversed.Gaps = slices.Clone(policy.Gaps)
	slices.Reverse(reversed.Gaps)
	slices.Reverse(flights)
	second, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{reversed}, Flights: flights})
	require.NoError(t, err)
	firstJSON, err := json.Marshal(first)
	require.NoError(t, err)
	secondJSON, err := json.Marshal(second)
	require.NoError(t, err)
	require.Equal(t, firstJSON, secondJSON)
}

func TestRunwayGapNoCapacityReturnsEmptyResultWithoutMutatingInput(t *testing.T) {
	start := testTime()
	policy := simplePolicy("A", start, 60)
	policy.Gaps = []sequence.Gap{runwayGap(start.Add(3*time.Minute), start.Add(time.Hour))}
	orderOne, orderTwo := 1, 2
	protected := protectedFlight("PROTECTED", "A", start, "M", start.Add(2*time.Minute), aman.FreezeManual)
	protected.ManualOrder = &orderTwo
	target := flight("TARGET", "A", start.Add(2*time.Minute), "M")
	target.ManualOrder = &orderOne
	input := sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{protected, target}}
	before, err := json.Marshal(input)
	require.NoError(t, err)

	result, err := sequence.Generate(input)
	require.ErrorContains(t, err, `runway group "A" could not allocate flight "TARGET"`)
	require.Empty(t, result.Entries)
	require.Empty(t, result.Movements)
	after, marshalErr := json.Marshal(input)
	require.NoError(t, marshalErr)
	require.Equal(t, before, after)
}

func runwayGap(start, end time.Time) sequence.Gap {
	return sequence.Gap{Start: start, End: end}
}
