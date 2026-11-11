package sequence_test

import (
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestHoldingPriorityExchangesConsecutiveInboundSlotsDeterministically(t *testing.T) {
	start := testTime()
	entered, arriving := start.Add(-time.Minute), start.Add(5*time.Minute)
	input := sequence.Input{Policies: []sequence.Policy{simplePolicy("A", start, 20)}}
	for i, id := range []aman.Callsign{"INBOUND1", "INBOUND2", "SAS891"} {
		slot := start.Add(time.Duration(20+i*3) * time.Minute)
		f := queueFlight(string(id), "A", start.Add(10*time.Minute), "M", i+1, slot)
		f.ProtectCurrentSlot, f.HoldingPriorityExchange = true, true
		physical := start.Add(10 * time.Minute)
		f.PromotionNotBefore, f.ArrivalQueueTime = &physical, &arriving
		if id == "SAS891" {
			f.HoldingSlotProtected, f.HoldingQueueID, f.HoldingStackID = true, "HOLD-A", "HOLD-A"
			f.ActiveHoldingSince, f.HoldingQueueTime, f.ArrivalQueueTime = &entered, &entered, &entered
			f.HoldingTransit = 10 * time.Minute
		}
		input.Flights = append(input.Flights, f)
	}
	result, err := sequence.Generate(input)
	require.NoError(t, err)
	require.False(t, result.HasConflicts())
	require.Equal(t, []aman.Callsign{"SAS891", "INBOUND1", "INBOUND2"}, entryIDs(result))
	require.Equal(t, []time.Time{start.Add(20 * time.Minute), start.Add(23 * time.Minute), start.Add(26 * time.Minute)}, entryTimes(result))
	require.Contains(t, result.Operations, sequence.Operation{Callsign: "SAS891", Kind: "holding_priority_exchange"})
	slices.Reverse(input.Flights)
	replayed, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Equal(t, result, replayed)
	require.Equal(t, start.Add(26*time.Minute), input.Flights[0].CurrentSlot.Time, "generation must not mutate committed slots")
}

func TestHoldingPriorityExchangeChecksWakeAndOtherHoldingReleases(t *testing.T) {
	for _, scenario := range []string{"wake", "older release", "release reservation", "inbound cannot reach later slot", "local controller order"} {
		t.Run(scenario, func(t *testing.T) {
			start := testTime()
			entered, arriving := start.Add(-time.Minute), start.Add(5*time.Minute)
			incoming := queueFlight("INBOUND", "A", start.Add(10*time.Minute), "M", 1, start.Add(20*time.Minute))
			held := queueFlight("HELD", "A", start.Add(10*time.Minute), "M", 2, start.Add(23*time.Minute))
			for _, f := range []*sequence.Flight{&incoming, &held} {
				f.ProtectCurrentSlot, f.HoldingPriorityExchange = true, true
				physical := start.Add(10 * time.Minute)
				f.PromotionNotBefore, f.ArrivalQueueTime = &physical, &arriving
			}
			held.ActiveHoldingSince, held.HoldingQueueTime, held.HoldingQueueID, held.HoldingTransit = &entered, &entered, "HOLD-A", 10*time.Minute
			policy := simplePolicy("A", start, 20)
			input := sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{incoming, held}}
			switch scenario {
			case "local controller order":
				input.Flights[0].ManualPosition = &aman.ManualSequencePosition{BeforeCallsign: "HELD"}
			case "wake":
				// The existing M->H order fits; H->M would need four minutes.
				input.Policies[0].SeparationRules = []sequence.SeparationRule{
					{Leading: "M", Trailing: "M", Minimum: time.Minute},
					{Leading: "M", Trailing: "H", Minimum: time.Minute},
					{Leading: "H", Trailing: "M", Minimum: 4 * time.Minute},
					{Leading: "H", Trailing: "H", Minimum: time.Minute},
				}
				input.Flights[1].WakeCategory = "H"
			case "older release":
				older := queueFlight("OLDER", "A", start.Add(10*time.Minute), "M", 1, start.Add(17*time.Minute))
				older.ProtectCurrentSlot = true
				entry := entered.Add(-time.Minute)
				older.HoldingQueueID, older.HoldingQueueTime, older.HoldingTransit = "HOLD-A", &entry, 5*time.Minute
				input.Flights = append(input.Flights, older)
			case "release reservation":
				rank := 1
				input.Flights[1].ReleaseRank = &rank
			case "inbound cannot reach later slot":
				bound := start.Add(24 * time.Minute)
				input.Flights[0].PromotionNotBefore = &bound
			}
			result, err := sequence.Generate(input)
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Equal(t, incoming.CurrentSlot.Time, entryFor(t, result, incoming.Callsign).Time)
			require.Equal(t, held.CurrentSlot.Time, entryFor(t, result, held.Callsign).Time)
		})
	}
}

func TestHoldingPriorityExchangePreservesFollowerReleaseOrder(t *testing.T) {
	for _, freeze := range []aman.FreezeReason{aman.FreezeNone, aman.FreezeManual} {
		for _, test := range []struct {
			name        string
			transit     time.Duration
			swap        bool
			nonadjacent bool
		}{
			{"release reversal", 10 * time.Minute, false, false},
			{"one minute release separation", 7 * time.Minute, true, false},
			{"below one minute release separation", 7*time.Minute + time.Nanosecond, false, false},
			{"nonadjacent release reversal", 13 * time.Minute, false, true},
			{"nonadjacent one minute release separation", 10 * time.Minute, true, true},
		} {
			t.Run(string(freeze)+"/"+test.name, func(t *testing.T) {
				start := testTime()
				entered, arriving, following := start.Add(-time.Minute), start.Add(5*time.Minute), start.Add(6*time.Minute)
				incoming := queueFlight("INBOUND", "A", start.Add(10*time.Minute), "M", 1, start.Add(20*time.Minute))
				held := queueFlight("HELD", "A", start.Add(10*time.Minute), "M", 2, start.Add(23*time.Minute))
				follower := queueFlight("FOLLOWER", "A", start.Add(10*time.Minute), "M", 3, start.Add(26*time.Minute))
				for _, f := range []*sequence.Flight{&incoming, &held} {
					f.ProtectCurrentSlot, f.HoldingPriorityExchange = true, true
					f.PromotionNotBefore = &f.OperationalTETA
				}
				incoming.HoldingQueueID, incoming.HoldingQueueTime, incoming.ArrivalQueueTime, incoming.HoldingTransit = "HOLD-B", &arriving, &arriving, 5*time.Minute
				held.HoldingQueueID, held.HoldingQueueTime, held.ActiveHoldingSince, held.HoldingTransit = "HOLD-A", &entered, &entered, 10*time.Minute
				follower.ProtectCurrentSlot, follower.FreezeReason = true, freeze
				follower.HoldingQueueID, follower.HoldingQueueTime, follower.ArrivalQueueTime, follower.HoldingTransit = "HOLD-B", &following, &following, test.transit
				if test.nonadjacent {
					follower.CurrentSlot.Time = start.Add(29 * time.Minute)
					follower.CurrentSlot.Sequence = 4
				}
				if freeze == aman.FreezeManual {
					follower.CapturedSlot = follower.CurrentSlot
				}
				input := sequence.Input{Policies: []sequence.Policy{simplePolicy("A", start, 20)}, Flights: []sequence.Flight{incoming, held, follower}}
				if test.nonadjacent {
					unrelated := queueFlight("UNRELATED", "A", start.Add(10*time.Minute), "M", 3, start.Add(26*time.Minute))
					unrelated.ProtectCurrentSlot = true
					input.Flights = append(input.Flights, unrelated)
				}
				result, err := sequence.Generate(input)
				require.NoError(t, err)
				require.False(t, result.HasConflicts())
				wantOrder := []aman.Callsign{"INBOUND", "HELD"}
				if test.swap {
					wantOrder = []aman.Callsign{"HELD", "INBOUND"}
				} else {
					require.Empty(t, result.Movements)
				}
				if test.nonadjacent {
					wantOrder = append(wantOrder, "UNRELATED")
				}
				require.Equal(t, append(wantOrder, "FOLLOWER"), entryIDs(result))
				require.Equal(t, follower.CurrentSlot.Time, entryFor(t, result, follower.Callsign).Time)
				incomingRelease := entryFor(t, result, incoming.Callsign).Time.Add(-incoming.HoldingTransit)
				followerRelease := entryFor(t, result, follower.Callsign).Time.Add(-follower.HoldingTransit)
				require.GreaterOrEqual(t, followerRelease.Sub(incomingRelease), time.Minute)
				slices.Reverse(input.Flights)
				replayed, err := sequence.Generate(input)
				require.NoError(t, err)
				require.Equal(t, result, replayed)
			})
		}
	}
}

func TestOrdinaryAllocationDoesNotFillHoldingGapWithInboundTraffic(t *testing.T) {
	start := testTime()
	policy := continuousCPHPolicy(start)
	policy.AllowCrossFeederOvertake = true
	policy.SameSTARSpacing = sequence.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	entered, arriving := start.Add(-time.Minute), start.Add(5*time.Minute)
	lead := queueFlight("LEAD", "A", start.Add(12*time.Minute), "M", 1, start.Add(12*time.Minute))
	held := queueFlight("HELD", "A", start.Add(60*time.Minute), "M", 2, start.Add(60*time.Minute))
	for _, f := range []*sequence.Flight{&lead, &held} {
		f.ProtectCurrentSlot, f.STARFamily, f.HoldingQueueID = true, "MONAK", "HOLD-A"
		f.HoldingQueueTime, f.ActiveHoldingSince = &entered, &entered
	}
	incoming := flight("AUA460", "A", start.Add(20*time.Minute), "M")
	incoming.STARFamily, incoming.HoldingQueueID, incoming.ArrivalQueueTime = "TESPI", "HOLD-B", &arriving
	result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{lead, held, incoming}})
	require.NoError(t, err)
	require.False(t, result.HasConflicts())
	require.Equal(t, start.Add(60*time.Minute+90*time.Second), entryFor(t, result, incoming.Callsign).Time)
}

func TestInboundPromotionRequiresExplicitGapFill(t *testing.T) {
	start := testTime()
	policy := continuousCPHPolicy(start)
	policy.AllowCrossFeederOvertake = true
	policy.SameSTARSpacing = sequence.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	entered, arriving := start.Add(-time.Minute), start.Add(5*time.Minute)
	lead := queueFlight("LEAD", "A", start.Add(12*time.Minute), "M", 1, start.Add(12*time.Minute))
	held := queueFlight("HELD", "A", start.Add(60*time.Minute), "M", 2, start.Add(60*time.Minute))
	for rank, f := range []*sequence.Flight{&lead, &held} {
		f.ProtectCurrentSlot, f.STARFamily, f.HoldingQueueID = true, "MONAK", "HOLD-A"
		f.HoldingQueueTime, f.ActiveHoldingSince = &entered, &entered
		value := rank + 1
		f.ReleaseRank = &value
	}
	incoming := queueFlight("AUA460", "A", start.Add(20*time.Minute), "M", 3, start.Add(60*time.Minute+90*time.Second))
	incoming.ProtectCurrentSlot, incoming.HoldingPriorityExchange = true, true
	incoming.STARFamily, incoming.HoldingQueueID, incoming.ArrivalQueueTime = "TESPI", "HOLD-B", &arriving
	incoming.PromotionNotBefore = &incoming.OperationalTETA
	input := sequence.Input{Revision: 7, Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{lead, incoming, held}}
	bindQueueRevision(&input)
	result, _, err := sequence.GenerateWithVacancyPromotions(input, nil, start)
	require.NoError(t, err)
	require.False(t, result.HasConflicts())
	require.Equal(t, incoming.OperationalTETA, entryFor(t, result, incoming.Callsign).Time)
	require.Contains(t, result.Operations, sequence.Operation{Callsign: incoming.Callsign, Kind: "gap_fill"},
		"inbound must pass the ordered gap-fill search rather than bypass holding priority during routine compaction")
	require.Equal(t, held.CurrentSlot.Time, entryFor(t, result, held.Callsign).Time)
	require.Equal(t, lead.CurrentSlot.Time, entryFor(t, result, lead.Callsign).Time)
}
