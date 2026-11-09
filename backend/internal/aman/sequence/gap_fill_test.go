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
	filler.ActiveHoldingSince = &at
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

func TestInboundCanFillGapWhenNoAircraftAreHolding(t *testing.T) {
	input, at := starGapInput()
	for i := range input.Flights {
		input.Flights[i].ActiveHoldingSince = nil
		input.Flights[i].DemandArrivalAt = &at
	}
	input.Policies[0].SameSTARSpacing.ActivationRatePerHour = 1
	result, promotions, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
	require.NoError(t, err)
	require.False(t, result.HasConflicts())
	require.Equal(t, []aman.Callsign{"SAS40G", "CTN480", "FIN3WR"}, entryIDs(result))
	require.Equal(t, at.Add(12*time.Minute), entryFor(t, result, "CTN480").Time)
	require.Len(t, promotions, 1)
}

func TestGapFillingPrefersHoldingThenEarlierSequence(t *testing.T) {
	for _, scenario := range []string{"earlier sequence", "holding first", "earlier aircraft cannot reach gap"} {
		t.Run(scenario, func(t *testing.T) {
			input, at := starGapInput()
			for i := range input.Flights {
				input.Flights[i].ActiveHoldingSince = nil
				input.Flights[i].DemandArrivalAt = &at
			}
			input.Policies[0].SameSTARSpacing.ActivationRatePerHour = 1
			later := queueFlight("LATER", "A", at.Add(12*time.Minute), "M", 4, at.Add(20*time.Minute))
			later.STARFamily, later.HoldingQueueID = "TUDLO", "OTHER-HOLD"
			later.ProtectCurrentSlot, later.FreezeReason = true, aman.FreezeSuperstable
			later.FrozenAt, later.FrozenOperationalTETA, later.CapturedSlot = &at, &later.OperationalTETA, later.CurrentSlot
			later.PromotionNotBefore = &later.OperationalTETA
			winner := input.Flights[2].Callsign
			switch scenario {
			case "holding first":
				later.ActiveHoldingSince = &at
				winner = later.Callsign
			case "earlier aircraft cannot reach gap":
				bound := at.Add(15 * time.Minute)
				input.Flights[2].PromotionNotBefore = &bound
				winner = later.Callsign
			}
			input.Flights = append(input.Flights, later)
			result, _, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Equal(t, winner, result.Entries[1].Callsign)
			require.Equal(t, at.Add(12*time.Minute), result.Entries[1].Time)
			slices.Reverse(input.Flights)
			replayed, _, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
			require.NoError(t, err)
			require.Equal(t, result, replayed)
		})
	}
}

func TestInboundFillsGapOnlyAfterPreferredArrivalsCannotUseIt(t *testing.T) {
	for _, scenario := range []string{"holding fits", "holding cannot reach gap", "holding wake does not fit", "earlier inbound cannot reach gap"} {
		t.Run(scenario, func(t *testing.T) {
			input, at := starGapInput()
			preferred := &input.Flights[2]
			incoming := queueFlight("INBOUND", "A", at.Add(12*time.Minute), "M", 4, at.Add(22*time.Minute))
			incoming.STARFamily, incoming.HoldingQueueID = "TUDLO", "OTHER-HOLD"
			incoming.ProtectCurrentSlot, incoming.FreezeReason = true, aman.FreezeSuperstable
			incoming.FrozenAt, incoming.FrozenOperationalTETA, incoming.CapturedSlot = &at, &incoming.OperationalTETA, incoming.CurrentSlot
			incoming.PromotionNotBefore = &incoming.OperationalTETA
			arrives := at.Add(5 * time.Minute)
			incoming.ArrivalQueueTime, incoming.HoldingQueueTime = &arrives, &arrives
			winner := incoming.Callsign
			switch scenario {
			case "holding fits":
				winner = preferred.Callsign
			case "holding wake does not fit":
				preferred.WakeCategory = "?"
			default:
				bound := at.Add(15 * time.Minute)
				preferred.PromotionNotBefore = &bound
				if scenario == "earlier inbound cannot reach gap" {
					preferred.ActiveHoldingSince = nil
				}
			}
			input.Flights = append(input.Flights, incoming)
			result, _, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Equal(t, winner, result.Entries[1].Callsign)
			require.Equal(t, at.Add(12*time.Minute), result.Entries[1].Time)
			require.Equal(t, input.Flights[0].CurrentSlot.Time, entryFor(t, result, input.Flights[0].Callsign).Time)
			require.Equal(t, input.Flights[1].CurrentSlot.Time, entryFor(t, result, input.Flights[1].Callsign).Time)
			slices.Reverse(input.Flights)
			replayed, _, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
			require.NoError(t, err)
			require.Equal(t, result, replayed)
		})
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
				// Two aircraft actually holding in this family also activate
				// its own six-minute STAR spacing.
				second.CurrentSlot.Time = at.Add(24 * time.Minute)
			}
			second.ProtectCurrentSlot = true
			second.FreezeReason = aman.FreezeSuperstable
			second.FrozenAt, second.FrozenOperationalTETA = &at, &second.OperationalTETA
			second.PromotionNotBefore = &second.OperationalTETA
			second.CapturedSlot = second.CurrentSlot
			second.ActiveHoldingSince = &at
			input.Flights = append(input.Flights, second)

			result, promotions, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			if test.secondFamily == "MONAK" {
				require.Equal(t, []aman.Callsign{"SAS40G", "CTN480", "FIN3WR", "SECOND"}, entryIDs(result))
				require.Equal(t, at.Add(12*time.Minute), entryFor(t, result, "CTN480").Time)
				require.False(t, entryFor(t, result, "SECOND").Time.Before(at.Add(18*time.Minute)))
			} else {
				require.Equal(t, []aman.Callsign{"SAS40G", "CTN480", "SECOND", "FIN3WR"}, entryIDs(result))
				require.Equal(t, []time.Time{at.Add(10 * time.Minute), at.Add(12 * time.Minute), at.Add(14 * time.Minute), at.Add(16 * time.Minute)}, entryTimes(result))
			}
			wantPromotions := 2
			if test.firstPlaced || test.secondFamily == "MONAK" {
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
		{"same hold release before leading", func(input *sequence.Input, at time.Time) {
			entered, arriving := at.Add(-time.Minute), at.Add(time.Minute)
			input.Flights[0].HoldingQueueID = input.Flights[2].HoldingQueueID
			input.Flights[0].HoldingQueueTime, input.Flights[0].HoldingTransit = &entered, 5*time.Minute
			input.Flights[2].ActiveHoldingSince = nil
			input.Flights[2].HoldingQueueTime, input.Flights[2].ArrivalQueueTime = &arriving, &arriving
			input.Flights[2].HoldingTransit = 10 * time.Minute
		}},
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

func TestRebuiltHoldingFillsGapOnlyWhenReleaseOrderRemainsValid(t *testing.T) {
	for _, transit := range []time.Duration{8 * time.Minute, 10 * time.Minute} {
		t.Run(transit.String(), func(t *testing.T) {
			input, at := starGapInput()
			olderRank, laterRank := 1, 2
			input.Flights[1].ReleaseRank, input.Flights[1].HoldingTransit = &olderRank, 10*time.Minute
			held := &input.Flights[2]
			held.ReleaseRank, held.HoldingTransit = &laterRank, transit
			// Rebuild carries confirmed stack-entry evidence even when the
			// ordinary admission timestamps are cleared for that calculation.
			held.ActiveHoldingSince = nil
			held.HoldingStartedAt, held.HoldingStackID = &at, held.HoldingQueueID
			result, _, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			if transit == 8*time.Minute {
				require.Equal(t, at.Add(12*time.Minute), entryFor(t, result, held.Callsign).Time)
			} else {
				require.Equal(t, held.CurrentSlot.Time, entryFor(t, result, held.Callsign).Time)
			}
			require.False(t, entryFor(t, result, held.Callsign).Time.Add(-transit).Before(
				entryFor(t, result, input.Flights[1].Callsign).Time.Add(-10*time.Minute)), "gap filling must retain rebuilt release precedence")
		})
	}
}
