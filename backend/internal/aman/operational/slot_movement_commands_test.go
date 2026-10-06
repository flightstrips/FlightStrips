package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestSetRateMovesCommittedTargetsAndKeepsNewTimesOnRoutineUpdates(t *testing.T) {
	for _, test := range []struct {
		name                   string
		oldRate, newRate       uint32
		oldSpacing, newSpacing time.Duration
	}{
		{"reduced capacity", 20, 10, 3 * time.Minute, 6 * time.Minute},
		{"increased capacity", 10, 20, 6 * time.Minute, 3 * time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
			state := runwayGapDisplacementState(now)
			state.RunwayGroups[0].ActiveRatePerHour = test.oldRate
			state.Flights = []aman.AMANFlight{
				gapCommandFlight("LEAD", "north", now, 1, aman.StateStable, aman.FreezeNone),
				gapCommandFlight("STABLE", "north", now.Add(3*time.Minute), 2, aman.StateStable, aman.FreezeNone),
				gapCommandFlight("HOLDING", "north", now.Add(6*time.Minute), 3, aman.StateUnstable, aman.FreezeNone),
			}
			for index := range state.Flights {
				state.Flights[index].Slot.Time = now.Add(time.Duration(index) * test.oldSpacing)
			}
			state.Flights[2].HoldingClearance = &aman.HoldingClearance{Hold: "MONAK", HoldType: aman.HoldingClearanceEnroute, ObservedAt: now}
			hold := "MONAK-HOLD"
			state.Flights[2].SelectedHolding = &hold
			entered := now.Add(-5 * time.Minute)
			state.Flights[2].HoldingStack = &aman.HoldingStackState{HoldingID: hold, FirstObservedAt: entered, CandidateObservedAt: entered, ConsecutiveObservations: 2, Confirmed: true}
			state.Flights[2].Prediction.HoldingFixETA = &now
			state.Flights[2].Prediction.GeneratedAt, state.Flights[2].Prediction.InputObservedAt = now, now
			state.Flights[2].HoldingReleaseBasis = &aman.HoldingReleaseBasis{HoldingID: hold, Fix: "MONAK", RunwayGroupID: "north",
				RouteDigest: "holding-path", HoldingEntryTime: entered, PostHoldingTransit: time.Minute}
			repository := &memoryRepository{has: true, state: state}
			_, err := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second)).SetRate(context.Background(), closureAuth(now), aman.SetRateCommand{
				Metadata: aman.CommandMetadata{CommandID: "rate", ExpectedRevision: state.Revision}, RunwayGroupID: "north", ArrivalsPerHour: test.newRate, EffectiveAt: now,
			})
			require.NoError(t, err)
			for index, id := range []aman.Callsign{"LEAD", "STABLE", "HOLDING"} {
				require.Equal(t, now.Add(time.Duration(index)*test.newSpacing), stateFlight(t, repository.state, id).Slot.Time)
				require.Equal(t, index+1, stateFlight(t, repository.state, id).Slot.Sequence)
			}
			held := stateFlight(t, repository.state, "HOLDING")
			require.NotNil(t, held.Prediction.HoldingPlan)
			require.Equal(t, held.Slot.Time.Add(-time.Minute), held.Prediction.HoldingPlan.ApproachReleaseTime)
			committed := cloneGapState(t, repository.state)
			for index := range repository.state.Flights {
				repository.state.Flights[index].Prediction.OperationalTETA = now.Add(30 * time.Minute)
			}
			(&Service{}).resequence(&repository.state, now.Add(2*time.Second))
			for _, flight := range repository.state.Flights {
				require.Equal(t, stateFlight(t, committed, flight.Callsign).Slot, flight.Slot)
			}
			require.Equal(t, held.Prediction.HoldingPlan, stateFlight(t, repository.state, "HOLDING").Prediction.HoldingPlan)
		})
	}
}

func TestCapacityCommandsCanDisplaceUnstableHoldingReservation(t *testing.T) {
	for _, command := range []string{"closure", "capacity reservation"} {
		t.Run(command, func(t *testing.T) {
			now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
			state := runwayGapDisplacementState(now)
			state.RunwayGroups[0].ActiveRatePerHour = 20
			state.RunwayGroups[0].Selected = true
			state.ActiveRunwayGroups = []aman.RunwayGroupID{"north"}
			state.Flights = []aman.AMANFlight{
				gapCommandFlight("LEAD", "north", now, 1, aman.StateStable, aman.FreezeNone),
				gapCommandFlight("HOLDING", "north", now.Add(3*time.Minute), 2, aman.StateUnstable, aman.FreezeNone),
			}
			state.Flights[1].HoldingClearance = &aman.HoldingClearance{Hold: "MONAK", HoldType: aman.HoldingClearanceEnroute, ObservedAt: now}
			repository := &memoryRepository{has: true, state: state}
			actions := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second))
			metadata := aman.CommandMetadata{CommandID: "capacity", ExpectedRevision: state.Revision}
			var err error
			if command == "closure" {
				start, end := now.Add(3*time.Minute), now.Add(6*time.Minute)
				_, err = actions.CreateRunwayClosure(context.Background(), closureAuth(now), aman.CreateRunwayClosureCommand{
					Metadata: metadata, Interval: aman.RunwayClosureIntervalInput{RunwayGroupID: "north", Start: &start, End: &end}, Reason: "inspection",
				})
			} else {
				_, err = actions.CreateCapacityReservation(context.Background(), closureAuth(now), aman.CreateCapacityReservationCommand{
					Metadata: metadata, RunwayGroupID: "north", AfterCallsign: "LEAD", Reason: "priority",
				})
			}
			require.NoError(t, err)
			held := stateFlight(t, repository.state, "HOLDING")
			require.Equal(t, now.Add(6*time.Minute), held.Slot.Time)
			require.Equal(t, state.Flights[1].HoldingClearance, held.HoldingClearance)
			(&Service{}).resequence(&repository.state, now.Add(2*time.Second))
			require.Equal(t, held.Slot, stateFlight(t, repository.state, "HOLDING").Slot)
		})
	}
}

func TestManualETAAllocatesAndCapturesNewStableAndHoldingTarget(t *testing.T) {
	for _, holding := range []bool{false, true} {
		name := "stable"
		if holding {
			name = "holding"
		}
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
			state := runwayGapDisplacementState(now)
			state.RunwayGroups[0].ActiveRatePerHour = 20
			target := gapCommandFlight("TARGET", "north", now.Add(3*time.Minute), 1, aman.StateStable, aman.FreezeNone)
			target.UpdatedAt = now
			if holding {
				target.State = aman.StateUnstable
				target.HoldingClearance = &aman.HoldingClearance{Hold: "MONAK", HoldType: aman.HoldingClearanceEnroute, ObservedAt: now}
			}
			state.Flights = []aman.AMANFlight{target}
			repository := &memoryRepository{has: true, state: state}
			manual := now.Add(9 * time.Minute)
			_, err := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second)).SetManualETA(context.Background(), closureAuth(now), aman.SetManualETACommand{
				Metadata: aman.CommandMetadata{CommandID: "manual-eta", ExpectedRevision: state.Revision}, Callsign: "TARGET", ManualETA: manual,
			})
			require.NoError(t, err)
			updated := stateFlight(t, repository.state, "TARGET")
			require.Equal(t, manual, updated.Prediction.OperationalTETA)
			require.Equal(t, manual, updated.Slot.Time)
			require.NotNil(t, updated.FrozenSlot)
			require.Equal(t, updated.Slot.Time, updated.FrozenSlot.Time)
			require.Equal(t, updated.Slot.RunwayGroupID, updated.FrozenSlot.RunwayGroupID)
			(&Service{}).resequence(&repository.state, now.Add(2*time.Second))
			require.Equal(t, updated.Slot, stateFlight(t, repository.state, "TARGET").Slot)
		})
	}
}

func TestManualETAWithoutLegalCapacityRollsBack(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	state := runwayGapDisplacementState(now)
	state.RunwayGroups[0].ActiveRatePerHour = 20
	target := gapCommandFlight("TARGET", "north", now.Add(3*time.Minute), 1, aman.StateStable, aman.FreezeNone)
	target.UpdatedAt = now
	blocked := gapCommandFlight("BLOCKED", "north", now.Add(9*time.Minute), 2, aman.StateStable, aman.FreezeTMA)
	state.Flights = []aman.AMANFlight{target, blocked}
	repository := &memoryRepository{has: true, state: cloneGapState(t, state)}
	_, err := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second)).SetManualETA(context.Background(), closureAuth(now), aman.SetManualETACommand{
		Metadata: aman.CommandMetadata{CommandID: "impossible-eta", ExpectedRevision: state.Revision}, Callsign: "TARGET", ManualETA: now.Add(9 * time.Minute),
	})
	requireDomainErrorClass(t, err, aman.ErrorInvalidTransition)
	require.Empty(t, repository.commits)
	require.Equal(t, state, repository.state)
}

func TestPendingManualETAAllocatesOnceFlightBecomesSequenceEligible(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	state := runwayGapDisplacementState(now)
	target := gapCommandFlight("TARGET", "north", now.Add(3*time.Minute), 1, aman.StatePlanned, aman.FreezeNone)
	target.UpdatedAt, target.Slot = now, nil
	state.Flights = []aman.AMANFlight{target}
	repository := &memoryRepository{has: true, state: state}
	_, err := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second)).SetManualETA(context.Background(), closureAuth(now), aman.SetManualETACommand{
		Metadata: aman.CommandMetadata{CommandID: "planned-eta", ExpectedRevision: state.Revision}, Callsign: "TARGET", ManualETA: now.Add(9 * time.Minute),
	})
	require.NoError(t, err)
	require.Nil(t, repository.state.Flights[0].Slot)
	repository.state.Flights[0].State = aman.StateAirborne
	(&Service{}).resequence(&repository.state, now.Add(2*time.Second))
	require.NotNil(t, repository.state.Flights[0].FrozenSlot)
	require.Equal(t, now.Add(9*time.Minute), repository.state.Flights[0].Slot.Time)
	require.Equal(t, aman.FreezeManual, repository.state.Flights[0].FreezeReason)
}
