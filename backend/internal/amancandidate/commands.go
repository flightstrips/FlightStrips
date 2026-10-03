package amancandidate

import (
	"context"
	"fmt"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/operational"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (w *Worker) LoadAirportState(ctx context.Context, airport string) (aman.AirportState, error) {
	board, err := w.options.State.Read(ctx, airport)
	if err != nil {
		return aman.AirportState{}, err
	}
	if board.Airport == nil {
		return aman.AirportState{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "airport not initialized"}
	}
	return decodeBoard(board)
}
func (w *Worker) ActiveGeometrySnapshot(ctx context.Context, airport navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	n, err := loadNavigation(ctx, w.options.Source, w.options.RouteWorker, w.options.RouteResolver, string(airport))
	if err != nil {
		return navdata.ActiveGeometrySnapshot{}, err
	}
	return n.ActiveGeometrySnapshot(ctx, airport)
}
func (w *Worker) ActiveVersion(ctx context.Context, airport navdata.AirportID) (navdata.DatasetVersion, error) {
	v, err := w.ActiveGeometrySnapshot(ctx, airport)
	return v.Manifest.Version, err
}
func (w *Worker) Route(ctx context.Context, key navdata.RouteKey) (navdata.RouteGeometry, error) {
	n, err := loadNavigation(ctx, w.options.Source, w.options.RouteWorker, w.options.RouteResolver, string(w.options.Terminal.Airport))
	if err != nil {
		return navdata.RouteGeometry{}, err
	}
	return n.Route(ctx, key)
}
func (w *Worker) TerminalPath(ctx context.Context, airport navdata.AirportID, feeder navdata.FeederID, group aman.RunwayGroupID) (navdata.TerminalPath, error) {
	n, err := loadNavigation(ctx, w.options.Source, w.options.RouteWorker, w.options.RouteResolver, string(airport))
	if err != nil {
		return navdata.TerminalPath{}, err
	}
	return n.TerminalPath(ctx, airport, feeder, group)
}
func (w *Worker) ReconcileVatsim(ctx context.Context, airport string) error {
	return (cluster.VatsimAirportReconciler{Worker: w.candidate, EvaluatePresent: w.EvaluateObservation, EvaluateMissing: w.EvaluateObservation}).Reconcile(ctx, airport)
}

// commandCapture is a detached evaluator for the production coordinator. The
// outer Writer owns idempotency/CAS and publishes its typed result atomically.
type commandCapture struct{ evaluationRepository }

func (*commandCapture) LoadCommandOutcome(context.Context, string) (aman.CommandOutcome, error) {
	return aman.CommandOutcome{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "new command"}
}
func (r *commandCapture) Commit(ctx context.Context, c aman.StateCommit) (aman.CommitResult, error) {
	result, err := r.evaluationRepository.Commit(ctx, c)
	result.CommandOutcome = c.CommandOutcome
	return result, err
}

func (w *Worker) PlanCommand(ctx context.Context, request *pb.CommandRequest, state *cluster.Aggregate, role string) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	action := request.GetClient().GetAman()
	airport := request.GetAggregate().GetAirport().GetIcao()
	if action == nil || airport != string(w.options.Terminal.Airport) {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid AMAN airport/action")
	}
	board, err := cluster.ReadAmanBoard(state)
	if err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, err
	}
	current := board.Airport.GetRevision()
	if action.ExpectedAirportRevision != current {
		return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("stale AMAN revision")
	}
	if board.Airport == nil || w.options.Mode != aman.ModeAuthoritative || !board.Airport.Authoritative || !board.Airport.GetHealth().GetReady() {
		return nil, pb.CommandReply_UNAVAILABLE, current, fmt.Errorf("AMAN is not authoritative and ready")
	}
	if action.GetSubmitCoordinationRequest() != nil || action.GetAcceptCoordinationRequest() != nil || action.GetRejectCoordinationRequest() != nil {
		return w.planCoordination(ctx, request, state, board, role)
	}
	if !aman.IsFMPRole(role) {
		return nil, pb.CommandReply_UNAUTHORIZED, current, fmt.Errorf("AMAN command requires FMP")
	}
	initial, err := decodeBoard(board)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	at := w.options.Now().UTC()
	nav, err := loadNavigation(ctx, w.options.Source, w.options.RouteWorker, w.options.RouteResolver, airport)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	inputs, err := w.acceptedInputs(ctx, airport, board, at, nil)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	repo := &commandCapture{evaluationRepository: evaluationRepository{initial: initial, exists: true}}
	service, err := operational.New(operational.Dependencies{Repository: repo, Materializer: nav, Geometry: nav, Wind: w.windReader(airport, at), Runways: inputs, AircraftEngines: w.options.AircraftEngines, Terminal: nav.config, TMAVolume: nav.tma, Airports: []string{airport}, Mode: w.options.Mode, SourceMode: w.options.SourceMode, Publisher: evaluationPublisher{}, Now: func() time.Time { return at }})
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{States: repo, Outcomes: repo, Committer: repo, Publisher: evaluationPublisher{}, Now: func() time.Time { return at }})
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	actions, err := sequence.NewActionService(coordinator, service)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	auth := aman.CommandContext{Airport: airport, Actor: request.Actor.Id, Role: role, ReceivedAt: at}
	metadata := aman.CommandMetadata{CommandID: request.CommandId, ExpectedRevision: aman.SequenceRevision(current)}
	if err = executeAction(ctx, actions, auth, metadata, action); err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
	}
	if repo.commit == nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, fmt.Errorf("AMAN policy did not produce a commit")
	}
	transition := encodeBoard(repo.commit.State, aman.TechnicalHealth{})
	transition.Airport.Health = proto.Clone(board.Airport.Health).(*pb.AmanTechnicalHealth)
	// Preserve accepted observations/projections that are outside policy state.
	transition.Airport.ConfiguredMode = board.Airport.ConfiguredMode
	transition.Airport.TimelineMappings = board.Airport.TimelineMappings
	if err := refreshDisplay(transition.Airport, repo.commit.State, board.Airport); err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	for _, f := range transition.Flights {
		for _, prior := range board.Flights {
			if f.Callsign == prior.Callsign {
				f.SourceObservations = prior.SourceObservations
				f.HoldingEatProjection = prior.HoldingEatProjection
			}
		}
	}
	for i, record := range repo.commit.AuditRecords {
		audit, e := convertAudit(record, request.CommandId, i, request.Actor)
		if e != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, e
		}
		transition.Audits = append(transition.Audits, audit)
	}
	if uint64(repo.commit.State.Revision) == current {
		change := &pb.DomainChange{}
		for _, audit := range transition.Audits {
			change.Changes = append(change.Changes, &pb.EntityChange{Key: audit.Id, Revision: 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_AmanAudit{AmanAudit: audit}}}})
		}
		return change, pb.CommandReply_COMMITTED, 0, nil
	}
	transition.Workflows, err = w.holdingIntents(ctx, request.CommandId, repo.commit.State, aman.TechnicalHealth{Ready: board.Airport.Health.Ready}, nav.snapshot, inputs.sessions)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	copy := proto.Clone(request).(*pb.CommandRequest)
	copy.ExpectedEntityRevision = &current
	return cluster.PlanAmanTransition(copy, state, transition)
}

func commandTime(v *timestamppb.Timestamp) time.Time {
	if v == nil || v.CheckValid() != nil {
		return time.Time{}
	}
	return v.AsTime().UTC()
}
func commandTimePointer(v *timestamppb.Timestamp) *time.Time {
	if v == nil {
		return nil
	}
	t := commandTime(v)
	return &t
}
func executeAction(ctx context.Context, actions *sequence.ActionService, auth aman.CommandContext, metadata aman.CommandMetadata, action *pb.AmanAction) error {
	var err error
	switch v := action.Change.(type) {
	case *pb.AmanAction_LockFlight:
		_, err = actions.LockFlight(ctx, auth, aman.LockFlightCommand{Metadata: metadata, Callsign: aman.Callsign(v.LockFlight.Callsign)})
	case *pb.AmanAction_UnlockFlight:
		_, err = actions.UnlockFlight(ctx, auth, aman.UnlockFlightCommand{Metadata: metadata, Callsign: aman.Callsign(v.UnlockFlight.Callsign)})
	case *pb.AmanAction_DesequenceFlight:
		_, err = actions.DesequenceFlight(ctx, auth, aman.DesequenceFlightCommand{Metadata: metadata, Callsign: aman.Callsign(v.DesequenceFlight.Callsign)})
	case *pb.AmanAction_ResumeFlight:
		_, err = actions.ResumeFlight(ctx, auth, aman.ResumeFlightCommand{Metadata: metadata, Callsign: aman.Callsign(v.ResumeFlight.Callsign)})
	case *pb.AmanAction_RemoveFlight:
		_, err = actions.RemoveFlight(ctx, auth, aman.RemoveFlightCommand{Metadata: metadata, Callsign: aman.Callsign(v.RemoveFlight.Callsign)})
	case *pb.AmanAction_AcceptTeta:
		_, err = actions.AcceptTETA(ctx, auth, aman.AcceptTETACommand{Metadata: metadata, Callsign: aman.Callsign(v.AcceptTeta.Callsign)})
	case *pb.AmanAction_KeepFplEta:
		_, err = actions.KeepFPLETA(ctx, auth, aman.KeepFPLETACommand{Metadata: metadata, Callsign: aman.Callsign(v.KeepFplEta.Callsign)})
	case *pb.AmanAction_ResetTetaOverride:
		_, err = actions.ResetTETAOverride(ctx, auth, aman.ResetTETAOverrideCommand{Metadata: metadata, Callsign: aman.Callsign(v.ResetTetaOverride.Callsign)})
	case *pb.AmanAction_ResetManualFeederEta:
		_, err = actions.ResetManualFeederETA(ctx, auth, aman.ResetManualFeederETACommand{Metadata: metadata, Callsign: aman.Callsign(v.ResetManualFeederEta.Callsign)})
	case *pb.AmanAction_RecomputeFlight:
		_, err = actions.RecomputeFlight(ctx, auth, aman.RecomputeFlightCommand{Metadata: metadata, Callsign: aman.Callsign(v.RecomputeFlight.Callsign)})
	case *pb.AmanAction_MoveFlight:
		_, err = actions.MoveFlight(ctx, auth, aman.MoveFlightCommand{Metadata: metadata, BeforeCallsign: callsignPointer(v.MoveFlight.GetBeforeCallsign()), AfterCallsign: callsignPointer(v.MoveFlight.GetAfterCallsign()), Callsign: aman.Callsign(v.MoveFlight.Callsign), RunwayGroupID: aman.RunwayGroupID(v.MoveFlight.RunwayGroupId)})
	case *pb.AmanAction_PlaceFlightAtTime:
		_, err = actions.PlaceFlightAtTime(ctx, auth, aman.PlaceFlightAtTimeCommand{Metadata: metadata, Callsign: aman.Callsign(v.PlaceFlightAtTime.Callsign), RunwayGroupID: aman.RunwayGroupID(v.PlaceFlightAtTime.RunwayGroupId), SlotTime: commandTime(v.PlaceFlightAtTime.SlotTime), AllowGap: v.PlaceFlightAtTime.AllowGap})
	case *pb.AmanAction_SetRate:
		_, err = actions.SetRate(ctx, auth, aman.SetRateCommand{Metadata: metadata, RunwayGroupID: aman.RunwayGroupID(v.SetRate.RunwayGroupId), ArrivalsPerHour: v.SetRate.ArrivalsPerHour, EffectiveAt: commandTime(v.SetRate.EffectiveAt)})
	case *pb.AmanAction_SelectRunwayGroup:
		_, err = actions.SelectRunwayGroup(ctx, auth, aman.SelectRunwayGroupCommand{Metadata: metadata, RunwayGroupID: aman.RunwayGroupID(v.SelectRunwayGroup.RunwayGroupId), EffectiveAt: commandTime(v.SelectRunwayGroup.EffectiveAt)})
	case *pb.AmanAction_SetActiveRunwayGroups:
		_, err = actions.SetActiveRunwayGroups(ctx, auth, aman.SetActiveRunwayGroupsCommand{Metadata: metadata, RunwayGroupIDs: runwayIDs(v.SetActiveRunwayGroups.RunwayGroupIds)})
	case *pb.AmanAction_SetManualEta:
		_, err = actions.SetManualETA(ctx, auth, aman.SetManualETACommand{Metadata: metadata, Callsign: aman.Callsign(v.SetManualEta.Callsign), ManualETA: commandTime(v.SetManualEta.Value)})
	case *pb.AmanAction_SetManualFeederEta:
		_, err = actions.SetManualFeederETA(ctx, auth, aman.SetManualFeederETACommand{Metadata: metadata, Callsign: aman.Callsign(v.SetManualFeederEta.Callsign), FeederETA: commandTime(v.SetManualFeederEta.Value)})
	case *pb.AmanAction_ChangeRunway:
		_, err = actions.ChangeRunway(ctx, auth, aman.ChangeRunwayCommand{Metadata: metadata, Callsign: aman.Callsign(v.ChangeRunway.Callsign), RunwayGroupID: aman.RunwayGroupID(v.ChangeRunway.RunwayGroupId)})
	case *pb.AmanAction_ReportGoAround:
		_, err = actions.ReportGoAround(ctx, auth, aman.ReportGoAroundCommand{Metadata: metadata, Callsign: aman.Callsign(v.ReportGoAround.Callsign), DetectedAt: commandTime(v.ReportGoAround.DetectedAt)})
	case *pb.AmanAction_ConfirmGoAround:
		_, err = actions.ConfirmGoAround(ctx, auth, aman.ConfirmGoAroundCommand{Metadata: metadata, Callsign: aman.Callsign(v.ConfirmGoAround.Callsign), EpisodeID: v.ConfirmGoAround.EpisodeId})
	case *pb.AmanAction_RejectGoAround:
		_, err = actions.RejectGoAround(ctx, auth, aman.RejectGoAroundCommand{Metadata: metadata, Callsign: aman.Callsign(v.RejectGoAround.Callsign), EpisodeID: v.RejectGoAround.EpisodeId})
	case *pb.AmanAction_CreateGap:
		_, err = actions.CreateRunwayGap(ctx, auth, aman.CreateRunwayGapCommand{Metadata: metadata, RunwayGroupID: aman.RunwayGroupID(v.CreateGap.RunwayGroupId), Interval: gapInterval(v.CreateGap), Label: v.CreateGap.Label})
	case *pb.AmanAction_RemoveGap:
		_, err = actions.RemoveRunwayGap(ctx, auth, aman.RemoveRunwayGapCommand{Metadata: metadata, RunwayGroupID: aman.RunwayGroupID(v.RemoveGap.RunwayGroupId), GapID: aman.RunwayGapID(v.RemoveGap.GapId)})
	case *pb.AmanAction_CreateRunwayClosure:
		_, err = actions.CreateRunwayClosure(ctx, auth, aman.CreateRunwayClosureCommand{Metadata: metadata, Interval: closureInterval(v.CreateRunwayClosure), Reason: v.CreateRunwayClosure.Reason})
	case *pb.AmanAction_RemoveRunwayClosure:
		_, err = actions.RemoveRunwayClosure(ctx, auth, aman.RemoveRunwayClosureCommand{Metadata: metadata, RunwayGroupID: aman.RunwayGroupID(v.RemoveRunwayClosure.RunwayGroupId), ClosureID: aman.RunwayClosureID(v.RemoveRunwayClosure.ClosureId), Reason: v.RemoveRunwayClosure.Reason})
	case *pb.AmanAction_CreateCapacityReservation:
		_, err = actions.CreateCapacityReservation(ctx, auth, aman.CreateCapacityReservationCommand{Metadata: metadata, RunwayGroupID: aman.RunwayGroupID(v.CreateCapacityReservation.RunwayGroupId), AfterCallsign: aman.Callsign(v.CreateCapacityReservation.AfterCallsign), Label: v.CreateCapacityReservation.GetLabel(), Reason: v.CreateCapacityReservation.Reason})
	case *pb.AmanAction_RemoveCapacityReservation:
		_, err = actions.RemoveCapacityReservation(ctx, auth, aman.RemoveCapacityReservationCommand{Metadata: metadata, RunwayGroupID: aman.RunwayGroupID(v.RemoveCapacityReservation.RunwayGroupId), ReservationID: aman.RunwayCapacityReservationID(v.RemoveCapacityReservation.ReservationId), Reason: v.RemoveCapacityReservation.Reason})
	default:
		return fmt.Errorf("unsupported AMAN action %T", action.Change)
	}
	return err
}
func callsignPointer(value string) *aman.Callsign {
	if value == "" {
		return nil
	}
	v := aman.Callsign(value)
	return &v
}
func runwayIDs(values []string) []aman.RunwayGroupID {
	out := make([]aman.RunwayGroupID, len(values))
	for i, v := range values {
		out[i] = aman.RunwayGroupID(v)
	}
	return out
}
func gapInterval(v *pb.AmanCreateGap) aman.RunwayGapIntervalInput {
	result := aman.RunwayGapIntervalInput{Start: commandTime(v.Start)}
	switch e := v.Extent.(type) {
	case *pb.AmanCreateGap_End:
		result.End = commandTimePointer(e.End)
	case *pb.AmanCreateGap_SlotCount:
		result.SlotCount = &e.SlotCount
	}
	return result
}
func closureInterval(v *pb.AmanCreateClosure) aman.RunwayClosureIntervalInput {
	result := aman.RunwayClosureIntervalInput{RunwayGroupID: aman.RunwayGroupID(v.RunwayGroupId), End: commandTimePointer(v.End)}
	switch p := v.Placement.(type) {
	case *pb.AmanCreateClosure_Start:
		result.Start = commandTimePointer(p.Start)
	case *pb.AmanCreateClosure_AfterCallsign:
		result.AfterCallsign = callsignPointer(p.AfterCallsign)
	}
	return result
}
