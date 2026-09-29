package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PrivateMessagePlanner is composed ahead of the session planner in the
// candidate owner. A recipient is a pilot callsign in the browser wire action;
// the immutable effect target is the authenticated controller's EuroScope CID.
type PrivateMessagePlanner struct {
	Secrets    EffectSecrets
	Projection *Projection
	Fallback   Planner
}

func (p PrivateMessagePlanner) Plan(ctx context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	message := request.GetClient().GetMessage().GetPrivateMessage()
	if message == nil {
		if p.Fallback == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("message planner unavailable")
		}
		return p.Fallback(ctx, request, state)
	}
	if request.GetAggregate().GetSession() == nil || request.Actor == nil || request.Actor.Kind != pb.Actor_CONTROLLER ||
		request.Actor.GetSessionId() != request.Aggregate.GetSession().Id || request.ExpectedEntityRevision != nil ||
		request.Actor.Id == "" || state.Indexes[pb.EntityKind_CONTROLLER][request.Actor.Id] == nil {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("controller session identity required")
	}
	recipient := strings.ToUpper(strings.TrimSpace(message.TargetCid))
	if recipient == "" || recipient != message.TargetCid || strings.TrimSpace(message.Text) == "" || len(message.Text) > 4096 {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid private message")
	}
	secret, err := p.Secrets.StagePrivateMessage(request.CommandId, request.Actor.Id, recipient, message.Text)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, err
	}
	effect := &pb.EffectRecord{CommandId: request.CommandId, TargetCid: request.Actor.Id,
		OwnerEpoch: state.ownerEpoch(), Status: pb.EffectRecord_WAITING,
		Payload:          &pb.EffectRecord_PrivateMessage{PrivateMessage: secret},
		DispatchDeadline: timestamppb.New(time.Now().UTC().Add(effectDispatchWindow))}
	if p.Projection != nil {
		client, err := selectLiveEffectTarget(p.Projection, request.Aggregate.GetSession().Id, request.Actor.Id)
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, 0, err
		}
		if client != nil {
			effect.TargetConnectionId = &client.ConnectionId
		}
	}
	return &pb.DomainChange{Effects: []*pb.EffectRecord{effect}}, pb.CommandReply_COMMITTED, 0, nil
}
