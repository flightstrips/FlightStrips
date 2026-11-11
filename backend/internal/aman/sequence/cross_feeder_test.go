package sequence_test

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestCrossFeederArrivalsFollowOccupiedHoldsUnlessCapacityWouldBeUnused(t *testing.T) {
	for _, protected := range []bool{false, true} {
		for _, otherHold := range []string{"HOLD-B", ""} {
			start := testTime()
			policy := continuousCPHPolicy(start)
			policy.AllowCrossFeederOvertake = true
			entered, arrives := start.Add(-20*time.Minute), start.Add(5*time.Minute)
			first := flight("HELD1", "A", start.Add(40*time.Minute), "M")
			first.STARFamily, first.HoldingQueueID = "MONAK", "HOLD-A"
			first.HoldingQueueTime, first.ActiveHoldingSince, first.ArrivalQueueTime = &entered, &entered, &entered
			physical := start.Add(10 * time.Minute)
			first.PromotionNotBefore = &physical
			if protected {
				first.ProtectCurrentSlot = true
				first.CurrentSlot = &aman.Slot{RunwayGroupID: "A", Time: first.OperationalTETA, Sequence: 1}
			}
			second := first
			second.Callsign = "HELD2"
			second.OperationalTETA = start.Add(41*time.Minute + 30*time.Second)
			if protected {
				second.CurrentSlot = &aman.Slot{RunwayGroupID: "A", Time: second.OperationalTETA, Sequence: 2}
			}
			incoming := flight("NEW", "A", start.Add(15*time.Minute), "M")
			incoming.STARFamily, incoming.HoldingQueueID = "TESPI", otherHold
			incoming.ArrivalQueueTime = &arrives
			input := sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{incoming, second, first}}
			result, err := sequence.Generate(input)
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Equal(t, []aman.Callsign{"HELD1", "HELD2", "NEW"}, entryIDs(result))
			require.Equal(t, start.Add(43*time.Minute), entryFor(t, result, "NEW").Time)
		}
	}
}

func TestCrossFeederOvertakeRequiresEvidenceOfUnusedCapacity(t *testing.T) {
	for _, test := range []struct {
		name     string
		physical time.Duration
		wantPass bool
	}{
		{"held aircraft can reach the slot", 10 * time.Minute, false},
		{"held aircraft cannot reach the slot", 20 * time.Minute, true},
		{"assigned delay is not physical evidence", 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			start := testTime()
			policy := continuousCPHPolicy(start)
			policy.AllowCrossFeederOvertake = true
			entered, arrives := start.Add(-time.Minute), start.Add(5*time.Minute)
			held := flight("HELD", "A", start.Add(40*time.Minute), "M")
			held.STARFamily, held.HoldingQueueID = "MONAK", "HOLD-A"
			held.ActiveHoldingSince = &entered
			held.ProtectCurrentSlot = true
			held.CurrentSlot = &aman.Slot{RunwayGroupID: "A", Time: held.OperationalTETA, Sequence: 1}
			if test.physical > 0 {
				physical := start.Add(test.physical)
				held.PromotionNotBefore = &physical
			}
			incoming := flight("NEW", "A", start.Add(15*time.Minute), "M")
			incoming.STARFamily, incoming.HoldingQueueID = "TESPI", "HOLD-B"
			incoming.ArrivalQueueTime = &arrives
			result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{incoming, held}})
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Equal(t, held.CurrentSlot.Time, entryFor(t, result, "HELD").Time)
			require.Equal(t, test.wantPass, entryFor(t, result, "NEW").Time.Before(held.CurrentSlot.Time))
		})
	}
}

func TestCrossFeederOvertakeUsesSameSTARGapWithoutMovingProtectedSlots(t *testing.T) {
	start := testTime()
	policy := continuousCPHPolicy(start)
	policy.AllowCrossFeederOvertake = true
	policy.SameSTARSpacing = sequence.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	entered, later := start.Add(-time.Minute), start
	first := flight("HELD1", "A", start.Add(12*time.Minute), "M")
	first.STARFamily = "MONAK"
	first.HoldingQueueID = "HOLD-A"
	first.HoldingQueueTime = &entered
	first.ActiveHoldingSince = &entered
	first.CurrentSlot = &aman.Slot{RunwayGroupID: "A", Time: start.Add(12 * time.Minute), Sequence: 1}
	first.ProtectCurrentSlot = true
	second := first
	second.Callsign = "HELD2"
	second.OperationalTETA = start.Add(15 * time.Minute)
	second.CurrentSlot = &aman.Slot{RunwayGroupID: "A", Time: start.Add(15 * time.Minute), Sequence: 2}
	incoming := flight("OTHER", "A", start.Add(12*time.Minute), "M")
	incoming.STARFamily = "TESPI"
	incoming.HoldingQueueID = "HOLD-B"
	incoming.ArrivalQueueTime = &later
	incoming.ActiveHoldingSince = &later
	input := sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{first, second, incoming}}
	out, err := sequence.Generate(input)
	require.NoError(t, err)
	require.False(t, out.HasConflicts())
	require.Equal(t, start.Add(90*time.Second+12*time.Minute), entryFor(t, out, "OTHER").Time)
	require.Equal(t, first.CurrentSlot.Time, entryFor(t, out, "HELD1").Time)
	require.Equal(t, second.CurrentSlot.Time, entryFor(t, out, "HELD2").Time)
	input.Policies[0].AllowCrossFeederOvertake = false
	old, err := sequence.Generate(input)
	require.NoError(t, err)
	require.True(t, entryFor(t, old, "OTHER").Time.After(second.CurrentSlot.Time), "disabled policy retains legacy cross-hold priority")
}
