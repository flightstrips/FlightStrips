package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

const asyncSessionCapacity = 1024

type asyncSessionTurn struct {
	runtime *AsyncSessionOwners
	session *asyncSession
}
type asyncSessionTurnKey struct{}
type asyncSessionJob struct {
	event   *pb.StateEvent
	persist func(context.Context) error
}
type asyncSession struct {
	ref                           *pb.AggregateRef
	epoch                         uint64
	turn                          chan struct{}
	jobs                          chan asyncSessionJob
	ram                           *Aggregate
	pending                       int
	tail                          []*pb.StateEvent
	durableStream, durableSubject uint64
}

// AsyncSessionOwners separates provisional owner state from durable projections.
// An acknowledged mutation has reserved bounded persistence capacity, but may be
// lost on process failure. Only broker-confirmed state reaches Projection.
type AsyncSessionOwners struct {
	projection       *Projection
	owner            *OwnerRuntime
	store            EventStore
	mu               sync.Mutex
	sessions         map[string]*asyncSession
	slots            chan struct{}
	changed          chan struct{}
	draining, closed bool
	turns            int
	failure          error
	translator       func(*pb.StateEvent) error
	workers          sync.WaitGroup
	controls         sync.Map
	ctx              context.Context
	cancel           context.CancelFunc
}

// Control is an immutable lock-free overlay for callers already holding the
// projection lock. It proves no health or persistence frontier on its own.
func (m *AsyncSessionOwners) Control(ref *pb.AggregateRef) *Aggregate {
	if m == nil {
		return nil
	}
	key, err := Subject(ref)
	if err != nil {
		return nil
	}
	v, ok := m.controls.Load(key)
	if !ok {
		return nil
	}
	return v.(*Aggregate)
}
func (m *AsyncSessionOwners) publishControlLocked(s *asyncSession) {
	if m.failure != nil {
		m.controls.Delete(mustAsyncSubject(s.ref))
		return
	}
	a := s.ram
	copy := *a
	view := &copy
	view.StreamSequence, view.SubjectSequence = s.durableStream, s.durableSubject
	if a.Owner != nil {
		view.Owner = proto.Clone(a.Owner).(*pb.OwnerTerm)
	}
	if a.Master != nil {
		view.Master = proto.Clone(a.Master).(*pb.MasterTerm)
	}
	if a.Sync != nil {
		view.Sync = proto.Clone(a.Sync).(*pb.SessionSync)
	}
	m.controls.Store(mustAsyncSubject(s.ref), view)
}

func NewAsyncSessionOwners(p *Projection, owner *OwnerRuntime, store EventStore) *AsyncSessionOwners {
	ctx, cancel := context.WithCancel(context.Background())
	return &AsyncSessionOwners{projection: p, owner: owner, store: store, sessions: map[string]*asyncSession{}, slots: make(chan struct{}, asyncSessionCapacity), changed: make(chan struct{}), ctx: ctx, cancel: cancel}
}
func (m *AsyncSessionOwners) notifyLocked() { close(m.changed); m.changed = make(chan struct{}) }
func (m *AsyncSessionOwners) Err() error    { m.mu.Lock(); defer m.mu.Unlock(); return m.failure }

// Invalidate freezes every provisional turn after an integrity/authority fault.
// It may be called under the projection lock; it never acquires that lock.
func (m *AsyncSessionOwners) Invalidate(ref *pb.AggregateRef, err error) {
	if err == nil {
		err = fmt.Errorf("async session invalidated")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure == nil {
		m.failure = err
	}
	m.cancel()
	m.controls.Range(func(key, value any) bool { m.controls.Delete(key); return true })
	m.notifyLocked()
}
func (m *AsyncSessionOwners) RegisterPositionTranslator(f func(*pb.StateEvent) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.translator = f
}
func (m *AsyncSessionOwners) checkpoint(ref *pb.AggregateRef) (*Aggregate, error) {
	// Borrow a published immutable durable state. Any caller which reduces a
	// fact against it must clone first; admission only needs its control terms.
	if ref == nil || m.projection == nil || m.owner == nil || m.store == nil || !m.owner.CanCommitLocal(ref) {
		return nil, fmt.Errorf("async session authority unavailable")
	}
	key, _ := Subject(ref)
	m.projection.mu.RLock()
	defer m.projection.mu.RUnlock()
	if err := m.projection.healthLocked(); err != nil {
		return nil, err
	}
	a := m.projection.states[key]
	if a == nil || a.Owner == nil || a.Owner.NodeId != m.owner.NodeID {
		return nil, fmt.Errorf("async durable owner changed")
	}
	if a.Owner.LeaseUntil == nil || !time.Now().Before(a.Owner.LeaseUntil.AsTime()) {
		return nil, fmt.Errorf("async session lease expired")
	}
	return a, nil
}
func (m *AsyncSessionOwners) Active(ref *pb.AggregateRef) bool {
	a, err := m.checkpoint(ref)
	if err != nil {
		return false
	}
	key, _ := Subject(ref)
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[key]
	return m.failure == nil && (s == nil || m.adoptIdleGenerationLocked(s, a) == nil)
}

// A fully flushed incarnation may reclaim its lease in the same process. Its
// provisional state is replaced by the new durable baseline, never carried
// across terms. An old pending tail instead freezes admission and persistence.
func (m *AsyncSessionOwners) adoptIdleGenerationLocked(s *asyncSession, raw *Aggregate) error {
	if m.failure != nil {
		return m.failure
	}
	if s.epoch == raw.Owner.Epoch {
		// Effect/provider workers may advance the durable domain prefix without
		// admitting a RAM command. Idle reads must see those facts immediately.
		if s.pending != 0 || len(s.tail) != 0 || s.ram.Revision >= raw.Revision {
			return nil
		}
	}
	if s.pending != 0 || len(s.tail) != 0 {
		m.failure = fmt.Errorf("async owner generation changed with pending tail")
		m.cancel()
		m.controls.Range(func(key, value any) bool { m.controls.Delete(key); return true })
		m.notifyLocked()
		return m.failure
	}
	baseline, err := cloneAggregate(raw)
	if err != nil {
		return err
	}
	s.epoch = raw.Owner.Epoch
	s.ram = baseline
	s.durableStream, s.durableSubject = raw.StreamSequence, raw.SubjectSequence
	m.publishControlLocked(s)
	return nil
}
func (m *AsyncSessionOwners) Pending(ref *pb.AggregateRef) bool {
	key, _ := Subject(ref)
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[key]
	return s != nil && s.pending > 0
}
func (m *AsyncSessionOwners) Read(ref *pb.AggregateRef) (*Aggregate, error) {
	if ref == nil {
		return nil, nil
	}
	key, _ := Subject(ref)
	m.mu.Lock()
	existing := m.sessions[key] != nil
	m.mu.Unlock()
	if !existing {
		m.projection.mu.RLock()
		raw := m.projection.states[key]
		local := raw != nil && raw.Owner != nil && m.owner != nil && raw.Owner.NodeId == m.owner.NodeID
		m.projection.mu.RUnlock()
		if !local {
			return nil, nil
		}
	}
	a, err := m.checkpoint(ref)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.failure != nil {
		err = m.failure
		m.mu.Unlock()
		return nil, err
	}
	s := m.sessions[key]
	if s == nil {
		m.mu.Unlock()
		return nil, nil
	}
	if err = m.adoptIdleGenerationLocked(s, a); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	// Every RAM mutation publishes a replacement graph: reducers clone before
	// applying facts, and control renewals replace the struct and owner term.
	// Capture that immutable graph under the executor lock, then detach this
	// potentially large read without blocking unrelated session admissions.
	ram := s.ram
	m.mu.Unlock()
	out, err := cloneAggregate(ram)
	if err != nil {
		return nil, err
	}
	out.Owner = proto.Clone(a.Owner).(*pb.OwnerTerm)
	out.StreamSequence, out.SubjectSequence = a.StreamSequence, a.SubjectSequence
	// An integrity failure during detachment must still reject the read.
	m.mu.Lock()
	err = m.failure
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return out, nil
}
func (m *AsyncSessionOwners) Execute(ctx context.Context, ref *pb.AggregateRef, run func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t, ok := ctx.Value(asyncSessionTurnKey{}).(asyncSessionTurn); ok && t.runtime == m {
		if !proto.Equal(t.session.ref, ref) {
			return fmt.Errorf("cross-session reentrant mutation")
		}
		return run(ctx)
	}
	a, err := m.checkpoint(ref)
	if err != nil {
		return err
	}
	key, _ := Subject(ref)
	m.mu.Lock()
	if m.draining || m.failure != nil {
		err = m.failure
		if err == nil {
			err = fmt.Errorf("async session admissions sealed")
		}
		m.mu.Unlock()
		return err
	}
	s := m.sessions[key]
	if s == nil {
		ram, cloneErr := cloneAggregate(a)
		if cloneErr != nil {
			m.mu.Unlock()
			return cloneErr
		}
		s = &asyncSession{ref: proto.Clone(ref).(*pb.AggregateRef), epoch: a.Owner.Epoch, ram: ram, durableStream: a.StreamSequence, durableSubject: a.SubjectSequence, turn: make(chan struct{}, 1), jobs: make(chan asyncSessionJob, asyncSessionCapacity)}
		s.turn <- struct{}{}
		m.sessions[key] = s
		m.publishControlLocked(s)
		m.workers.Add(1)
		go m.worker(s)
	}
	m.turns++
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.turns--; m.notifyLocked(); m.mu.Unlock() }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.turn:
	}
	defer func() { s.turn <- struct{}{} }()
	a, err = m.checkpoint(ref)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if err = m.adoptIdleGenerationLocked(s, a); err != nil {
		m.mu.Unlock()
		return err
	}
	if s.pending == 0 && s.ram.Revision != a.Revision {
		s.ram, err = cloneAggregate(a)
		if err != nil {
			m.mu.Unlock()
			return err
		}
	} else {
		updated := *s.ram
		updated.Owner = proto.Clone(a.Owner).(*pb.OwnerTerm)
		s.ram = &updated
	}
	s.durableStream, s.durableSubject = a.StreamSequence, a.SubjectSequence
	m.publishControlLocked(s)
	m.mu.Unlock()
	return run(context.WithValue(ctx, asyncSessionTurnKey{}, asyncSessionTurn{m, s}))
}
func (m *AsyncSessionOwners) currentTurn(ctx context.Context, ref *pb.AggregateRef) (*asyncSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t, ok := ctx.Value(asyncSessionTurnKey{}).(asyncSessionTurn)
	if !ok || t.runtime != m || !proto.Equal(t.session.ref, ref) {
		return nil, fmt.Errorf("mutation requires session turn")
	}
	a, err := m.checkpoint(ref)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	err = m.adoptIdleGenerationLocked(t.session, a)
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return t.session, nil
}

// RefreshDurable follows a forced durable command inside its existing session
// turn. The pending queue must already have been flushed by that caller.
func (m *AsyncSessionOwners) RefreshDurable(ctx context.Context, ref *pb.AggregateRef) error {
	s, err := m.currentTurn(ctx, ref)
	if err != nil {
		return err
	}
	durable, err := m.checkpoint(ref)
	if err != nil {
		return err
	}
	durable, err = cloneAggregate(durable)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.pending != 0 || len(s.tail) != 0 {
		return fmt.Errorf("cannot refresh durable state over pending RAM tail")
	}
	if m.failure != nil {
		return m.failure
	}
	s.ram = durable
	s.durableStream, s.durableSubject = durable.StreamSequence, durable.SubjectSequence
	m.publishControlLocked(s)
	return nil
}
func (m *AsyncSessionOwners) reserve(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case m.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-m.ctx.Done():
		if err := m.Err(); err != nil {
			return err
		}
		return m.ctx.Err()
	}
}
func (m *AsyncSessionOwners) Append(ctx context.Context, ref *pb.AggregateRef, mutate, persist func(context.Context) error) error {
	s, err := m.currentTurn(ctx, ref)
	if err != nil {
		return err
	}
	if mutate == nil || persist == nil {
		return fmt.Errorf("async mutation callbacks required")
	}
	if err = m.reserve(ctx); err != nil {
		return err
	}
	if _, err = m.currentTurn(ctx, ref); err != nil {
		<-m.slots
		return err
	}
	if err = m.Err(); err != nil {
		<-m.slots
		return err
	}
	if err = mutate(ctx); err != nil {
		<-m.slots
		return err
	}
	m.mu.Lock()
	if m.failure != nil {
		err = m.failure
		m.mu.Unlock()
		<-m.slots
		return err
	}
	s.pending++
	m.notifyLocked()
	m.mu.Unlock()
	s.jobs <- asyncSessionJob{persist: persist}
	return nil
}
func (m *AsyncSessionOwners) AcceptState(ctx context.Context, base *Aggregate, event *pb.StateEvent) (*pb.CommandReply, error) {
	if base == nil || event == nil {
		return nil, fmt.Errorf("async state required")
	}
	s, err := m.currentTurn(ctx, base.Ref)
	if err != nil {
		return nil, err
	}
	if err = m.reserve(ctx); err != nil {
		return nil, err
	}
	fresh, err := m.checkpoint(base.Ref)
	if err != nil {
		<-m.slots
		return nil, err
	}
	queued := proto.Clone(event).(*pb.StateEvent)
	m.mu.Lock()
	if err = m.adoptIdleGenerationLocked(s, fresh); err != nil {
		m.mu.Unlock()
		<-m.slots
		return nil, err
	}
	if m.failure != nil || s.ram.Revision != base.Revision || !proto.Equal(queued.Aggregate, base.Ref) {
		err = m.failure
		if err == nil {
			err = fmt.Errorf("async planning state changed")
		}
		m.mu.Unlock()
		<-m.slots
		return nil, err
	}
	// Reduction replaces changed records; retain immutable unchanged protobufs
	// using the same copy contract as the durable projection reducer.
	next := copyAggregateForApply(s.ram)
	next.Owner = proto.Clone(fresh.Owner).(*pb.OwnerTerm)
	data, marshalErr := proto.Marshal(queued)
	effective := false
	if marshalErr == nil {
		effective, marshalErr = next.Apply(AppliedEvent{Subject: mustAsyncSubject(base.Ref), StreamSequence: next.StreamSequence + 1, SubjectSequence: next.SubjectSequence + 1, ServerTime: time.Now(), Data: data})
	}
	if marshalErr != nil || !effective {
		if marshalErr == nil {
			marshalErr = fmt.Errorf("async state fact fenced")
		}
		m.mu.Unlock()
		<-m.slots
		return nil, marshalErr
	}
	outcome := next.Ledger[queued.GetCommandId()]
	if outcome == nil {
		m.mu.Unlock()
		<-m.slots
		return nil, fmt.Errorf("async state has no outcome")
	}
	outcome.CommittedStreamSequence = 0
	s.ram = next
	m.publishControlLocked(s)
	s.tail = append(s.tail, queued)
	s.pending++
	m.notifyLocked()
	reply := &pb.CommandReply{ProtocolRevision: 1, MemoryAccepted: true, CommandId: queued.GetCommandId(), Status: statusForOutcome(outcome), AggregateRevision: proto.Uint64(outcome.AggregateRevision), Outcome: proto.Clone(outcome).(*pb.CommandOutcome), CurrentOwner: proto.Clone(next.Owner).(*pb.OwnerTerm)}
	m.mu.Unlock()
	m.projection.PublishMemoryEvent(queued)
	s.jobs <- asyncSessionJob{event: queued}
	return reply, nil
}
func mustAsyncSubject(ref *pb.AggregateRef) string { s, _ := Subject(ref); return s }

func (m *AsyncSessionOwners) persistState(ctx context.Context, s *asyncSession, event *pb.StateEvent) error {
	event = proto.Clone(event).(*pb.StateEvent)
	m.mu.Lock()
	translate := m.translator
	m.mu.Unlock()
	if translate != nil {
		if err := translate(event); err != nil {
			return err
		}
	}
	reconcile := false
	var attemptedSubject uint64
	for ctx.Err() == nil {
		var durable *Aggregate
		if reconcile {
			m.projection.mu.RLock()
			durable = m.projection.states[mustAsyncSubject(s.ref)]
			m.projection.mu.RUnlock()
		}
		if reconcile && durable != nil {
			if outcome, lookupErr := durable.LookupOutcome(event.GetCommandId()); lookupErr != nil {
				return lookupErr
			} else if outcome != nil {
				entry, receiptErr := m.stateReceipt(ctx, outcome.CommittedStreamSequence)
				if receiptErr != nil {
					return receiptErr
				}
				if entry.Subject != mustAsyncSubject(s.ref) || entry.StreamSequence != outcome.CommittedStreamSequence || !proto.Equal(event, decodeAsyncEvent(entry.Data)) {
					return fmt.Errorf("async reconciled event identity mismatch")
				}
				return nil
			}
		}
		state, err := m.checkpoint(s.ref)
		if err != nil {
			retry := m.waitPersistenceRetry(ctx, s.ref, s.epoch)
			if retry == nil {
				continue
			}
			if reconcile && asyncRetryAuthorityLost(retry) {
				return m.reconcileLostAuthorityReceipt(ctx, s, event, attemptedSubject, fmt.Errorf("%w; retry rejected: %w", err, retry))
			}
			return fmt.Errorf("%w; retry rejected: %w", err, retry)
		}
		if state.Owner.Epoch != s.epoch || event.OwnerEpoch != s.epoch {
			return fmt.Errorf("async persistence owner generation changed")
		}
		if event.AggregateRevision != state.Revision+1 {
			return fmt.Errorf("async durable domain prefix changed")
		}
		data, err := proto.Marshal(event)
		if err != nil {
			return err
		}
		attemptedSubject = state.SubjectSequence
		seq, err := m.store.Publish(ctx, mustAsyncSubject(s.ref), state.SubjectSequence, data)
		if errors.Is(err, ErrCAS) {
			reconcile = true
			if err = m.persistenceSubjectAdvance(ctx, s.ref, s.epoch, state.SubjectSequence); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			if asyncTransportRetryable(err) {
				reconcile = true
				m.projection.mu.RLock()
				advanced := m.projection.states[mustAsyncSubject(s.ref)]
				m.projection.mu.RUnlock()
				if advanced != nil {
					if outcome, lookupErr := advanced.LookupOutcome(event.GetCommandId()); lookupErr == nil && outcome != nil {
						continue
					}
				}
			}
			if asyncTransportRetryable(err) {
				retry := m.waitPersistenceRetry(ctx, s.ref, s.epoch)
				if retry == nil {
					continue
				}
				if asyncRetryAuthorityLost(retry) {
					return m.reconcileLostAuthorityReceipt(ctx, s, event, state.SubjectSequence, fmt.Errorf("%w; retry rejected: %w", err, retry))
				}
				return fmt.Errorf("%w; retry rejected: %w", err, retry)
			}
			return err
		}
		entry, err := m.stateReceipt(ctx, seq)
		if err != nil {
			return err
		}
		if entry.Subject != mustAsyncSubject(s.ref) || entry.StreamSequence != seq || !proto.Equal(event, decodeAsyncEvent(entry.Data)) {
			return fmt.Errorf("async committed event identity mismatch")
		}
		if err = m.projection.applyCommitted(entry, state.SubjectSequence); err != nil {
			if err = m.persistenceReplayApplied(ctx, seq); err != nil {
				return err
			}
		}
		confirmed, err := m.persistenceConfirmation(ctx, s.ref, event.GetCommandId())
		if err != nil {
			return err
		}
		outcome, err := confirmed.LookupOutcome(event.GetCommandId())
		if err != nil {
			return err
		}
		if outcome == nil || outcome.CommittedStreamSequence != seq {
			return fmt.Errorf("async durable fact was fenced")
		}
		return nil
	}
	return ctx.Err()
}

func asyncTransportRetryable(err error) bool {
	if errors.Is(err, nats.ErrNoStreamResponse) || errors.Is(err, nats.ErrTimeout) || errors.Is(err, nats.ErrNoResponders) || errors.Is(err, nats.ErrConnectionClosed) || errors.Is(err, nats.ErrDisconnected) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var api *nats.APIError
	return errors.As(err, &api) && api.Code == 503
}

// Only fixed reasons and numeric checkpoint facts leave this guard; no command
// contents, actor IDs, or underlying error text enter persistence diagnostics.
type asyncRetryRejected struct {
	reason                                       string
	expectedEpoch, currentEpoch, subjectSequence uint64
	ownerMatches                                 bool
	leaseRemainingMS, metadataAgeMS              int64
}

func (e *asyncRetryRejected) Error() string {
	return "async retry owner generation or integrity changed: " + e.reason
}

func (m *AsyncSessionOwners) waitPersistenceRetry(ctx context.Context, ref *pb.AggregateRef, epoch uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.Err(); err != nil {
		return err
	}
	p := m.projection
	p.mu.RLock()
	a := p.states[mustAsyncSubject(ref)]
	rejected := &asyncRetryRejected{expectedEpoch: epoch, metadataAgeMS: -1}
	if !p.checked.IsZero() {
		rejected.metadataAgeMS = time.Since(p.checked).Milliseconds()
	}
	if a != nil && a.Owner != nil {
		rejected.currentEpoch = a.Owner.Epoch
		rejected.subjectSequence = a.SubjectSequence
		rejected.ownerMatches = a.Owner.NodeId == m.owner.NodeID
		if a.Owner.LeaseUntil != nil {
			rejected.leaseRemainingMS = time.Until(a.Owner.LeaseUntil.AsTime()).Milliseconds()
		}
	}
	switch {
	case p.observationErr != nil:
		rejected.reason = "observation_integrity"
	case p.healthErr != nil && !asyncPersistenceHealthRetryable(p.healthErr):
		rejected.reason = "metadata_integrity"
	}
	for _, err := range p.snapshotErrors {
		if !errors.Is(err, ErrImmutableSnapshotCollision) {
			rejected.reason = "snapshot_integrity"
			break
		}
	}
	if rejected.reason == "" {
		switch {
		case a == nil || a.Owner == nil:
			rejected.reason = "owner_missing"
		case !rejected.ownerMatches:
			rejected.reason = "owner_node_changed"
		case a.Owner.Epoch != epoch:
			rejected.reason = "owner_epoch_changed"
		case a.Owner.LeaseUntil == nil || !time.Now().Before(a.Owner.LeaseUntil.AsTime()):
			rejected.reason = "owner_lease_expired"
		}
	}
	p.mu.RUnlock()
	if rejected.reason != "" {
		return rejected
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

func asyncRetryAuthorityLost(err error) bool {
	var rejected *asyncRetryRejected
	if !errors.As(err, &rejected) {
		return false
	}
	switch rejected.reason {
	case "owner_missing", "owner_node_changed", "owner_epoch_changed", "owner_lease_expired":
		return true
	}
	return false
}

// An unknown acknowledgement may already be durable. Once authority is lost,
// no retransmission is possible: await healthy replay and verify the exact own
// outcome/receipt, or reject. The final worker also rejects any remaining tail.
func (m *AsyncSessionOwners) reconcileLostAuthorityReceipt(ctx context.Context, s *asyncSession, event *pb.StateEvent, previous uint64, rejected error) error {
	if err := m.persistenceSubjectAdvance(ctx, s.ref, s.epoch, previous); err != nil {
		return fmt.Errorf("%w; receipt reconciliation unavailable: %w", rejected, err)
	}
	state, err := m.persistenceConfirmation(ctx, s.ref, event.GetCommandId())
	if err != nil {
		return fmt.Errorf("%w; receipt reconciliation unavailable: %w", rejected, err)
	}
	outcome, err := state.LookupOutcome(event.GetCommandId())
	if err != nil {
		return fmt.Errorf("%w; receipt reconciliation unavailable: %w", rejected, err)
	}
	if outcome == nil || outcome.CommittedStreamSequence == 0 {
		return rejected
	}
	entry, err := m.stateReceipt(ctx, outcome.CommittedStreamSequence)
	if err != nil {
		return fmt.Errorf("%w; receipt reconciliation unavailable: %w", rejected, err)
	}
	if entry.Subject != mustAsyncSubject(s.ref) || entry.StreamSequence != outcome.CommittedStreamSequence || !proto.Equal(event, decodeAsyncEvent(entry.Data)) {
		return fmt.Errorf("%w; async authority-loss receipt identity mismatch", rejected)
	}
	return nil
}

func logAsyncPersistenceFailure(s *asyncSession, job asyncSessionJob, phase string, err error) {
	kind := "position"
	if job.event != nil {
		kind = "state"
	}
	attrs := []any{"job_kind", kind, "aggregate", mustAsyncSubject(s.ref), "expected_epoch", s.epoch, "pending_jobs", s.pending, "pending_domain_facts", len(s.tail), "phase", phase, "error_type", fmt.Sprintf("%T", err)}
	if job.event != nil {
		attrs = append(attrs, "event_kind", fmt.Sprintf("%T", job.event.GetFact()))
	}
	var rejected *asyncRetryRejected
	if errors.As(err, &rejected) {
		attrs = append(attrs, "guard_reason", rejected.reason, "raw_epoch", rejected.currentEpoch, "owner_matches", rejected.ownerMatches, "lease_remaining_ms", rejected.leaseRemainingMS, "metadata_age_ms", rejected.metadataAgeMS, "raw_subject_sequence", rejected.subjectSequence)
	}
	slog.Warn("async session persistence failed", attrs...)
}

// Temporary proof unavailability keeps accepted work queued, but never makes a
// stale view readable or permits retransmission outside its live owner term.
func asyncPersistenceHealthRetryable(err error) bool {
	if asyncTransportRetryable(err) {
		return true
	}
	if err == nil {
		return false
	}
	switch err.Error() {
	case "state metadata is stale", "KV observation replay incomplete", "NATS disconnected":
		return true
	}
	for _, stream := range []string{"FS_STATE", "KV_FS_POSITIONS", "KV_FS_PRESENCE", "KV_FS_SNAPSHOT_INDEX", "OBJ_FS_OBJECTS"} {
		if err.Error() == stream+" has no current quorum" {
			return true
		}
	}
	return false
}

func (m *AsyncSessionOwners) persistenceCheckpoint(ctx context.Context, ref *pb.AggregateRef, epoch uint64) (*Aggregate, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		a, err := m.checkpoint(ref)
		if err == nil {
			if a.Owner.Epoch != epoch {
				return nil, fmt.Errorf("async persistence owner generation changed")
			}
			return a, nil
		}
		if retryErr := m.waitPersistenceRetry(ctx, ref, epoch); retryErr != nil {
			return nil, retryErr
		}
	}
}

// A broker-verified receipt needs no new owner permission. Await only a fresh
// healthy replay proof before inspecting its durable result or retiring it.
func (m *AsyncSessionOwners) waitPersistenceReadRetry(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.Err(); err != nil {
		return err
	}
	p := m.projection
	p.mu.RLock()
	err := p.healthLocked()
	if p.observationErr != nil {
		err = p.observationErr
		p.mu.RUnlock()
		return err
	}
	for _, snapshotErr := range p.snapshotErrors {
		if !errors.Is(snapshotErr, ErrImmutableSnapshotCollision) {
			p.mu.RUnlock()
			return snapshotErr
		}
	}
	p.mu.RUnlock()
	if err != nil && !asyncPersistenceHealthRetryable(err) {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// The receipt was already verified; this wait is read-only even if the owner
// term expires. It never publishes or bypasses replay health/history checks.
func (m *AsyncSessionOwners) persistenceReplayApplied(ctx context.Context, sequence uint64) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := m.projection.WaitApplied(ctx, sequence)
		if err == nil || !asyncPersistenceHealthRetryable(err) {
			return err
		}
		if retryErr := m.waitPersistenceReadRetry(ctx); retryErr != nil {
			return retryErr
		}
	}
}

// A CAS conflict has not proved this command committed. Waiting for replay is
// read-only; after advancement the caller must either verify the exact stored
// event or enter its live owner checkpoint before any new publication.
func (m *AsyncSessionOwners) persistenceSubjectAdvance(ctx context.Context, ref *pb.AggregateRef, epoch, previous uint64) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := m.projection.WaitSubjectAdvance(ctx, mustAsyncSubject(ref), previous)
		if err == nil || !asyncPersistenceHealthRetryable(err) {
			return err
		}
		if retryErr := m.waitPersistenceReadRetry(ctx); retryErr != nil {
			return retryErr
		}
	}
}

func (m *AsyncSessionOwners) persistenceConfirmation(ctx context.Context, ref *pb.AggregateRef, id string) (*Aggregate, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		a, err := m.projection.readDurableCommandCheckpoint(ref, id, false)
		if err == nil {
			return a, nil
		}
		if !asyncPersistenceHealthRetryable(err) {
			return nil, err
		}
		if retryErr := m.waitPersistenceReadRetry(ctx); retryErr != nil {
			return nil, retryErr
		}
	}
}

// Once a PubAck or matching replay outcome supplies the sequence, recovery is
// read-only: retry receipt retrieval without publishing the event again.
func (m *AsyncSessionOwners) stateReceipt(ctx context.Context, seq uint64) (AppliedEvent, error) {
	receipt, ok := m.store.(interface {
		Committed(context.Context, uint64) (AppliedEvent, error)
	})
	if !ok || seq == 0 {
		return AppliedEvent{}, fmt.Errorf("async store lacks committed metadata")
	}
	for {
		entry, err := receipt.Committed(ctx, seq)
		if err == nil || !asyncTransportRetryable(err) {
			return entry, err
		}
		select {
		case <-ctx.Done():
			return AppliedEvent{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func decodeAsyncEvent(data []byte) *pb.StateEvent {
	e := &pb.StateEvent{}
	if pb.UnmarshalStrict(data, e) != nil {
		return nil
	}
	return e
}
func (m *AsyncSessionOwners) worker(s *asyncSession) {
	defer m.workers.Done()
	for job := range s.jobs {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		phase := "state_publish_or_reconcile"
		m.mu.Lock()
		err := m.failure
		m.mu.Unlock()
		if err == nil {
			if job.event != nil {
				err = m.persistState(ctx, s, job.event)
			} else {
				phase = "position_initial_checkpoint"
				a, e := m.persistenceCheckpoint(ctx, s.ref, s.epoch)
				err = e
				if err == nil && a.Owner.Epoch != s.epoch {
					err = fmt.Errorf("async position owner generation changed")
				}
				if err == nil {
					phase = "position_publish_or_reconcile"
					err = job.persist(ctx)
				}
			}
		}
		m.mu.Lock()
		if err != nil && m.failure == nil {
			logAsyncPersistenceFailure(s, job, phase, err)
			m.failure = fmt.Errorf("async session persistence: %w", err)
			m.cancel()
			m.controls.Range(func(key, value any) bool { m.controls.Delete(key); return true })
		}
		if job.event != nil && len(s.tail) > 0 {
			s.tail = s.tail[1:]
		}
		m.mu.Unlock()
		// Refresh only the durable portion and reapply outstanding domain facts.
		if err == nil {
			phase = "final_checkpoint"
			durable, e := m.persistenceCheckpoint(ctx, s.ref, s.epoch)
			completedOnly := false
			if e != nil {
				// The final job was proved durable, so ownership loss cannot
				// turn receipt retirement into a new write. No pending old-term
				// facts may be rebased under this newer immutable checkpoint.
				m.mu.Lock()
				finishedTail := s.pending == 1 && len(s.tail) == 0
				m.mu.Unlock()
				if finishedTail {
					for ctx.Err() == nil {
						m.projection.mu.RLock()
						durable = m.projection.states[mustAsyncSubject(s.ref)]
						health := m.projection.healthLocked()
						m.projection.mu.RUnlock()
						if health != nil && asyncPersistenceHealthRetryable(health) {
							if retryErr := m.waitPersistenceReadRetry(ctx); retryErr == nil {
								continue
							} else {
								e = retryErr
							}
						}
						if health == nil && durable != nil {
							if job.event == nil {
								e = nil
								completedOnly = true
							} else {
								outcome, lookupErr := durable.LookupOutcome(job.event.GetCommandId())
								if lookupErr == nil && outcome != nil && outcome.CommittedStreamSequence > 0 && outcome.CommittedStreamSequence <= durable.StreamSequence {
									e = nil
									completedOnly = true
								}
							}
						}
						break
					}
				}
			}
			if e == nil {
				m.mu.Lock()
				if completedOnly && (m.failure != nil || s.pending != 1 || len(s.tail) != 0) {
					// Admission can change the tail while the raw proof is read.
					// Recheck under the lock that publishes the replacement view;
					// this read-only retirement never rebases another pending job.
					e = fmt.Errorf("async final receipt retirement requires an unchanged completed tail")
					m.mu.Unlock()
				} else {
					s.durableStream, s.durableSubject = durable.StreamSequence, durable.SubjectSequence
					if job.event == nil && durable.Revision+uint64(len(s.tail)) == s.ram.Revision {
						updated := *s.ram
						updated.Owner = proto.Clone(durable.Owner).(*pb.OwnerTerm)
						s.ram = &updated
						m.publishControlLocked(s)
						m.mu.Unlock()
					} else {
						phase = "final_rebase"
						durable, e = cloneAggregate(durable)
						if e != nil {
							logAsyncPersistenceFailure(s, job, phase, e)
							m.mu.Unlock()
							m.Invalidate(s.ref, e)
							m.mu.Lock()
							s.pending--
							m.notifyLocked()
							m.mu.Unlock()
							<-m.slots
							cancel()
							continue
						}
						for _, pending := range s.tail {
							data, _ := proto.Marshal(pending)
							effective, applyErr := durable.Apply(AppliedEvent{Subject: mustAsyncSubject(s.ref), StreamSequence: durable.StreamSequence + 1, SubjectSequence: durable.SubjectSequence + 1, ServerTime: time.Now(), Data: data})
							if applyErr != nil || !effective {
								e = fmt.Errorf("async pending state rebase failed")
								break
							}
							if outcome := durable.Ledger[pending.GetCommandId()]; outcome != nil {
								outcome.CommittedStreamSequence = 0
							}
						}
						if e == nil {
							s.ram = durable
							// Pending reduction uses private counters; external views retain
							// only the checkpoint captured before reapplying that tail.
							m.publishControlLocked(s)
						}
						m.mu.Unlock()
					}
				}
			}
			if e != nil {
				m.mu.Lock()
				if m.failure == nil {
					logAsyncPersistenceFailure(s, job, phase, e)
					m.failure = e
					m.cancel()
					m.controls.Range(func(key, value any) bool { m.controls.Delete(key); return true })
				}
				m.notifyLocked()
				m.mu.Unlock()
			}
		}
		m.mu.Lock()
		s.pending--
		m.notifyLocked()
		m.mu.Unlock()
		<-m.slots
		cancel()
	}
}
func (m *AsyncSessionOwners) BeginDrain() {
	m.mu.Lock()
	m.draining = true
	m.notifyLocked()
	m.mu.Unlock()
}
func (m *AsyncSessionOwners) Drain(ctx context.Context) error {
	m.BeginDrain()
	for {
		m.mu.Lock()
		pending := m.turns
		for _, s := range m.sessions {
			pending += s.pending
		}
		if pending == 0 {
			if !m.closed {
				m.closed = true
				for _, s := range m.sessions {
					close(s.jobs)
				}
			}
			m.mu.Unlock()
			joined := make(chan struct{})
			go func() { m.workers.Wait(); close(joined) }()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-joined:
			}
			m.cancel()
			return m.Err()
		}
		changed := m.changed
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}
