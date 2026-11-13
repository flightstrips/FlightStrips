package sequence_test

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestInsertAfterCascadesAllFreezeKindsAndSurvivesRecalculation(t *testing.T) {
	for _, freeze := range []aman.FreezeReason{aman.FreezeNone, aman.FreezeManual, aman.FreezeSuperstable, aman.FreezeTMA} {
		t.Run(string(freeze), func(t *testing.T) {
			at := testTime()
			policy := continuousCPHPolicy(at)
			policy.Rates[0].ArrivalsPerHour = 60
			a := queueFlight("A", "A", at.Add(10*time.Minute), "M", 1, at.Add(10*time.Minute))
			b := queueFlight("B", "A", at.Add(11*time.Minute), "M", 2, at.Add(11*time.Minute))
			x := flight("X", "A", at.Add(10*time.Minute), "M")
			b.ProtectCurrentSlot = true
			b.FreezeReason = freeze
			if freeze != aman.FreezeNone {
				b.FrozenAt = &at
				b.FrozenOperationalTETA = &b.OperationalTETA
				copy := *b.CurrentSlot
				b.CapturedSlot = &copy
			}
			input := sequence.Input{Revision: 7, Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{a, b, x}}
			decision, err := sequence.InsertAfter(input, "X", "A", at)
			require.NoError(t, err)
			require.Equal(t, []aman.Callsign{"A", "X", "B"}, entryIDs(decision.Candidate))
			require.Equal(t, a.CurrentSlot.Time, entryFor(t, decision.Candidate, "A").Time)
			require.True(t, entryFor(t, decision.Candidate, "B").Time.After(b.CurrentSlot.Time))
			committed := decision.Input
			for i := range committed.Flights {
				e := entryFor(t, decision.Candidate, committed.Flights[i].Callsign)
				committed.Flights[i].CurrentSlot = &aman.Slot{Time: e.Time, RunwayGroupID: e.RunwayGroupID, Sequence: e.Sequence, Revision: 7}
				committed.Flights[i].ProtectCurrentSlot = true
			}
			require.Equal(t, freeze, policyFlight(t, committed, "B").FreezeReason)
			again, err := sequence.Generate(committed)
			require.NoError(t, err)
			require.False(t, again.HasConflicts())
			require.Equal(t, entryIDs(decision.Candidate), entryIDs(again))
			require.Equal(t, entryTimes(decision.Candidate), entryTimes(again))
			require.Nil(t, input.Flights[2].ManualPosition, "input is immutable")
		})
	}
}

func TestInsertAfterInterleavesAnotherSTARBeforeCommittingDelay(t *testing.T) {
	input, at := starGapInput()
	for i := range input.Flights {
		input.Flights[i].FreezeReason = aman.FreezeNone
		input.Flights[i].CapturedSlot = nil
		input.Flights[i].FrozenAt = nil
		input.Flights[i].FrozenOperationalTETA = nil
	}
	target := flight("X", "A", at.Add(10*time.Minute), "M")
	target.STARFamily = "ERNOV"
	target.PromotionNotBefore = &target.OperationalTETA
	input.Flights = append(input.Flights, target)
	input.Policies[0].SameSTARSpacing.MinimumEmptySlots = 2
	decision, err := sequence.InsertAfter(input, "X", "SAS40G", at)
	require.NoError(t, err)
	ids := entryIDs(decision.Candidate)
	require.Equal(t, []aman.Callsign{"SAS40G", "CTN480", "X", "FIN3WR"}, ids)
	require.Less(t, entryFor(t, decision.Candidate, "X").Time.Sub(entryFor(t, decision.Candidate, "SAS40G").Time), 6*time.Minute)
	require.Equal(t, input.Flights[0].CurrentSlot.Time, entryFor(t, decision.Candidate, "SAS40G").Time)
	committed := decision.Input
	for i := range committed.Flights {
		e := entryFor(t, decision.Candidate, committed.Flights[i].Callsign)
		committed.Flights[i].CurrentSlot = &aman.Slot{Time: e.Time, RunwayGroupID: e.RunwayGroupID, Sequence: e.Sequence, Revision: 7}
		committed.Flights[i].ProtectCurrentSlot = true
	}
	again, err := sequence.Generate(committed)
	require.NoError(t, err)
	require.False(t, again.HasConflicts())
	require.Equal(t, ids, entryIDs(again))
}

func TestInsertAfterSkipsUnreachableAndProtectedFillers(t *testing.T) {
	for _, blocked := range []string{"late", "manual", "tma"} {
		t.Run(blocked, func(t *testing.T) {
			input, at := starGapInput()
			target := flight("X", "A", at.Add(10*time.Minute), "M")
			target.STARFamily = "ERNOV"
			input.Flights = append(input.Flights, target)
			f := &input.Flights[2]
			switch blocked {
			case "late":
				lower := at.Add(time.Hour)
				f.PromotionNotBefore = &lower
			case "manual":
				f.FreezeReason = aman.FreezeManual
			case "tma":
				f.FreezeReason = aman.FreezeTMA
			}
			decision, err := sequence.InsertAfter(input, "X", "SAS40G", at)
			require.NoError(t, err)
			require.Equal(t, "X", string(decision.Candidate.Entries[1].Callsign))
		})
	}
}

func TestInsertAfterFillsMultipleIntervalsWithoutDelayingTarget(t *testing.T) {
	at := testTime()
	policy := continuousCPHPolicy(at)
	policy.Rates[0].ArrivalsPerHour = 60
	policy.AllowCrossFeederOvertake = true
	flights := []sequence.Flight{
		queueFlight("A", "A", at.Add(10*time.Minute), "M", 1, at.Add(10*time.Minute)),
		queueFlight("B", "A", at.Add(20*time.Minute), "M", 2, at.Add(20*time.Minute)),
		queueFlight("Y", "A", at.Add(11*time.Minute), "M", 3, at.Add(25*time.Minute)),
		queueFlight("Z", "A", at.Add(12*time.Minute), "M", 4, at.Add(26*time.Minute)),
		flight("X", "A", at.Add(18*time.Minute), "M"),
	}
	for i := range flights {
		flights[i].STARFamily = "TESPI"
		flights[i].PromotionNotBefore = &flights[i].OperationalTETA
	}
	flights[2].STARFamily, flights[3].STARFamily = "MONAK", "TUDLO"
	input := sequence.Input{Revision: 7, Policies: []sequence.Policy{policy}, Flights: flights}
	decision, err := sequence.InsertAfter(input, "X", "A", at)
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"A", "Y", "Z", "X", "B"}, entryIDs(decision.Candidate))
	require.Equal(t, at.Add(18*time.Minute), entryFor(t, decision.Candidate, "X").Time)
}

func TestInsertAfterRespectsClosuresAndCapacityGaps(t *testing.T) {
	at := testTime()
	policy := continuousCPHPolicy(at)
	policy.Rates[0].ArrivalsPerHour = 60
	policy.Gaps = []sequence.Gap{{Start: at.Add(11 * time.Minute), End: at.Add(15 * time.Minute)}}
	input := sequence.Input{Revision: 7, Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{queueFlight("A", "A", at.Add(10*time.Minute), "M", 1, at.Add(10*time.Minute)), flight("X", "A", at.Add(10*time.Minute), "M")}}
	decision, err := sequence.InsertAfter(input, "X", "A", at)
	require.NoError(t, err)
	require.Equal(t, at.Add(15*time.Minute), entryFor(t, decision.Candidate, "X").Time)
	input.Policies[0].Closures = []aman.RunwayClosure{{Start: at.Add(11 * time.Minute)}}
	_, err = sequence.InsertAfter(input, "X", "A", at)
	require.Error(t, err)
}

func TestInsertAfterRetainsCompatibleControllerRelationships(t *testing.T) {
	at := testTime()
	policy := continuousCPHPolicy(at)
	policy.AllowCrossFeederOvertake = true
	a := queueFlight("A", "A", at.Add(10*time.Minute), "M", 1, at.Add(10*time.Minute))
	x := queueFlight("X", "A", at.Add(10*time.Minute), "M", 2, at.Add(14*time.Minute))
	y := queueFlight("Y", "A", at.Add(12*time.Minute), "M", 3, at.Add(18*time.Minute))
	a.STARFamily, x.STARFamily, y.STARFamily = "TESPI", "TESPI", "MONAK"
	y.ManualPosition = &aman.ManualSequencePosition{AfterCallsign: "X"}
	y.PromotionNotBefore = &y.OperationalTETA
	input := sequence.Input{Revision: 7, Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{a, x, y}}
	decision, err := sequence.InsertAfter(input, "X", "A", at)
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"A", "X", "Y"}, entryIDs(decision.Candidate))
	require.Equal(t, "X", string(policyFlight(t, decision.Input, "Y").ManualPosition.AfterCallsign))
}

func TestInsertAfterKeepsAnExistingProtectedInterleaverAtItsSlot(t *testing.T) {
	for _, freeze := range []aman.FreezeReason{aman.FreezeManual, aman.FreezeSuperstable, aman.FreezeTMA} {
		t.Run(string(freeze), func(t *testing.T) {
			at := testTime()
			policy := continuousCPHPolicy(at)
			policy.Rates[0].ArrivalsPerHour = 30
			a := queueFlight("A", "A", at.Add(10*time.Minute), "M", 1, at.Add(10*time.Minute))
			y := queueFlight("Y", "A", at.Add(11*time.Minute), "M", 2, at.Add(12*time.Minute))
			x := flight("X", "A", at.Add(14*time.Minute), "M")
			a.STARFamily, x.STARFamily, y.STARFamily = "TESPI", "TESPI", "MONAK"
			y.FreezeReason = freeze
			y.FrozenAt = &at
			y.FrozenOperationalTETA = &y.OperationalTETA
			y.CapturedSlot = y.CurrentSlot
			input := sequence.Input{Revision: 7, Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{a, y, x}}
			decision, err := sequence.InsertAfter(input, "X", "A", at)
			require.NoError(t, err)
			require.Equal(t, []aman.Callsign{"A", "Y", "X"}, entryIDs(decision.Candidate))
			require.Equal(t, y.CurrentSlot.Time, entryFor(t, decision.Candidate, "Y").Time)
			require.Equal(t, freeze, policyFlight(t, decision.Input, "Y").FreezeReason)
		})
	}
}

func TestInsertAfterAppliesHoldingReleaseBoundsThroughoutCascade(t *testing.T) {
	for _, freeze := range []aman.FreezeReason{aman.FreezeNone, aman.FreezeManual, aman.FreezeSuperstable, aman.FreezeTMA} {
		for _, nonadjacent := range []bool{false, true} {
			t.Run(string(freeze)+map[bool]string{false: "/adjacent", true: "/nonadjacent"}[nonadjacent], func(t *testing.T) {
				at := testTime()
				policy := continuousCPHPolicy(at)
				policy.Rates[0].ArrivalsPerHour = 60
				a := queueFlight("A", "A", at.Add(20*time.Minute), "M", 1, at.Add(20*time.Minute))
				x := flight("X", "A", at.Add(20*time.Minute), "M")
				b := queueFlight("B", "A", at.Add(23*time.Minute), "M", 2, at.Add(23*time.Minute))
				entered, following, last := at.Add(-2*time.Minute), at.Add(-time.Minute), at
				a.STARFamily, x.STARFamily, b.STARFamily = "TESPI", "TESPI", "TESPI"
				a.HoldingQueueID, a.HoldingQueueTime, a.HoldingTransit = "HOLD", &entered, 10*time.Minute
				x.HoldingQueueID, x.HoldingQueueTime, x.HoldingTransit = "HOLD", &following, 15*time.Minute
				b.HoldingQueueID, b.HoldingQueueTime, b.HoldingTransit = "HOLD", &last, 20*time.Minute
				b.ProtectCurrentSlot, b.FreezeReason = true, freeze
				if freeze != aman.FreezeNone {
					b.CapturedSlot, b.FrozenAt, b.FrozenOperationalTETA = b.CurrentSlot, &at, &b.OperationalTETA
				}
				input := sequence.Input{Revision: 7, Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{a, x, b}}
				releasePredecessor := a
				expectedTarget := at.Add(26 * time.Minute)
				if nonadjacent {
					h := a
					h.Callsign = "H"
					h.CurrentSlot = &aman.Slot{Time: at.Add(19 * time.Minute), RunwayGroupID: "A", Sequence: 1, Revision: 7}
					a.HoldingQueueID, a.HoldingQueueTime, a.HoldingTransit, a.STARFamily = "", nil, 0, "MONAK"
					input.Flights[0] = a
					input.Flights = append(input.Flights, h)
					releasePredecessor, expectedTarget = h, at.Add(25*time.Minute)
				}
				decision, err := sequence.InsertAfter(input, "X", "A", at)
				require.NoError(t, err)
				require.Equal(t, a.CurrentSlot.Time, entryFor(t, decision.Candidate, "A").Time)
				targetSlot, followerSlot := entryFor(t, decision.Candidate, "X"), entryFor(t, decision.Candidate, "B")
				require.Equal(t, expectedTarget, targetSlot.Time)
				firstRelease := releasePredecessor.CurrentSlot.Time.Add(-releasePredecessor.HoldingTransit)
				targetRelease, followerRelease := targetSlot.Time.Add(-x.HoldingTransit), followerSlot.Time.Add(-b.HoldingTransit)
				require.Equal(t, time.Minute, targetRelease.Sub(firstRelease))
				require.Equal(t, time.Minute, followerRelease.Sub(targetRelease))
				// Normal sequencing must retain the committed release plan.
				committed := decision.Input
				for i := range committed.Flights {
					entry := entryFor(t, decision.Candidate, committed.Flights[i].Callsign)
					committed.Flights[i].CurrentSlot = &aman.Slot{Time: entry.Time, RunwayGroupID: entry.RunwayGroupID, Sequence: entry.Sequence, Revision: 7}
					committed.Flights[i].ProtectCurrentSlot = true
				}
				again, err := sequence.Generate(committed)
				require.NoError(t, err)
				require.False(t, again.HasConflicts())
				require.Equal(t, entryIDs(decision.Candidate), entryIDs(again))
				require.Equal(t, entryTimes(decision.Candidate), entryTimes(again))
			})
		}
	}
}

func TestInsertAfterRetainsRebuiltReleaseOrderAcrossDifferentHolds(t *testing.T) {
	at := testTime()
	policy := continuousCPHPolicy(at)
	policy.Rates[0].ArrivalsPerHour = 60
	a := queueFlight("A", "A", at.Add(20*time.Minute), "M", 1, at.Add(20*time.Minute))
	x := flight("X", "A", at.Add(20*time.Minute), "M")
	first, second := 1, 2
	a.ReleaseRank, a.HoldingTransit = &first, 10*time.Minute
	x.ReleaseRank, x.HoldingTransit = &second, 15*time.Minute
	a.HoldingQueueID, x.HoldingQueueID = "HOLD-A", "HOLD-B"
	decision, err := sequence.InsertAfter(sequence.Input{Revision: 7, Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{a, x}}, "X", "A", at)
	require.NoError(t, err)
	require.Equal(t, at.Add(25*time.Minute), entryFor(t, decision.Candidate, "X").Time)
	require.Equal(t, a.CurrentSlot.Time, entryFor(t, decision.Candidate, "A").Time)
	require.Equal(t, a.CurrentSlot.Time.Add(-a.HoldingTransit), entryFor(t, decision.Candidate, "X").Time.Add(-x.HoldingTransit))
}

func TestInsertAfterPreservesRebuiltTMACapturesAcrossCascadeAndRestart(t *testing.T) {
	at := testTime()
	policy := continuousCPHPolicy(at)
	policy.Rates[0].ArrivalsPerHour = 20
	policy.SameSTARSpacing = sequence.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	input := sequence.Input{Revision: 7, Policies: []sequence.Policy{policy}}
	for i, callsign := range []aman.Callsign{"A", "ANCHOR", "C"} {
		slotAt := at.Add(10*time.Minute + time.Duration(i)*90*time.Second)
		f := queueFlight(callsign, "A", slotAt, "M", i+1, slotAt)
		f.STARFamily, f.FreezeReason, f.ProtectCurrentSlot = "TESPI", aman.FreezeTMA, true
		captured := *f.CurrentSlot
		captured.Reason = string(sequence.ReasonRebuildTMA)
		f.CapturedSlot, f.FrozenAt, f.FrozenOperationalTETA = &captured, &at, &f.OperationalTETA
		input.Flights = append(input.Flights, f)
	}
	target := flight("X", "A", at.Add(12*time.Minute), "M")
	target.STARFamily = "TESPI"
	input.Flights = append(input.Flights, target)
	decision, err := sequence.InsertAfter(input, "X", "ANCHOR", at)
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"A", "ANCHOR", "X", "C"}, entryIDs(decision.Candidate))
	for _, callsign := range []aman.Callsign{"A", "ANCHOR", "C"} {
		captured := policyFlight(t, decision.Input, callsign).CapturedSlot
		require.Equal(t, string(sequence.ReasonRebuildTMA), captured.Reason)
		require.Equal(t, entryFor(t, decision.Candidate, callsign).Time, captured.Time)
	}
	require.Equal(t, input.Flights[0].CurrentSlot.Time, entryFor(t, decision.Candidate, "A").Time)
	require.Equal(t, input.Flights[1].CurrentSlot.Time, entryFor(t, decision.Candidate, "ANCHOR").Time)
	require.True(t, entryFor(t, decision.Candidate, "C").Time.After(input.Flights[2].CurrentSlot.Time))
	committed := decision.Input
	for i := range committed.Flights {
		entry := entryFor(t, decision.Candidate, committed.Flights[i].Callsign)
		committed.Flights[i].CurrentSlot = &aman.Slot{Time: entry.Time, RunwayGroupID: entry.RunwayGroupID, Sequence: entry.Sequence, Revision: 7, Reason: string(entry.Reason)}
		committed.Flights[i].ProtectCurrentSlot = true
	}
	stored, err := json.Marshal(committed)
	require.NoError(t, err)
	var restored sequence.Input
	require.NoError(t, json.Unmarshal(stored, &restored))
	again, err := sequence.Generate(restored)
	require.NoError(t, err)
	require.False(t, again.HasConflicts(), "reconciliation must retain the rebuilt TMA spacing exemption")
	require.Equal(t, entryIDs(decision.Candidate), entryIDs(again))
	require.Equal(t, entryTimes(decision.Candidate), entryTimes(again))
	require.Equal(t, at.Add(13*time.Minute), input.Flights[2].CapturedSlot.Time, "insertion must not mutate the original capture")
}
