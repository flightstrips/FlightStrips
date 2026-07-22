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
func (m *recordingActionMutations) LockFlight(auth aman.CommandContext, _ aman.LockFlightCommand) (CommandMutation, error) {
	return m.mutation("lock", auth)
}
func (m *recordingActionMutations) UnlockFlight(auth aman.CommandContext, _ aman.UnlockFlightCommand) (CommandMutation, error) {
	return m.mutation("unlock", auth)
}
func (m *recordingActionMutations) SetRate(auth aman.CommandContext, _ aman.SetRateCommand) (CommandMutation, error) {
	return m.mutation("rate", auth)
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
func (m *recordingActionMutations) ReportGoAround(auth aman.CommandContext, _ aman.ReportGoAroundCommand) (CommandMutation, error) {
	return m.mutation("go_around", auth)
}
