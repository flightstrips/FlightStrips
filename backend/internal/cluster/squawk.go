package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"FlightStrips/pkg/helpers"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const squawkInterval = 5 * time.Second

// SquawkPlanner is dormant until bound to the candidate session owner.
// WAITING effects are the queue; CID is selected once, at admission.
type SquawkPlanner struct {
	Projection *Projection
	Next       Planner
}

func ValidAssignedSquawk(code string) bool {
	return helpers.IsValidAssignedSquawk(code)
}

func (p SquawkPlanner) Plan(ctx context.Context, req *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	auto := req.GetSystem().GetRequestSquawk()
	manual := req.GetClient().GetStrip()
	if auto == nil && manual.GetGenerateSquawk() == nil {
		if p.Next == nil {
			return workerReject(pb.CommandReply_UNAVAILABLE, 0, "squawk planner unavailable")
		}
		return p.Next(ctx, req, state)
	}
	if _, err := stripSession(req, state); err != nil {
		return workerReject(pb.CommandReply_NOT_FOUND, 0, err.Error())
	}
	id := req.Aggregate.GetSession().Id
	key, cid := "", ""
	if auto != nil {
		if req.GetActor().GetKind() != pb.Actor_SYSTEM || req.GetActor().GetId() != "euroscope-squawk" {
			return workerReject(pb.CommandReply_UNAUTHORIZED, 0, "automatic squawk adapter required")
		}
		key, cid = auto.Callsign, auto.TargetCid
	} else {
		key, cid = manual.Callsign, req.GetActor().GetId()
		controller := state.Indexes[pb.EntityKind_CONTROLLER][cid].GetValue().GetController()
		if req.GetActor().GetKind() != pb.Actor_CONTROLLER || req.GetActor().GetSessionId() != id || controller == nil || controller.Observer {
			return workerReject(pb.CommandReply_UNAUTHORIZED, 0, "active controller required")
		}
	}
	if key != strings.ToUpper(strings.TrimSpace(key)) || !canonicalAircraft(key) {
		return workerReject(pb.CommandReply_INVALID_ARGUMENT, 0, "invalid squawk callsign")
	}
	strip := state.Indexes[pb.EntityKind_STRIP][key]
	if strip == nil {
		return workerReject(pb.CommandReply_NOT_FOUND, 0, "strip not found")
	}
	if manual.GetGenerateSquawk() != nil {
		if req.ExpectedEntityRevision == nil || *req.ExpectedEntityRevision != strip.Revision {
			return workerReject(pb.CommandReply_REVISION_CONFLICT, strip.Revision, "stale strip revision")
		}
		s := strip.Value.GetStrip()
		if s.OwnerCid != "" && s.OwnerCid != cid {
			return workerReject(pb.CommandReply_UNAUTHORIZED, strip.Revision, "strip owned by another controller")
		}
		if s.Validation.GetActive() {
			return workerReject(pb.CommandReply_INVALID_ARGUMENT, strip.Revision, "strip is locked by validation")
		}
	}
	for _, e := range state.Effects {
		if e.GetGenerateSquawk().GetCallsign() == key && e.Status == pb.EffectRecord_WAITING {
			return workerReject(pb.CommandReply_INVALID_ARGUMENT, strip.Revision, "SQUAWK_ALREADY_PENDING")
		}
	}
	if ValidAssignedSquawk(strip.Value.GetStrip().AssignedSquawk) {
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, strip.Revision, nil
	}
	client, err := selectLiveEffectTarget(p.Projection, id, cid)
	if err != nil || client == nil || client.Observer {
		return workerReject(pb.CommandReply_UNAVAILABLE, strip.Revision, "operational squawk target unavailable")
	}
	controller := state.Indexes[pb.EntityKind_CONTROLLER][cid].GetValue().GetController()
	if controller == nil || controller.Observer || controller.Callsign != client.Callsign || controller.Position != client.Position {
		return workerReject(pb.CommandReply_UNAVAILABLE, strip.Revision, "squawk target identity changed")
	}
	e := &pb.EffectRecord{CommandId: req.CommandId, TargetCid: cid, OwnerEpoch: state.ownerEpoch(), Status: pb.EffectRecord_WAITING,
		DispatchDeadline: timestamppb.New(time.Now().Add(effectDispatchWindow)), Payload: &pb.EffectRecord_GenerateSquawk{GenerateSquawk: &pb.GenerateSquawkEffect{Callsign: key}}}
	return &pb.DomainChange{Effects: []*pb.EffectRecord{e}}, pb.CommandReply_COMMITTED, strip.Revision, nil
}

func squawkClaimAllowed(a *Aggregate, effect *pb.EffectRecord, at time.Time) error {
	if effect.GetGenerateSquawk() == nil {
		return nil
	}
	key := fmt.Sprint(a.Ref.GetSession().GetId())
	throttle := a.Indexes[pb.EntityKind_SESSION_SQUAWK_THROTTLE][key].GetValue().GetSessionSquawkThrottle()
	if throttle != nil && at.Before(throttle.NextAllowedAt.AsTime()) {
		return errEffectDeadlineRace
	}
	strip := a.Indexes[pb.EntityKind_STRIP][effect.GetGenerateSquawk().Callsign].GetValue().GetStrip()
	if strip == nil || ValidAssignedSquawk(strip.AssignedSquawk) {
		return errEffectDeadlineRace
	}
	// Only the first accepted squawk may claim. UUID breaks sequence ties.
	for id, other := range a.Effects {
		if other.GetGenerateSquawk() == nil || other.Status != pb.EffectRecord_WAITING || id == effect.CommandId {
			continue
		}
		left, right := a.Ledger[id], a.Ledger[effect.CommandId]
		if left == nil || right == nil {
			return fmt.Errorf("squawk queue outcome absent")
		}
		if left.CommittedStreamSequence < right.CommittedStreamSequence || left.CommittedStreamSequence == right.CommittedStreamSequence && id < effect.CommandId {
			return errEffectDeadlineRace
		}
	}
	return nil
}
