package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PlanCdm handles the pilot HTTP TOBT edit as one typed session transition.
// The external CDM/plugin update remains a pending effect until acknowledged.
func PlanCdm(_ context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if request.GetAggregate().GetSession() == nil || request.GetClient().GetCdm() == nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("CDM session command required")
	}
	action := request.GetClient().GetCdm()
	key := strings.ToUpper(strings.TrimSpace(action.Callsign))
	session, err := stripSession(request, state)
	if err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	stripEntity := state.Indexes[pb.EntityKind_STRIP][key]
	if stripEntity == nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("strip not found")
	}
	if request.Actor.GetKind() != pb.Actor_PILOT || request.Actor.GetId() == "" || request.Actor.GetSessionId() != request.Aggregate.GetSession().Id {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("pilot session required")
	}
	if stripEntity.GetValue().GetStrip().Departure != session.GetValue().GetSession().Airport {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("TOBT is only available for departures")
	}
	old := state.Indexes[pb.EntityKind_CDM_STATE][key]
	current, status, err := candidateRevision(request, old)
	if err != nil {
		return nil, status, current, err
	}
	set := action.GetSetTobt()
	if set == nil || set.Value != nil && set.HhmmUtc != "" {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("one TOBT value required")
	}
	value := set.Value
	if set.HhmmUtc != "" {
		clock, parseErr := time.Parse("1504", set.HhmmUtc)
		if parseErr != nil || clock.Format("1504") != set.HhmmUtc {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("valid TOBT HHMM required")
		}
		date := time.Now().UTC()
		if eobt := stripEntity.GetValue().GetStrip().Eobt; eobt != nil {
			date = eobt.AsTime().UTC()
		}
		value = timestamppb.New(time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, time.UTC))
	}
	if value == nil || value.CheckValid() != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("valid TOBT timestamp required")
	}
	master := session.GetValue().GetSession().GetMaster()
	if master == nil || master.Cid == "" {
		return nil, pb.CommandReply_UNAVAILABLE, current, fmt.Errorf("CDM master is unavailable")
	}
	strip := proto.Clone(stripEntity.GetValue().GetStrip()).(*pb.Strip)
	strip.Revision = stripEntity.Revision + 1
	strip.Tobt = value
	strip.TobtSetBy = &request.Actor.Id
	cdm := &pb.CdmState{Callsign: key}
	if old != nil {
		cdm = proto.Clone(old.GetValue().GetCdmState()).(*pb.CdmState)
	}
	cdm.Tobt = value
	change := &pb.DomainChange{Changes: []*pb.EntityChange{
		candidateUpsert(key, old, &pb.EntityRecord{Value: &pb.EntityRecord_CdmState{CdmState: cdm}}),
		candidateUpsert(key, stripEntity, &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}),
	}, Effects: []*pb.EffectRecord{{CommandId: request.CommandId, TargetCid: master.Cid, TargetConnectionId: &master.ConnectionId, OwnerEpoch: state.ownerEpoch(), MasterEpoch: master.Epoch, Status: pb.EffectRecord_WAITING, Payload: &pb.EffectRecord_Cdm{Cdm: &pb.CdmEffect{Callsign: key, Action: "SET_TOBT", Value: value.AsTime().UTC().Format("1504")}}, DispatchDeadline: timestamppb.New(time.Now().UTC().Add(30 * time.Second))}}}
	sortCandidateChanges(change.Changes)
	return change, pb.CommandReply_COMMITTED, current, nil
}
