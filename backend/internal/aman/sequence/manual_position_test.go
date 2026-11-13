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

func commitManualCandidate(input sequence.Input, result sequence.Result) sequence.Input {
	for i := range input.Flights {
		for _, entry := range result.Entries {
			if entry.Callsign == input.Flights[i].Callsign {
				input.Flights[i].CurrentSlot = &aman.Slot{Time: entry.Time, Sequence: entry.Sequence, RunwayGroupID: entry.RunwayGroupID, Revision: input.Revision}
			}
		}
	}
	return input
}

func TestSwappedManualPositionSurvivesMixedSTARProgress(t *testing.T) {
	for _, protected := range []bool{false, true} {
		name := "movable"
		if protected {
			name = "protected"
		}
		t.Run(name, func(t *testing.T) {
			at := testTime()
			input := sequence.Input{Revision: 7, Policies: []sequence.Policy{simplePolicy("A", at, 60)}, Flights: []sequence.Flight{
				queueFlight("FIRST", "A", at, "M", 1, at),
				queueFlight("MIDDLE", "A", at.Add(time.Minute), "M", 2, at.Add(time.Minute)),
				queueFlight("LAST", "A", at.Add(2*time.Minute), "M", 3, at.Add(2*time.Minute)),
			}}
			for i := range input.Flights {
				f := &input.Flights[i]
				f.SelectedSTARFamily, f.ProtectCurrentSlot = "TESPI", protected
				if !protected {
					f.State = aman.StateAirborne
				}
			}
			// FIRST physically precedes LAST, while MIDDLE has no comparable
			// progress. The controller explicitly reverses the committed order.
			firstETA, lastETA := at.Add(10*time.Minute), at.Add(12*time.Minute)
			input.Flights[0].STARProgress = &sequence.STARProgress{Fix: "ROSBI", ETA: &firstETA, TerminalDigest: "terminal"}
			input.Flights[2].STARProgress = &sequence.STARProgress{Fix: "ROSBI", ETA: &lastETA, TerminalDigest: "terminal"}
			decision, err := sequence.ApplySwap(input, aman.SwapFlightsCommand{
				Metadata: aman.CommandMetadata{CommandID: "swap", ExpectedRevision: 7}, Callsign: "FIRST", OtherCallsign: "LAST",
			})
			require.NoError(t, err)
			require.Equal(t, []aman.Callsign{"LAST", "MIDDLE", "FIRST"}, entryIDs(decision.Candidate))
			working := commitManualCandidate(decision.Input, decision.Candidate)
			encoded, err := json.Marshal(working)
			require.NoError(t, err)
			var restored sequence.Input
			require.NoError(t, json.Unmarshal(encoded, &restored))
			for attempt := 0; attempt < 2; attempt++ {
				slices.Reverse(restored.Flights)
				precedence, err := sequence.ManualPositionPrecedence(restored)
				require.NoError(t, err)
				require.True(t, precedence["FIRST"]["MIDDLE"])
				require.True(t, precedence["MIDDLE"]["LAST"])
				result, err := sequence.Generate(restored)
				require.NoError(t, err)
				require.False(t, result.HasConflicts())
				require.Equal(t, entryIDs(decision.Candidate), entryIDs(result))
				restored = commitManualCandidate(restored, result)
			}
		})
	}
}

func TestManuallyPositionedFlightCanTakeEarlierSlot(t *testing.T) {
	at := testTime()
	input := sequence.Input{Revision: 7, Policies: []sequence.Policy{simplePolicy("A", at, 60)}, Flights: []sequence.Flight{
		queueFlight("PREVIOUS", "A", at.Add(10*time.Minute), "M", 1, at.Add(10*time.Minute)),
		queueFlight("NEXT", "A", at.Add(14*time.Minute), "M", 2, at.Add(14*time.Minute)),
		queueFlight("MOVED", "A", at.Add(12*time.Minute), "M", 3, at.Add(18*time.Minute)),
	}}
	for i := range input.Flights {
		input.Flights[i].ProtectCurrentSlot = true
	}
	anchor := aman.Callsign("NEXT")
	decision, err := sequence.ApplyMove(input, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "move", ExpectedRevision: 7}, Callsign: "MOVED", RunwayGroupID: "A", BeforeCallsign: &anchor,
	})
	require.NoError(t, err)
	require.Equal(t, &aman.ManualSequencePosition{AfterCallsign: "PREVIOUS", BeforeCallsign: "NEXT"}, policyFlight(t, decision.Input, "MOVED").ManualPosition)
	working := commitManualCandidate(decision.Input, decision.Candidate)
	working.Flights[2].OperationalTETA = at.Add(9 * time.Minute)
	result, promotions, err := sequence.GenerateWithVacancyPromotions(working, nil, at)
	require.NoError(t, err)
	require.False(t, result.HasConflicts())
	require.Equal(t, []aman.Callsign{"PREVIOUS", "MOVED", "NEXT"}, entryIDs(result))
	require.Equal(t, at.Add(11*time.Minute), entryFor(t, result, "MOVED").Time)
	require.Contains(t, promotions, sequence.VacancyPromotion{Callsign: "MOVED", From: *working.Flights[2].CurrentSlot, To: aman.Slot{
		Time: at.Add(11 * time.Minute), Sequence: 2, RunwayGroupID: "A", Revision: 7, Reason: string(sequence.ReasonQueuePromotion),
	}})
}

func TestManuallyPositionedFlightCanFillNewSTARGap(t *testing.T) {
	input, at := starGapInput()
	filler := input.Flights[2]
	// First place the flight between two MONAK neighbours. The ERNOV traffic
	// joins later and must not become part of that controller constraint.
	filler.FreezeReason, filler.FrozenAt, filler.FrozenOperationalTETA, filler.CapturedSlot = aman.FreezeNone, nil, nil, nil
	filler.OperationalTETA = at.Add(18 * time.Minute)
	filler.CurrentSlot = &aman.Slot{Time: at.Add(24 * time.Minute), Sequence: 3, RunwayGroupID: "A"}
	previous := queueFlight("PREVIOUS", "A", at.Add(8*time.Minute), "M", 1, at.Add(8*time.Minute))
	next := queueFlight("NEXT", "A", at.Add(20*time.Minute), "M", 2, at.Add(20*time.Minute))
	previous.ProtectCurrentSlot, next.ProtectCurrentSlot = true, true
	initial := input
	initial.Flights = []sequence.Flight{previous, next, filler}
	anchor := previous.Callsign
	decision, err := sequence.ApplyMove(initial, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "move", ExpectedRevision: 7}, Callsign: filler.Callsign, RunwayGroupID: "A", AfterCallsign: &anchor,
	})
	require.NoError(t, err)
	working := commitManualCandidate(decision.Input, decision.Candidate)
	// The flight is still delayed, but its local position permits the new gap.
	working.Flights[2].OperationalTETA = at.Add(12 * time.Minute)
	working.Flights = append(working.Flights, input.Flights[0], input.Flights[1])
	working.Flights[3].CurrentSlot.Sequence = 2
	working.Flights[4].CurrentSlot.Sequence = 3
	working.Flights[2].CurrentSlot.Sequence = 4
	working.Flights[1].CurrentSlot.Sequence = 5
	result, _, err := sequence.GenerateWithVacancyPromotions(working, nil, at)
	require.NoError(t, err)
	require.False(t, result.HasConflicts())
	require.Equal(t, []aman.Callsign{"PREVIOUS", "SAS40G", "CTN480", "FIN3WR", "NEXT"}, entryIDs(result))
	require.Equal(t, at.Add(12*time.Minute), entryFor(t, result, filler.Callsign).Time)
	require.Contains(t, result.Operations, sequence.Operation{Callsign: filler.Callsign, Kind: "gap_fill"})
	encoded, err := json.Marshal(working)
	require.NoError(t, err)
	var restored sequence.Input
	require.NoError(t, json.Unmarshal(encoded, &restored))
	slices.Reverse(restored.Flights)
	replayed, _, err := sequence.GenerateWithVacancyPromotions(restored, nil, at)
	require.NoError(t, err)
	require.Equal(t, result, replayed)
}

func TestManualPositionSurvivesPredictionChangesAndSubsequentMove(t *testing.T) {
	at := testTime()
	input := withCommittedSlots(t, sequence.Input{Revision: 7, Policies: []sequence.Policy{simplePolicy("A", at, 60)}, Flights: []sequence.Flight{
		flight("A", "A", at, "M"), flight("B", "A", at.Add(time.Minute), "M"),
		flight("C", "A", at.Add(2*time.Minute), "M"), flight("D", "A", at.Add(3*time.Minute), "M"),
	}})
	anchor := aman.Callsign("B")
	first, err := sequence.ApplyMove(input, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "first", ExpectedRevision: 7}, Callsign: "C", RunwayGroupID: "A", BeforeCallsign: &anchor,
	})
	require.NoError(t, err)
	working := commitManualCandidate(first.Input, first.Candidate)
	precedence, err := sequence.ManualPositionPrecedence(working)
	require.NoError(t, err)
	require.True(t, precedence["B"]["A"])
	require.False(t, precedence["A"]["B"])
	working.Flights[0].OperationalTETA = at.Add(10 * time.Minute)
	working.Flights[1].OperationalTETA, working.Flights[2].OperationalTETA = at, at
	result, err := sequence.Generate(working)
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"D", "A", "C", "B"}, entryIDs(result), "unrelated traffic may improve its position")
	// Explicitly moving a neighbour again overrides its obsolete relationship.
	anchor = "B"
	second, err := sequence.ApplyMove(working, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "second", ExpectedRevision: 7}, Callsign: "A", RunwayGroupID: "A", AfterCallsign: &anchor,
	})
	require.NoError(t, err)
	result, err = sequence.Generate(commitManualCandidate(second.Input, second.Candidate))
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"C", "B", "D", "A"}, entryIDs(result))
}

func TestManualPositionRejectsCyclesAndIgnoresMissingNeighbours(t *testing.T) {
	at := testTime()
	input := sequence.Input{Policies: []sequence.Policy{simplePolicy("A", at, 60)}, Flights: []sequence.Flight{
		flight("A", "A", at, "M"), flight("B", "A", at, "M"),
	}}
	input.Flights[0].ManualPosition = &aman.ManualSequencePosition{AfterCallsign: "MISSING", BeforeCallsign: "B"}
	result, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"A", "B"}, entryIDs(result))
	input.Flights[1].ManualPosition = &aman.ManualSequencePosition{BeforeCallsign: "A"}
	_, err = sequence.Generate(input)
	require.ErrorContains(t, err, "cyclic manual sequence position")
}

func TestManualPositionResolvesIndirectHoldingCycle(t *testing.T) {
	at := testTime()
	input := sequence.Input{Revision: 7, Policies: []sequence.Policy{simplePolicy("A", at, 60)}, Flights: []sequence.Flight{
		flight("A", "A", at, "M"), flight("B", "A", at, "M"), flight("C", "A", at, "M"),
	}}
	for i := range input.Flights {
		entered := at.Add(time.Duration(-i-1) * time.Minute)
		input.Flights[i].HoldingQueueID = "HOLD"
		input.Flights[i].HoldingQueueTime = &entered
		input.Flights[i].HoldingTransit = 5 * time.Minute
		input.Flights[i].SelectedSTARFamily = "MONAK"
	}
	// The physical queue C -> B -> A conflicts indirectly with the retained
	// A -> C controller relationship. Keep C ahead of B where possible.
	input.Flights[0].ManualPosition = &aman.ManualSequencePosition{BeforeCallsign: "C"}
	result, err := sequence.Generate(input)
	require.NoError(t, err)
	require.False(t, result.HasConflicts())
	require.Equal(t, []aman.Callsign{"A", "C", "B"}, entryIDs(result))
	require.Equal(t, []time.Time{at, at.Add(time.Minute), at.Add(2 * time.Minute)}, entryTimes(result))
	slices.Reverse(input.Flights)
	replayed, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Equal(t, result, replayed)

	working := commitManualCandidate(input, result)
	anchor := aman.Callsign("B")
	_, err = sequence.ApplyMove(working, sequence.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "move", ExpectedRevision: 7}, Callsign: "A", RunwayGroupID: "A", BeforeCallsign: &anchor,
	})
	require.NoError(t, err, "a retained local order must not block subsequent controller moves")
	_, err = sequence.ApplySwap(working, aman.SwapFlightsCommand{
		Metadata: aman.CommandMetadata{CommandID: "swap", ExpectedRevision: 7}, Callsign: "A", OtherCallsign: "B",
	})
	require.NoError(t, err, "a retained local order must not block subsequent controller swaps")
}

func TestLocalOrderPreservesHoldingPriorityAndUnusedCapacity(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		physical time.Duration
		wantPass bool
	}{
		{"held flight can use the gap", 10 * time.Minute, false},
		{"held flight cannot use the gap", 20 * time.Minute, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			at := testTime()
			policy := continuousCPHPolicy(at)
			policy.AllowCrossFeederOvertake = true
			entered, arrives, physical := at.Add(-time.Minute), at.Add(5*time.Minute), at.Add(scenario.physical)
			held := queueFlight("HELD", "A", at.Add(40*time.Minute), "M", 1, at.Add(40*time.Minute))
			held.ProtectCurrentSlot = true
			held.STARFamily, held.HoldingQueueID = "MONAK", "HOLD-A"
			held.ActiveHoldingSince, held.PromotionNotBefore = &entered, &physical
			held.ManualPosition = &aman.ManualSequencePosition{BeforeCallsign: "TAIL"}
			incoming := flight("NEW", "A", at.Add(15*time.Minute), "M")
			incoming.STARFamily, incoming.HoldingQueueID, incoming.ArrivalQueueTime = "TESPI", "HOLD-B", &arrives
			tail := flight("TAIL", "A", at.Add(time.Hour), "M")
			result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{incoming, held, tail}})
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Equal(t, held.CurrentSlot.Time, entryFor(t, result, "HELD").Time)
			require.Equal(t, scenario.wantPass, entryFor(t, result, "NEW").Time.Before(held.CurrentSlot.Time))
		})
	}
}
