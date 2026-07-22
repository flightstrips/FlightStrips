package sequence_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestManualFreezeApplyReleaseAndRoutineRecompute(t *testing.T) {
	start := testTime()
	input := withCommittedSlots(t, sequence.Input{Revision: 7, Policies: []sequence.Policy{simplePolicy("A", start, 60)}, Flights: []sequence.Flight{
		flight("A", "A", start, "M"),
		flight("B", "A", start.Add(time.Minute), "M"),
	}})
	metadata := aman.CommandMetadata{CommandID: "freeze-b", ExpectedRevision: 7}
	frozen, err := sequence.ApplyManualFreeze(input, sequence.ApplyManualFreezeCommand{Metadata: metadata, FlightID: "B", At: start.Add(30 * time.Second)})
	require.NoError(t, err)
	value := policyFlight(t, frozen.Input, "B")
	require.Equal(t, aman.FreezeManual, value.FreezeReason)
	require.Equal(t, value.CurrentSlot, value.CapturedSlot)
	require.Equal(t, value.OperationalTETA, *value.FrozenOperationalTETA)
	require.False(t, frozen.Candidate.HasConflicts())

	rate, err := sequence.ApplyRate(frozen.Input, sequence.SetRateCommand{
		Metadata: aman.CommandMetadata{CommandID: "rate", ExpectedRevision: 7}, RunwayGroupID: "A", ArrivalsPerHour: 30, EffectiveAt: start,
	})
	require.NoError(t, err)
	require.Equal(t, value.CapturedSlot.Time, entryFor(t, rate.Candidate, "B").Time)

	released, err := sequence.ReleaseManualFreeze(frozen.Input, sequence.ReleaseManualFreezeCommand{
		Metadata: aman.CommandMetadata{CommandID: "release-b", ExpectedRevision: 7}, FlightID: "B", At: start.Add(time.Minute),
	})
	require.NoError(t, err)
	value = policyFlight(t, released.Input, "B")
	require.Equal(t, aman.FreezeNone, value.FreezeReason)
	require.Nil(t, value.FrozenAt)
	require.Nil(t, value.FrozenOperationalTETA)
	require.Nil(t, value.CapturedSlot)
}

func TestSuperstableAndManualSlotsAreTheOnlyRoutineConstraints(t *testing.T) {
	start := testTime()
	superstable := protectedFlight("SUPER", "A", start, "M", start, aman.FreezeSuperstable)
	manual := protectedFlight("MANUAL", "A", start.Add(2*time.Minute), "M", start.Add(2*time.Minute), aman.FreezeManual)
	input := sequence.Input{Revision: 3, Policies: []sequence.Policy{simplePolicy("A", start, 30)}, Flights: []sequence.Flight{superstable, manual}}
	decision, err := sequence.ApplyRate(input, sequence.SetRateCommand{
		Metadata: aman.CommandMetadata{CommandID: "reduce-rate", ExpectedRevision: 3}, RunwayGroupID: "A", ArrivalsPerHour: 20, EffectiveAt: start,
	})
	require.NoError(t, err)
	require.Equal(t, []time.Time{start, start.Add(2 * time.Minute)}, entryTimes(decision.Candidate))
	require.True(t, decision.Candidate.HasConflicts(), "new rate exposes the frozen-spacing conflict without moving either slot")
	require.Equal(t, sequence.WarningProtectedSpacing, decision.Candidate.Warnings[0].Code)
}

func TestRateTimelineImmediateFutureSameInstantAndRunwayIsolation(t *testing.T) {
	start := testTime()
	input := sequence.Input{Revision: 4, Policies: []sequence.Policy{simplePolicy("B", start, 30), simplePolicy("A", start, 60)}, Flights: []sequence.Flight{
		flight("A", "A", start, "M"), flight("B", "B", start, "M"),
	}}
	first, err := sequence.ApplyRate(input, sequence.SetRateCommand{
		Metadata: aman.CommandMetadata{CommandID: "future", ExpectedRevision: 4}, RunwayGroupID: "A", ArrivalsPerHour: 40, EffectiveAt: start.Add(10 * time.Minute),
	})
	require.NoError(t, err)
	second, err := sequence.ApplyRate(first.Input, sequence.SetRateCommand{
		Metadata: aman.CommandMetadata{CommandID: "same-instant", ExpectedRevision: 4}, RunwayGroupID: "A", ArrivalsPerHour: 20, EffectiveAt: start.Add(10 * time.Minute),
	})
	require.NoError(t, err)
	immediate := sequence.SetRateCommand{
		Metadata: aman.CommandMetadata{CommandID: "immediate", ExpectedRevision: 4}, RunwayGroupID: "A", ArrivalsPerHour: 45, EffectiveAt: start,
	}
	third, err := sequence.ApplyRate(second.Input, immediate)
	require.NoError(t, err)

	policyA := policyFor(t, third.Input, "A")
	require.Equal(t, []sequence.RatePoint{{EffectiveAt: start, ArrivalsPerHour: 45}, {EffectiveAt: start.Add(10 * time.Minute), ArrivalsPerHour: 20}}, policyA.Rates)
	require.Equal(t, []sequence.RatePoint{{EffectiveAt: start, ArrivalsPerHour: 30}}, policyFor(t, third.Input, "B").Rates)

	snapshot, err := json.Marshal(second.Input)
	require.NoError(t, err)
	var restored sequence.Input
	require.NoError(t, json.Unmarshal(snapshot, &restored))
	replayed, err := sequence.ApplyRate(restored, immediate)
	require.NoError(t, err)
	wantJSON, err := json.Marshal(third)
	require.NoError(t, err)
	replayedJSON, err := json.Marshal(replayed)
	require.NoError(t, err)
	require.Equal(t, wantJSON, replayedJSON)
}

func TestManualMoveAnchorsAndCommandValidation(t *testing.T) {
	start := testTime()
	input := withCommittedSlots(t, sequence.Input{Revision: 11, Policies: []sequence.Policy{simplePolicy("A", start, 60)}, Flights: []sequence.Flight{
		flight("A", "A", start, "M"), flight("B", "A", start.Add(time.Minute), "M"), flight("C", "A", start.Add(2*time.Minute), "M"),
	}})
	before := aman.FlightID("A")
	decision, err := sequence.ApplyMove(input, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "move-c", ExpectedRevision: 11}, FlightID: "C", RunwayGroupID: "A", BeforeFlightID: &before,
	})
	require.NoError(t, err)
	require.Equal(t, []aman.FlightID{"C", "A", "B"}, entryIDs(decision.Candidate))
	require.Equal(t, 1, *policyFlight(t, decision.Input, "C").ManualOrder)

	_, err = sequence.ApplyMove(input, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "stale", ExpectedRevision: 10}, FlightID: "C", RunwayGroupID: "A", BeforeFlightID: &before,
	})
	requireDomainClass(t, err, aman.ErrorRevisionConflict)

	_, err = sequence.ApplyMove(input, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "missing", ExpectedRevision: 11}, FlightID: "UNKNOWN", RunwayGroupID: "A", BeforeFlightID: &before,
	})
	requireDomainClass(t, err, aman.ErrorNotFound)

	_, err = sequence.ApplyMove(input, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "wrong-group", ExpectedRevision: 11}, FlightID: "C", RunwayGroupID: "B", BeforeFlightID: &before,
	})
	requireDomainClass(t, err, aman.ErrorInvalidArgument)

	after := aman.FlightID("B")
	_, err = sequence.ApplyMove(input, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "two-anchors", ExpectedRevision: 11}, FlightID: "C", RunwayGroupID: "A", BeforeFlightID: &before, AfterFlightID: &after,
	})
	requireDomainClass(t, err, aman.ErrorInvalidArgument)
}

func TestManualMoveRejectsImpossibleWTCAndFrozenTarget(t *testing.T) {
	start := testTime()
	policy := wtcPolicy("A", start, 60)
	lead := protectedFlight("LEAD", "A", start, "H", start, aman.FreezeManual)
	trail := protectedFlight("TRAIL", "A", start.Add(4*time.Minute), "H", start.Add(4*time.Minute), aman.FreezeManual)
	target := flight("TARGET", "A", start.Add(6*time.Minute), "M")
	input := withCommittedSlots(t, sequence.Input{Revision: 2, Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{lead, trail, target}})
	before := aman.FlightID("TRAIL")
	_, err := sequence.ApplyMove(input, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "bad-spacing", ExpectedRevision: 2}, FlightID: "TARGET", RunwayGroupID: "A", BeforeFlightID: &before,
	})
	requireDomainClass(t, err, aman.ErrorInvalidTransition)

	targetIndex := policyFlightIndex(input, "TARGET")
	input.Flights[targetIndex].FreezeReason = aman.FreezeManual
	input.Flights[targetIndex].CapturedSlot = input.Flights[targetIndex].CurrentSlot
	_, err = sequence.ApplyMove(input, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "frozen", ExpectedRevision: 2}, FlightID: "TARGET", RunwayGroupID: "A", BeforeFlightID: &before,
	})
	requireDomainClass(t, err, aman.ErrorInvalidTransition)
}

func TestGoAroundCascadeMovesStableAndSuperstableWithinBound(t *testing.T) {
	start := testTime()
	target := withState(flight("GO", "A", start, "M"), aman.StateGoAround)
	target.FreezeReason = aman.FreezeSuperstable
	target.CapturedSlot = slot(start, "A", 1)
	target.CurrentSlot = slot(start, "A", 1)
	stable := withState(flight("STABLE", "A", start.Add(time.Minute), "M"), aman.StateStable)
	stable.CurrentSlot = slot(start.Add(time.Minute), "A", 2)
	superstable := protectedFlight("SUPER", "A", start.Add(2*time.Minute), "M", start.Add(2*time.Minute), aman.FreezeSuperstable)
	superstable.CurrentSlot = slot(start.Add(2*time.Minute), "A", 3)
	input := sequence.Input{Revision: 9, Policies: []sequence.Policy{simplePolicy("A", start, 60)}, Flights: []sequence.Flight{target, stable, superstable}}
	command := sequence.ApplyGoAroundCommand{Metadata: aman.CommandMetadata{CommandID: "go-around", ExpectedRevision: 9}, FlightID: "GO", DetectedAt: start}
	decision, err := sequence.ApplyGoAround(input, sequence.GoAroundPolicy{Delay: time.Minute, MaxCascade: 2}, command)
	require.NoError(t, err)
	require.Equal(t, start.Add(time.Minute), entryFor(t, decision.Candidate, "GO").Time)
	require.Equal(t, start.Add(2*time.Minute), entryFor(t, decision.Candidate, "STABLE").Time)
	require.Equal(t, start.Add(3*time.Minute), entryFor(t, decision.Candidate, "SUPER").Time)
	require.Equal(t, aman.FreezeNone, policyFlight(t, decision.Input, "GO").FreezeReason)
	require.Equal(t, aman.FreezeSuperstable, policyFlight(t, decision.Input, "SUPER").FreezeReason)
	require.Equal(t, start.Add(3*time.Minute), policyFlight(t, decision.Input, "SUPER").CapturedSlot.Time)

	_, err = sequence.ApplyGoAround(input, sequence.GoAroundPolicy{Delay: time.Minute, MaxCascade: 1}, command)
	requireDomainClass(t, err, aman.ErrorInvalidTransition)
}

func TestGoAroundPreservesManualFreezeAndRestartsDeterministically(t *testing.T) {
	start := testTime()
	target := withState(flight("GO", "A", start, "M"), aman.StateGoAround)
	target.CurrentSlot = slot(start, "A", 1)
	manual := protectedFlight("MANUAL", "A", start.Add(time.Minute), "M", start.Add(time.Minute), aman.FreezeManual)
	manual.CurrentSlot = slot(start.Add(time.Minute), "A", 2)
	input := sequence.Input{Revision: 5, Policies: []sequence.Policy{simplePolicy("A", start, 60)}, Flights: []sequence.Flight{target, manual}}
	command := sequence.ApplyGoAroundCommand{Metadata: aman.CommandMetadata{CommandID: "replay-go-around", ExpectedRevision: 5}, FlightID: "GO", DetectedAt: start}
	policy := sequence.GoAroundPolicy{Delay: time.Minute, MaxCascade: 4}

	want, err := sequence.ApplyGoAround(input, policy, command)
	require.NoError(t, err)
	require.Equal(t, start.Add(time.Minute), entryFor(t, want.Candidate, "MANUAL").Time)
	require.Equal(t, start.Add(2*time.Minute), entryFor(t, want.Candidate, "GO").Time)

	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)
	commandJSON, err := json.Marshal(command)
	require.NoError(t, err)
	var restoredInput sequence.Input
	var restoredCommand sequence.ApplyGoAroundCommand
	require.NoError(t, json.Unmarshal(inputJSON, &restoredInput))
	require.NoError(t, json.Unmarshal(commandJSON, &restoredCommand))
	replayed, err := sequence.ApplyGoAround(restoredInput, policy, restoredCommand)
	require.NoError(t, err)
	wantJSON, err := json.Marshal(want)
	require.NoError(t, err)
	replayedJSON, err := json.Marshal(replayed)
	require.NoError(t, err)
	require.Equal(t, wantJSON, replayedJSON)
}

func withCommittedSlots(t *testing.T, input sequence.Input) sequence.Input {
	t.Helper()
	result, err := sequence.Generate(input)
	require.NoError(t, err)
	for index := range input.Flights {
		if input.Flights[index].State == aman.StateLanded || input.Flights[index].State == aman.StateRemoved {
			continue
		}
		entry := entryFor(t, result, input.Flights[index].ID)
		input.Flights[index].CurrentSlot = &aman.Slot{Time: entry.Time, RunwayGroupID: entry.RunwayGroupID, Sequence: entry.Sequence, Revision: input.Revision, Reason: string(entry.Reason)}
		if input.Flights[index].FreezeReason != aman.FreezeNone {
			input.Flights[index].CapturedSlot = input.Flights[index].CurrentSlot
		}
	}
	return input
}

func policyFlight(t *testing.T, input sequence.Input, id aman.FlightID) sequence.Flight {
	t.Helper()
	index := policyFlightIndex(input, id)
	require.NotEqual(t, -1, index)
	return input.Flights[index]
}

func policyFlightIndex(input sequence.Input, id aman.FlightID) int {
	for index := range input.Flights {
		if input.Flights[index].ID == id {
			return index
		}
	}
	return -1
}

func policyFor(t *testing.T, input sequence.Input, group aman.RunwayGroupID) sequence.Policy {
	t.Helper()
	for _, policy := range input.Policies {
		if policy.RunwayGroupID == group {
			return policy
		}
	}
	t.Fatalf("policy %q not found", group)
	return sequence.Policy{}
}

func slot(at time.Time, group aman.RunwayGroupID, order int) *aman.Slot {
	return &aman.Slot{Time: at, RunwayGroupID: group, Sequence: order, Reason: "committed"}
}

func requireDomainClass(t *testing.T, err error, class aman.ErrorClass) {
	t.Helper()
	require.Error(t, err)
	var domain *aman.DomainError
	require.True(t, errors.As(err, &domain))
	require.Equal(t, class, domain.Class)
}
