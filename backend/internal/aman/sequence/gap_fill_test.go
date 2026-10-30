package sequence_test

import (
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func starGapInput() (sequence.Input, time.Time) {
	at := testTime()
	policy := continuousCPHPolicy(at)
	policy.Rates[0].ArrivalsPerHour = 30
	policy.AllowCrossFeederOvertake = true
	policy.SameSTARSpacing = sequence.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	lead := queueFlight("SAS40G", "A", at.Add(10*time.Minute), "M", 1, at.Add(10*time.Minute))
	tail := queueFlight("FIN3WR", "A", at.Add(14*time.Minute), "M", 2, at.Add(14*time.Minute))
	filler := queueFlight("CTN480", "A", at.Add(12*time.Minute), "M", 3, at.Add(18*time.Minute))
	for _, flight := range []*sequence.Flight{&lead, &tail, &filler} {
		flight.ProtectCurrentSlot = true
		flight.FreezeReason = aman.FreezeSuperstable
		flight.FrozenAt, flight.FrozenOperationalTETA = &at, &flight.OperationalTETA
		flight.CapturedSlot = flight.CurrentSlot
		flight.PromotionNotBefore = &flight.OperationalTETA
	}
	lead.STARFamily, tail.STARFamily, filler.STARFamily = "ERNOV", "ERNOV", "MONAK"
	lead.HoldingQueueID, tail.HoldingQueueID, filler.HoldingQueueID = "ERNOV", "ERNOV", "OLPIB"
	lead.ActiveHoldingSince = &at
	return sequence.Input{Revision: 7, Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{lead, tail, filler}}, at
}

func TestCrossSTARVacancyFillsFourMinuteGapBetweenProtectedArrivals(t *testing.T) {
	for _, frozen := range []bool{false, true} {
		input, at := starGapInput()
		if !frozen {
			input.Flights[2].FreezeReason = aman.FreezeNone
			input.Flights[2].FrozenAt, input.Flights[2].FrozenOperationalTETA, input.Flights[2].CapturedSlot = nil, nil, nil
		}
		result, promotions, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
		require.NoError(t, err)
		require.False(t, result.HasConflicts())
		require.Equal(t, []aman.Callsign{"SAS40G", "CTN480", "FIN3WR"}, entryIDs(result))
		require.Equal(t, at.Add(12*time.Minute), entryFor(t, result, "CTN480").Time)
		require.Equal(t, input.Flights[0].CurrentSlot.Time, entryFor(t, result, "SAS40G").Time)
		require.Equal(t, input.Flights[1].CurrentSlot.Time, entryFor(t, result, "FIN3WR").Time)
		require.Len(t, promotions, 1)
		require.Equal(t, "CTN480", string(promotions[0].Callsign))
		if frozen {
			require.Equal(t, sequence.ReasonFreezeSuperstable, entryFor(t, result, "CTN480").Reason)
		}
		slices.Reverse(input.Flights)
		replayed, replayedPromotions, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
		require.NoError(t, err)
		require.Equal(t, result, replayed)
		require.Equal(t, promotions, replayedPromotions)
	}
}

func TestCrossSTARVacancyFillsEveryReachableSlot(t *testing.T) {
	for _, test := range []struct {
		name         string
		secondFamily string
		firstPlaced  bool
	}{
		{"different streams", "TUDLO", false},
		{"same stream", "MONAK", false},
		{"partly filled on previous reconciliation", "TUDLO", true},
		{"same stream on previous reconciliation", "MONAK", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, at := starGapInput()
			input.Policies[0].SameSTARSpacing.MinimumEmptySlots = 2
			input.Flights[1].CurrentSlot.Time = at.Add(16 * time.Minute)
			input.Flights[1].OperationalTETA = input.Flights[1].CurrentSlot.Time
			if test.firstPlaced {
				input.Flights[2].CurrentSlot.Time = at.Add(12 * time.Minute)
				input.Flights[2].CurrentSlot.Sequence = 2
				input.Flights[1].CurrentSlot.Sequence = 3
			}
			second := queueFlight("SECOND", "A", at.Add(14*time.Minute), "M", 4, at.Add(20*time.Minute))
			second.STARFamily = test.secondFamily
			second.HoldingQueueID = "SECOND-HOLD"
			if test.secondFamily == "MONAK" {
				second.HoldingQueueID = input.Flights[2].HoldingQueueID
			}
			second.ProtectCurrentSlot = true
			second.FreezeReason = aman.FreezeSuperstable
			second.FrozenAt, second.FrozenOperationalTETA = &at, &second.OperationalTETA
			second.PromotionNotBefore = &second.OperationalTETA
			second.CapturedSlot = second.CurrentSlot
			input.Flights = append(input.Flights, second)

			result, promotions, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Equal(t, []aman.Callsign{"SAS40G", "CTN480", "SECOND", "FIN3WR"}, entryIDs(result))
			require.Equal(t, []time.Time{at.Add(10 * time.Minute), at.Add(12 * time.Minute), at.Add(14 * time.Minute), at.Add(16 * time.Minute)}, entryTimes(result))
			wantPromotions := 2
			if test.firstPlaced {
				wantPromotions = 1
			}
			require.Len(t, promotions, wantPromotions)
			slices.Reverse(input.Flights)
			replayed, replayedPromotions, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
			require.NoError(t, err)
			require.Equal(t, result, replayed)
			require.Equal(t, promotions, replayedPromotions)
		})
	}
}

func TestCrossSTARGapFillingPreservesOperationalConstraints(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*sequence.Input, time.Time)
	}{
		{"disabled", func(input *sequence.Input, _ time.Time) { input.Policies[0].AllowCrossFeederOvertake = false }},
		{"manual target", func(input *sequence.Input, _ time.Time) { order := 3; input.Flights[2].ManualOrder = &order }},
		{"manual boundary", func(input *sequence.Input, _ time.Time) { order := 2; input.Flights[1].ManualOrder = &order }},
		{"TMA target", func(input *sequence.Input, _ time.Time) { input.Flights[2].FreezeReason = aman.FreezeTMA }},
		{"TMA boundary", func(input *sequence.Input, _ time.Time) { input.Flights[1].FreezeReason = aman.FreezeTMA }},
		{"manual freeze", func(input *sequence.Input, _ time.Time) { input.Flights[2].FreezeReason = aman.FreezeManual }},
		{"same STAR", func(input *sequence.Input, _ time.Time) { input.Flights[2].STARFamily = "ERNOV" }},
		{"unknown STAR", func(input *sequence.Input, _ time.Time) { input.Flights[2].STARFamily = "" }},
		{"same hold", func(input *sequence.Input, _ time.Time) { input.Flights[2].HoldingQueueID = "ERNOV" }},
		{"release rank", func(input *sequence.Input, _ time.Time) { rank := 2; input.Flights[2].ReleaseRank = &rank }},
		{"unreachable", func(input *sequence.Input, at time.Time) {
			lower := at.Add(13 * time.Minute)
			input.Flights[2].PromotionNotBefore = &lower
		}},
		{"unknown wake", func(input *sequence.Input, _ time.Time) { input.Flights[2].WakeCategory = "?" }},
		{"wake separation", func(input *sequence.Input, at time.Time) {
			input.Policies[0].Rates[0].ArrivalsPerHour = 40
			input.Flights[0].WakeCategory = "H"
			input.Flights[1].CurrentSlot.Time = at.Add(13 * time.Minute)
		}},
		{"no physical timing", func(input *sequence.Input, _ time.Time) { input.Flights[2].PromotionNotBefore = nil }},
		{"inactive STAR spacing", func(input *sequence.Input, _ time.Time) { input.Flights[0].ActiveHoldingSince = nil }},
		{"closed interval", func(input *sequence.Input, at time.Time) {
			input.Policies[0].Gaps = []sequence.Gap{{Start: at.Add(11 * time.Minute), End: at.Add(13 * time.Minute)}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, at := starGapInput()
			test.mutate(&input, at)
			result, _, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Equal(t, []aman.Callsign{"SAS40G", "FIN3WR", "CTN480"}, entryIDs(result))
			require.True(t, entryFor(t, result, "CTN480").Time.After(input.Flights[1].CurrentSlot.Time))
			require.Equal(t, input.Flights[0].CurrentSlot.Time, entryFor(t, result, "SAS40G").Time)
			require.Equal(t, input.Flights[1].CurrentSlot.Time, entryFor(t, result, "FIN3WR").Time)
		})
	}
}
