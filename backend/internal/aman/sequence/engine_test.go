package sequence_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestRateGridAndEarlyToleranceBoundaries(t *testing.T) {
	start := testTime()
	tests := []struct {
		name      string
		teta      time.Time
		tolerance time.Duration
		want      time.Time
	}{
		{name: "exact grid slot", teta: start.Add(2 * time.Minute), tolerance: 30 * time.Second, want: start.Add(2 * time.Minute)},
		{name: "exact early tolerance", teta: start.Add(2*time.Minute + 30*time.Second), tolerance: 30 * time.Second, want: start.Add(2 * time.Minute)},
		{name: "one nanosecond outside tolerance", teta: start.Add(2*time.Minute + 30*time.Second + time.Nanosecond), tolerance: 30 * time.Second, want: start.Add(4 * time.Minute)},
		{name: "zero tolerance", teta: start.Add(2*time.Minute + time.Nanosecond), tolerance: 0, want: start.Add(4 * time.Minute)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := simplePolicy("A", start, 30)
			policy.EarlyTolerance = test.tolerance
			result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{flight("F", "A", test.teta, "M")}})
			require.NoError(t, err)
			require.Len(t, result.Entries, 1)
			require.Equal(t, test.want, result.Entries[0].Time)
		})
	}
}

func TestFutureRateTransitionStartsAtExactInstant(t *testing.T) {
	start := testTime()
	policy := simplePolicy("A", start, 30)
	policy.Rates = append(policy.Rates, sequence.RatePoint{EffectiveAt: start.Add(5 * time.Minute), ArrivalsPerHour: 60})

	result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
		flight("BEFORE", "A", start.Add(4*time.Minute), "M"),
		flight("AT_CHANGE", "A", start.Add(4*time.Minute+time.Nanosecond), "M"),
		flight("AFTER_CHANGE", "A", start.Add(5*time.Minute+time.Nanosecond), "M"),
	}})
	require.NoError(t, err)
	require.Equal(t, []time.Time{start.Add(4 * time.Minute), start.Add(5 * time.Minute), start.Add(6 * time.Minute)}, entryTimes(result))
}

func TestDirectionalWTCSpacingChecksBothAdjacentBoundaries(t *testing.T) {
	start := testTime()
	policy := wtcPolicy("A", start, 3600)
	tests := []struct {
		name     string
		category sequence.WakeCategory
		teta     time.Time
		want     time.Time
	}{
		{name: "leading boundary exact", category: "M", teta: start.Add(3 * time.Minute), want: start.Add(3 * time.Minute)},
		{name: "leading boundary too short", category: "M", teta: start.Add(2 * time.Minute), want: start.Add(3 * time.Minute)},
		{name: "trailing boundary exact", category: "H", teta: start.Add(3 * time.Minute), want: start.Add(3 * time.Minute)},
		{name: "trailing boundary one nanosecond short", category: "H", teta: start.Add(3*time.Minute + time.Nanosecond), want: start.Add(8 * time.Minute)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			leading := protectedFlight("LEAD", "A", start, "H", start, aman.FreezeManual)
			trailing := protectedFlight("TRAIL", "A", start.Add(6*time.Minute), "M", start.Add(6*time.Minute), aman.FreezeManual)
			result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
				leading, trailing, flight("CANDIDATE", "A", test.teta, test.category),
			}})
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Equal(t, test.want, entryFor(t, result, "CANDIDATE").Time)
		})
	}
}

func TestUnknownCategoryUsesConservativeSpacingAndWarns(t *testing.T) {
	start := testTime()
	policy := wtcPolicy("A", start, 60)
	// Even a too-small configured unknown fallback is raised to the largest
	// configured known-category rule, never treated as zero spacing.
	policy.UnknownSeparation = time.Minute
	result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
		protectedFlight("LEAD", "A", start, "H", start, aman.FreezeManual),
		flight("UNKNOWN", "A", start.Add(time.Minute), "not-configured"),
	}})
	require.NoError(t, err)
	require.Equal(t, start.Add(3*time.Minute), entryFor(t, result, "UNKNOWN").Time)
	require.Equal(t, sequence.WakeUnknown, entryFor(t, result, "UNKNOWN").WakeCategory)
	require.Equal(t, []sequence.Warning{{Severity: sequence.SeverityDegraded, Code: sequence.WarningUnknownWakeCategory, RunwayGroupID: "A", FlightID: "UNKNOWN"}}, result.Warnings)
	require.False(t, result.HasConflicts())
}

func TestProtectedConstraintsArePreservedOrReportedAsConflicts(t *testing.T) {
	start := testTime()
	policy := wtcPolicy("A", start, 30)

	t.Run("off-grid captured slot is preserved", func(t *testing.T) {
		captured := start.Add(90 * time.Second)
		result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
			protectedFlight("FROZEN", "A", start.Add(time.Minute), "M", captured, aman.FreezeSuperstable),
		}})
		require.NoError(t, err)
		require.False(t, result.HasConflicts())
		require.Equal(t, captured, result.Entries[0].Time)
		require.Equal(t, sequence.ReasonFreezeSuperstable, result.Entries[0].Reason)
	})

	t.Run("missing captured slot is an explicit conflict", func(t *testing.T) {
		missing := flight("MISSING", "A", start.Add(time.Minute), "M")
		missing.FreezeReason = aman.FreezeManual
		result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{missing}})
		require.NoError(t, err)
		require.Empty(t, result.Entries)
		require.True(t, result.HasConflicts())
		require.Equal(t, sequence.WarningProtectedSlotMissing, result.Warnings[0].Code)
	})

	t.Run("conflicting captured slots remain unchanged", func(t *testing.T) {
		result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
			protectedFlight("LEAD", "A", start, "H", start, aman.FreezeManual),
			protectedFlight("TRAIL", "A", start.Add(2*time.Minute), "M", start.Add(2*time.Minute), aman.FreezeSuperstable),
		}})
		require.NoError(t, err)
		require.Equal(t, []time.Time{start, start.Add(2 * time.Minute)}, entryTimes(result))
		require.True(t, result.HasConflicts())
		require.Equal(t, sequence.WarningProtectedSpacing, result.Warnings[0].Code)
		require.Equal(t, aman.FlightID("LEAD"), *result.Warnings[0].RelatedFlightID)
	})
}

func TestRunwayGroupsRemainIsolated(t *testing.T) {
	start := testTime()
	policyA := simplePolicy("A", start, 60)
	policyB := simplePolicy("B", start, 30)
	policyA.EarlyTolerance = 30 * time.Second
	policyB.EarlyTolerance = 30 * time.Second
	first := 1
	result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policyB, policyA}, Flights: []sequence.Flight{
		withOrder(flight("A2", "A", start.Add(30*time.Second), "M"), &first),
		flight("B1", "B", start.Add(30*time.Second), "M"),
		flight("A1", "A", start.Add(30*time.Second), "M"),
	}})
	require.NoError(t, err)
	require.Equal(t, []aman.FlightID{"A2", "A1", "B1"}, entryIDs(result))
	require.Equal(t, []time.Time{start, start.Add(time.Minute), start}, entryTimes(result))
	require.Equal(t, []aman.RunwayGroupID{"A", "A", "B"}, entryGroups(result))
}

func TestTieBreakersAndReplayAreDeterministic(t *testing.T) {
	start := testTime()
	baselineEarly, baselineLate := start.Add(-2*time.Hour), start.Add(-time.Hour)
	manualOrder := 1
	flights := []sequence.Flight{
		withBaseline(flight("ID-Z", "A", start, "M"), &baselineLate),
		withBaseline(flight("ID-A", "A", start, "M"), &baselineLate),
		withBaseline(flight("BASELINE", "A", start, "M"), &baselineEarly),
		withState(flight("STABLE", "A", start, "M"), aman.StateStable),
		withOrder(flight("MANUAL", "A", start, "M"), &manualOrder),
	}
	policy := simplePolicy("A", start, 60)

	first, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: flights})
	require.NoError(t, err)
	require.Equal(t, []aman.FlightID{"MANUAL", "STABLE", "BASELINE", "ID-A", "ID-Z"}, entryIDs(first))

	reversedFlights := slices.Clone(flights)
	slices.Reverse(reversedFlights)
	reversedPolicy := policy
	reversedPolicy.SeparationRules = slices.Clone(policy.SeparationRules)
	slices.Reverse(reversedPolicy.SeparationRules)
	second, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{reversedPolicy}, Flights: reversedFlights})
	require.NoError(t, err)
	firstJSON, err := json.Marshal(first)
	require.NoError(t, err)
	secondJSON, err := json.Marshal(second)
	require.NoError(t, err)
	require.Equal(t, firstJSON, secondJSON)
}

func TestNoOpRecomputationProducesNoMovements(t *testing.T) {
	start := testTime()
	input := sequence.Input{Policies: []sequence.Policy{simplePolicy("A", start, 60)}, Flights: []sequence.Flight{
		flight("A", "A", start, "M"),
		flight("B", "A", start, "M"),
	}}
	initial, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Len(t, initial.Movements, 2)
	for index := range input.Flights {
		entry := entryFor(t, initial, input.Flights[index].ID)
		input.Flights[index].CurrentSlot = &aman.Slot{Time: entry.Time, RunwayGroupID: entry.RunwayGroupID, Sequence: entry.Sequence, Reason: string(entry.Reason)}
	}
	recomputed, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Equal(t, initial.Entries, recomputed.Entries)
	require.Empty(t, recomputed.Movements)
}

func TestPolicyValidationRejectsIncompleteWTCMatrixAndDuplicateRateBoundary(t *testing.T) {
	start := testTime()
	policy := wtcPolicy("A", start, 30)
	policy.SeparationRules = policy.SeparationRules[:3]
	_, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}})
	require.ErrorContains(t, err, "incomplete WTC separation matrix")

	policy = simplePolicy("A", start, 30)
	policy.Rates = append(policy.Rates, sequence.RatePoint{EffectiveAt: start, ArrivalsPerHour: 60})
	_, err = sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}})
	require.ErrorContains(t, err, "duplicate rate effective time")
}

func simplePolicy(group aman.RunwayGroupID, start time.Time, rate uint32) sequence.Policy {
	return sequence.Policy{
		RunwayGroupID:     group,
		Rates:             []sequence.RatePoint{{EffectiveAt: start, ArrivalsPerHour: rate}},
		SeparationRules:   []sequence.SeparationRule{{Leading: "M", Trailing: "M", Minimum: 0}},
		UnknownSeparation: time.Minute,
	}
}

func wtcPolicy(group aman.RunwayGroupID, start time.Time, rate uint32) sequence.Policy {
	return sequence.Policy{
		RunwayGroupID: group,
		Rates:         []sequence.RatePoint{{EffectiveAt: start, ArrivalsPerHour: rate}},
		SeparationRules: []sequence.SeparationRule{
			{Leading: "H", Trailing: "H", Minimum: 2 * time.Minute},
			{Leading: "H", Trailing: "M", Minimum: 3 * time.Minute},
			{Leading: "M", Trailing: "H", Minimum: 2 * time.Minute},
			{Leading: "M", Trailing: "M", Minimum: 2 * time.Minute},
		},
		UnknownSeparation: 3 * time.Minute,
	}
}

func flight(id aman.FlightID, group aman.RunwayGroupID, teta time.Time, category sequence.WakeCategory) sequence.Flight {
	return sequence.Flight{ID: id, RunwayGroupID: group, State: aman.StateAirborne, OperationalTETA: teta, WakeCategory: category, FreezeReason: aman.FreezeNone}
}

func protectedFlight(id aman.FlightID, group aman.RunwayGroupID, teta time.Time, category sequence.WakeCategory, slotTime time.Time, reason aman.FreezeReason) sequence.Flight {
	value := flight(id, group, teta, category)
	value.FreezeReason = reason
	value.CapturedSlot = &aman.Slot{Time: slotTime, RunwayGroupID: group, Sequence: 1, Reason: "captured"}
	return value
}

func withOrder(value sequence.Flight, order *int) sequence.Flight {
	value.ManualOrder = order
	return value
}

func withState(value sequence.Flight, state aman.FlightState) sequence.Flight {
	value.State = state
	return value
}

func withBaseline(value sequence.Flight, baseline *time.Time) sequence.Flight {
	value.InitialBaselineTETA = baseline
	return value
}

func entryFor(t *testing.T, result sequence.Result, id aman.FlightID) sequence.CandidateEntry {
	t.Helper()
	for _, entry := range result.Entries {
		if entry.FlightID == id {
			return entry
		}
	}
	t.Fatalf("entry %q not found", id)
	return sequence.CandidateEntry{}
}

func entryTimes(result sequence.Result) []time.Time {
	values := make([]time.Time, len(result.Entries))
	for index, entry := range result.Entries {
		values[index] = entry.Time
	}
	return values
}

func entryIDs(result sequence.Result) []aman.FlightID {
	values := make([]aman.FlightID, len(result.Entries))
	for index, entry := range result.Entries {
		values[index] = entry.FlightID
	}
	return values
}

func entryGroups(result sequence.Result) []aman.RunwayGroupID {
	values := make([]aman.RunwayGroupID, len(result.Entries))
	for index, entry := range result.Entries {
		values[index] = entry.RunwayGroupID
	}
	return values
}

func testTime() time.Time {
	return time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
}
