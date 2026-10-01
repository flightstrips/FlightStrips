package cluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

var ErrCAS = errors.New("subject revision changed")

// EventStore is intentionally small: a future projection can replace Replay
// with its independent consumer without changing the conditional-write rule.
type EventStore interface {
	Replay(context.Context, string) ([]AppliedEvent, error)
	Publish(context.Context, string, uint64, []byte) (uint64, error)
}

// Planner validates a typed domain action against a fresh aggregate and returns
// the complete replacement changes, effects and workflows to commit. Domain
// adapters supply this; the writer never interprets an HTTP or WebSocket body.
type Planner func(context.Context, *pb.CommandRequest, *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error)

type Writer struct {
	Store      EventStore
	NodeID     string
	Plan       Planner
	Projection *Projection
	Lease      *OwnerRuntime
}

// Read returns the same coherent owner planning snapshot used by Execute.
func (w Writer) Read(ctx context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	return w.load(ctx, subject, ref)
}

// Outcome reads the durable aggregate ledger for an authenticated actor.
func (w Writer) Outcome(ctx context.Context, ref *pb.AggregateRef, commandID string, actor *pb.Actor) *pb.CommandReply {
	reply := &pb.CommandReply{ProtocolRevision: 1, CommandId: commandID}
	if !canonicalUUID(commandID) || actor == nil {
		reply.Status = pb.CommandReply_INVALID_ARGUMENT
		return reply
	}
	subject, err := Subject(ref)
	if err != nil {
		reply.Status, reply.Detail = pb.CommandReply_INVALID_ARGUMENT, err.Error()
		return reply
	}
	if w.Store == nil {
		reply.Status = pb.CommandReply_UNAVAILABLE
		return reply
	}
	state, err := w.load(ctx, subject, ref)
	if err != nil {
		reply.Status, reply.Detail = pb.CommandReply_UNAVAILABLE, err.Error()
		return reply
	}
	outcome, err := state.LookupOutcome(commandID)
	if err != nil {
		reply.Status = pb.CommandReply_UNAVAILABLE
		return reply
	}
	if outcome == nil {
		reply.Status = pb.CommandReply_NOT_FOUND
		return reply
	}
	if !proto.Equal(actor, outcome.Actor) {
		reply.Status = pb.CommandReply_UNAUTHORIZED
		return reply
	}
	reply.Status = statusForOutcome(outcome)
	reply.AggregateRevision, reply.StreamSequence = &outcome.AggregateRevision, &outcome.CommittedStreamSequence
	reply.Outcome = proto.Clone(outcome).(*pb.CommandOutcome)
	return reply
}

func (w Writer) Execute(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	reply, _ := w.execute(ctx, request)
	return reply
}

// ExecuteFresh reports whether this invocation received the PubAck for a new
// commit. A replayed command or uncertain PubAck is never permission to repeat
// an external provider request, even when its durable outcome is successful.
func (w Writer) ExecuteFresh(ctx context.Context, request *pb.CommandRequest) (*pb.CommandReply, bool) {
	return w.execute(ctx, request)
}

func (w Writer) execute(ctx context.Context, request *pb.CommandRequest) (*pb.CommandReply, bool) {
	published := false
	reply := &pb.CommandReply{ProtocolRevision: 1}
	if request != nil {
		reply.CommandId = request.CommandId
		request = proto.Clone(request).(*pb.CommandRequest)
		if err := normalize(request.ProtoReflect()); err != nil {
			reply.Status, reply.Detail = pb.CommandReply_INVALID_ARGUMENT, err.Error()
			return reply, published
		}
	}
	hash, err := RequestHash(request)
	if err != nil {
		reply.Status, reply.Detail = pb.CommandReply_INVALID_ARGUMENT, err.Error()
		return reply, published
	}
	subject, _ := Subject(request.Aggregate)
	if w.Store == nil || w.NodeID == "" {
		reply.Status, reply.Detail = pb.CommandReply_UNAVAILABLE, "event writer not configured"
		return reply, published
	}
	if w.Projection == nil {
		switch w.Store.(type) {
		case NATSStore, *NATSStore:
			reply.Status, reply.Detail = pb.CommandReply_UNAVAILABLE, "NATS projection barrier not configured"
			return reply, published
		}
	}
	if w.Plan == nil {
		if request.GetSystem().GetUpdateEntity() == nil && request.GetSystem().GetRemoveEntity() == nil {
			reply.Status, reply.Detail = pb.CommandReply_UNAVAILABLE, "domain planner not installed"
			return reply, published
		}
		w.Plan = PlanSystemEntity
	}
	for {
		if ctx.Err() != nil {
			reply.Status = pb.CommandReply_UNAVAILABLE
			return reply, published
		}
		if w.Lease != nil && !w.Lease.CanWrite(request.Aggregate) {
			reply.Status, reply.Detail = pb.CommandReply_UNAVAILABLE, "owner lease or projection unavailable"
			return reply, published
		}
		state, err := w.load(ctx, subject, request.Aggregate)
		if err != nil {
			reply.Status, reply.Detail = pb.CommandReply_UNAVAILABLE, err.Error()
			return reply, published
		}
		old, lookupErr := state.LookupOutcome(request.CommandId)
		if lookupErr != nil {
			reply.Status = pb.CommandReply_UNAVAILABLE
			return reply, published
		}
		if old != nil {
			if !proto.Equal(old.Actor, request.Actor) {
				reply.Status, reply.Detail = pb.CommandReply_UNAUTHORIZED, "command outcome belongs to another actor"
				return reply, published
			}
			if old.RequestSha256 != hash {
				reply.Status, reply.Detail = pb.CommandReply_INVALID_ARGUMENT, "command ID has different content"
				return reply, published
			}
			reply.Status = statusForOutcome(old)
			reply.StreamSequence = &old.CommittedStreamSequence
			reply.AggregateRevision = &old.AggregateRevision
			reply.Outcome = proto.Clone(old).(*pb.CommandOutcome)
			return reply, published
		}
		// The server timestamp on the published event decides whether the
		// lease is still valid. Local wall time must not decide ownership.
		if state.Owner == nil || state.Owner.NodeId != w.NodeID {
			reply.Status = pb.CommandReply_NOT_OWNER
			if state.Owner != nil {
				reply.CurrentOwner = proto.Clone(state.Owner).(*pb.OwnerTerm)
			}
			return reply, published
		}
		providerPdc := request.Actor.Kind == pb.Actor_PROVIDER && request.Actor.Id == "hoppie" && request.GetSystem().GetApplyPdcProviderMessage() != nil && request.Aggregate.GetSession() != nil && request.Actor.GetSessionId() == request.Aggregate.GetSession().Id
		controllerRouteFact := request.Actor.Kind == pb.Actor_CONTROLLER && request.Actor.Id != "" && request.Actor.SessionId != nil && request.Aggregate.GetAirport() != nil && request.GetSystem().GetReportAmanRouteFact() != nil
		if request.GetSystem() != nil && request.Actor.Kind != pb.Actor_SYSTEM && !providerPdc && !controllerRouteFact {
			reply.Status = pb.CommandReply_UNAUTHORIZED
			return reply, published
		}
		change, status, currentRevision, err := w.Plan(ctx, request, state)
		if err != nil && status == pb.CommandReply_COMMITTED {
			status = pb.CommandReply_INVALID_ARGUMENT
		}
		if err != nil && status == pb.CommandReply_STATUS_UNSPECIFIED {
			status = pb.CommandReply_INVALID_ARGUMENT
		}
		if status == pb.CommandReply_UNAUTHORIZED || status == pb.CommandReply_UNAVAILABLE || status == pb.CommandReply_NOT_OWNER {
			reply.Status, reply.Detail = status, errorString(err)
			return reply, published
		}
		if status != pb.CommandReply_STATUS_UNSPECIFIED && status != pb.CommandReply_COMMITTED && status != pb.CommandReply_INVALID_ARGUMENT && status != pb.CommandReply_NOT_FOUND && status != pb.CommandReply_REVISION_CONFLICT {
			reply.Status, reply.Detail = pb.CommandReply_INVALID_ARGUMENT, "invalid planner status"
			return reply, published
		}
		if status == pb.CommandReply_REVISION_CONFLICT {
			reply.CurrentEntityRevision = &currentRevision
		}
		if change == nil {
			change = &pb.DomainChange{}
		}
		if status == pb.CommandReply_STATUS_UNSPECIFIED {
			status = pb.CommandReply_COMMITTED
		}
		outcome := &pb.CommandOutcome{CommandId: request.CommandId, RequestSha256: hash, Actor: proto.Clone(request.Actor).(*pb.Actor), Status: pb.CommandOutcome_SUCCEEDED, AggregateRevision: state.Revision + 1, Aggregate: proto.Clone(request.Aggregate).(*pb.AggregateRef)}
		if request.ExpectedEntityRevision != nil {
			value := *request.ExpectedEntityRevision
			outcome.ExpectedEntityRevision = &value
		}
		if status != pb.CommandReply_COMMITTED {
			outcome.Status, outcome.ReasonCode, outcome.Detail = pb.CommandOutcome_FAILED, status.String(), errorString(err)
			if errorString(err) == "SQUAWK_ALREADY_PENDING" {
				outcome.ReasonCode = "SQUAWK_ALREADY_PENDING"
			}
			change = &pb.DomainChange{}
		} else if len(change.Effects) > 0 {
			outcome.Status = pb.CommandOutcome_ACCEPTED
			id := change.Effects[0].CommandId
			outcome.EffectId = &id
		}
		change.Outcome = outcome
		e := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), CommandId: &request.CommandId, Aggregate: proto.Clone(request.Aggregate).(*pb.AggregateRef), AggregateRevision: state.Revision + 1, OwnerEpoch: state.Owner.Epoch, Actor: proto.Clone(request.Actor).(*pb.Actor), Fact: &pb.StateEvent_DomainChanged{DomainChanged: change}}
		if err := validatePlanned(state, e); err != nil {
			reply.Status, reply.Detail = pb.CommandReply_INVALID_ARGUMENT, err.Error()
			return reply, published
		}
		data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(e)
		if err != nil || len(data) > MaxStateBytes {
			reply.Status, reply.Detail = pb.CommandReply_INVALID_ARGUMENT, "invalid or oversized event"
			return reply, published
		}
		if w.Lease != nil && !w.Lease.CanWrite(request.Aggregate) {
			reply.Status, reply.Detail = pb.CommandReply_UNAVAILABLE, "owner lease or projection unavailable"
			return reply, published
		}
		sequence, err := w.Store.Publish(ctx, subject, state.SubjectSequence, data)
		if errors.Is(err, ErrCAS) {
			if w.Projection != nil {
				if waitErr := w.Projection.WaitSubjectAdvance(ctx, subject, state.SubjectSequence); waitErr != nil {
					reply.Status, reply.Detail = pb.CommandReply_UNAVAILABLE, waitErr.Error()
					return reply, published
				}
			}
			continue
		}
		if err != nil {
			// A lost PubAck is ambiguous. Replay may prove commitment; otherwise
			// the caller receives UNAVAILABLE and must retain this command ID.
			if resolved := w.resolve(ctx, subject, request, hash); resolved != nil {
				return resolved, false
			}
			reply.Status, reply.Detail = pb.CommandReply_UNAVAILABLE, "publish acknowledgment uncertain"
			return reply, published
		}
		published = true
		if w.Projection != nil {
			if waitErr := w.Projection.WaitApplied(ctx, sequence); waitErr != nil {
				reply.Status, reply.Detail = pb.CommandReply_UNAVAILABLE, waitErr.Error()
				return reply, published
			}
		}
		for ctx.Err() == nil {
			fresh, err := w.load(ctx, subject, request.Aggregate)
			if err == nil && fresh.StreamSequence >= sequence {
				old, lookupErr := fresh.LookupOutcome(request.CommandId)
				if lookupErr != nil {
					reply.Status = pb.CommandReply_UNAVAILABLE
					return reply, published
				}
				if old != nil {
					if !proto.Equal(old.Actor, request.Actor) {
						reply.Status, reply.Detail = pb.CommandReply_UNAUTHORIZED, "command outcome belongs to another actor"
						return reply, published
					}
					if old.RequestSha256 != hash {
						reply.Status, reply.Detail = pb.CommandReply_INVALID_ARGUMENT, "command ID has different content"
						return reply, published
					}
					reply.Status, reply.Outcome = statusForOutcome(old), proto.Clone(old).(*pb.CommandOutcome)
					reply.StreamSequence, reply.AggregateRevision = &old.CommittedStreamSequence, &old.AggregateRevision
					return reply, published
				}
				reply.Status, reply.CurrentOwner = pb.CommandReply_NOT_OWNER, fresh.Owner
				return reply, published
			}
			select {
			case <-ctx.Done():
			case <-time.After(10 * time.Millisecond):
			}
		}
		reply.Status = pb.CommandReply_UNAVAILABLE
		return reply, published
	}
}

func (w Writer) resolve(ctx context.Context, subject string, request *pb.CommandRequest, hash string) *pb.CommandReply {
	state, err := w.load(ctx, subject, request.Aggregate)
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
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_UNAUTHORIZED, Detail: "command outcome belongs to another actor"}
	}
	if old.RequestSha256 != hash {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_INVALID_ARGUMENT, Detail: "command ID has different content"}
	}
	return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: statusForOutcome(old), AggregateRevision: &old.AggregateRevision, StreamSequence: &old.CommittedStreamSequence, Outcome: proto.Clone(old).(*pb.CommandOutcome)}
}

func statusForOutcome(outcome *pb.CommandOutcome) pb.CommandReply_Status {
	if outcome.Status == pb.CommandOutcome_ACCEPTED {
		return pb.CommandReply_PENDING
	}
	return pb.CommandReply_COMMITTED
}

func (w Writer) load(ctx context.Context, subject string, ref *pb.AggregateRef) (*Aggregate, error) {
	if w.Projection != nil {
		return w.Projection.Read(ref)
	}
	entries, err := w.Store.Replay(ctx, subject)
	if err != nil {
		return nil, err
	}
	state := NewAggregate(ref)
	for _, entry := range entries {
		if _, err := state.Apply(entry); err != nil {
			return nil, fmt.Errorf("replay stream %d: %w", entry.StreamSequence, err)
		}
	}
	return state, nil
}

func validatePlanned(state *Aggregate, e *pb.StateEvent) error {
	if effects := e.GetDomainChanged().GetEffects(); len(effects) > 0 {
		if len(effects) != 1 || e.GetDomainChanged().GetOutcome().GetStatus() != pb.CommandOutcome_ACCEPTED {
			return fmt.Errorf("invalid requested effect outcome")
		}
		if err := validateEffectRequest(effects[0], e.GetCommandId(), state.Effects[effects[0].CommandId]); err != nil {
			return err
		}
	}
	staged := make(map[string]*pb.EntitySnapshot, len(state.Entities))
	for k, v := range state.Entities {
		staged[k] = v
	}
	lastKind, lastKey := int32(0), ""
	for _, c := range e.GetDomainChanged().GetChanges() {
		kind, err := changeKind(c)
		if err != nil {
			return err
		}
		if kind == pb.EntityKind_SESSION_SQUAWK_THROTTLE {
			return fmt.Errorf("squawk throttle is derived only from a dispatch claim")
		}
		if int32(kind) < lastKind || (int32(kind) == lastKind && c.Key <= lastKey) {
			return fmt.Errorf("unsorted entity changes")
		}
		slot := entitySlot(staged, kind, c.Key)
		if err := validateChange(state.Ref, c, staged[slot]); err != nil {
			return err
		}
		if c.GetUpsert() != nil {
			staged[slot] = &pb.EntitySnapshot{Key: c.Key, Revision: c.Revision, Value: c.GetUpsert()}
		} else {
			delete(staged, slot)
		}
		lastKind, lastKey = int32(kind), c.Key
	}
	if err := validateControllerSectorState(state.Ref, staged); err != nil {
		return err
	}
	if err := validateStripState(state.Ref, staged); err != nil {
		return err
	}
	if err := validateStripTransition(state, e.GetDomainChanged().GetChanges(), staged); err != nil {
		return err
	}
	if err := validateCoordinationTransition(state, e.GetDomainChanged().GetChanges(), staged); err != nil {
		return err
	}
	if err := validatePdcTacticalState(state, e.GetDomainChanged(), staged); err != nil {
		return err
	}
	if err := validateStandState(state.Ref, staged); err != nil {
		return err
	}
	return nil
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
