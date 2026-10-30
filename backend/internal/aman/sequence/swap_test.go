package sequence_test

import (
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestSwapExchangesNonAdjacentPositionsAndRetainsOtherOrder(t *testing.T) {
	start := testTime()
	input := withCommittedSlots(t, sequence.Input{Revision: 11, Policies: []sequence.Policy{wtcPolicy("A", start, 60), simplePolicy("B", start, 60)}, Flights: []sequence.Flight{
		flight("FIRST", "A", start, "M"), flight("MIDDLE", "A", start, "M"), flight("LAST", "A", start, "H"), flight("OTHER-RUNWAY", "B", start, "M"),
	}})
	for i := range input.Flights {
		input.Flights[i].State = aman.StateStable
		input.Flights[i].ProtectCurrentSlot = true
		input.Flights[i].SelectedSTARFamily = "TESPI"
	}
	input.Flights[1].SelectedSTARFamily = "TIDVU"
	before, err := json.Marshal(input)
	require.NoError(t, err)
	decision, err := sequence.ApplySwap(input, aman.SwapFlightsCommand{Metadata: aman.CommandMetadata{CommandID: "swap", ExpectedRevision: 11}, Callsign: "FIRST", OtherCallsign: "LAST"})
	require.NoError(t, err)
	require.True(t, decision.Changed)
	require.Equal(t, []aman.Callsign{"LAST", "MIDDLE", "FIRST", "OTHER-RUNWAY"}, entryIDs(decision.Candidate))
	require.False(t, decision.Candidate.HasConflicts())
	require.Equal(t, entryFor(t, decision.Candidate, "OTHER-RUNWAY").Time, input.Flights[3].CurrentSlot.Time)
	require.Nil(t, policyFlight(t, decision.Input, "OTHER-RUNWAY").ManualOrder)
	require.GreaterOrEqual(t, entryFor(t, decision.Candidate, "MIDDLE").Time.Sub(entryFor(t, decision.Candidate, "LAST").Time), 2*time.Minute)
	after, err := json.Marshal(input)
	require.NoError(t, err)
	require.Equal(t, before, after, "pure policy must not mutate its input")

	// Commit the candidate slots and change physical predictions. The manual
	// sequence must remain effective during subsequent routine calculations.
	for i := range decision.Input.Flights {
		f := &decision.Input.Flights[i]
		entry := entryFor(t, decision.Candidate, f.Callsign)
		f.CurrentSlot = &aman.Slot{Time: entry.Time, Sequence: entry.Sequence, RunwayGroupID: entry.RunwayGroupID, Revision: 12}
		f.OperationalTETA = start.Add(30 * time.Minute)
	}
	regenerated, err := sequence.Generate(decision.Input)
	require.NoError(t, err)
	require.Equal(t, entryIDs(decision.Candidate), entryIDs(regenerated))
}

func TestSwapRejectsInvalidPairsAndProtectedConflicts(t *testing.T) {
	for _, name := range []string{"same flight", "missing flight", "missing STAR", "different STAR", "different runway", "frozen first", "frozen second", "landed", "no committed slot", "stale revision", "protected barrier"} {
		t.Run(name, func(t *testing.T) {
			start := testTime()
			input := withCommittedSlots(t, sequence.Input{Revision: 11, Policies: []sequence.Policy{simplePolicy("A", start, 60), simplePolicy("B", start, 60)}, Flights: []sequence.Flight{
				flight("FIRST", "A", start, "M"), flight("MIDDLE", "A", start.Add(time.Minute), "M"), flight("LAST", "A", start.Add(2*time.Minute), "M"),
			}})
			for i := range input.Flights {
				input.Flights[i].SelectedSTARFamily = "TESPI"
			}
			command := aman.SwapFlightsCommand{Metadata: aman.CommandMetadata{CommandID: "swap", ExpectedRevision: 11}, Callsign: "FIRST", OtherCallsign: "LAST"}
			switch name {
			case "same flight":
				command.OtherCallsign = command.Callsign
			case "missing flight":
				command.OtherCallsign = "MISSING"
			case "missing STAR":
				input.Flights[0].SelectedSTARFamily = ""
			case "different STAR":
				input.Flights[2].SelectedSTARFamily = "TIDVU"
			case "different runway":
				input.Flights[2].RunwayGroupID = "B"
			case "frozen first":
				input.Flights[0] = protectedFlight("FIRST", "A", start, "M", start, aman.FreezeManual)
				input.Flights[0].SelectedSTARFamily = "TESPI"
			case "frozen second":
				input.Flights[2] = protectedFlight("LAST", "A", start, "M", start.Add(2*time.Minute), aman.FreezeTMA)
				input.Flights[2].SelectedSTARFamily = "TESPI"
			case "landed":
				input.Flights[2].State = aman.StateLanded
			case "no committed slot":
				input.Flights[2].CurrentSlot = nil
			case "stale revision":
				command.Metadata.ExpectedRevision--
			case "protected barrier":
				input.Flights[1] = protectedFlight("MIDDLE", "A", start, "M", start.Add(time.Minute), aman.FreezeManual)
			}
			before, err := json.Marshal(input)
			require.NoError(t, err)
			_, err = sequence.ApplySwap(input, command)
			require.Error(t, err)
			after, err := json.Marshal(input)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestSwapUsesCommittedPositionsWhenAutomaticOrderHasChanged(t *testing.T) {
	start := testTime()
	input := withCommittedSlots(t, sequence.Input{Revision: 11, Policies: []sequence.Policy{simplePolicy("A", start, 60)}, Flights: []sequence.Flight{
		flight("FIRST", "A", start, "M"), flight("MIDDLE", "A", start.Add(time.Minute), "M"), flight("LAST", "A", start.Add(2*time.Minute), "M"),
	}})
	for i := range input.Flights {
		input.Flights[i].SelectedSTARFamily = "TESPI"
	}
	input.Flights[0].OperationalTETA = start.Add(10 * time.Minute)
	input.Flights[2].OperationalTETA = start
	decision, err := sequence.ApplySwap(input, aman.SwapFlightsCommand{Metadata: aman.CommandMetadata{CommandID: "swap", ExpectedRevision: 11}, Callsign: "FIRST", OtherCallsign: "LAST"})
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"LAST", "MIDDLE", "FIRST"}, entryIDs(decision.Candidate))
}
