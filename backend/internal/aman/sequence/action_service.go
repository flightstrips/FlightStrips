package sequence

import (
	"context"
	"fmt"

	"FlightStrips/internal/aman"
)

// ActionMutations maps each validated typed command to its matching pure
// domain mutation. The owning state engine supplies the policy/config inputs;
// this service retains coordinator ownership of command IDs and revisions.
type ActionMutations interface {
	MoveFlight(aman.CommandContext, aman.MoveFlightCommand) (CommandMutation, error)
	LockFlight(aman.CommandContext, aman.LockFlightCommand) (CommandMutation, error)
	UnlockFlight(aman.CommandContext, aman.UnlockFlightCommand) (CommandMutation, error)
	SetRate(aman.CommandContext, aman.SetRateCommand) (CommandMutation, error)
	AcceptTETA(aman.CommandContext, aman.AcceptTETACommand) (CommandMutation, error)
	KeepFPLETA(aman.CommandContext, aman.KeepFPLETACommand) (CommandMutation, error)
	SetManualETA(aman.CommandContext, aman.SetManualETACommand) (CommandMutation, error)
	ResetTETAOverride(aman.CommandContext, aman.ResetTETAOverrideCommand) (CommandMutation, error)
	ReportGoAround(aman.CommandContext, aman.ReportGoAroundCommand) (CommandMutation, error)
}

type commandCoordinator interface {
	CurrentState(context.Context, string) (aman.AirportState, error)
	ExecuteCommand(context.Context, string, aman.CommandMetadata, CommandMutation) (CommandResult, error)
}

// ActionService is the command-facing sequence component. It performs common
// validation, then routes every operation through Coordinator.ExecuteCommand;
// it never assigns a revision itself.
type ActionService struct {
	coordinator commandCoordinator
	mutations   ActionMutations
}

func NewActionService(coordinator *Coordinator, mutations ActionMutations) (*ActionService, error) {
	if coordinator == nil {
		return nil, fmt.Errorf("AMAN action service requires sequence coordinator")
	}
	if isNilDependency(mutations) {
		return nil, fmt.Errorf("AMAN action service requires typed mutations")
	}
	return &ActionService{coordinator: coordinator, mutations: mutations}, nil
}

func (*ActionService) Name() string { return "AMAN typed action service" }

func (s *ActionService) CurrentRevision(ctx context.Context, airport string) (aman.SequenceRevision, error) {
	state, err := s.coordinator.CurrentState(ctx, airport)
	return state.Revision, err
}

func (s *ActionService) MoveFlight(ctx context.Context, auth aman.CommandContext, command aman.MoveFlightCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.MoveFlight(auth, command) })
}

func (s *ActionService) LockFlight(ctx context.Context, auth aman.CommandContext, command aman.LockFlightCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.LockFlight(auth, command) })
}

func (s *ActionService) UnlockFlight(ctx context.Context, auth aman.CommandContext, command aman.UnlockFlightCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.UnlockFlight(auth, command) })
}

func (s *ActionService) SetRate(ctx context.Context, auth aman.CommandContext, command aman.SetRateCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.SetRate(auth, command) })
}

func (s *ActionService) AcceptTETA(ctx context.Context, auth aman.CommandContext, command aman.AcceptTETACommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.AcceptTETA(auth, command) })
}

func (s *ActionService) KeepFPLETA(ctx context.Context, auth aman.CommandContext, command aman.KeepFPLETACommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.KeepFPLETA(auth, command) })
}

func (s *ActionService) SetManualETA(ctx context.Context, auth aman.CommandContext, command aman.SetManualETACommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, func() error { return command.Validate(auth.ReceivedAt) }, func() (CommandMutation, error) { return s.mutations.SetManualETA(auth, command) })
}

func (s *ActionService) ResetTETAOverride(ctx context.Context, auth aman.CommandContext, command aman.ResetTETAOverrideCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.ResetTETAOverride(auth, command) })
}

func (s *ActionService) ReportGoAround(ctx context.Context, auth aman.CommandContext, command aman.ReportGoAroundCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, func() error { return command.Validate(auth.ReceivedAt) }, func() (CommandMutation, error) { return s.mutations.ReportGoAround(auth, command) })
}

func executeTyped(service *ActionService, ctx context.Context, auth aman.CommandContext, metadata aman.CommandMetadata, validate func() error, build func() (CommandMutation, error)) (aman.CommandExecution, error) {
	if err := auth.Validate(); err != nil {
		return aman.CommandExecution{}, err
	}
	if err := validate(); err != nil {
		return aman.CommandExecution{}, err
	}
	mutation, err := build()
	if err != nil {
		return aman.CommandExecution{}, err
	}
	result, err := service.coordinator.ExecuteCommand(ctx, auth.Airport, metadata, mutation)
	execution := aman.CommandExecution{
		CurrentRevision: result.State.Revision,
		Outcome:         result.Outcome,
		Changed:         result.Changed,
		Duplicate:       result.Duplicate,
	}
	return execution, err
}

var _ aman.CommandService = (*ActionService)(nil)
