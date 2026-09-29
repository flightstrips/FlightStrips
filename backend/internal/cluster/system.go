package cluster

import (
	"context"
	"fmt"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// PlanSystemEntity is the narrow built-in planner for owner-validated typed
// replacement and deletion. Feature-specific transitions supply a Planner.
// Lease maintenance is deliberately outside the domain command ledger.
func PlanSystemEntity(_ context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	var key string
	var upsert *pb.EntityRecord
	var deletion *pb.DeleteEntity
	switch action := request.GetSystem().GetAction().(type) {
	case *pb.SystemCommand_UpdateEntity:
		if action.UpdateEntity == nil || action.UpdateEntity.Value == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("missing typed entity")
		}
		key = action.UpdateEntity.Key
		upsert = proto.Clone(action.UpdateEntity.Value).(*pb.EntityRecord)
	case *pb.SystemCommand_RemoveEntity:
		if action.RemoveEntity == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("missing deletion")
		}
		key = action.RemoveEntity.Key
		deletion = &pb.DeleteEntity{Kind: action.RemoveEntity.Kind}
	default:
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("system action requires a domain planner")
	}
	kind := pb.EntityKind(0)
	if upsert != nil {
		kind, _ = recordKind(upsert)
	} else if deletion != nil {
		kind = deletion.Kind
	}
	if kind == pb.EntityKind_STAND_ASSIGNMENT || kind == pb.EntityKind_STAND_BLOCK {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("stand records require the stand planner")
	}
	old := state.Indexes[kind][key]
	current := uint64(0)
	if old != nil {
		current = old.Revision
	}
	if request.ExpectedEntityRevision == nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("expected entity revision required")
	}
	if *request.ExpectedEntityRevision != current {
		return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("stale entity revision")
	}
	if old == nil {
		if deletion != nil {
			return nil, pb.CommandReply_NOT_FOUND, current, fmt.Errorf("entity not found")
		}
	}
	change := &pb.EntityChange{Key: key, Revision: current + 1}
	if upsert != nil {
		change.Operation = &pb.EntityChange_Upsert{Upsert: upsert}
	} else {
		change.Operation = &pb.EntityChange_Delete{Delete: deletion}
	}
	if err := validateChange(state.Ref, change, old); err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
	}
	return &pb.DomainChange{Changes: []*pb.EntityChange{change}}, pb.CommandReply_COMMITTED, current, nil
}
