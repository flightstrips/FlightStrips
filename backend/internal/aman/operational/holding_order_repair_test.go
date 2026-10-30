package operational

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/holdingclearance"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestIndependentHoldingFillsSTARSpacingGapWithoutMissingEATs(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		for _, reversed := range []bool{false, true} {
			t.Run(fmt.Sprintf("overtake=%t/reversed=%t", enabled, reversed), func(t *testing.T) {
				now := time.Date(2026, 10, 19, 18, 0, 0, 0, time.UTC)
				group := aman.RunwayGroupID("ARRIVAL-04L")
				makeFlight := func(callsign, family, hold string, age int, raw, entry, target time.Duration, order int) aman.AMANFlight {
					flight := operationalFlight(callsign, group, family, "M", now.Add(target))
					flight.State, flight.DataStatus = aman.StateStable, aman.DataFresh
					flight.SelectedSTARFamily, flight.SelectedHolding = &family, &hold
					flight.LatestObservation.Destination = "EKCH"
					flight.HoldingClearance = &aman.HoldingClearance{Hold: hold, HoldType: aman.HoldingClearanceEnroute, ObservedAt: now}
					flight.HoldingStack = &aman.HoldingStackState{HoldingID: hold, FirstObservedAt: now.Add(-time.Duration(age) * time.Minute), CandidateObservedAt: now, Confirmed: true}
					fixETA := now.Add(entry)
					flight.Prediction.RawTETA, flight.Prediction.HoldingFixETA, flight.Prediction.GeneratedAt = now.Add(raw), &fixETA, now
					flight.Slot = &aman.Slot{Time: now.Add(target), RunwayGroupID: group, Sequence: order, Revision: 7, Reason: "rate_wtc"}
					return flight
				}
				lead := makeFlight("LEAD", "MONAK", "HOLD-A", 20, 10*time.Minute, 0, 20*time.Minute, 1)
				lead.FreezeReason, lead.FrozenSlot = aman.FreezeManual, lead.Slot
				tail := makeFlight("TAIL", "MONAK", "HOLD-A", 10, 10*time.Minute, 0, 26*time.Minute, 2)
				filler := makeFlight("FILLER", "TESPI", "HOLD-B", 5, 23*time.Minute-time.Second, 6*time.Minute-time.Second, 33*time.Minute, 3)
				filler.Prediction.OperationalTETA = now.Add(23 * time.Minute)
				service := &Service{deps: Dependencies{Terminal: terminal.Configuration{
					RunwayGroups:    []terminal.RunwayGroup{{ID: group}},
					ApproachRelease: &aman.ApproachReleasePolicy{Enabled: enabled, TargetBuffer: 10, TaperStart: 6, MaxAdvanceSeconds: 600, LookaheadSeconds: 120},
				}}}
				state := aman.AirportState{Airport: "EKCH", Revision: 7, GeneratedAt: now, Flights: []aman.AMANFlight{lead, tail, filler},
					RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &now,
						SameSTARSpacing: &aman.SameSTARSpacingPolicy{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}}}}
				if reversed {
					slices.Reverse(state.Flights)
				}

				service.resequence(&state, now)

				byID := make(map[aman.Callsign]aman.AMANFlight)
				for _, flight := range state.Flights {
					byID[flight.Callsign] = flight
					require.NotNil(t, flight.Prediction.HoldingPlan)
					require.Nil(t, flight.Prediction.HoldingPlanBlockedBy)
				}
				require.Equal(t, lead.Slot.Time, byID["LEAD"].Slot.Time)
				require.Equal(t, tail.Slot.Time, byID["TAIL"].Slot.Time)
				if enabled {
					require.Equal(t, now.Add(23*time.Minute), byID["FILLER"].Slot.Time, "the independent hold must retain the legal gap between same-STAR arrivals")
					require.Equal(t, now.Add(6*time.Minute), byID["FILLER"].Prediction.HoldingPlan.ApproachReleaseTime, "a longer transit may require an earlier EAT from an independent hold")
				} else {
					require.True(t, byID["FILLER"].Slot.Time.After(tail.Slot.Time), "disabled overtaking must retain cross-hold precedence")
				}
				model := holdingclearance.BuildReadModel(state)
				require.Len(t, model.Entries, 3)
				require.Empty(t, model.Warnings)
				for _, entry := range model.Entries {
					require.NotNil(t, entry.EAT, "gap filling must keep every holding EAT visible")
				}
				candidate, err := sequence.Generate(service.sequenceInput(state))
				require.NoError(t, err)
				require.False(t, candidate.HasConflicts(), "gap filling must still satisfy rate, wake and same-STAR spacing")
			})
		}
	}
}

func TestAutomaticHoldingOrderRepairSurvivesCaptureAndRestart(t *testing.T) {
	for _, freeze := range []aman.FreezeReason{aman.FreezeNone, aman.FreezeSuperstable, aman.FreezeTMA} {
		t.Run(string(freeze), func(t *testing.T) {
			service, state, now := holdingOrderRepairFixture(freeze)
			service.resequence(&state, now)
			leader, follower := state.Flights[0], state.Flights[1]
			require.Equal(t, now.Add(20*time.Minute), leader.Slot.Time)
			require.Equal(t, now.Add(28*time.Minute), follower.Slot.Time)
			require.Equal(t, now.Add(11*time.Minute), follower.Prediction.HoldingPlan.ApproachReleaseTime)
			require.Nil(t, follower.Prediction.HoldingPlanBlockedBy)
			candidate, err := sequence.Generate(service.sequenceInput(state))
			require.NoError(t, err)
			require.False(t, candidate.HasConflicts(), "repair must still satisfy runway rate and wake spacing")
			require.Equal(t, freeze, follower.FreezeReason)
			if freeze != aman.FreezeNone {
				require.Equal(t, follower.Slot.Time, follower.FrozenSlot.Time)
				require.Equal(t, follower.Slot.Time, *follower.FrozenOperationalTETA)
			}
			model := holdingclearance.BuildReadModel(state)
			require.Len(t, model.Entries, 2)
			require.Empty(t, model.Warnings)
			for _, entry := range model.Entries {
				require.NotNil(t, entry.EAT)
			}
			encoded, err := json.Marshal(state)
			require.NoError(t, err)
			var restored aman.AirportState
			require.NoError(t, json.Unmarshal(encoded, &restored))
			service.resequence(&restored, now.Add(time.Minute))
			require.Equal(t, follower.Slot.Time, restored.Flights[1].Slot.Time)
			require.Equal(t, follower.Prediction.HoldingPlan, restored.Flights[1].Prediction.HoldingPlan)
			require.Nil(t, restored.Flights[1].Prediction.HoldingPlanBlockedBy)
		})
	}
}

func TestHoldingOrderConflictKeepsManualEAT(t *testing.T) {
	service, state, now := holdingOrderRepairFixture(aman.FreezeManual)
	service.resequence(&state, now)
	follower := state.Flights[1]
	require.Equal(t, now.Add(24*time.Minute), follower.Slot.Time)
	require.Equal(t, now.Add(7*time.Minute), follower.Prediction.HoldingPlan.ApproachReleaseTime)
	require.Equal(t, state.Flights[0].Callsign, *follower.Prediction.HoldingPlanBlockedBy)
	model := holdingclearance.BuildReadModel(state)
	require.Len(t, model.Warnings, 1)
	for _, entry := range model.Entries {
		require.NotNil(t, entry.EAT, "manual constraints must never erase the calculated EAT")
	}
}

func holdingOrderRepairFixture(freeze aman.FreezeReason) (*Service, aman.AirportState, time.Time) {
	now := time.Date(2026, 10, 19, 18, 43, 0, 0, time.UTC)
	group, hold := aman.RunwayGroupID("ARRIVAL-04L"), "ROSBI-HOLD"
	flights := make([]aman.AMANFlight, 2)
	for i, callsign := range []aman.Callsign{"SAS1877", "BAW52Y"} {
		entry := now.Add(time.Duration(-10+i*5) * time.Minute)
		flight := operationalFlight(callsign, group, "TESPI", "M", now.Add(time.Duration(i*12)*time.Minute))
		flight.State, flight.SelectedHolding = aman.StateStable, &hold
		flight.LatestObservation.Destination = "EKCH"
		flight.HoldingClearance = &aman.HoldingClearance{Hold: "ROSBI", HoldType: aman.HoldingClearanceEnroute, ObservedAt: entry}
		flight.HoldingStack = &aman.HoldingStackState{HoldingID: hold, FirstObservedAt: entry, CandidateObservedAt: now, Confirmed: true}
		flight.Prediction.RawTETA, flight.Prediction.HoldingFixETA = now.Add(time.Duration(i*12)*time.Minute), &entry
		flight.Slot = &aman.Slot{Time: now.Add(time.Duration(20+i*4) * time.Minute), RunwayGroupID: group, Sequence: i + 1, Revision: 7, Reason: "rate_wtc"}
		if i == 0 {
			flight.FreezeReason = aman.FreezeManual
			captured := *flight.Slot
			flight.FrozenSlot = &captured
		} else {
			flight.FreezeReason = freeze
			if freeze != aman.FreezeNone {
				captured, target, at := *flight.Slot, flight.Prediction.OperationalTETA, now.Add(-time.Minute)
				flight.FrozenSlot, flight.FrozenOperationalTETA, flight.FrozenAt = &captured, &target, &at
			}
		}
		flights[i] = flight
	}
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	state := aman.AirportState{Airport: "EKCH", Revision: 7, GeneratedAt: now, Flights: flights,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &now}}}
	return service, state, now
}
