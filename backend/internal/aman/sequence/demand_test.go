package sequence

import (
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func demandInput(at time.Time, count int) Input {
	input := Input{Policies: []Policy{{RunwayGroupID: "A", ContinuousSpacing: true, Rates: []RatePoint{{EffectiveAt: at.Add(-time.Hour), ArrivalsPerHour: 40}}, SeparationRules: []SeparationRule{{Leading: "M", Trailing: "M"}}, UnknownSeparation: time.Minute, SameSTARSpacing: SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}}}}
	for i := 0; i < count; i++ {
		input.Flights = append(input.Flights, Flight{Callsign: aman.Callsign(string(rune('A' + i))), RunwayGroupID: "A", State: aman.StateUnstable, FreezeReason: aman.FreezeNone, WakeCategory: "M", STARFamily: "MONAK", OperationalTETA: at, DemandArrivalAt: &at})
	}
	return input
}

func TestDemandSpacingThresholdAndSnapshot(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	for _, count := range []int{4, 5} {
		input := demandInput(at, count)
		result, err := Generate(input)
		require.NoError(t, err)
		gap := 90 * time.Second
		if count == 5 {
			gap = 3 * time.Minute
		}
		require.Equal(t, gap, result.Entries[1].Time.Sub(result.Entries[0].Time))
		for i := range input.Flights {
			input.Flights[i].CurrentSlot = &aman.Slot{RunwayGroupID: "A", Time: at.Add(time.Hour + time.Duration(i)*time.Minute), Sequence: i + 1}
		}
		slices.Reverse(input.Flights)
		again, err := Generate(input)
		require.NoError(t, err)
		require.Equal(t, result.Entries, again.Entries, "slots and input order cannot change predicted demand")
	}
}

func TestDemandWindowBoundariesAndExclusions(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	input := demandInput(at, 5)
	start, end := at.Add(-demandWindow/2), at.Add(demandWindow/2)
	input.Flights[0].DemandArrivalAt = &start
	policies, _, err := prepareInput(input)
	require.NoError(t, err)
	require.True(t, policies["A"].demandBusy(&at, 20), "start included")
	input.Flights[0].DemandArrivalAt = &end
	policies, _, err = prepareInput(input)
	require.NoError(t, err)
	require.False(t, policies["A"].demandBusy(&at, 20), "end excluded")
	for _, state := range []aman.FlightState{aman.StatePlanned, aman.StateLanded, aman.StateRemoved} {
		input = demandInput(at, 5)
		input.Flights[0].State = state
		policies, _, err = prepareInput(input)
		require.NoError(t, err)
		require.False(t, policies["A"].demandBusy(&at, 20))
	}
	input = demandInput(at, 5)
	input.Flights[0].DemandArrivalAt = nil
	policies, _, err = prepareInput(input)
	require.NoError(t, err)
	require.False(t, policies["A"].demandBusy(&at, 20), "missing prediction does not fall back to slot or sequencing TETA")
	input = demandInput(at, 5)
	other := input.Policies[0]
	other.RunwayGroupID = "B"
	input.Policies = append(input.Policies, other)
	input.Flights[0].RunwayGroupID = "B"
	policies, _, err = prepareInput(input)
	require.NoError(t, err)
	require.False(t, policies["A"].demandBusy(&at, 20))
	require.False(t, policies["B"].demandBusy(&at, 20))
}

func TestHoldingSpacingFamilyScopeAndApproach(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	input := demandInput(at, 2)
	input.Flights[0].DemandArrivalAt = nil
	input.Flights[1].DemandArrivalAt = nil
	for _, active := range []bool{false, true} {
		input.Flights[0].HoldingQueueID = "MONAK"
		input.Flights[0].HoldingQueueTime = &at
		if active {
			input.Flights[0].ActiveHoldingSince = &at
		}
		policies, flights, err := prepareInput(input)
		require.NoError(t, err)
		want := time.Duration(0)
		if active {
			want = 3 * time.Minute
		}
		require.Equal(t, want, sameSTARGap(policies["A"], flights["A"][0], flights["A"][1], at))
	}
	input.Flights[0].STARFamily = "TUDLO"
	policies, flights, err := prepareInput(input)
	require.NoError(t, err)
	trail := flights["A"][1]
	require.Zero(t, sameSTARGap(policies["A"], trail, trail, at), "holding on another family must not activate MONAK")
	require.Zero(t, sameSTARGap(policies["A"], flights["A"][0], trail, at), "different families never receive extra spacing")
}

func TestDemandCountsAcrossSTARFamilies(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	input := demandInput(at, 5)
	for i := 2; i < len(input.Flights); i++ {
		input.Flights[i].STARFamily = "TUDLO"
	}
	policies, flights, err := prepareInput(input)
	require.NoError(t, err)
	require.Equal(t, 3*time.Minute, sameSTARGap(policies["A"], flights["A"][0], flights["A"][1], at), "other STARs contribute to runway-group demand")
}
