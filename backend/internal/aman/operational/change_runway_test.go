package operational

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestChangeRunwayIsProtectedAuditedRevisionedAndDurablyIdempotent(t *testing.T) {
	service, state, now, alternate := changeRunwayFixture(t)
	flight := &state.Flights[0]
	flight.State, flight.FreezeReason = aman.StateStable, aman.FreezeSuperstable
	flight.Lifecycle = &aman.LifecycleState{EnteredAt: now, Reason: aman.LifecycleReasonStableHorizon, LastEventID: "stable", LastEventFingerprint: "test", LastEventAt: now}
	frozenAt, frozenTETA := now.Add(-time.Minute), flight.Prediction.OperationalTETA
	flight.FrozenAt, flight.FrozenOperationalTETA = &frozenAt, &frozenTETA
	frozenSlot := *flight.Slot
	flight.FrozenSlot = &frozenSlot
	repository := &memoryRepository{state: state, has: true}
	publisher := &recordingPublisher{}
	actions := recomputeActions(t, service, repository, publisher, now)
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKDK_FMP", ReceivedAt: now}
	command := aman.ChangeRunwayCommand{Metadata: aman.CommandMetadata{CommandID: "runway-1", ExpectedRevision: state.Revision}, Callsign: flight.Callsign, RunwayGroupID: alternate}

	first, err := actions.ChangeRunway(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, first.Changed)
	require.Equal(t, state.Revision+1, first.CurrentRevision)
	require.Equal(t, alternate, *repository.state.Flights[0].SelectedRunwayGroup)
	require.Equal(t, frozenSlot.Time, repository.state.Flights[0].Slot.Time)
	require.Equal(t, alternate, repository.state.Flights[0].FrozenSlot.RunwayGroupID)
	require.Len(t, publisher.states, 1)
	var audit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[0].AuditRecords[0].Payload, &audit))
	require.Equal(t, "ARRIVAL-22L", audit["before_runway_group_id"])
	require.Equal(t, string(alternate), audit["after_runway_group_id"])
	require.Contains(t, audit, "displacements")
	require.Equal(t, "1234567", audit["actor"])

	retry, err := actions.ChangeRunway(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	restarted := recomputeActions(t, service, repository, publisher, now)
	retry, err = restarted.ChangeRunway(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.Len(t, repository.commits, 1)

	stale := command
	stale.Metadata = aman.CommandMetadata{CommandID: "runway-stale", ExpectedRevision: state.Revision}
	result, err := restarted.ChangeRunway(context.Background(), auth, stale)
	requireDomainClass(t, err, aman.ErrorRevisionConflict)
	require.Equal(t, state.Revision+1, result.CurrentRevision)
}

func TestChangeRunwayRejectsInactiveIncompatibleAndProtectedConflictAtomically(t *testing.T) {
	for _, test := range []struct {
		name      string
		prepare   func(*Service, *aman.AirportState, aman.RunwayGroupID)
		requested aman.RunwayGroupID
		class     aman.ErrorClass
	}{
		{"inactive", func(_ *Service, _ *aman.AirportState, _ aman.RunwayGroupID) {}, "INACTIVE", aman.ErrorInvalidArgument},
		{"incompatible", func(service *Service, _ *aman.AirportState, alternate aman.RunwayGroupID) {
			service.deps.Terminal.Paths = service.deps.Terminal.Paths[:1]
		}, "ARRIVAL-04R", aman.ErrorInvalidArgument},
		{"protected conflict", func(_ *Service, state *aman.AirportState, alternate aman.RunwayGroupID) {
			target := &state.Flights[0]
			target.State = aman.StateStable
			target.Lifecycle = &aman.LifecycleState{EnteredAt: target.UpdatedAt, Reason: aman.LifecycleReasonStableHorizon, LastEventID: "stable", LastEventFingerprint: "test", LastEventAt: target.UpdatedAt}
			other := protectedOperationalFlight("flight-2", alternate, target.STARFamilyIdentity(), "L", target.Slot.Time, 1, aman.FreezeManual)
			state.Flights = append(state.Flights, other)
		}, "ARRIVAL-04R", aman.ErrorInvalidTransition},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, state, now, alternate := changeRunwayFixture(t)
			test.prepare(service, &state, alternate)
			repository := &memoryRepository{state: state, has: true}
			actions := recomputeActions(t, service, repository, &recordingPublisher{}, now)
			_, err := actions.ChangeRunway(context.Background(), aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKDK_FMP", ReceivedAt: now}, aman.ChangeRunwayCommand{Metadata: aman.CommandMetadata{CommandID: test.name, ExpectedRevision: state.Revision}, Callsign: "SAS123", RunwayGroupID: test.requested})
			requireDomainClass(t, err, test.class)
			require.Empty(t, repository.commits)
			require.Equal(t, state, repository.state)
		})
	}
}

func TestChangeRunwayPreservesEveryProtectedOrderingContract(t *testing.T) {
	manualOrder := 1
	for _, test := range []struct {
		name   string
		freeze aman.FreezeReason
		manual *int
	}{
		{"stable", aman.FreezeNone, nil},
		{"superstable", aman.FreezeSuperstable, nil},
		{"manual freeze", aman.FreezeManual, nil},
		{"TMA freeze", aman.FreezeTMA, nil},
		{"manual order", aman.FreezeNone, &manualOrder},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, state, now, alternate := changeRunwayFixture(t)
			flight := &state.Flights[0]
			flight.State = aman.StateStable
			flight.Lifecycle = &aman.LifecycleState{EnteredAt: now, Reason: aman.LifecycleReasonStableHorizon, LastEventID: "stable", LastEventFingerprint: "test", LastEventAt: now}
			flight.FreezeReason, flight.ManualOrder = test.freeze, test.manual
			if test.freeze != aman.FreezeNone {
				frozenAt, frozenTETA, frozenSlot := now.Add(-time.Minute), flight.Prediction.OperationalTETA, *flight.Slot
				flight.FrozenAt, flight.FrozenOperationalTETA, flight.FrozenSlot = &frozenAt, &frozenTETA, &frozenSlot
			}
			beforeTime, beforeOrder := flight.Slot.Time, flight.ManualOrder
			mutation, err := service.ChangeRunway(aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKDK_FMP", ReceivedAt: now}, aman.ChangeRunwayCommand{Callsign: flight.Callsign, RunwayGroupID: alternate})
			require.NoError(t, err)
			change, err := mutation(state)
			require.NoError(t, err)
			require.Equal(t, beforeTime, change.State.Flights[0].Slot.Time)
			require.Equal(t, beforeOrder, change.State.Flights[0].ManualOrder)
			require.Equal(t, alternate, change.State.Flights[0].Slot.RunwayGroupID)
		})
	}
}

func TestHoldingReservationSurvivesAutomaticAndExplicitRunwayChanges(t *testing.T) {
	for _, action := range []string{"active runways", "selected runway", "explicit flight runway"} {
		t.Run(action, func(t *testing.T) {
			service, state, now, alternate := changeRunwayFixture(t)
			originalGroup := *state.Flights[0].SelectedRunwayGroup
			state.RunwayGroups[0].Selected = true
			service.deps.Terminal.ActiveRunwayGroupSets = [][]aman.RunwayGroupID{{alternate}}
			flight := &state.Flights[0]
			flight.State = aman.StateUnstable
			flight.HoldingClearance = &aman.HoldingClearance{Hold: "TESPI", HoldType: aman.HoldingClearanceEnroute, ObservedAt: now}
			flight.Slot.Time = flight.Slot.Time.Add(6 * time.Minute)
			before := cloneGapState(t, state)
			auth := aman.CommandContext{Airport: "EKCH", Role: "EKDK_FMP", ReceivedAt: now}
			var mutation sequence.CommandMutation
			var err error
			switch action {
			case "active runways":
				mutation, err = service.SetActiveRunwayGroups(auth, aman.SetActiveRunwayGroupsCommand{RunwayGroupIDs: []aman.RunwayGroupID{alternate}})
			case "selected runway":
				mutation, err = service.SelectRunwayGroup(auth, aman.SelectRunwayGroupCommand{RunwayGroupID: alternate, EffectiveAt: now})
			case "explicit flight runway":
				mutation, err = service.ChangeRunway(auth, aman.ChangeRunwayCommand{Callsign: flight.Callsign, RunwayGroupID: alternate})
			}
			require.NoError(t, err)
			change, err := mutation(state)
			require.NoError(t, err)
			updated := change.State.Flights[0]
			require.Equal(t, before.Flights[0].Slot.Time, updated.Slot.Time)
			if action == "explicit flight runway" {
				require.Equal(t, alternate, *updated.SelectedRunwayGroup)
			} else {
				require.Equal(t, originalGroup, *updated.SelectedRunwayGroup)
				selected, ok := service.selectedGroup(updated, change.State.RunwayGroups)
				require.True(t, ok)
				require.Equal(t, originalGroup, selected, "later observations must retain the protected runway too")
			}
			require.True(t, updated.SequenceDisposition.Participates())
			require.Equal(t, before.Flights[0].HoldingClearance, updated.HoldingClearance)
		})
	}
}

func changeRunwayFixture(t *testing.T) (*Service, aman.AirportState, time.Time, aman.RunwayGroupID) {
	t.Helper()
	service, state, now := recomputeFlightFixture(t)
	alternate := aman.RunwayGroupID("ARRIVAL-04R")
	service.deps.Terminal.Paths = append(service.deps.Terminal.Paths, terminal.Path{Feeder: "TESPI", RunwayGroup: alternate})
	service.deps.Terminal.RunwayGroups = append(service.deps.Terminal.RunwayGroups, terminal.RunwayGroup{ID: alternate})
	effective := state.GeneratedAt
	state.RunwayGroups = append(state.RunwayGroups, aman.RunwayGroupPolicy{ID: alternate, ActiveRatePerHour: 20, RateEffectiveAt: &effective, RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: effective, ArrivalsPerHour: 20}}})
	state.ActiveRunwayGroups = []aman.RunwayGroupID{*state.Flights[0].SelectedRunwayGroup, alternate}
	return service, state, now, alternate
}

var _ sequence.ActionMutations = (*Service)(nil)
