package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (p *Projection) selectedPositionLocked(session int32, aircraft string) (KVPosition, bool) {
	state := p.acceptedStateLocked(fmt.Sprintf("fs.v1.state.session.%d", session))
	epoch := uint64(0)
	if state == nil {
		epoch = ^uint64(0)
	} else if state.Owner != nil {
		epoch = state.Owner.Epoch
	}
	fresh := p.operationalSyncLocked(state, session) != nil
	var selected KVPosition
	found := false
	for _, item := range p.positionViewLocked() {
		value := item.Value
		if value == nil || value.SessionId != session || value.AircraftKey != aircraft || value.OwnerEpoch > epoch || (fresh && value.OwnerEpoch != epoch) {
			continue
		}
		if !found || value.OwnerEpoch > selected.Value.OwnerEpoch || (value.OwnerEpoch == selected.Value.OwnerEpoch && item.Revision > selected.Revision) {
			item.Stale = !fresh || value.OwnerEpoch != epoch || state.Master == nil || value.SourceConnectionId != state.Master.ConnectionId
			selected, found = item, true
		}
	}
	return selected, found
}

func (p *Projection) watchPresence(ctx context.Context) {
	watcher, err := p.Presence.WatchAll(nats.Context(ctx))
	if err != nil {
		p.failObservation(err)
		return
	}
	defer watcher.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				p.failObservation(fmt.Errorf("presence watcher stopped"))
				return
			}
			p.mu.Lock()
			if entry == nil {
				p.presenceReady = true
				p.mu.Unlock()
				continue
			}
			if entry.Operation() != nats.KeyValuePut {
				if old, ok := p.presence[entry.Key()]; ok {
					p.publishObservationLocked(presenceSession(old.Value), presenceObservation(old, true))
				}
				delete(p.presence, entry.Key())
				p.mu.Unlock()
				continue
			}
			value := &pb.PresenceValue{}
			err := pb.UnmarshalStrict(entry.Value(), value)
			if err == nil {
				err = validateTyped(value.ProtoReflect())
			}
			key := ""
			if node := value.GetNode(); node != nil {
				key = "node." + node.NodeId
			}
			if client := value.GetClient(); client != nil {
				key = "client." + client.ConnectionId
				if client.ConnectionId == "" || client.NodeId == "" || client.SessionId < 1 ||
					client.Cid == "" || client.Kind == pb.ClientPresence_KIND_UNSPECIFIED ||
					client.ConnectedAt == nil || client.ConnectedAt.CheckValid() != nil {
					err = fmt.Errorf("invalid client presence identity")
				}
			}
			if node := value.GetNode(); node != nil && (node.NodeId == "" || node.StartedAt == nil ||
				node.StartedAt.CheckValid() != nil) {
				err = fmt.Errorf("invalid node presence identity")
			}
			if err == nil && (value.SchemaVersion != 1 || key == "" || entry.Key() != key || strings.Count(key, ".") != 1) {
				err = fmt.Errorf("presence key or schema mismatch")
			}
			if err != nil {
				p.observationErr = err
				p.wakeWaitersLocked()
				p.mu.Unlock()
				return
			}
			p.presence[entry.Key()] = KVPresence{Value: value, Revision: entry.Revision(), Observed: entry.Created()}
			p.publishObservationLocked(presenceSession(value), presenceObservation(p.presence[entry.Key()], false))
			p.mu.Unlock()
		}
	}
}

func (p *Projection) failObservation(err error) {
	p.mu.Lock()
	p.observationErr = err
	if p.Async != nil {
		p.Async.Invalidate(nil, err)
	}
	p.wakeWaitersLocked()
	p.mu.Unlock()
}

// ObservationSnapshot keeps KV revisions separate from the FS_STATE stream
// revision. Expired presence is filtered even if no delete notification arrived.
func (p *Projection) ObservationSnapshot(sessionID int32) ([]KVPosition, []KVPresence, error) {
	if err := p.readyForRead(); err != nil {
		return nil, nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.positionSnapshotLocked(sessionID), p.presenceSnapshotLocked(sessionID), nil
}

// WaitPositionApplied observes the exact accepted KV revision without a polling
// interval or cloning the entire position set for every single report.
func (p *Projection) WaitPositionApplied(ctx context.Context, session int32, aircraft string, epoch, revision uint64) error {
	ctx, span := otel.Tracer("cluster").Start(ctx, "euroscope.position.wait_applied")
	var ready, readLock, registerLock, notification time.Duration
	var rechecks int64
	defer func() {
		span.SetAttributes(
			attribute.Float64("position.wait_state_ready_ms", float64(ready)/float64(time.Millisecond)),
			attribute.Float64("position.wait_read_lock_ms", float64(readLock)/float64(time.Millisecond)),
			attribute.Float64("position.wait_register_lock_ms", float64(registerLock)/float64(time.Millisecond)),
			attribute.Float64("position.wait_notification_ms", float64(notification)/float64(time.Millisecond)),
			attribute.Int64("position.wait_rechecks", rechecks))
		span.End()
	}()
	key := positionKey(session, aircraft, epoch)
	for {
		rechecks++
		// One coherent read checks readiness and the exact revision. Only
		// replay lag needs the existing bounded state-readiness barrier.
		stage := time.Now()
		p.mu.RLock()
		readLock += time.Since(stage)
		stage = time.Now()
		if err := p.healthLocked(); err != nil {
			ready += time.Since(stage)
			p.mu.RUnlock()
			return err
		}
		if p.applied < p.highWater {
			p.mu.RUnlock()
			if err := ctx.Err(); err != nil {
				ready += time.Since(stage)
				return err
			}
			readyErr := p.readyForRead()
			ready += time.Since(stage)
			if readyErr != nil {
				return readyErr
			}
			continue
		}
		if p.history != nil {
			if err := p.history.check(); err != nil {
				ready += time.Since(stage)
				p.mu.RUnlock()
				return err
			}
		}
		ready += time.Since(stage)
		if item := p.positions[key]; item.Revision >= revision {
			p.mu.RUnlock()
			return nil
		}
		// Keep the publication read lock through registration. The watcher
		// cannot publish and wake this key between the missing check and insertion.
		// Notification bookkeeping never requires the publication write lock.
		stage = time.Now()
		waiter := p.registerPositionWaiterLocked(key)
		registerLock += time.Since(stage)
		changed := waiter.changed
		p.mu.RUnlock()
		stage = time.Now()
		select {
		case <-ctx.Done():
			notification += time.Since(stage)
			p.releasePositionWaiter(key, waiter)
			return ctx.Err()
		case <-changed:
			notification += time.Since(stage)
			p.releasePositionWaiter(key, waiter)
		}
	}
}

func (p *Projection) positionSnapshotLocked(sessionID int32) []KVPosition {
	selected := map[string]KVPosition{}
	state := p.acceptedStateLocked(fmt.Sprintf("fs.v1.state.session.%d", sessionID))
	var epoch uint64
	if state != nil && state.Owner != nil {
		epoch = state.Owner.Epoch
	} else if state == nil {
		// Diagnostic KV reads can inspect an orphan observation; an initial
		// frontend still requires an active session aggregate.
		epoch = ^uint64(0)
	}
	fresh := p.operationalSyncLocked(state, sessionID) != nil
	for _, item := range p.positionViewLocked() {
		if item.Value == nil || item.Value.SessionId != sessionID || item.Value.OwnerEpoch > epoch {
			continue
		}
		if fresh && item.Value.OwnerEpoch != epoch {
			continue
		}
		key := item.Value.AircraftKey
		old, exists := selected[key]
		if !exists || item.Value.OwnerEpoch > old.Value.OwnerEpoch || (item.Value.OwnerEpoch == old.Value.OwnerEpoch && item.Revision > old.Revision) {
			item.Stale = !fresh || item.Value.OwnerEpoch != epoch || state.Master == nil || item.Value.SourceConnectionId != state.Master.ConnectionId
			selected[key] = item
		}
	}
	result := make([]KVPosition, 0, len(selected))
	for _, item := range selected {
		item.Value = proto.Clone(item.Value).(*pb.PositionValue)
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Value.AircraftKey < result[j].Value.AircraftKey })
	return result
}

func (p *Projection) presenceSnapshotLocked(sessionID int32) []KVPresence {
	result := make([]KVPresence, 0)
	for _, item := range p.presence {
		if item.Value == nil || time.Since(item.Observed) >= 10*time.Second || item.Observed.Before(p.startedAt) {
			continue
		}
		if client := item.Value.GetClient(); client != nil && client.SessionId != sessionID {
			continue
		}
		result = append(result, KVPresence{Value: proto.Clone(item.Value).(*pb.PresenceValue), Revision: item.Revision, Observed: item.Observed})
	}
	sort.Slice(result, func(i, j int) bool { return presenceKey(result[i].Value) < presenceKey(result[j].Value) })
	return result
}

func presenceKey(v *pb.PresenceValue) string {
	if n := v.GetNode(); n != nil {
		return "node." + n.NodeId
	}
	if c := v.GetClient(); c != nil {
		return "client." + c.ConnectionId
	}
	return ""
}

// OperationalSync accepts a durable sync marker only for a currently live
// master connection and node. Both presence entries must have been renewed
// since this replica started, so a full cluster restart cannot briefly reuse
// TTL entries left by the previous process incarnations.
func (p *Projection) OperationalSync(ref *pb.AggregateRef) (*pb.SessionSync, error) {
	if ref == nil || ref.GetSession() == nil {
		return nil, fmt.Errorf("operational sync requires a session")
	}
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	if err := p.sessionReadHealth(ref); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.operationalSyncLocked(p.acceptedStateLocked(subject), ref.GetSession().GetId()), nil
}

func (p *Projection) operationalSyncLocked(state *Aggregate, sessionID int32) *pb.SessionSync {
	if state == nil || state.Sync == nil || state.Master == nil {
		return nil
	}
	memoryControl := p.Async != nil && p.Async.Control(sessionRef(sessionID)) == state
	if memoryControl && (state.Sync.CompletedAt == nil || state.Sync.CompletedAt.AsTime().Before(p.startedAt)) {
		return nil
	}
	if !memoryControl && p.syncFresh != nil {
		subject, _ := Subject(state.Ref)
		if !p.syncFresh[subject] {
			return nil
		}
	}
	master, sync := state.Master, state.Sync
	if state.Owner == nil || master.OwnerEpoch != state.Owner.Epoch || sync.ConnectionId != master.ConnectionId || sync.MasterEpoch != master.Epoch || sync.CompletedAt == nil {
		return nil
	}
	clientEntry := p.presence["client."+master.ConnectionId]
	client := clientEntry.Value.GetClient()
	if client == nil || client.ConnectionId != master.ConnectionId || client.Cid != master.Cid || client.SessionId != sessionID || client.Kind != pb.ClientPresence_EUROSCOPE || clientEntry.Observed.Before(p.startedAt) || time.Since(clientEntry.Observed) >= 10*time.Second {
		return nil
	}
	nodeEntry := p.presence["node."+client.NodeId]
	node := nodeEntry.Value.GetNode()
	if node == nil || !node.Ready || node.NodeId != client.NodeId || nodeEntry.Observed.Before(p.startedAt) || time.Since(nodeEntry.Observed) >= 10*time.Second {
		return nil
	}
	return proto.Clone(sync).(*pb.SessionSync)
}

func positionObservation(item KVPosition, stale, removed bool) *pb.FrontendObservation {
	return &pb.FrontendObservation{Value: &pb.FrontendObservation_Position{Position: proto.Clone(item.Value).(*pb.PositionValue)},
		SourceRevision: item.Revision, Stale: stale, Removed: removed, ObservedAt: timestamppb.New(item.Observed)}
}

func presenceObservation(item KVPresence, removed bool) *pb.FrontendObservation {
	return &pb.FrontendObservation{Value: &pb.FrontendObservation_Presence{Presence: proto.Clone(item.Value).(*pb.PresenceValue)},
		SourceRevision: item.Revision, Removed: removed, ObservedAt: timestamppb.New(item.Observed)}
}

func presenceSession(value *pb.PresenceValue) int32 {
	if client := value.GetClient(); client != nil {
		return client.SessionId
	}
	return 0 // node presence can affect every session's operational sync
}

// Called under the projection lock so an initial reader cannot miss a KV
// update between the observation copy and listener registration.
func (p *Projection) publishObservationLocked(sessionID int32, value *pb.FrontendObservation) {
	for id, listener := range p.listeners {
		if listener.observations == nil || (sessionID != 0 && listener.subject != fmt.Sprintf("fs.v1.state.session.%d", sessionID)) {
			continue
		}
		select {
		case listener.observations <- value:
		default:
			p.closeListenerLocked(id)
		}
	}
}

// SubscribeObservedInitial takes the state checkpoint and both KV views under
// one lock, after registering buffered delivery. State deltas and observations
// produced later arrive on separate channels with their own revisions.
func (p *Projection) SubscribeObservedInitial(sessionID int32) (*pb.FrontendInitial, <-chan *pb.FrontendDelta, <-chan *pb.FrontendObservation, func(), error) {
	if sessionID < 1 {
		return nil, nil, nil, nil, fmt.Errorf("invalid session")
	}
	if err := p.Ready(); err != nil {
		return nil, nil, nil, nil, err
	}
	subject, _ := Subject(sessionRef(sessionID))
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.states[subject]
	if state == nil {
		return nil, nil, nil, nil, fmt.Errorf("session not found")
	}
	sessionEntry := state.Entities[fmt.Sprint(sessionID)]
	session := sessionEntry.GetValue().GetSession()
	if session == nil || session.Tombstoned {
		return nil, nil, nil, nil, fmt.Errorf("session not active")
	}
	p.nextListener++
	id := p.nextListener
	listener := &projectionListener{subject: subject, updates: make(chan *pb.FrontendDelta, 1024), observations: make(chan *pb.FrontendObservation, 1024)}
	p.listeners[id] = listener
	closeFn := func() { p.mu.Lock(); p.closeListenerLocked(id); p.mu.Unlock() }
	initial := &pb.FrontendInitial{SessionId: sessionID, Airport: session.Airport, SessionName: session.Name,
		AggregateRevision: state.Revision, StreamSequence: state.StreamSequence, LayoutId: session.LayoutId,
		AvailableSids: session.AvailableSids, InitialCflByRunway: session.InitialCflByRunway,
		TransitionAltitudeFeet: session.TransitionAltitudeFeet, StandAssignmentEnabled: session.StandAssignmentEnabled}
	initial.Writable = p.operationalSyncLocked(state, sessionID) != nil
	initial.PositionAvailable = initial.Writable
	for _, entity := range state.Entities {
		initial.Entities = append(initial.Entities, proto.Clone(entity).(*pb.EntitySnapshot))
	}
	sort.Slice(initial.Entities, func(i, j int) bool {
		ki, _ := recordKind(initial.Entities[i].Value)
		kj, _ := recordKind(initial.Entities[j].Value)
		if ki != kj {
			return ki < kj
		}
		return initial.Entities[i].Key < initial.Entities[j].Key
	})
	for _, item := range p.positionSnapshotLocked(sessionID) {
		initial.Positions = append(initial.Positions, item.Value)
		initial.TaggedObservations = append(initial.TaggedObservations, positionObservation(item, item.Stale, false))
	}
	for _, item := range p.presenceSnapshotLocked(sessionID) {
		if client := item.Value.GetClient(); client != nil {
			initial.Clients = append(initial.Clients, client)
		}
		initial.TaggedObservations = append(initial.TaggedObservations, presenceObservation(item, false))
	}
	return initial, listener.updates, listener.observations, closeFn, nil
}

// RequirePositionRevision is the source check for a position-derived domain
// command. The worker must rederive on a mismatch rather than committing a
// stand, bay, or AMAN transition from an older observation.
func (p *Projection) RequirePositionRevision(sessionID int32, aircraft string, epoch, revision uint64) error {
	if !canonicalAircraft(aircraft) || epoch == 0 || revision == 0 {
		return fmt.Errorf("invalid source position")
	}
	positions, _, err := p.ObservationSnapshot(sessionID)
	if err != nil {
		return err
	}
	for _, item := range positions {
		if item.Value.AircraftKey == aircraft && item.Value.OwnerEpoch == epoch && item.Revision == revision && !item.Stale && item.Value.GetPosition() != nil {
			return nil
		}
	}
	return fmt.Errorf("source position observation changed")
}

// One accepted key wakes only callers waiting for that session/aircraft/epoch.
// Errors still broadcast through wakeWaitersLocked. Reference counts remove
// cancelled registrations even when the requested key never receives an update.
type positionWaitNotification struct {
	changed chan struct{}
	users   int
}

// Lock order is always publication mu (read or write), then positionWaitersMu.
// The caller retains a publication read lock until registration is complete.
func (p *Projection) registerPositionWaiterLocked(key string) *positionWaitNotification {
	p.positionWaitersMu.Lock()
	defer p.positionWaitersMu.Unlock()
	if p.positionWaiters == nil {
		p.positionWaiters = map[string]*positionWaitNotification{}
	}
	waiter := p.positionWaiters[key]
	if waiter == nil {
		waiter = &positionWaitNotification{changed: make(chan struct{})}
		p.positionWaiters[key] = waiter
	}
	waiter.users++
	return waiter
}

// Called after publication while holding the projection write lock.
func (p *Projection) wakePositionWaitersLocked(key string) {
	p.positionWaitersMu.Lock()
	defer p.positionWaitersMu.Unlock()
	if waiter := p.positionWaiters[key]; waiter != nil {
		close(waiter.changed)
		delete(p.positionWaiters, key)
	}
}

func (p *Projection) releasePositionWaiter(key string, waiter *positionWaitNotification) {
	p.positionWaitersMu.Lock()
	defer p.positionWaitersMu.Unlock()
	if current := p.positionWaiters[key]; current == waiter {
		waiter.users--
		if waiter.users == 0 {
			delete(p.positionWaiters, key)
		}
	}
}
