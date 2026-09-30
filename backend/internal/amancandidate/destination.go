package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/operational"
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"strings"
	"time"
)

func (w *Worker) holdingIntents(_ context.Context, id string, state aman.AirportState, health aman.TechnicalHealth, snapshot navdata.ActiveGeometrySnapshot, sessions []*cluster.Aggregate) ([]*pb.WorkflowRecord, error) {
	if !w.options.HoldingEATEnabled {
		return nil, nil
	}
	desired := operational.HoldingEATs(state, health, true, snapshot)
	var intents []*pb.WorkflowRecord
	for _, s := range sessions {
		for _, e := range s.EntitiesByKind(pb.EntityKind_STRIP) {
			strip := e.GetValue().GetStrip()
			if strip.Destination != state.Airport {
				continue
			}
			value, eligible := desired[strip.Callsign]
			if eligible {
				if strip.Hold != value.Hold || strip.HoldType != string(value.HoldType) || strip.HoldEat == value.HoldEAT {
					continue
				}
			} else if strip.AmanWrittenHoldingEat == nil {
				continue
			}
			workflowID, err := cluster.AmanIntentID(id, fmt.Sprintf("session/%d/%s", s.Ref.GetSession().Id, strip.Callsign))
			if err != nil {
				return nil, err
			}
			step := "holding-eat/" + strip.Callsign
			derived, err := cluster.AmanIntentID(workflowID, step)
			if err != nil {
				return nil, err
			}
			revision := uint64(state.Revision)
			intents = append(intents, &pb.WorkflowRecord{WorkflowId: workflowID, Source: airportRef(state.Airport), Destination: proto.Clone(s.Ref).(*pb.AggregateRef), Step: step, DerivedCommandId: derived, Status: pb.WorkflowRecord_PENDING, SourceRevision: &revision})
		}
	}
	return intents, nil
}

// Step is the concrete Task 20 builder. Its command contains the immutable
// source intent and AMAN action, never a stale complete session strip.
func (w *Worker) Step(intent *pb.WorkflowRecord) (*pb.CommandRequest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return w.BuildStep(ctx, intent)
}
func (w *Worker) BuildStep(ctx context.Context, intent *pb.WorkflowRecord) (*pb.CommandRequest, error) {
	if intent == nil || intent.Source.GetAirport() == nil || intent.Destination.GetSession() == nil || intent.SourceRevision == nil || !strings.HasPrefix(intent.Step, "holding-eat/") {
		return nil, fmt.Errorf("invalid AMAN session intent")
	}
	callsign := strings.TrimPrefix(intent.Step, "holding-eat/")
	if callsign == "" {
		return nil, fmt.Errorf("missing AMAN intent callsign")
	}
	source, err := (cluster.LocalLifecycleStore{Writer: w.options.State.Writer}).Read(ctx, intent.Source)
	if err != nil {
		return nil, err
	}
	accepted := source.Workflows[intent.WorkflowId]
	if accepted == nil || !proto.Equal(accepted, intent) {
		return nil, fmt.Errorf("intent differs from accepted airport workflow")
	}
	board, err := cluster.ReadAmanBoard(source)
	if err != nil {
		return nil, err
	}
	if board.Airport == nil || board.Airport.Revision != *intent.SourceRevision {
		return nil, fmt.Errorf("AMAN intent source superseded")
	}
	clearance := &pb.AmanHoldingClearance{ObservedAt: board.Airport.GeneratedAt}
	for _, f := range board.Flights {
		if f.Callsign == callsign && f.HoldingEatProjection != nil {
			clearance = proto.Clone(f.HoldingEatProjection).(*pb.AmanHoldingClearance)
		}
	}
	return &pb.CommandRequest{ProtocolRevision: 1, CommandId: intent.DerivedCommandId, Aggregate: proto.Clone(intent.Destination).(*pb.AggregateRef), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "aman-intent"},
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_ApplyAmanSession{ApplyAmanSession: &pb.ApplyAmanSession{Intent: proto.Clone(intent).(*pb.WorkflowRecord), Callsign: callsign, Action: &pb.ApplyAmanSession_HoldingEat{HoldingEat: clearance}}}}}}, nil
}

// DestinationPlanner is installed by Task 20 on the session command owner.
// Writer checks the durable ledger before this planner, so a committed result
// remains recoverable even after the source intent/revision changes.
func DestinationPlanner(source cluster.LifecycleStore) cluster.Planner {
	return func(ctx context.Context, request *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		action := request.GetSystem().GetApplyAmanSession()
		intent := action.GetIntent()
		if source == nil || intent == nil || request.Actor.GetKind() != pb.Actor_SYSTEM || request.Actor.Id != "aman-intent" || intent.SourceRevision == nil || !proto.Equal(request.Aggregate, intent.Destination) || request.CommandId != intent.DerivedCommandId || intent.Source.GetAirport() == nil || intent.Status != pb.WorkflowRecord_PENDING || action.GetHoldingEat() == nil || intent.Step != "holding-eat/"+action.Callsign {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid AMAN destination command")
		}
		id, err := cluster.AmanIntentID(intent.WorkflowId, intent.Step)
		if err != nil || id != request.CommandId {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("AMAN derived command identity mismatch")
		}
		airport, err := source.Read(ctx, intent.Source)
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, 0, err
		}
		accepted := airport.Workflows[intent.WorkflowId]
		board, err := cluster.ReadAmanBoard(airport)
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, 0, err
		}
		if accepted == nil || !proto.Equal(accepted, intent) || board.Airport == nil || board.Airport.Revision != *intent.SourceRevision {
			return nil, pb.CommandReply_REVISION_CONFLICT, 0, fmt.Errorf("AMAN source revision superseded")
		}
		derived := &pb.AmanHoldingClearance{ObservedAt: board.Airport.GeneratedAt}
		for _, f := range board.Flights {
			if f.Callsign == action.Callsign && f.HoldingEatProjection != nil {
				derived = f.HoldingEatProjection
			}
		}
		if !proto.Equal(derived, action.GetHoldingEat()) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("AMAN action differs from accepted policy output")
		}
		session := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(state.Ref.GetSession().Id)]
		if session == nil || session.GetValue().GetSession().Airport != intent.Source.GetAirport().Icao {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("AMAN destination airport mismatch")
		}
		old := state.Indexes[pb.EntityKind_STRIP][action.Callsign]
		if old == nil {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
		}
		strip := old.GetValue().GetStrip()
		want := action.GetHoldingEat()
		if strip.Destination != intent.Source.GetAirport().Icao {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
		}
		next := proto.Clone(strip).(*pb.Strip)
		if want.Hold == "" {
			marker := strip.AmanWrittenHoldingEat
			if marker == nil {
				return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
			}
			// Withdraw only the value AMAN wrote. A newer controller hold/EAT remains
			// intact, while the ownership marker is cleared durably.
			if strip.Hold == marker.Hold && strip.HoldType == marker.HoldType && strip.HoldEat == marker.HoldEat {
				next.HoldEat = ""
			}
			next.AmanWrittenHoldingEat = nil
		} else {
			if want.HoldType != "enroute" || !validEAT(want.HoldEat) {
				return nil, pb.CommandReply_INVALID_ARGUMENT, old.Revision, fmt.Errorf("invalid operational EAT")
			}
			if strip.Hold != want.Hold || strip.HoldType != want.HoldType {
				return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
			}
			if board.Airport.GetHealth().GetReady() == false || !board.Airport.Authoritative {
				return nil, pb.CommandReply_REVISION_CONFLICT, old.Revision, fmt.Errorf("AMAN authority gate blocked")
			}
			next.HoldEat = want.HoldEat
			next.AmanWrittenHoldingEat = proto.Clone(want).(*pb.AmanHoldingClearance)
		}
		if proto.Equal(next, strip) {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
		}
		// Reuse strip invariants and counter/version allocation on the session's
		// current state. The transport command hash remains the immutable action.
		patch := &pb.CommandRequest{ProtocolRevision: 1, CommandId: request.CommandId, Aggregate: request.Aggregate, Actor: request.Actor, ExpectedEntityRevision: &old.Revision, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: strip.Callsign, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: next}}}}}}}
		change, status, revision, err := cluster.PlanStrip(ctx, patch, state)
		if err != nil || change == nil || next.HoldEat == strip.HoldEat {
			return change, status, revision, err
		}
		target := strip.OwnerCid
		if target == "" && state.Master != nil {
			target = state.Master.Cid
		}
		if target == "" || state.Master == nil || state.Owner == nil {
			return nil, pb.CommandReply_UNAVAILABLE, old.Revision, fmt.Errorf("AMAN effect target unavailable")
		}
		change.Effects = []*pb.EffectRecord{{CommandId: request.CommandId, TargetCid: target,
			OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch, Status: pb.EffectRecord_WAITING,
			DispatchDeadline: timestamp(time.Now().UTC().Add(30 * time.Second)),
			Payload: &pb.EffectRecord_AmanHoldingEat{AmanHoldingEat: &pb.AmanHoldingEatEffect{Callsign: strip.Callsign,
				Hold: strip.Hold, HoldType: strip.HoldType, Eat: next.HoldEat, Airport: board.Airport.Airport, SourceRevision: *intent.SourceRevision}}}}
		return change, status, revision, nil
	}
}
func validEAT(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s[:2] < "24" && s[2:] < "60"
}
