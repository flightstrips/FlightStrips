package cluster

import (
	"context"
	"errors"
	"fmt"
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
	if ref == nil || ref.GetSession() == nil || m.projection == nil || m.owner == nil || m.store == nil || !m.owner.CanCommitLocal(ref) {
		return nil, fmt.Errorf("async session authority unavailable")
	}
	key, _ := Subject(ref)
	m.projection.mu.RLock()
	defer m.projection.mu.RUnlock()
	if err := m.projection.healthLocked(); err != nil {
		return nil, err
	}
	if m.projection.history != nil {
		if err := m.projection.history.check(); err != nil {
			return nil, err
		}
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
	if ref == nil || ref.GetSession() == nil {
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
			if retry := m.waitPersistenceRetry(ctx, s.ref, s.epoch); retry == nil {
				continue
			}
			return err
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
		seq, err := m.store.Publish(ctx, mustAsyncSubject(s.ref), state.SubjectSequence, data)
		if errors.Is(err, ErrCAS) {
			reconcile = true
			if err = m.projection.WaitSubjectAdvance(ctx, mustAsyncSubject(s.ref), state.SubjectSequence); err != nil {
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
			if asyncTransportRetryable(err) && m.waitPersistenceRetry(ctx, s.ref, s.epoch) == nil {
				continue
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
			if err = m.projection.WaitApplied(ctx, seq); err != nil {
				return err
			}
		}
		confirmed, err := m.projection.readDurableCommandCheckpoint(s.ref, event.GetCommandId(), false)
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
	valid := p.observationErr == nil && a != nil && a.Owner != nil && a.Owner.NodeId == m.owner.NodeID && a.Owner.Epoch == epoch && a.Owner.LeaseUntil != nil && time.Now().Before(a.Owner.LeaseUntil.AsTime())
	if p.healthErr != nil && !asyncTransportRetryable(p.healthErr) {
		valid = false
	}
	if p.history != nil && p.history.check() != nil {
		valid = false
	}
	for _, err := range p.snapshotErrors {
		if !errors.Is(err, ErrImmutableSnapshotCollision) {
			valid = false
		}
	}
	p.mu.RUnlock()
	if !valid {
		return fmt.Errorf("async retry owner generation or integrity changed")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
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
		m.mu.Lock()
		err := m.failure
		m.mu.Unlock()
		if err == nil {
			ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			if job.event != nil {
				err = m.persistState(ctx, s, job.event)
			} else {
				a, e := m.checkpoint(s.ref)
				err = e
				if err == nil && a.Owner.Epoch != s.epoch {
					err = fmt.Errorf("async position owner generation changed")
				}
				if err == nil {
					err = job.persist(ctx)
				}
			}
			cancel()
		}
		m.mu.Lock()
		if err != nil && m.failure == nil {
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
			durable, e := m.checkpoint(s.ref)
			if e != nil {
				// The final job was proved durable, so ownership loss cannot
				// turn receipt retirement into a new write. No pending old-term
				// facts may be rebased under this newer immutable checkpoint.
				m.mu.Lock()
				finishedTail := s.pending == 1 && len(s.tail) == 0
				m.mu.Unlock()
				if finishedTail {
					m.projection.mu.RLock()
					durable = m.projection.states[mustAsyncSubject(s.ref)]
					health := m.projection.healthLocked()
					if health == nil && m.projection.history != nil {
						health = m.projection.history.check()
					}
					m.projection.mu.RUnlock()
					if health == nil && durable != nil {
						if job.event == nil {
							e = nil
						} else {
							outcome, lookupErr := durable.LookupOutcome(job.event.GetCommandId())
							if lookupErr == nil && outcome != nil && outcome.CommittedStreamSequence > 0 && outcome.CommittedStreamSequence <= durable.StreamSequence {
								e = nil
							}
						}
					}
				}
			}
			if e == nil {
				m.mu.Lock()
				s.durableStream, s.durableSubject = durable.StreamSequence, durable.SubjectSequence
				if job.event == nil && durable.Revision+uint64(len(s.tail)) == s.ram.Revision {
					updated := *s.ram
					updated.Owner = proto.Clone(durable.Owner).(*pb.OwnerTerm)
					s.ram = &updated
					m.publishControlLocked(s)
					m.mu.Unlock()
				} else {
					durable, e = cloneAggregate(durable)
					if e != nil {
						m.mu.Unlock()
						m.Invalidate(s.ref, e)
						m.mu.Lock()
						s.pending--
						m.notifyLocked()
						m.mu.Unlock()
						<-m.slots
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
			if e != nil {
				m.mu.Lock()
				if m.failure == nil {
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
