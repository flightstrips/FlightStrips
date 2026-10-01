package cluster

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"FlightStrips/internal/natsresources"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// Projection is a per-process, independent FS_STATE reader. Published state is
// never exposed until a complete event has passed the reducer.
type Projection struct {
	history                                    *historyCache
	NC                                         *nats.Conn
	JS                                         nats.JetStreamContext
	Config                                     natsresources.Config
	Snapshots                                  SnapshotStore
	Positions                                  nats.KeyValue
	Presence                                   nats.KeyValue
	mu                                         sync.RWMutex
	states                                     map[string]*Aggregate
	listeners                                  map[uint64]*projectionListener
	nextListener                               uint64
	applied, highWater                         uint64
	checked                                    time.Time
	lastCheckAttempt                           time.Time
	lastAppliedServerTime, highWaterServerTime time.Time
	healthErr                                  error
	started                                    bool
	startedAt                                  time.Time
	lastSnapshot                               map[string]time.Time
	sinceSnapshot                              map[string]uint64
	snapshotErrors                             map[string]error
	positions                                  map[string]KVPosition
	presence                                   map[string]KVPresence
	syncFresh                                  map[string]bool
	positionReady, presenceReady               bool
	observationErr                             error
	watchers                                   sync.WaitGroup
	snapshotJobs                               sync.WaitGroup
	takeovers, staleEpochs, snapshotFailures   atomic.Uint64
	snapshotSizeSkips                          atomic.Uint64
}

type KVPosition struct {
	Value    *pb.PositionValue
	Revision uint64
	Observed time.Time
	Stale    bool
}
type KVPresence struct {
	Value    *pb.PresenceValue
	Revision uint64
	Observed time.Time
}

type projectionListener struct {
	subject      string
	updates      chan *pb.FrontendDelta
	observations chan *pb.FrontendObservation
}

func NewProjection(nc *nats.Conn, cfg natsresources.Config) (*Projection, error) {
	if nc == nil {
		return nil, fmt.Errorf("missing NATS connection")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	js, err := nc.JetStream(nats.MaxWait(cfg.RequestTimeout))
	if err != nil {
		return nil, err
	}
	index, err := js.KeyValue(cfg.Names.SnapshotIndex)
	if err != nil {
		return nil, err
	}
	objects, err := js.ObjectStore(cfg.Names.Objects)
	if err != nil {
		return nil, err
	}
	positions, err := js.KeyValue(cfg.Names.Positions)
	if err != nil {
		return nil, err
	}
	presence, err := js.KeyValue(cfg.Names.Presence)
	if err != nil {
		return nil, err
	}
	return &Projection{NC: nc, JS: js, Config: cfg, Snapshots: SnapshotStore{Index: index, Objects: objects}, Positions: positions, Presence: presence, states: map[string]*Aggregate{}, listeners: map[uint64]*projectionListener{}, lastSnapshot: map[string]time.Time{}, sinceSnapshot: map[string]uint64{}, snapshotErrors: map[string]error{}, positions: map[string]KVPosition{}, presence: map[string]KVPresence{}, syncFresh: map[string]bool{}}, nil
}

// Run loads verified checkpoints, then consumes from the earliest safe stream
// sequence. The subject CAS checkpoint is the last global stream sequence on
// that subject, including events skipped through a verified snapshot.
func (p *Projection) Run(ctx context.Context) error {
	watchCtx, stopWatchers := context.WithCancel(ctx)
	defer func() {
		stopWatchers()
		p.watchers.Wait()
		p.snapshotJobs.Wait()
		if p.history != nil {
			_ = p.history.close()
		}
	}()
	if err := natsresources.Verify(ctx, p.NC, p.Config); err != nil {
		return err
	}
	var historyErr error
	p.history, historyErr = newHistoryCache()
	if historyErr != nil {
		return historyErr
	}
	p.watchers.Add(2)
	go func() { defer p.watchers.Done(); p.watchPositions(watchCtx) }()
	go func() { defer p.watchers.Done(); p.watchPresence(watchCtx) }()
	keys, err := p.Snapshots.Index.Keys()
	if err != nil && !errors.Is(err, nats.ErrNoKeysFound) {
		return err
	}
	for _, key := range keys {
		ref, err := refFromKey(key)
		if err != nil {
			return err
		}
		state, err := p.Snapshots.Load(ref)
		if err != nil {
			p.snapshotFailures.Add(1)
			return err
		}
		state.history = p.history
		if err := state.boundHistory(); err != nil {
			return err
		}
		subject, _ := Subject(ref)
		p.states[subject] = state
		p.lastSnapshot[subject] = time.Now()
	}
	info, err := p.JS.StreamInfo(p.Config.Names.State, &nats.StreamInfoRequest{SubjectsFilter: "fs.v1.state.>"}, nats.Context(ctx))
	if err != nil {
		return err
	}
	if info.State.Msgs > 0 && (info.State.FirstSeq != 1 || info.State.NumDeleted != 0) {
		return fmt.Errorf("FS_STATE history is incomplete")
	}
	for subject, state := range p.states {
		if state.StreamSequence > info.State.LastSeq {
			return fmt.Errorf("snapshot %s is beyond stream high-water", subject)
		}
	}
	start := uint64(1)
	if len(info.State.Subjects) > 0 {
		allCovered := true
		for subject := range info.State.Subjects {
			state := p.states[subject]
			if state == nil || state.StreamSequence == 0 || state.StreamSequence > info.State.LastSeq {
				allCovered = false
				break
			}
			if start == 1 || state.StreamSequence+1 < start {
				start = state.StreamSequence + 1
			}
		}
		if !allCovered {
			start = 1
		}
	}
	p.applied = start - 1
	sub, err := p.JS.SubscribeSync("fs.v1.state.>", nats.BindStream(p.Config.Names.State), nats.StartSequence(start), nats.OrderedConsumer())
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()
	p.mu.Lock()
	p.started = true
	p.startedAt = time.Now()
	p.mu.Unlock()
	for ctx.Err() == nil {
		readCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		msg, err := sub.NextMsgWithContext(readCtx)
		cancel()
		if err == nil {
			meta, e := msg.Metadata()
			if e != nil {
				p.fail(e)
				return e
			}
			if e = p.apply(AppliedEvent{Subject: msg.Subject, StreamSequence: meta.Sequence.Stream, ServerTime: meta.Timestamp, Data: msg.Data}); e != nil {
				p.fail(e)
				return e
			}
		} else if (errors.Is(err, nats.ErrDisconnected) || errors.Is(err, nats.ErrConnectionReconnecting)) && ctx.Err() == nil {
			p.fail(err)
			select {
			case <-ctx.Done():
			case <-time.After(100 * time.Millisecond):
			}
		} else if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, nats.ErrTimeout) && ctx.Err() == nil {
			p.fail(err)
			return err
		}
		p.refresh(ctx)
		p.maybeSnapshot()
	}
	p.fail(ctx.Err())
	return ctx.Err()
}

func (p *Projection) apply(entry AppliedEvent) error {
	ref, err := refFromSubject(entry.Subject)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry.StreamSequence <= p.applied {
		return fmt.Errorf("out of order stream sequence")
	}
	entry.SubjectSequence = entry.StreamSequence
	state := p.states[entry.Subject]
	if state == nil {
		state = NewAggregate(ref)
		state.history = p.history
		p.lastSnapshot[entry.Subject] = time.Now()
	}
	if entry.StreamSequence <= state.StreamSequence {
		p.applied = entry.StreamSequence
		return nil
	}
	if entry.SubjectSequence <= state.SubjectSequence {
		return fmt.Errorf("subject sequence regression")
	}
	// Clone before applying: a reader holding the former state sees an
	// immutable value even while new events arrive.
	clone := copyAggregateForApply(state)
	effective, err := clone.Apply(entry)
	if err != nil {
		return fmt.Errorf("stream %d: %w", entry.StreamSequence, err)
	}
	if err := clone.boundHistory(); err != nil {
		return err
	}
	p.states[entry.Subject] = clone
	p.applied = entry.StreamSequence
	p.lastAppliedServerTime = entry.ServerTime
	if p.syncFresh != nil && clone.Ref.GetSession() != nil && !proto.Equal(state.Sync, clone.Sync) {
		p.syncFresh[entry.Subject] = clone.Sync != nil && !entry.ServerTime.Before(p.startedAt)
	}
	if effective {
		e := &pb.StateEvent{}
		if err := pb.UnmarshalStrict(entry.Data, e); err != nil {
			return err
		}
		if e.GetOwnerClaimed() == nil && e.GetOwnerRenewed() == nil {
			delta := &pb.FrontendDelta{Aggregate: proto.Clone(ref).(*pb.AggregateRef), AggregateRevision: clone.Revision, StreamSequence: entry.StreamSequence}
			d := e.GetDomainChanged()
			for _, change := range d.GetChanges() {
				delta.Changes = append(delta.Changes, proto.Clone(change).(*pb.EntityChange))
			}
			for _, workflow := range d.GetWorkflows() {
				delta.Workflows = append(delta.Workflows, proto.Clone(workflow).(*pb.WorkflowRecord))
			}
			for id, listener := range p.listeners {
				if listener.subject != entry.Subject {
					continue
				}
				select {
				case listener.updates <- delta:
				default:
					p.closeListenerLocked(id)
				}
			}
		}
		if e.GetOwnerClaimed() != nil && state.Owner != nil && clone.Owner.GetEpoch() > state.Owner.GetEpoch() {
			p.takeovers.Add(1)
		}
	}
	p.sinceSnapshot[entry.Subject]++
	p.scheduleSnapshotLocked(entry.Subject, clone)
	return nil
}

func (p *Projection) scheduleSnapshotLocked(subject string, state *Aggregate) {
	if p.sinceSnapshot[subject] < 10000 && time.Since(p.lastSnapshot[subject]) < 5*time.Minute {
		return
	}
	p.sinceSnapshot[subject] = 0
	p.lastSnapshot[subject] = time.Now()
	// State is immutable after publication; I/O may continue outside the
	// reducer lock without exposing a partial checkpoint.
	p.snapshotJobs.Add(1)
	go func() { defer p.snapshotJobs.Done(); p.persistSnapshot(subject, state) }()
}

func (p *Projection) persistSnapshot(subject string, state *Aggregate) {
	err := p.Snapshots.Save(state)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.snapshotErrors == nil {
		p.snapshotErrors = make(map[string]error)
	}
	if errors.Is(err, ErrSnapshotTooLarge) {
		p.snapshotSizeSkips.Add(1)
		delete(p.snapshotErrors, subject)
		p.lastSnapshot[subject] = time.Now()
		return
	}
	if err != nil {
		p.snapshotErrors[subject] = err
		p.snapshotFailures.Add(1)
		if errors.Is(err, ErrImmutableSnapshotCollision) {
			p.lastSnapshot[subject] = time.Now()
		} else {
			p.lastSnapshot[subject] = time.Now().Add(-5*time.Minute + 5*time.Second)
		}
	} else {
		delete(p.snapshotErrors, subject)
	}
}

func (p *Projection) maybeSnapshot() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for subject, state := range p.states {
		if state.StreamSequence > 0 {
			p.scheduleSnapshotLocked(subject, state)
		}
	}
}

func cloneAggregate(a *Aggregate) (*Aggregate, error) {
	// This state has already passed Apply or verified snapshot loading. An
	// in-memory read needs detached values, not serialization, hashing and a
	// second validation of the entire retained command history.
	copy := NewAggregate(a.Ref)
	copy.history = a.history
	copy.Revision, copy.StreamSequence, copy.SubjectSequence = a.Revision, a.StreamSequence, a.SubjectSequence
	if a.Owner != nil {
		copy.Owner = proto.Clone(a.Owner).(*pb.OwnerTerm)
	}
	if a.Master != nil {
		copy.Master = proto.Clone(a.Master).(*pb.MasterTerm)
	}
	if a.Sync != nil {
		copy.Sync = proto.Clone(a.Sync).(*pb.SessionSync)
	}
	for key, value := range a.Entities {
		copy.Entities[key] = proto.Clone(value).(*pb.EntitySnapshot)
	}
	for key, value := range a.Ledger {
		copy.Ledger[key] = proto.Clone(value).(*pb.CommandOutcome)
	}
	for key, value := range a.Workflows {
		copy.Workflows[key] = proto.Clone(value).(*pb.WorkflowRecord)
	}
	for key, value := range a.Effects {
		copy.Effects[key] = proto.Clone(value).(*pb.EffectRecord)
	}
	copy.rebuildIndexes()
	return copy, nil
}

func copyAggregateForApply(a *Aggregate) *Aggregate {
	// Published protobuf records are immutable. Apply replaces changed records
	// and rebuilds changed indexes; only its maps and mutable owner term need
	// detaching. Snapshot jobs can keep reading the prior published state.
	copy := *a
	copy.Entities = maps.Clone(a.Entities)
	copy.Ledger = maps.Clone(a.Ledger)
	copy.Workflows = maps.Clone(a.Workflows)
	copy.Effects = maps.Clone(a.Effects)
	if a.Owner != nil {
		copy.Owner = proto.Clone(a.Owner).(*pb.OwnerTerm)
	}
	return &copy
}

// readOwner copies only the accepted control checkpoint. Ownership admission
// and renewal must not clone unrelated entities or retained command history.
func (p *Projection) readOwner(ref *pb.AggregateRef) (*ownerState, error) {
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	if err := p.Ready(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	view := &ownerState{Ref: proto.Clone(ref).(*pb.AggregateRef)}
	if state := p.states[subject]; state != nil {
		view.Revision, view.StreamSequence, view.SubjectSequence = state.Revision, state.StreamSequence, state.SubjectSequence
		if state.Owner != nil {
			view.Owner = proto.Clone(state.Owner).(*pb.OwnerTerm)
		}
	}
	return view, nil
}

func (p *Projection) refresh(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	// Poll resource/quorum metadata independently of event rate. Checking all
	// five resources after every event otherwise prevents a busy projection
	// from ever catching up to its own high-water mark.
	p.mu.Lock()
	if time.Since(p.lastCheckAttempt) < 250*time.Millisecond {
		p.mu.Unlock()
		return
	}
	p.lastCheckAttempt = time.Now()
	p.mu.Unlock()
	// A metadata read is required for readiness; stale cached high-water can
	// never keep a disconnected or minority node ready.
	check, cancel := context.WithTimeout(ctx, p.Config.RequestTimeout)
	defer cancel()
	if p.NC.Status() != nats.CONNECTED {
		p.fail(fmt.Errorf("NATS disconnected"))
		return
	}
	if err := natsresources.Verify(check, p.NC, p.Config); err != nil {
		p.fail(err)
		return
	}
	var highWater uint64
	var highWaterServerTime time.Time
	for _, name := range []string{p.Config.Names.State, "KV_" + p.Config.Names.Positions, "KV_" + p.Config.Names.Presence, "KV_" + p.Config.Names.SnapshotIndex, "OBJ_" + p.Config.Names.Objects} {
		info, err := p.JS.StreamInfo(name, nats.Context(check))
		if err != nil {
			p.fail(err)
			return
		}
		current := 0
		if info.Cluster != nil {
			for _, replica := range info.Cluster.Replicas {
				if replica.Current {
					current++
				}
			}
		}
		if info.Cluster == nil || info.Cluster.Leader == "" || len(info.Cluster.Replicas) != 2 || current < 1 {
			p.fail(fmt.Errorf("%s has no current quorum", name))
			return
		}
		if name == p.Config.Names.State {
			highWaterServerTime = info.State.LastTime
			if info.State.Msgs > 0 && (info.State.FirstSeq != 1 || info.State.NumDeleted != 0) {
				p.fail(fmt.Errorf("FS_STATE history is incomplete"))
				return
			}
			highWater = info.State.LastSeq
		}
	}
	p.mu.Lock()
	p.highWater = highWater
	p.checked = time.Now()
	p.highWaterServerTime = highWaterServerTime
	p.healthErr = nil
	p.mu.Unlock()
}

func (p *Projection) fail(err error) { p.mu.Lock(); p.healthErr = err; p.mu.Unlock() }

func (p *Projection) Ready() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if err := p.healthLocked(); err != nil {
		return err
	}
	if p.applied < p.highWater {
		return fmt.Errorf("replay behind stream: %d < %d", p.applied, p.highWater)
	}
	if p.history != nil {
		if err := p.history.check(); err != nil {
			return fmt.Errorf("history cache unavailable: %w", err)
		}
	}
	return nil
}

func (p *Projection) healthLocked() error {
	if !p.started {
		return fmt.Errorf("projection not started")
	}
	if p.healthErr != nil {
		return p.healthErr
	}
	if p.observationErr != nil {
		return p.observationErr
	}
	for _, err := range p.snapshotErrors {
		if !errors.Is(err, ErrImmutableSnapshotCollision) {
			return err
		}
	}
	if !p.positionReady || !p.presenceReady {
		return fmt.Errorf("KV observation replay incomplete")
	}
	if p.checked.IsZero() || time.Since(p.checked) > 2*time.Second {
		return fmt.Errorf("state metadata is stale")
	}
	return nil
}

func (p *Projection) Read(ref *pb.AggregateRef) (*Aggregate, error) {
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	if err := p.Ready(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if state := p.states[subject]; state != nil {
		return cloneAggregate(state)
	}
	return NewAggregate(ref), nil
}

// ReadEntity returns a detached accepted entity through the readiness barrier.
func (p *Projection) ReadEntity(ref *pb.AggregateRef, kind pb.EntityKind, key string) (*pb.EntitySnapshot, error) {
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	if err := p.Ready(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if state := p.states[subject]; state != nil {
		if entity := state.Indexes[kind][key]; entity != nil {
			return proto.Clone(entity).(*pb.EntitySnapshot), nil
		}
	}
	return nil, nil
}

// ReadOwner returns the detached accepted owner term through the readiness
// barrier, without copying domain state or retained command history.
func (p *Projection) ReadOwner(ref *pb.AggregateRef) (*pb.OwnerTerm, error) {
	state, err := p.readOwner(ref)
	if err != nil {
		return nil, err
	}
	return state.Owner, nil
}

// ReadEntities returns detached entities of one kind without copying retained
// command outcomes or workflows. It uses the same readiness barrier as Read.
func (p *Projection) ReadEntities(ref *pb.AggregateRef, kind pb.EntityKind) ([]*pb.EntitySnapshot, error) {
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	if err := p.Ready(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	var entities []*pb.EntitySnapshot
	if state := p.states[subject]; state != nil {
		for _, entity := range state.EntitiesByKind(kind) {
			entities = append(entities, proto.Clone(entity).(*pb.EntitySnapshot))
		}
	}
	return entities, nil
}

// Outcome finds a command across aggregate ledgers for the HTTP command-status
// URL, which intentionally does not expose an aggregate selector. Only the
// authenticated actor receives the stored outcome.
func (p *Projection) Outcome(_ context.Context, commandID string, actor *pb.Actor) *pb.CommandReply {
	reply := &pb.CommandReply{ProtocolRevision: 1, CommandId: commandID}
	if !canonicalUUID(commandID) || actor == nil {
		reply.Status = pb.CommandReply_INVALID_ARGUMENT
		return reply
	}
	if err := p.Ready(); err != nil {
		reply.Status, reply.Detail = pb.CommandReply_UNAVAILABLE, err.Error()
		return reply
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	reply.Status = pb.CommandReply_NOT_FOUND
	for _, state := range p.states {
		outcome, err := state.LookupOutcome(commandID)
		if err != nil {
			return unavailable(commandID)
		}
		if outcome == nil {
			continue
		}
		if !outcomeActorMatches(actor, outcome.Actor) {
			return &pb.CommandReply{ProtocolRevision: 1, CommandId: commandID, Status: pb.CommandReply_UNAUTHORIZED}
		}
		if reply.Outcome != nil {
			return &pb.CommandReply{ProtocolRevision: 1, CommandId: commandID, Status: pb.CommandReply_UNAVAILABLE, Detail: "command ID exists in multiple aggregates"}
		}
		reply.Status = statusForOutcome(outcome)
		reply.AggregateRevision, reply.StreamSequence = &outcome.AggregateRevision, &outcome.CommittedStreamSequence
		reply.Outcome = proto.Clone(outcome).(*pb.CommandOutcome)
	}
	return reply
}

func outcomeActorMatches(query, stored *pb.Actor) bool {
	if query == nil || stored == nil || query.Id != stored.Id {
		return false
	}
	if query.Kind == pb.Actor_KIND_UNSPECIFIED {
		return stored.Kind == pb.Actor_PILOT || stored.Kind == pb.Actor_CONTROLLER
	}
	if query.Kind != stored.Kind {
		return false
	}
	// The HTTP status URL has no session selector. A pilot's authenticated CID
	// remains the identity across sessions and callsign changes.
	return (query.Kind == pb.Actor_PILOT || query.Kind == pb.Actor_CONTROLLER) && query.SessionId == nil || proto.Equal(query, stored)
}

// FlightSnapshot is the typed read used by the candidate pilot HTTP adapter.
type FlightSnapshot struct {
	SessionID     int32
	Airport       string
	Strip         *pb.Strip
	PDC           *pb.PdcSequence
	PDCRevision   uint64
	Stand         *pb.StandAssignment
	StandRevision uint64
	CDM           *pb.CdmState
	CDMRevision   uint64
}

var ErrFlightNotFound = errors.New("strip not found")
var ErrAmbiguousFlight = errors.New("callsign matched multiple sessions")

func (p *Projection) FindFlight(_ context.Context, callsign string) (*FlightSnapshot, error) {
	if err := p.Ready(); err != nil {
		return nil, err
	}
	key := strings.ToUpper(strings.TrimSpace(callsign))
	if key == "" {
		return nil, fmt.Errorf("callsign is required")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	var found *FlightSnapshot
	for _, state := range p.states {
		ref := state.Ref.GetSession()
		if ref == nil {
			continue
		}
		strip := state.Indexes[pb.EntityKind_STRIP][key]
		if strip == nil {
			continue
		}
		if found != nil {
			return nil, ErrAmbiguousFlight
		}
		found = &FlightSnapshot{SessionID: ref.Id, Strip: proto.Clone(strip.GetValue().GetStrip()).(*pb.Strip)}
		if session := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(ref.Id)]; session != nil {
			found.Airport = session.GetValue().GetSession().Airport
		}
		if record := state.Indexes[pb.EntityKind_PDC_SEQUENCE][key]; record != nil {
			found.PDC, found.PDCRevision = proto.Clone(record.GetValue().GetPdcSequence()).(*pb.PdcSequence), record.Revision
		}
		if record := state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][key]; record != nil {
			found.Stand, found.StandRevision = proto.Clone(record.GetValue().GetStandAssignment()).(*pb.StandAssignment), record.Revision
		}
		if record := state.Indexes[pb.EntityKind_CDM_STATE][key]; record != nil {
			found.CDM, found.CDMRevision = proto.Clone(record.GetValue().GetCdmState()).(*pb.CdmState), record.Revision
		}
	}
	if found == nil {
		return nil, ErrFlightNotFound
	}
	return found, nil
}

// WaitApplied is the local PubAck barrier. A stalled consumer returns a
// timeout; the caller must retry the same command ID or query its outcome.
func (p *Projection) WaitApplied(ctx context.Context, sequence uint64) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		p.mu.RLock()
		err := p.healthLocked()
		applied := p.applied
		p.mu.RUnlock()
		if err != nil {
			return err
		}
		if applied >= sequence {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *Projection) WaitSubjectAdvance(ctx context.Context, subject string, previous uint64) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		p.mu.RLock()
		err := p.healthLocked()
		current := uint64(0)
		if state := p.states[subject]; state != nil {
			current = state.SubjectSequence
		}
		p.mu.RUnlock()
		if err != nil {
			return err
		}
		if current > previous {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// SubscribeInitial registers before exposing the snapshot, so later deltas
// cannot be lost between initial read and live delivery. Channel overflow
// closes delivery and requires the client to resynchronize.
func (p *Projection) SubscribeInitial(ref *pb.AggregateRef) (*Aggregate, <-chan *pb.FrontendDelta, func(), error) {
	subject, err := Subject(ref)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := p.Ready(); err != nil {
		return nil, nil, nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.states[subject]
	if state == nil {
		state = NewAggregate(ref)
	}
	p.nextListener++
	id := p.nextListener
	updates := make(chan *pb.FrontendDelta, 1024)
	p.listeners[id] = &projectionListener{subject: subject, updates: updates}
	closeFn := func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if _, ok := p.listeners[id]; ok {
			p.closeListenerLocked(id)
		}
	}
	initial, err := cloneAggregate(state)
	if err != nil {
		close(updates)
		delete(p.listeners, id)
		return nil, nil, nil, err
	}
	return initial, updates, closeFn, nil
}

func (p *Projection) closeListenerLocked(id uint64) {
	listener := p.listeners[id]
	if listener == nil {
		return
	}
	close(listener.updates)
	if listener.observations != nil {
		close(listener.observations)
	}
	delete(p.listeners, id)
}

func (p *Projection) Readyz(w http.ResponseWriter, _ *http.Request) {
	if err := p.Ready(); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func refFromSubject(subject string) (*pb.AggregateRef, error) {
	if !strings.HasPrefix(subject, "fs.v1.state.") {
		return nil, fmt.Errorf("unexpected state subject")
	}
	return refFromKey(strings.TrimPrefix(subject, "fs.v1.state."))
}

func refFromKey(key string) (*pb.AggregateRef, error) {
	parts := strings.Split(key, ".")
	switch {
	case key == "global":
		return &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}, nil
	case len(parts) == 2 && parts[0] == "airport":
		ref := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: parts[1]}}}
		canonical, err := snapshotKey(ref)
		if err != nil || canonical != key {
			return nil, fmt.Errorf("noncanonical aggregate key")
		}
		return ref, nil
	case len(parts) == 2 && parts[0] == "session":
		var id int32
		if _, err := fmt.Sscan(parts[1], &id); err != nil {
			return nil, err
		}
		ref := &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: id}}}
		canonical, err := snapshotKey(ref)
		if err != nil || canonical != key {
			return nil, fmt.Errorf("noncanonical aggregate key")
		}
		return ref, nil
	}
	return nil, fmt.Errorf("invalid aggregate key %q", key)
}
