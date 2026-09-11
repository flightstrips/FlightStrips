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
	PlaceFlightAtTime(aman.CommandContext, aman.PlaceFlightAtTimeCommand) (CommandMutation, error)
	LockFlight(aman.CommandContext, aman.LockFlightCommand) (CommandMutation, error)
	UnlockFlight(aman.CommandContext, aman.UnlockFlightCommand) (CommandMutation, error)
	DesequenceFlight(aman.CommandContext, aman.DesequenceFlightCommand) (CommandMutation, error)
	ResumeFlight(aman.CommandContext, aman.ResumeFlightCommand) (CommandMutation, error)
	RemoveFlight(aman.CommandContext, aman.RemoveFlightCommand) (CommandMutation, error)
	SetRate(aman.CommandContext, aman.SetRateCommand) (CommandMutation, error)
	SelectRunwayGroup(aman.CommandContext, aman.SelectRunwayGroupCommand) (CommandMutation, error)
	SetActiveRunwayGroups(aman.CommandContext, aman.SetActiveRunwayGroupsCommand) (CommandMutation, error)
	CreateRunwayGap(aman.CommandContext, aman.CreateRunwayGapCommand) (CommandMutation, error)
	RemoveRunwayGap(aman.CommandContext, aman.RemoveRunwayGapCommand) (CommandMutation, error)
	AcceptTETA(aman.CommandContext, aman.AcceptTETACommand) (CommandMutation, error)
	KeepFPLETA(aman.CommandContext, aman.KeepFPLETACommand) (CommandMutation, error)
	SetManualETA(aman.CommandContext, aman.SetManualETACommand) (CommandMutation, error)
	ResetTETAOverride(aman.CommandContext, aman.ResetTETAOverrideCommand) (CommandMutation, error)
	SetManualFeederETA(aman.CommandContext, aman.SetManualFeederETACommand) (CommandMutation, error)
	ResetManualFeederETA(aman.CommandContext, aman.ResetManualFeederETACommand) (CommandMutation, error)
	RecomputeFlight(context.Context, aman.CommandContext, aman.RecomputeFlightCommand) (CommandMutation, error)
	ChangeRunway(aman.CommandContext, aman.ChangeRunwayCommand) (CommandMutation, error)
	ReportGoAround(aman.CommandContext, aman.ReportGoAroundCommand) (CommandMutation, error)
	ConfirmGoAround(aman.CommandContext, aman.ConfirmGoAroundCommand) (CommandMutation, error)
	RejectGoAround(aman.CommandContext, aman.RejectGoAroundCommand) (CommandMutation, error)
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

func (s *ActionService) PlaceFlightAtTime(ctx context.Context, auth aman.CommandContext, command aman.PlaceFlightAtTimeCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, func() error { return command.Validate(auth.ReceivedAt) }, func() (CommandMutation, error) {
		return s.mutations.PlaceFlightAtTime(auth, command)
	})
}

func (s *ActionService) LockFlight(ctx context.Context, auth aman.CommandContext, command aman.LockFlightCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.LockFlight(auth, command) })
}

func (s *ActionService) UnlockFlight(ctx context.Context, auth aman.CommandContext, command aman.UnlockFlightCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.UnlockFlight(auth, command) })
}

func (s *ActionService) DesequenceFlight(ctx context.Context, auth aman.CommandContext, command aman.DesequenceFlightCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.DesequenceFlight(auth, command) })
}

func (s *ActionService) ResumeFlight(ctx context.Context, auth aman.CommandContext, command aman.ResumeFlightCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.ResumeFlight(auth, command) })
}

func (s *ActionService) RemoveFlight(ctx context.Context, auth aman.CommandContext, command aman.RemoveFlightCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.RemoveFlight(auth, command) })
}

func (s *ActionService) SetRate(ctx context.Context, auth aman.CommandContext, command aman.SetRateCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.SetRate(auth, command) })
}

func (s *ActionService) SelectRunwayGroup(ctx context.Context, auth aman.CommandContext, command aman.SelectRunwayGroupCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.SelectRunwayGroup(auth, command) })
}

func (s *ActionService) SetActiveRunwayGroups(ctx context.Context, auth aman.CommandContext, command aman.SetActiveRunwayGroupsCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) {
		return s.mutations.SetActiveRunwayGroups(auth, command)
	})
}

func (s *ActionService) CreateRunwayGap(ctx context.Context, auth aman.CommandContext, command aman.CreateRunwayGapCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) {
		return s.mutations.CreateRunwayGap(auth, command)
	})
}

func (s *ActionService) RemoveRunwayGap(ctx context.Context, auth aman.CommandContext, command aman.RemoveRunwayGapCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) {
		return s.mutations.RemoveRunwayGap(auth, command)
	})
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

func (s *ActionService) SetManualFeederETA(ctx context.Context, auth aman.CommandContext, command aman.SetManualFeederETACommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.SetManualFeederETA(auth, command) })
}

func (s *ActionService) ResetManualFeederETA(ctx context.Context, auth aman.CommandContext, command aman.ResetManualFeederETACommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.ResetManualFeederETA(auth, command) })
}

func (s *ActionService) RecomputeFlight(ctx context.Context, auth aman.CommandContext, command aman.RecomputeFlightCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) {
		return s.mutations.RecomputeFlight(ctx, auth, command)
	})
}

func (s *ActionService) ChangeRunway(ctx context.Context, auth aman.CommandContext, command aman.ChangeRunwayCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.ChangeRunway(auth, command) })
}

func (s *ActionService) ReportGoAround(ctx context.Context, auth aman.CommandContext, command aman.ReportGoAroundCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, func() error { return command.Validate(auth.ReceivedAt) }, func() (CommandMutation, error) { return s.mutations.ReportGoAround(auth, command) })
}

func (s *ActionService) ConfirmGoAround(ctx context.Context, auth aman.CommandContext, command aman.ConfirmGoAroundCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.ConfirmGoAround(auth, command) })
}

func (s *ActionService) RejectGoAround(ctx context.Context, auth aman.CommandContext, command aman.RejectGoAroundCommand) (aman.CommandExecution, error) {
	return executeTyped(s, ctx, auth, command.Metadata, command.Validate, func() (CommandMutation, error) { return s.mutations.RejectGoAround(auth, command) })
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
