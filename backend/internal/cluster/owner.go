package cluster

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ownerLease    = 8 * time.Second
	ownerRenew    = 2 * time.Second
	nodeHeartbeat = 3 * time.Second
	nodeTTL       = 10 * time.Second
)

// OwnerRuntime is opt-in. The SQL application does not construct or run it.
// A node ID identifies this process incarnation, never a machine or Swarm task.
type OwnerRuntime struct {
	NC         *nats.Conn
	Projection *Projection
	Store      EventStore
	Presence   nats.KeyValue
	NodeID     string
	StartedAt  time.Time

	mu        sync.RWMutex
	tracked   map[string]*pb.AggregateRef
	lastRenew map[string]time.Time
	failed    map[string]bool
}

type ownerState struct {
	Ref                                       *pb.AggregateRef
	Revision, StreamSequence, SubjectSequence uint64
	Owner                                     *pb.OwnerTerm
}

func (s *ownerState) ownerEpoch() uint64 { return s.Owner.GetEpoch() }

func NewOwnerRuntime(nc *nats.Conn, projection *Projection, store EventStore) (*OwnerRuntime, error) {
	if nc == nil || projection == nil || store == nil || projection.Presence == nil {
		return nil, fmt.Errorf("owner runtime requires NATS, projection, store and presence")
	}
	return &OwnerRuntime{NC: nc, Projection: projection, Store: store, Presence: projection.Presence,
		NodeID: uuid.NewString(), StartedAt: time.Now().UTC(), tracked: map[string]*pb.AggregateRef{},
		lastRenew: map[string]time.Time{}, failed: map[string]bool{}}, nil
}

// Track activates lease maintenance for an aggregate only in the explicit
// NATS runtime. Calling Track does not publish or start a goroutine.
func (o *OwnerRuntime) Track(ref *pb.AggregateRef) error {
	subject, err := Subject(ref)
	if err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.tracked == nil {
		o.tracked = map[string]*pb.AggregateRef{}
	}
	o.tracked[subject] = proto.Clone(ref).(*pb.AggregateRef)
	return nil
}

func (o *OwnerRuntime) healthy(subject string) bool {
	if o == nil || o.NC == nil || o.NC.Status() != nats.CONNECTED || o.Projection == nil || o.Projection.Ready() != nil {
		return false
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return !o.failed[subject] && !o.lastRenew[subject].IsZero() && time.Since(o.lastRenew[subject]) < ownerLease/2
}

// CanWrite gates domain commands and external effects on the same lease health.
func (o *OwnerRuntime) CanWrite(ref *pb.AggregateRef) bool {
	subject, err := Subject(ref)
	if err != nil || !o.healthy(subject) {
		return false
	}
	state, err := o.Projection.readOwner(ref)
	return err == nil && state.Owner != nil && state.Owner.NodeId == o.NodeID
}

// Ready includes the lease renewal gate for every aggregate this node owns.
func (o *OwnerRuntime) Ready() error {
	if o == nil || o.NC == nil || o.NC.Status() != nats.CONNECTED || o.Projection == nil {
		return fmt.Errorf("owner runtime disconnected")
	}
	if err := o.Projection.Ready(); err != nil {
		return err
	}
	o.mu.RLock()
	refs := make([]*pb.AggregateRef, 0, len(o.tracked))
	for _, ref := range o.tracked {
		refs = append(refs, ref)
	}
	o.mu.RUnlock()
	for _, ref := range refs {
		state, err := o.Projection.readOwner(ref)
		if err != nil {
			return err
		}
		if state.Owner != nil && state.Owner.NodeId == o.NodeID && !o.CanWrite(ref) {
			return fmt.Errorf("owner renewal unavailable")
		}
	}
	return nil
}

func (o *OwnerRuntime) mark(subject string, ok bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.failed == nil {
		o.failed = map[string]bool{}
	}
	if o.lastRenew == nil {
		o.lastRenew = map[string]time.Time{}
	}
	o.failed[subject] = !ok
	if ok {
		o.lastRenew[subject] = time.Now()
	} else {
		delete(o.lastRenew, subject)
	}
}

func (o *OwnerRuntime) Run(ctx context.Context) error {
	if o == nil || o.NC == nil || o.Projection == nil || o.Store == nil || o.Presence == nil || !canonicalUUID(o.NodeID) {
		return fmt.Errorf("invalid owner runtime")
	}
	defer func() { _ = o.Presence.Delete("node." + o.NodeID) }()
	step := time.NewTicker(250 * time.Millisecond)
	defer step.Stop()
	nextHeartbeat := time.Time{}
	nextLease := time.Time{}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		now := time.Now()
		if !now.Before(nextHeartbeat) {
			if err := o.heartbeat(ctx); err != nil {
				o.failAll()
				// A quorum or transport outage fences this incarnation immediately,
				// but is recoverable after projection/resource verification returns.
				nextHeartbeat = now.Add(time.Second)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-step.C:
				}
				continue
			}
			nextHeartbeat = now.Add(nodeHeartbeat)
		}
		if !now.Before(nextLease) {
			o.maintain(ctx)
			nextLease = now.Add(ownerRenew)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-step.C:
		}
	}
}

func (o *OwnerRuntime) failAll() {
	o.mu.Lock()
	defer o.mu.Unlock()
	for subject := range o.tracked {
		o.failed[subject] = true
		delete(o.lastRenew, subject)
	}
}

func (o *OwnerRuntime) heartbeat(ctx context.Context) error {
	ready := o.Ready() == nil
	value := &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{
		NodeId: o.NodeID, StartedAt: timestamppb.New(o.StartedAt), Ready: ready,
	}}}
	data, err := proto.Marshal(value)
	if err != nil {
		return err
	}
	_, err = o.Presence.Put("node."+o.NodeID, data)
	return err
}

func (o *OwnerRuntime) maintain(ctx context.Context) {
	if o.NC.Status() != nats.CONNECTED || o.Projection.Ready() != nil {
		o.failAll()
		return
	}
	o.mu.RLock()
	refs := make([]*pb.AggregateRef, 0, len(o.tracked))
	for _, ref := range o.tracked {
		refs = append(refs, ref)
	}
	o.mu.RUnlock()
	for _, ref := range refs {
		if ctx.Err() != nil {
			return
		}
		subject, _ := Subject(ref)
		state, err := o.Projection.readOwner(ref)
		if err != nil {
			o.mark(subject, false)
			continue
		}
		if state.Owner != nil && state.Owner.NodeId == o.NodeID && time.Now().Before(state.Owner.GetLeaseUntil().AsTime()) {
			// A failed renewal immediately fences local command and effect work.
			if err := o.control(ctx, state, true); err != nil {
				o.mark(subject, false)
				continue
			}
			o.mark(subject, true)
			continue
		}
		o.mark(subject, false)
		if state.Owner != nil && time.Now().Before(state.Owner.GetLeaseUntil().AsTime()) {
			continue
		}
		nodes, err := o.ReadyNodes()
		if err != nil {
			continue
		}
		if len(nodes) == 0 {
			// After an outage longer than every lease, each former owner can be
			// unready solely because its expired term needs a new claim. Waiting
			// for full owner readiness here would deadlock all warm replicas.
			// This node is connected and caught up (checked above); an empty
			// ready set permits a recovery attempt, never domain/effect work.
			// Concurrent attempts still use subject CAS and server-time fencing.
			nodes = []string{o.NodeID}
		}
		rank := RendezvousRank(subject, nodes)
		for i, id := range rank {
			if id != o.NodeID {
				continue
			}
			// The next live candidate tries a second later. JetStream server
			// timestamps and the reducer still decide whether a claim is valid.
			if state.Owner != nil && time.Now().Before(state.Owner.GetLeaseUntil().AsTime().Add(time.Duration(i)*time.Second)) {
				break
			}
			if err := o.control(ctx, state, false); err == nil {
				o.mark(subject, true)
			}
			break
		}
	}
}

// ReadyNodes uses the observed KV server timestamp and filters old incarnations.
func (o *OwnerRuntime) ReadyNodes() ([]string, error) {
	if err := o.Projection.Ready(); err != nil {
		return nil, err
	}
	o.Projection.mu.RLock()
	defer o.Projection.mu.RUnlock()
	nodes := []string{}
	for _, entry := range o.Projection.presence {
		n := entry.Value.GetNode()
		if n != nil && n.Ready && canonicalUUID(n.NodeId) && time.Since(entry.Observed) < nodeTTL && !entry.Observed.Before(o.Projection.startedAt) {
			nodes = append(nodes, n.NodeId)
		}
	}
	return nodes, nil
}

// RendezvousRank is independent of input order and stable across replicas.
func RendezvousRank(subject string, nodes []string) []string {
	type candidate struct {
		id    string
		score [32]byte
	}
	seen := map[string]bool{}
	list := []candidate{}
	for _, id := range nodes {
		if seen[id] {
			continue
		}
		seen[id] = true
		list = append(list, candidate{id, sha256.Sum256([]byte(subject + "\x00" + id))})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return string(list[i].score[:]) > string(list[j].score[:])
		}
		return list[i].id < list[j].id
	})
	rank := make([]string, len(list))
	for i, item := range list {
		rank[i] = item.id
	}
	return rank
}

func (o *OwnerRuntime) control(ctx context.Context, state *ownerState, renew bool) error {
	initialEpoch := state.ownerEpoch()
	for attempt := 0; attempt < 8; attempt++ {
		err := o.controlOnce(ctx, state, renew)
		if !errors.Is(err, ErrCAS) {
			return err
		}
		subject, _ := Subject(state.Ref)
		wait, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		_ = o.Projection.WaitSubjectAdvance(wait, subject, state.SubjectSequence)
		cancel()
		fresh, readErr := o.Projection.readOwner(state.Ref)
		if readErr != nil {
			return readErr
		}
		if fresh.ownerEpoch() != initialEpoch || renew && fresh.Owner.GetNodeId() != o.NodeID {
			return fmt.Errorf("owner changed during lease CAS")
		}
		state = fresh
	}
	return ErrCAS
}

func (o *OwnerRuntime) controlOnce(ctx context.Context, state *ownerState, renew bool) error {
	subject, _ := Subject(state.Ref)
	epoch := state.ownerEpoch()
	if !renew {
		epoch++
	}
	term := &pb.OwnerTerm{NodeId: o.NodeID, Epoch: epoch}
	e := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: proto.Clone(state.Ref).(*pb.AggregateRef),
		AggregateRevision: state.Revision, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: o.NodeID}}
	if renew {
		e.Fact = &pb.StateEvent_OwnerRenewed{OwnerRenewed: term}
	} else {
		e.Fact = &pb.StateEvent_OwnerClaimed{OwnerClaimed: term}
	}
	data, err := proto.Marshal(e)
	if err != nil {
		return err
	}
	sequence, err := o.Store.Publish(ctx, subject, state.SubjectSequence, data)
	if err != nil {
		return err
	}
	if err := o.Projection.WaitApplied(ctx, sequence); err != nil {
		return err
	}
	fresh, err := o.Projection.readOwner(state.Ref)
	if err != nil {
		return err
	}
	if fresh.StreamSequence < sequence || fresh.Owner == nil || fresh.Owner.NodeId != o.NodeID || fresh.Owner.Epoch != epoch {
		return errors.New("lease event was not effective")
	}
	return nil
}
