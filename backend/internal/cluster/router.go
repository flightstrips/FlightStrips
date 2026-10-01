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
// is ambiguous: a reply is authoritative only after local projection replay.
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
	if r.NC.Status() != nats.CONNECTED || r.Projection.Ready() != nil {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_UNAVAILABLE}
	}
	state, err := r.Projection.Read(request.Aggregate)
	if err != nil {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_UNAVAILABLE}
	}
	if state.Owner == nil || state.Owner.NodeId != r.Lease.NodeID {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_NOT_OWNER, CurrentOwner: state.Owner}
	}
	if !r.Lease.CanWrite(request.Aggregate) {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_UNAVAILABLE}
	}
	writer := r.Writer
	writer.Projection, writer.Lease = r.Projection, r.Lease
	return writer.Execute(ctx, request)
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
		if r.Projection.Ready() != nil {
			return unavailable(request.CommandId)
		}
		if outcome := r.projectedOutcome(request, hash); outcome != nil {
			return outcome
		}
		state, err := r.Projection.Read(request.Aggregate)
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
			msg, reqErr := r.NC.RequestWithContext(hop, "fs.v1.command."+target, data)
			cancel()
			if reqErr != nil {
				// The remote owner may have committed before its reply vanished.
				if outcome := r.projectedOutcome(request, hash); outcome != nil {
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
			if reply.StreamSequence == nil {
				return unavailable(request.CommandId)
			}
			if err := r.Projection.WaitApplied(ctx, reply.GetStreamSequence()); err != nil {
				return unavailable(request.CommandId)
			}
			if outcome := r.projectedOutcome(request, hash); outcome != nil {
				return outcome
			}
			// A stale owner may have received a PubAck for a no-op event.
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
			if outcome := r.projectedOutcome(request, hash); outcome != nil {
				return outcome
			}
			redirected = ""
		default:
			return reply
		}
	}
	return unavailable(request.CommandId)
}

func (r *CommandRouter) projectedOutcome(request *pb.CommandRequest, hash string) *pb.CommandReply {
	state, err := r.Projection.Read(request.Aggregate)
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
	return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: statusForOutcome(old),
		AggregateRevision: &old.AggregateRevision, StreamSequence: &old.CommittedStreamSequence,
		Outcome: proto.Clone(old).(*pb.CommandOutcome)}
}

func unavailable(id string) *pb.CommandReply {
	return &pb.CommandReply{ProtocolRevision: 1, CommandId: id, Status: pb.CommandReply_UNAVAILABLE}
}
