package sequence

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestActionServiceRoutesTypedMetadataThroughCoordinator(t *testing.T) {
	coordinator := &recordingActionCoordinator{state: aman.AirportState{Airport: "EKCH", Revision: 7}}
	mutations := &recordingActionMutations{}
	service := &ActionService{coordinator: coordinator, mutations: mutations}
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)}
	before := aman.FlightID("flight-2")

	result, err := service.MoveFlight(context.Background(), auth, aman.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "move-1", ExpectedRevision: 7}, FlightID: "flight-1", RunwayGroupID: "A", BeforeFlightID: &before,
	})

	require.NoError(t, err)
	require.Equal(t, "move", mutations.called)
	require.Equal(t, auth, mutations.auth)
	require.Equal(t, "EKCH", coordinator.airport)
	require.Equal(t, aman.CommandMetadata{CommandID: "move-1", ExpectedRevision: 7}, coordinator.metadata)
	require.True(t, coordinator.mutationCalled)
	require.Equal(t, aman.SequenceRevision(8), result.CurrentRevision)
	require.True(t, result.Changed)
}

func TestActionServiceRejectsInvalidCommandBeforeMutationOrCoordinator(t *testing.T) {
	coordinator := &recordingActionCoordinator{}
	mutations := &recordingActionMutations{}
	service := &ActionService{coordinator: coordinator, mutations: mutations}
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)}

	_, err := service.LockFlight(context.Background(), auth, aman.LockFlightCommand{Metadata: aman.CommandMetadata{CommandID: "lock-1"}})

	require.Error(t, err)
	require.Empty(t, mutations.called)
	require.Empty(t, coordinator.airport)
}

func TestActionServiceCurrentRevisionUsesCoordinatorState(t *testing.T) {
	service := &ActionService{coordinator: &recordingActionCoordinator{state: aman.AirportState{Airport: "EKCH", Revision: 13}}, mutations: &recordingActionMutations{}}
	revision, err := service.CurrentRevision(context.Background(), "EKCH")
	require.NoError(t, err)
	require.Equal(t, aman.SequenceRevision(13), revision)
}

func TestSetActiveRunwayGroupsUsesOnlyServerCommandContext(t *testing.T) {
	coordinator := &recordingActionCoordinator{}
	mutations := &recordingActionMutations{}
	service := &ActionService{coordinator: coordinator, mutations: mutations}
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)}
	command := aman.SetActiveRunwayGroupsCommand{Metadata: aman.CommandMetadata{CommandID: "set-runways", ExpectedRevision: 7}, RunwayGroupIDs: []aman.RunwayGroupID{"A", "B"}}

	_, err := service.SetActiveRunwayGroups(context.Background(), auth, command)
	require.NoError(t, err)
	require.Equal(t, "active_runway_groups", mutations.called)
	require.Equal(t, auth, mutations.auth)
	require.Equal(t, auth.Airport, coordinator.airport)

	invalidContext := auth
	invalidContext.Role = ""
	_, err = service.SetActiveRunwayGroups(context.Background(), invalidContext, command)
	var domain *aman.DomainError
	require.ErrorAs(t, err, &domain)
	require.Equal(t, aman.ErrorInvalidArgument, domain.Class)
	require.Equal(t, auth, mutations.auth, "invalid server authority must not reach the mutation")
}

type recordingActionCoordinator struct {
	state          aman.AirportState
	airport        string
	metadata       aman.CommandMetadata
	mutationCalled bool
}

func (c *recordingActionCoordinator) CurrentState(_ context.Context, airport string) (aman.AirportState, error) {
	c.airport = airport
	return c.state, nil
}

func (c *recordingActionCoordinator) ExecuteCommand(_ context.Context, airport string, metadata aman.CommandMetadata, mutation CommandMutation) (CommandResult, error) {
	c.airport, c.metadata = airport, metadata
	state := aman.AirportState{Airport: airport, Revision: metadata.ExpectedRevision}
	_, err := mutation(state)
	c.mutationCalled = true
	return CommandResult{State: aman.AirportState{Airport: airport, Revision: metadata.ExpectedRevision + 1}, Changed: true}, err
}

type recordingActionMutations struct {
	called string
	auth   aman.CommandContext
}

func (m *recordingActionMutations) mutation(name string, auth aman.CommandContext) (CommandMutation, error) {
	m.called, m.auth = name, auth
	return func(state aman.AirportState) (CommandChange, error) {
		return CommandChange{State: state, Changed: true, Outcome: json.RawMessage(`{"status":"accepted"}`), Audit: []AuditEntry{{Category: "command", Payload: json.RawMessage(`{}`)}}}, nil
	}, nil
}

func (m *recordingActionMutations) MoveFlight(auth aman.CommandContext, _ aman.MoveFlightCommand) (CommandMutation, error) {
	return m.mutation("move", auth)
}
func (m *recordingActionMutations) PlaceFlightAtTime(auth aman.CommandContext, _ aman.PlaceFlightAtTimeCommand) (CommandMutation, error) {
	return m.mutation("place_at_time", auth)
}
func (m *recordingActionMutations) LockFlight(auth aman.CommandContext, _ aman.LockFlightCommand) (CommandMutation, error) {
	return m.mutation("lock", auth)
}
func (m *recordingActionMutations) UnlockFlight(auth aman.CommandContext, _ aman.UnlockFlightCommand) (CommandMutation, error) {
	return m.mutation("unlock", auth)
}
func (m *recordingActionMutations) DesequenceFlight(auth aman.CommandContext, _ aman.DesequenceFlightCommand) (CommandMutation, error) {
	return m.mutation("deselect", auth)
}
func (m *recordingActionMutations) ResumeFlight(auth aman.CommandContext, _ aman.ResumeFlightCommand) (CommandMutation, error) {
	return m.mutation("resume", auth)
}
func (m *recordingActionMutations) RemoveFlight(auth aman.CommandContext, _ aman.RemoveFlightCommand) (CommandMutation, error) {
	return m.mutation("remove", auth)
}
func (m *recordingActionMutations) SetRate(auth aman.CommandContext, _ aman.SetRateCommand) (CommandMutation, error) {
	return m.mutation("rate", auth)
}
func (m *recordingActionMutations) SelectRunwayGroup(auth aman.CommandContext, _ aman.SelectRunwayGroupCommand) (CommandMutation, error) {
	return m.mutation("runway_selection", auth)
}
func (m *recordingActionMutations) SetActiveRunwayGroups(auth aman.CommandContext, _ aman.SetActiveRunwayGroupsCommand) (CommandMutation, error) {
	return m.mutation("active_runway_groups", auth)
}
func (m *recordingActionMutations) CreateRunwayGap(auth aman.CommandContext, _ aman.CreateRunwayGapCommand) (CommandMutation, error) {
	return m.mutation("create_runway_gap", auth)
}
func (m *recordingActionMutations) RemoveRunwayGap(auth aman.CommandContext, _ aman.RemoveRunwayGapCommand) (CommandMutation, error) {
	return m.mutation("remove_runway_gap", auth)
}
func (m *recordingActionMutations) AcceptTETA(auth aman.CommandContext, _ aman.AcceptTETACommand) (CommandMutation, error) {
	return m.mutation("accept", auth)
}
func (m *recordingActionMutations) KeepFPLETA(auth aman.CommandContext, _ aman.KeepFPLETACommand) (CommandMutation, error) {
	return m.mutation("keep", auth)
}
func (m *recordingActionMutations) SetManualETA(auth aman.CommandContext, _ aman.SetManualETACommand) (CommandMutation, error) {
	return m.mutation("manual", auth)
}
func (m *recordingActionMutations) ResetTETAOverride(auth aman.CommandContext, _ aman.ResetTETAOverrideCommand) (CommandMutation, error) {
	return m.mutation("reset", auth)
}
func (m *recordingActionMutations) SetManualFeederETA(auth aman.CommandContext, _ aman.SetManualFeederETACommand) (CommandMutation, error) {
	return m.mutation("manual_feeder", auth)
}
func (m *recordingActionMutations) ResetManualFeederETA(auth aman.CommandContext, _ aman.ResetManualFeederETACommand) (CommandMutation, error) {
	return m.mutation("reset_manual_feeder", auth)
}
func (m *recordingActionMutations) RecomputeFlight(_ context.Context, auth aman.CommandContext, _ aman.RecomputeFlightCommand) (CommandMutation, error) {
	return m.mutation("recompute", auth)
}
func (m *recordingActionMutations) ChangeRunway(auth aman.CommandContext, _ aman.ChangeRunwayCommand) (CommandMutation, error) {
	return m.mutation("change_runway", auth)
}
func (m *recordingActionMutations) ReportGoAround(auth aman.CommandContext, _ aman.ReportGoAroundCommand) (CommandMutation, error) {
	return m.mutation("go_around", auth)
}
func (m *recordingActionMutations) ConfirmGoAround(auth aman.CommandContext, _ aman.ConfirmGoAroundCommand) (CommandMutation, error) {
	return m.mutation("confirm_go_around", auth)
}
func (m *recordingActionMutations) RejectGoAround(auth aman.CommandContext, _ aman.RejectGoAroundCommand) (CommandMutation, error) {
	return m.mutation("reject_go_around", auth)
}
