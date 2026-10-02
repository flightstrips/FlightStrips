package cluster

import (
	"context"
	"fmt"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

const maxCommandAttempts = 4

// CommandRouter is only started by the explicit NATS runtime. Transport loss
// is ambiguous. A successful owner reply carries its verified committed outcome;
// the forwarding replica need not wait for its independent local replay.
type CommandRouter struct {
	NC         *nats.Conn
	Projection *Projection
	Lease      *OwnerRuntime
	Writer     Writer
}

func (r *CommandRouter) Serve(ctx context.Context) error {
	if r == nil || r.NC == nil || r.Projection == nil || r.Lease == nil || r.Writer.Store == nil ||
		r.Writer.NodeID != r.Lease.NodeID || !canonicalUUID(r.Lease.NodeID) {
		return fmt.Errorf("invalid command router")
	}
	_, closeSub, err := SubscribeJoined(r.NC, "fs.v1.command."+r.Lease.NodeID, func(msg *nats.Msg) {
		commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if msg.Header.Get("FS-Durable") == "1" {
			commandCtx = context.WithValue(commandCtx, durableExecutionKey{}, true)
		}
		reply := r.handle(commandCtx, msg.Data)
		data, marshalErr := proto.Marshal(reply)
		if marshalErr == nil && len(data) <= MaxStateBytes && msg.Reply != "" {
			_ = r.NC.Publish(msg.Reply, data)
		}
	})
	if err != nil {
		return err
	}
	defer closeSub()
	if err := FlushSubscription(ctx, r.NC); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func (r *CommandRouter) handle(ctx context.Context, data []byte) *pb.CommandReply {
	bad := &pb.CommandReply{ProtocolRevision: 1, Status: pb.CommandReply_INVALID_ARGUMENT}
	if len(data) == 0 || len(data) > MaxStateBytes {
		return bad
	}
	request := &pb.CommandRequest{}
	if err := pb.UnmarshalStrict(data, request); err != nil {
		return bad
	}
	bad.CommandId = request.CommandId
	if _, err := RequestHash(request); err != nil {
		bad.Detail = err.Error()
		return bad
	}
	if r.NC.Status() != nats.CONNECTED || r.Projection.commandHealth() != nil {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_UNAVAILABLE}
	}
	state, err := r.Projection.committedCommandCheckpoint(request.Aggregate, request.CommandId)
	if err != nil {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_UNAVAILABLE}
	}
	if state.Owner == nil || state.Owner.NodeId != r.Lease.NodeID {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_NOT_OWNER, CurrentOwner: state.Owner}
	}
	if !r.Lease.CanCommitLocal(request.Aggregate) {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_UNAVAILABLE}
	}
	writer := r.Writer
	writer.Projection, writer.Lease = r.Projection, r.Lease
	if ctx.Value(durableExecutionKey{}) == true {
		reply, _ := writer.ExecuteFresh(ctx, request)
		return reply
	}
	return writer.Execute(ctx, request)
}

// RouteDurable is reserved for durable cross-aggregate prerequisites and
// external workflow claims. Ordinary session commands use the immediate RAM path.
func (r *CommandRouter) RouteDurable(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	return r.Route(context.WithValue(ctx, durableExecutionKey{}, true), request)
}

func (r *CommandRouter) Route(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	bad := &pb.CommandReply{ProtocolRevision: 1, Status: pb.CommandReply_INVALID_ARGUMENT}
	if request == nil {
		return bad
	}
	bad.CommandId = request.CommandId
	hash, err := RequestHash(request)
	if err != nil {
		bad.Detail = err.Error()
		return bad
	}
	data, err := proto.Marshal(request)
	if err != nil || len(data) > MaxStateBytes {
		bad.Detail = "invalid or oversized command"
		return bad
	}
	if r == nil || r.NC == nil || r.Projection == nil || r.Lease == nil || r.NC.Status() != nats.CONNECTED {
		return unavailable(request.CommandId)
	}
	subject, _ := Subject(request.Aggregate)
	var redirected string
	for attempt := 0; attempt < maxCommandAttempts && ctx.Err() == nil; attempt++ {
		if r.Projection.commandHealth() != nil {
			return unavailable(request.CommandId)
		}
		if outcome := r.routeOutcome(ctx, request, hash); outcome != nil {
			return outcome
		}
		state, err := r.Projection.committedCommandCheckpoint(request.Aggregate, request.CommandId)
		if err != nil {
			return unavailable(request.CommandId)
		}
		target := redirected
		if target == "" && state.Owner != nil {
			target = state.Owner.NodeId
		}
		if target == "" {
			return unavailable(request.CommandId)
		}
		var reply *pb.CommandReply
		if target == r.Lease.NodeID {
			reply = r.handle(ctx, data)
		} else {
			hop, cancel := context.WithTimeout(ctx, time.Second)
			message := nats.NewMsg("fs.v1.command." + target)
			message.Data = data
			if ctx.Value(durableExecutionKey{}) == true {
				message.Header.Set("FS-Durable", "1")
			}
			msg, reqErr := r.NC.RequestMsgWithContext(hop, message)
			cancel()
			if reqErr != nil {
				// The remote owner may have committed before its reply vanished.
				if outcome := r.routeOutcome(ctx, request, hash); outcome != nil {
					return outcome
				}
				continue
			}
			if len(msg.Data) == 0 || len(msg.Data) > MaxStateBytes {
				return unavailable(request.CommandId)
			}
			reply = &pb.CommandReply{}
			if pb.UnmarshalStrict(msg.Data, reply) != nil || reply.ProtocolRevision != 1 || reply.CommandId != request.CommandId {
				return unavailable(request.CommandId)
			}
		}
		switch reply.Status {
		case pb.CommandReply_COMMITTED, pb.CommandReply_PENDING:
			if ctx.Value(durableExecutionKey{}) == true && reply.MemoryAccepted {
				continue
			}
			if reply.MemoryAccepted && (reply.CurrentOwner == nil || reply.CurrentOwner.NodeId != target ||
				(state.Owner != nil && reply.CurrentOwner.Epoch < state.Owner.Epoch)) {
				return unavailable(request.CommandId)
			}
			if validCommittedReply(request, hash, reply) {
				return reply
			}
			return unavailable(request.CommandId)
		case pb.CommandReply_NOT_OWNER:
			redirected = ""
			if reply.CurrentOwner != nil && reply.CurrentOwner.NodeId != target {
				redirected = reply.CurrentOwner.NodeId
			}
			if redirected == "" {
				wait, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
				_ = r.Projection.WaitSubjectAdvance(wait, subject, state.SubjectSequence)
				cancel()
			}
		case pb.CommandReply_UNAVAILABLE:
			if outcome := r.routeOutcome(ctx, request, hash); outcome != nil {
				return outcome
			}
			redirected = ""
		default:
			return reply
		}
	}
	return unavailable(request.CommandId)
}

// Only the owner which reduced the stored event may report success. Transport
// replies must bind that outcome to the exact actor, request and checkpoint;
// a bare PubAck (including a stale-owner no-op) is insufficient.
func validCommittedReply(request *pb.CommandRequest, hash string, reply *pb.CommandReply) bool {
	if request == nil || reply == nil || reply.ProtocolRevision != 1 || reply.CommandId != request.CommandId {
		return false
	}
	outcome := reply.Outcome
	bound := outcome != nil && outcome.CommandId == request.CommandId && outcome.RequestSha256 == hash &&
		proto.Equal(outcome.Actor, request.Actor) && proto.Equal(outcome.Aggregate, request.Aggregate) &&
		reply.AggregateRevision != nil && reply.GetAggregateRevision() > 0 && reply.GetAggregateRevision() == outcome.AggregateRevision &&
		(outcome.Status == pb.CommandOutcome_SUCCEEDED || outcome.Status == pb.CommandOutcome_ACCEPTED || outcome.Status == pb.CommandOutcome_FAILED) &&
		reply.Status == statusForOutcome(outcome)
	if !bound {
		return false
	}
	if reply.MemoryAccepted {
		return request.Aggregate.GetSession() != nil && reply.StreamSequence == nil && outcome.CommittedStreamSequence == 0 &&
			reply.CurrentOwner != nil && canonicalUUID(reply.CurrentOwner.NodeId) && reply.CurrentOwner.Epoch > 0
	}
	return reply.StreamSequence != nil && reply.GetStreamSequence() > 0 && reply.GetStreamSequence() == outcome.CommittedStreamSequence
}

// A durable prerequisite may never complete from an unpersisted cached reply,
// including after a lost transport response or an unavailable owner.
func (r *CommandRouter) routeOutcome(ctx context.Context, request *pb.CommandRequest, hash string) *pb.CommandReply {
	reply := r.projectedOutcome(request, hash)
	if reply != nil && reply.MemoryAccepted && ctx.Value(durableExecutionKey{}) == true {
		return nil
	}
	return reply
}

func (r *CommandRouter) projectedOutcome(request *pb.CommandRequest, hash string) *pb.CommandReply {
	state, err := r.Projection.committedCommandCheckpoint(request.Aggregate, request.CommandId)
	if err != nil {
		return nil
	}
	old, lookupErr := state.LookupOutcome(request.CommandId)
	if lookupErr != nil {
		return unavailable(request.CommandId)
	}
	if old == nil {
		return nil
	}
	if !proto.Equal(old.Actor, request.Actor) {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_UNAUTHORIZED}
	}
	if old.RequestSha256 != hash {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_INVALID_ARGUMENT, Detail: "command ID has different content"}
	}
	reply := &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: statusForOutcome(old),
		AggregateRevision: &old.AggregateRevision, StreamSequence: &old.CommittedStreamSequence,
		Outcome: proto.Clone(old).(*pb.CommandOutcome)}
	if old.CommittedStreamSequence == 0 && r.Projection.Async != nil && request.Aggregate.GetSession() != nil {
		reply.MemoryAccepted, reply.StreamSequence, reply.CurrentOwner = true, nil, state.Owner
	}
	return reply
}

func unavailable(id string) *pb.CommandReply {
	return &pb.CommandReply{ProtocolRevision: 1, CommandId: id, Status: pb.CommandReply_UNAVAILABLE}
}
