package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	euroscope "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// Effects is run only by the explicitly selected NATS runtime. A claimed
// effect is never dispatched by Sweep, including after owner takeover.
type Effects struct {
	Owner   *OwnerRuntime
	Fanout  *SessionFanout
	Secrets EffectSecrets
}

func (s Effects) Run(ctx context.Context) error {
	if s.Owner == nil || s.Fanout == nil || s.Fanout.Projection != s.Owner.Projection {
		return fmt.Errorf("effect runtime unavailable")
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	garbageTicker := time.NewTicker(time.Hour)
	defer garbageTicker.Stop()
	resultErrors := make(chan error, 1)
	resultCtx, stop := context.WithCancel(ctx)
	joined := make(chan struct{})
	go func() { defer close(joined); resultErrors <- s.ServeResults(resultCtx) }()
	defer func() { stop(); <-joined }()
	for {
		if err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
			// A transient CAS or projection failure is retried on the next tick.
			slog.ErrorContext(ctx, "effect sweep failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-resultErrors:
			return err
		case <-ticker.C:
		case <-garbageTicker.C:
			if s.Secrets.Objects != nil {
				if _, err := s.GarbageCollect(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
					slog.ErrorContext(ctx, "effect garbage collection failed", "error", err)
				}
			}
		}
	}
}

func (s Effects) Sweep(ctx context.Context) error {
	if s.Owner == nil || s.Owner.Projection == nil || s.Owner.Store == nil || s.Fanout == nil {
		return fmt.Errorf("effect runtime unavailable")
	}
	s.Owner.mu.RLock()
	refs := make([]*pb.AggregateRef, 0, len(s.Owner.tracked))
	for _, ref := range s.Owner.tracked {
		if ref.GetSession() != nil {
			refs = append(refs, ref)
		}
	}
	s.Owner.mu.RUnlock()
	for _, ref := range refs {
		if !s.Owner.CanWrite(ref) {
			continue
		}
		state, err := s.Owner.Projection.ReadDurable(ref)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(state.Effects))
		for id := range state.Effects {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool {
			left, right := state.Effects[ids[i]], state.Effects[ids[j]]
			if left.GetGenerateSquawk() != nil && right.GetGenerateSquawk() != nil {
				ls, rs := state.Ledger[ids[i]].GetCommittedStreamSequence(), state.Ledger[ids[j]].GetCommittedStreamSequence()
				if ls != rs {
					return ls < rs
				}
			}
			// Keep squawk queue entries together to preserve comparator transitivity.
			if (left.GetGenerateSquawk() != nil) != (right.GetGenerateSquawk() != nil) {
				return left.GetGenerateSquawk() != nil
			}
			return ids[i] < ids[j]
		})
		for _, id := range ids {
			effect := state.Effects[id]
			switch effect.Status {
			case pb.EffectRecord_WAITING:
				if squawk := effect.GetGenerateSquawk(); squawk != nil {
					fresh, err := s.Owner.Projection.ReadDurable(ref)
					if err != nil {
						return err
					}
					strip := fresh.Indexes[pb.EntityKind_STRIP][squawk.Callsign].GetValue().GetStrip()
					if strip == nil || ValidAssignedSquawk(strip.AssignedSquawk) {
						reason := "SQUAWK_ASSIGNED"
						if strip == nil {
							reason = "STRIP_REMOVED"
						}
						_, _ = s.advance(ctx, ref, effect, pb.EffectRecord_FAILED, "", reason)
						continue
					}
				}
				if effect.DispatchDeadline == nil {
					return fmt.Errorf("effect has no dispatch deadline")
				}
				if !time.Now().Before(effect.DispatchDeadline.AsTime()) {
					_, _ = s.advance(ctx, ref, effect, pb.EffectRecord_EXPIRED, "", "")
					continue
				}
				client, err := s.selectTarget(ref.GetSession().Id, effect.TargetCid)
				if err != nil || client == nil || effect.GetGenerateSquawk() != nil && client.Observer {
					continue
				}
				_ = s.claimAndDispatch(ctx, ref, effect, client.ConnectionId)
			case pb.EffectRecord_DISPATCH_CLAIMED:
				if effect.ResultDeadline != nil && !time.Now().Before(effect.ResultDeadline.AsTime()) {
					_, _ = s.advance(ctx, ref, effect, pb.EffectRecord_UNKNOWN, "", "")
				}
			}
		}
	}
	return nil
}

func (s Effects) selectTarget(sessionID int32, cid string) (*pb.ClientPresence, error) {
	return selectLiveEffectTarget(s.Owner.Projection, sessionID, cid)
}

// Keep durable claim and its sole dispatch attempt in one owner turn. Later
// RAM admissions must not invalidate the dispatch gate between those steps.
func (s Effects) claimAndDispatch(ctx context.Context, ref *pb.AggregateRef, effect *pb.EffectRecord, connection string) error {
	if owners := s.Owner.Projection.Async; owners != nil {
		return owners.Execute(ctx, ref, func(turn context.Context) error {
			if err := owners.FlushSession(turn, ref); err != nil {
				return err
			}
			return s.claimAndDispatchDurable(turn, ref, effect, connection)
		})
	}
	return s.claimAndDispatchDurable(ctx, ref, effect, connection)
}

func (s Effects) claimAndDispatchDurable(ctx context.Context, ref *pb.AggregateRef, effect *pb.EffectRecord, connection string) error {
	claimed, err := s.advance(ctx, ref, effect, pb.EffectRecord_DISPATCH_CLAIMED, connection, "")
	if err != nil {
		return err
	}
	if claimed == nil || !s.Owner.CanWrite(ref) {
		return fmt.Errorf("effect owner unavailable after claim")
	}
	// A lost reply is ambiguous. Never make a second socket write.
	return s.Fanout.SendToCID(ctx, ref.GetSession().Id, claimed)
}

func selectLiveEffectTarget(projection *Projection, sessionID int32, cid string) (*pb.ClientPresence, error) {
	if projection == nil {
		return nil, fmt.Errorf("effect presence unavailable")
	}
	_, entries, err := projection.ObservationSnapshot(sessionID)
	if err != nil {
		return nil, err
	}
	nodes := map[string]bool{}
	for _, entry := range entries {
		if node := entry.Value.GetNode(); node != nil && node.Ready && time.Since(entry.Observed) < nodeTTL {
			nodes[node.NodeId] = true
		}
	}
	var selected *pb.ClientPresence
	for _, entry := range entries {
		client := entry.Value.GetClient()
		if client == nil || client.Kind != pb.ClientPresence_EUROSCOPE || client.SessionId != sessionID ||
			client.Cid != cid || !nodes[client.NodeId] || time.Since(entry.Observed) >= nodeTTL || client.ConnectedAt == nil {
			continue
		}
		if selected == nil || selected.ConnectedAt.AsTime().Before(client.ConnectedAt.AsTime()) ||
			selected.ConnectedAt.AsTime().Equal(client.ConnectedAt.AsTime()) && selected.ConnectionId < client.ConnectionId {
			selected = client
		}
	}
	if selected == nil {
		return nil, nil
	}
	return proto.Clone(selected).(*pb.ClientPresence), nil
}

func (s Effects) advance(ctx context.Context, ref *pb.AggregateRef, prior *pb.EffectRecord, status pb.EffectRecord_Status, connectionID, reason string) (*pb.EffectRecord, error) {
	if owners := s.Owner.Projection.Async; owners != nil {
		var committed *pb.EffectRecord
		err := owners.Execute(ctx, ref, func(turn context.Context) error {
			if err := owners.FlushSession(turn, ref); err != nil {
				return err
			}
			var err error
			committed, err = s.advanceDurable(turn, ref, prior, status, connectionID, reason)
			if err != nil {
				return err
			}
			return owners.RefreshDurable(turn, ref)
		})
		return committed, err
	}
	return s.advanceDurable(ctx, ref, prior, status, connectionID, reason)
}

func (s Effects) advanceDurable(ctx context.Context, ref *pb.AggregateRef, prior *pb.EffectRecord, status pb.EffectRecord_Status, connectionID, reason string) (*pb.EffectRecord, error) {
	if !s.Owner.CanWrite(ref) {
		return nil, fmt.Errorf("effect owner unavailable")
	}
	state, err := s.Owner.Projection.ReadDurable(ref)
	if err != nil {
		return nil, err
	}
	current, err := state.LookupEffect(prior.CommandId)
	if err != nil {
		return nil, err
	}
	if current == nil || !proto.Equal(current, prior) {
		return nil, ErrCAS
	}
	next := proto.Clone(current).(*pb.EffectRecord)
	next.Status, next.ReasonCode = status, reason
	if status == pb.EffectRecord_DISPATCH_CLAIMED {
		if err := squawkClaimAllowed(state, current, time.Now().UTC()); err != nil {
			return nil, err
		}
		if connectionID == "" || state.Owner == nil {
			return nil, fmt.Errorf("effect target or owner missing")
		}
		next.DispatchConnectionId = &connectionID
		next.OwnerEpoch = state.Owner.Epoch
		if state.Master != nil {
			next.MasterEpoch = state.Master.Epoch
		}
	}
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	id := prior.CommandId
	event := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), CommandId: &id,
		Aggregate: proto.Clone(ref).(*pb.AggregateRef), AggregateRevision: state.Revision + 1,
		OwnerEpoch: state.Owner.Epoch, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: s.Owner.NodeID},
		Fact: &pb.StateEvent_EffectChanged{EffectChanged: next}}
	data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(event)
	if err != nil || len(data) > MaxStateBytes {
		return nil, fmt.Errorf("invalid effect event: %w", err)
	}
	sequence, err := s.Owner.Store.Publish(ctx, subject, state.SubjectSequence, data)
	if err != nil {
		return nil, err
	}
	if err := s.Owner.Projection.WaitApplied(ctx, sequence); err != nil {
		return nil, err
	}
	fresh, err := s.Owner.Projection.ReadDurable(ref)
	if err != nil {
		return nil, err
	}
	committed, err := fresh.LookupEffect(id)
	if err != nil {
		return nil, err
	}
	if fresh.StreamSequence < sequence || committed == nil || committed.Status != status {
		return nil, fmt.Errorf("effect transition was not effective")
	}
	return committed, nil
}

// RecordResult commits the plugin result before the caller acknowledges its
// outbox entry. Duplicate results only acknowledge an already stored result.
func (s Effects) RecordResult(ctx context.Context, sessionID int32, connectionID, cid string, frame *euroscope.Envelope) error {
	result := frame.GetCommandResult()
	if result == nil || !canonicalUUID(frame.CommandId) || frame.CommandId != result.CommandId ||
		len(result.Detail) > 256 || result.Status != euroscope.CommandResultEvent_EXECUTED && result.Status != euroscope.CommandResultEvent_FAILED ||
		result.Reason == euroscope.CommandResultEvent_REASON_UNSPECIFIED ||
		(result.Status == euroscope.CommandResultEvent_EXECUTED) != (result.Reason == euroscope.CommandResultEvent_OK) {
		return fmt.Errorf("invalid plugin result")
	}
	ref := sessionRef(sessionID)
	if err := s.Owner.Projection.RequireLiveSocket(sessionID, connectionID, cid, pb.ClientPresence_EUROSCOPE); err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		state, err := s.Owner.Projection.ReadDurable(ref)
		if err != nil {
			return err
		}
		effect, err := state.LookupEffect(result.CommandId)
		if err != nil {
			return err
		}
		if effect == nil || effect.TargetCid != cid || effect.DispatchConnectionId == nil ||
			frame.SessionId != sessionID || frame.OwnerEpoch != effect.OwnerEpoch || frame.MasterEpoch != effect.MasterEpoch {
			return fmt.Errorf("plugin result does not match dispatch claim")
		}
		want := pb.EffectRecord_EXECUTED
		if result.Status == euroscope.CommandResultEvent_FAILED {
			want = pb.EffectRecord_FAILED
		}
		if effect.Status == want && (want != pb.EffectRecord_FAILED || effect.ReasonCode == result.Reason.String()) {
			return nil
		}
		if effect.Status != pb.EffectRecord_DISPATCH_CLAIMED {
			return fmt.Errorf("effect has a different terminal outcome")
		}
		reason := ""
		if want == pb.EffectRecord_FAILED {
			reason = result.Reason.String()
		}
		if !s.Owner.CanWrite(ref) {
			return s.forwardResult(ctx, sessionID, connectionID, effect, want, reason, state.Owner)
		}
		_, err = s.advance(ctx, ref, effect, want, "", reason)
		if errors.Is(err, ErrCAS) {
			continue
		}
		return err
	}
	return ErrCAS
}

// The socket node forwards a typed terminal EffectRecord to the current owner.
// Unlike a plugin effect, a result is safe to resend after an ambiguous reply.
func (s Effects) forwardResult(ctx context.Context, sessionID int32, connectionID string, claim *pb.EffectRecord, status pb.EffectRecord_Status, reason string, owner *pb.OwnerTerm) error {
	if s.Fanout == nil || s.Fanout.NC == nil || owner == nil || owner.NodeId == "" {
		return fmt.Errorf("effect result owner unavailable")
	}
	terminal := proto.Clone(claim).(*pb.EffectRecord)
	terminal.Status, terminal.ReasonCode = status, reason
	state, err := s.Owner.Projection.ReadDurable(sessionRef(sessionID))
	if err != nil {
		return err
	}
	data, err := proto.Marshal(&pb.EffectDeliveryRequest{SessionId: sessionID, ConnectionId: connectionID,
		Effect: terminal, ClaimStreamSequence: state.StreamSequence})
	if err != nil || len(data) > MaxStateBytes {
		return fmt.Errorf("invalid effect result request")
	}
	for attempt := 0; attempt < 3; attempt++ {
		wait, cancel := context.WithTimeout(ctx, time.Second)
		message, err := s.Fanout.NC.RequestWithContext(wait, "fs.v1.result."+owner.NodeId, data)
		cancel()
		if err == nil {
			reply := &pb.EffectDeliveryReply{}
			if pb.UnmarshalStrict(message.Data, reply) == nil && reply.CommandId == claim.CommandId && reply.Accepted {
				return nil
			}
		}
		state, readErr := s.Owner.Projection.ReadDurable(sessionRef(sessionID))
		if readErr == nil {
			stored, lookupErr := state.LookupEffect(claim.CommandId)
			if lookupErr != nil {
				return lookupErr
			}
			if stored != nil && stored.Status == status && stored.ReasonCode == reason {
				return nil
			}
		}
	}
	return fmt.Errorf("effect result persistence unconfirmed")
}

// ServeResults is started on each candidate backend alongside ServeTargeted.
func (s Effects) ServeResults(ctx context.Context) error {
	if s.Owner == nil || s.Fanout == nil || s.Fanout.NC == nil || !canonicalUUID(s.Owner.NodeID) {
		return fmt.Errorf("effect result server unavailable")
	}
	_, closeSub, err := SubscribeJoined(s.Fanout.NC, "fs.v1.result."+s.Owner.NodeID, func(message *nats.Msg) {
		request := &pb.EffectDeliveryRequest{}
		reply := &pb.EffectDeliveryReply{}
		if len(message.Data) > 0 && len(message.Data) <= MaxStateBytes && pb.UnmarshalStrict(message.Data, request) == nil && request.Effect != nil {
			reply.CommandId = request.Effect.CommandId
			work, cancel := context.WithTimeout(ctx, 5*time.Second)
			reply.Accepted = s.recordForwardedResult(work, request) == nil
			cancel()
		}
		if message.Reply != "" {
			data, _ := proto.Marshal(reply)
			_ = s.Fanout.NC.Publish(message.Reply, data)
		}
	})
	if err != nil {
		return err
	}
	defer closeSub()
	if err := FlushSubscription(ctx, s.Fanout.NC); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func (s Effects) recordForwardedResult(ctx context.Context, request *pb.EffectDeliveryRequest) error {
	if owners := s.Owner.Projection.Async; owners != nil {
		return owners.Execute(ctx, sessionRef(request.SessionId), func(turn context.Context) error {
			if err := owners.FlushSession(turn, sessionRef(request.SessionId)); err != nil {
				return err
			}
			return s.recordForwardedResultDurable(turn, request)
		})
	}
	return s.recordForwardedResultDurable(ctx, request)
}

func (s Effects) recordForwardedResultDurable(ctx context.Context, request *pb.EffectDeliveryRequest) error {
	terminal := request.Effect
	ref := sessionRef(request.SessionId)
	if request.ClaimStreamSequence == 0 || s.Owner.Projection.WaitApplied(ctx, request.ClaimStreamSequence) != nil {
		return fmt.Errorf("effect claim checkpoint unavailable")
	}
	if terminal.DispatchConnectionId == nil || request.ConnectionId == "" ||
		terminal.Status != pb.EffectRecord_EXECUTED && terminal.Status != pb.EffectRecord_FAILED || !s.Owner.CanWrite(ref) {
		return fmt.Errorf("invalid forwarded result")
	}
	if err := s.Owner.Projection.RequireLiveSocket(request.SessionId, request.ConnectionId, terminal.TargetCid, pb.ClientPresence_EUROSCOPE); err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		state, err := s.Owner.Projection.ReadDurable(ref)
		if err != nil {
			return err
		}
		old, err := state.LookupEffect(terminal.CommandId)
		if err != nil {
			return err
		}
		if old == nil {
			return fmt.Errorf("effect claim not found")
		}
		if old.Status == terminal.Status && old.ReasonCode == terminal.ReasonCode {
			return nil
		}
		if old.Status != pb.EffectRecord_DISPATCH_CLAIMED || !proto.Equal(effectPayload(old), effectPayload(terminal)) ||
			old.TargetCid != terminal.TargetCid || !optionalStringEqual(old.DispatchConnectionId, terminal.DispatchConnectionId) ||
			old.OwnerEpoch != terminal.OwnerEpoch || old.MasterEpoch != terminal.MasterEpoch {
			return fmt.Errorf("forwarded result does not match claim")
		}
		_, err = s.advance(ctx, ref, old, terminal.Status, "", terminal.ReasonCode)
		if errors.Is(err, ErrCAS) {
			continue
		}
		return err
	}
	return ErrCAS
}

// GarbageCollect uses the terminal event's server timestamp, never the object
// upload time, as the 24-hour retention origin. An orphan gets 24 hours from
// upload so a request still in flight cannot lose its staged ciphertext.
func (s Effects) GarbageCollect(ctx context.Context, now time.Time) (int, error) {
	if s.Owner == nil || s.Owner.Projection == nil || s.Owner.Store == nil || s.Secrets.Objects == nil {
		return 0, fmt.Errorf("effect garbage collector unavailable")
	}
	if err := s.Owner.Projection.Ready(); err != nil {
		return 0, err
	}
	type reference struct {
		commandID string
		terminal  bool
		ref       *pb.AggregateRef
	}
	referenced := map[string]reference{}
	states := []*Aggregate{}
	s.Owner.Projection.mu.RLock()
	for _, state := range s.Owner.Projection.states {
		states = append(states, state)
		for id, effect := range state.Effects {
			if secret := effect.GetPrivateMessage(); secret != nil {
				referenced[secret.ObjectName] = reference{commandID: id, terminal: effect.Status == pb.EffectRecord_EXECUTED ||
					effect.Status == pb.EffectRecord_FAILED || effect.Status == pb.EffectRecord_EXPIRED || effect.Status == pb.EffectRecord_UNKNOWN,
					ref: state.Ref}
			}
		}
	}
	s.Owner.Projection.mu.RUnlock()
	objects, err := s.Secrets.Objects.List()
	if errors.Is(err, nats.ErrNoObjectsFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, info := range objects {
		if ctx.Err() != nil {
			return deleted, ctx.Err()
		}
		if info == nil || !strings.HasPrefix(info.Name, "effect/") {
			continue
		}
		ref, found := referenced[info.Name]
		if !found {
			id := strings.TrimPrefix(info.Name, "effect/")
			for _, state := range states {
				effect, err := state.LookupEffect(id)
				if err != nil {
					return deleted, err
				}
				if effect != nil && effect.GetPrivateMessage().GetObjectName() == info.Name {
					ref = reference{commandID: id, terminal: terminalEffect(effect), ref: state.Ref}
					found = true
					break
				}
			}
		}
		if !found {
			if now.Sub(info.ModTime) < effectRetention {
				continue
			}
		} else {
			if !ref.terminal {
				continue
			}
			subject, err := Subject(ref.ref)
			if err != nil {
				return deleted, err
			}
			var terminalAt time.Time
			consume := func(entry AppliedEvent) error {
				event := &pb.StateEvent{}
				if pb.UnmarshalStrict(entry.Data, event) != nil {
					return fmt.Errorf("invalid effect history")
				}
				if effect := event.GetEffectChanged(); effect != nil && effect.CommandId == ref.commandID &&
					(effect.Status == pb.EffectRecord_EXECUTED || effect.Status == pb.EffectRecord_FAILED ||
						effect.Status == pb.EffectRecord_EXPIRED || effect.Status == pb.EffectRecord_UNKNOWN) {
					terminalAt = entry.ServerTime
				}
				return nil
			}
			if visitor, ok := s.Owner.Store.(interface {
				Visit(context.Context, string, func(AppliedEvent) error) error
			}); ok {
				if err := visitor.Visit(ctx, subject, consume); err != nil {
					return deleted, err
				}
			} else {
				entries, err := s.Owner.Store.Replay(ctx, subject)
				if err != nil {
					return deleted, err
				}
				for _, entry := range entries {
					if err := consume(entry); err != nil {
						return deleted, err
					}
				}
			}
			if terminalAt.IsZero() || now.Sub(terminalAt) < effectRetention {
				continue
			}
		}
		if err := s.Secrets.Objects.Delete(info.Name); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}
