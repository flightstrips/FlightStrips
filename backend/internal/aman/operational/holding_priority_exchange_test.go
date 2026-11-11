package operational

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestNewInboundCannotJumpHoldsAfterRebuild(t *testing.T) {
	service, state, now := rebuildApproachStackFixture(t)
	mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now},
		aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "rebuild", ExpectedRevision: state.Revision}})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	got := change.State
	group, hold := *got.Flights[0].SelectedRunwayGroup, "OTHER-HOLD"
	incoming := operationalFlight("AUA460", group, "OTHER", "M", now.Add(12*time.Minute))
	incoming.DataStatus, incoming.SelectedHolding = aman.DataFresh, &hold
	entry := now.Add(6 * time.Minute)
	incoming.Prediction.RawTETA, incoming.Prediction.HoldingFixETA = incoming.Prediction.OperationalTETA, &entry
	incoming.Prediction.GeneratedAt, incoming.Prediction.InputObservedAt = now, now
	got.Flights = append(got.Flights, incoming)
	service.resequence(&got, now.Add(time.Second))
	for _, held := range got.Flights[:2] {
		require.NotNil(t, held.HoldingReleaseReservation)
		require.True(t, got.Flights[2].Slot.Time.After(held.Slot.Time),
			"approach-release policy must not let new inbound traffic jump rebuilt holding reservations")
	}
}

func TestHeldAircraftTakesReachableInboundSlot(t *testing.T) {
	for _, sameHold := range []bool{false, true} {
		for _, stable := range []bool{false, true} {
			for _, reversed := range []bool{false, true} {
				t.Run(fmt.Sprintf("sameHold=%t/stable=%t/reversed=%t", sameHold, stable, reversed), func(t *testing.T) {
					service, state, now := holdingPriorityExchangeFixture()
					if sameHold {
						state.Flights[0].SelectedHolding = state.Flights[1].SelectedHolding
					}
					if stable {
						state.Flights[0].State = aman.StateStable
					}
					if reversed {
						slices.Reverse(state.Flights)
					}
					service.resequence(&state, now)
					byID := map[aman.Callsign]aman.AMANFlight{}
					for _, f := range state.Flights {
						byID[f.Callsign] = f
					}
					require.False(t, byID["SAS891"].Slot.Time.After(now.Add(20*time.Minute)))
					require.False(t, byID["SAS891"].Slot.Time.Before(now.Add(10*time.Minute)))
					require.True(t, byID["AUA460"].Slot.Time.After(byID["SAS891"].Slot.Time))
					require.NotNil(t, byID["SAS891"].Prediction.HoldingPlan)
					require.Nil(t, byID["SAS891"].Prediction.HoldingPlanBlockedBy)
					firstSlots := []aman.Slot{*state.Flights[0].Slot, *state.Flights[1].Slot}
					service.resequence(&state, now.Add(time.Second))
					require.Equal(t, firstSlots, []aman.Slot{*state.Flights[0].Slot, *state.Flights[1].Slot})
				})
			}
		}
	}
}

func holdingPriorityExchangeFixture() (*Service, aman.AirportState, time.Time) {
	now := time.Date(2026, 11, 9, 19, 0, 0, 0, time.UTC)
	group, incomingHold, occupiedHold := aman.RunwayGroupID("ARRIVAL-04L"), "MONAK-HOLD", "ROSBI-HOLD"
	entry, incomingEntry := now.Add(-10*time.Minute), now.Add(10*time.Minute)
	incoming := operationalFlight("AUA460", group, "MONAK", "M", now.Add(20*time.Minute))
	incoming.DataStatus, incoming.SelectedHolding = aman.DataFresh, &incomingHold
	incoming.Prediction.RawTETA, incoming.Prediction.GeneratedAt, incoming.Prediction.HoldingFixETA = incoming.Prediction.OperationalTETA, now, &incomingEntry
	incoming.Slot = &aman.Slot{Time: now.Add(20 * time.Minute), RunwayGroupID: group, Sequence: 1, Revision: 7, Reason: "rate_wtc"}
	held := operationalFlight("SAS891", group, "TESPI", "M", now.Add(52*time.Minute))
	held.DataStatus, held.SelectedHolding = aman.DataFresh, &occupiedHold
	held.HoldingClearance = &aman.HoldingClearance{Hold: "ROSBI", HoldType: aman.HoldingClearanceEnroute, ObservedAt: entry}
	held.HoldingStack = &aman.HoldingStackState{HoldingID: occupiedHold, Confirmed: true, FirstObservedAt: entry, CandidateObservedAt: now}
	held.Prediction.RawTETA, held.Prediction.GeneratedAt, held.Prediction.HoldingFixETA = now.Add(10*time.Minute), now, &now
	held.Slot = &aman.Slot{Time: now.Add(52 * time.Minute), RunwayGroupID: group, Sequence: 2, Revision: 7, Reason: "stable"}
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}},
		ApproachRelease: &aman.ApproachReleasePolicy{Enabled: true}}}}
	state := aman.AirportState{Airport: "EKCH", Revision: 7, GeneratedAt: now, Flights: []aman.AMANFlight{incoming, held},
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &now}}}
	return service, state, now
}

func TestHoldingPriorityRetainsOperationalCommitments(t *testing.T) {
	for _, scenario := range []string{"manual held", "manual inbound", "manual order", "manual ETA", "superstable", "TMA freeze", "in approach", "past hold", "accepted release", "stale held", "stale inbound", "disconnected", "ended holding", "unconfirmed holding", "physical bound"} {
		t.Run(scenario, func(t *testing.T) {
			service, state, now := holdingPriorityExchangeFixture()
			incoming, held := &state.Flights[0], &state.Flights[1]
			incoming.State = aman.StateStable
			switch scenario {
			case "manual held":
				held.FreezeReason, held.FrozenSlot = aman.FreezeManual, held.Slot
			case "manual inbound":
				incoming.FreezeReason, incoming.FrozenSlot = aman.FreezeManual, incoming.Slot
			case "manual order":
				order := 1
				incoming.ManualOrder = &order
			case "manual ETA":
				incoming.Prediction.OperationalReason = aman.OperationalReasonManualOverride
			case "superstable":
				held.FreezeReason, held.FrozenSlot = aman.FreezeSuperstable, held.Slot
			case "TMA freeze":
				incoming.FreezeReason, incoming.FrozenSlot = aman.FreezeTMA, incoming.Slot
			case "in approach":
				incoming.TMAEntry = &aman.TMAEntryState{LastContainment: aman.TMAInside, LastObservedAt: now}
			case "past hold":
				incoming.Prediction.HoldingFixETA = nil
			case "accepted release":
				incoming.ApproachRelease = &aman.ApproachReleaseReservation{ReleaseAt: now, ReleasedAt: &now}
			case "stale held":
				held.DataStatus = aman.DataStale
			case "stale inbound":
				incoming.DataStatus = aman.DataStale
			case "disconnected":
				held.DataStatus = aman.DataDisconnected
			case "ended holding":
				held.HoldingClearanceCanceledAt = &now
			case "unconfirmed holding":
				held.HoldingStack.Confirmed = false
			case "physical bound":
				held.Prediction.RawTETA = now.Add(30 * time.Minute)
			}
			service.resequence(&state, now)
			require.True(t, incoming.Slot.Time.Before(held.Slot.Time))
		})
	}
}
