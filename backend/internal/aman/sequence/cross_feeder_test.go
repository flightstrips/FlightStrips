package sequence_test

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

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
