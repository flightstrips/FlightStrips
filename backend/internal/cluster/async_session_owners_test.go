package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAsyncOwnerIdleReadsRefreshDurableEffectAndDispatchOnce(t *testing.T) {
	for _, first := range []string{"read", "active", "dispatch"} {
		t.Run(first, func(t *testing.T) {
			m, p, ref, gate, _ := asyncOwnersFixture(t)
			defer close(gate)
			id, connection := uuid.NewString(), uuid.NewString()
			waiting := &pb.EffectRecord{CommandId: id, TargetCid: "111111", OwnerEpoch: 1, Status: pb.EffectRecord_WAITING,
				ResultDeadline: timestamppb.New(time.Now().Add(time.Minute))}
			key, _ := Subject(ref)
			p.mu.Lock()
			p.states[key].Effects[id] = waiting
			p.presence = map[string]KVPresence{}
			p.mu.Unlock()
			require.NoError(t, m.Execute(context.Background(), ref, func(context.Context) error { return nil }))
			old := m.Control(ref)
			claimed := proto.Clone(waiting).(*pb.EffectRecord)
			claimed.Status = pb.EffectRecord_DISPATCH_CLAIMED
			claimed.DispatchConnectionId = &connection
			// A durable effect worker publishes outside RAM command admission.
			p.mu.Lock()
			next, err := cloneAggregate(p.states[key])
			require.NoError(t, err)
			next.Revision++
			next.StreamSequence++
			next.SubjectSequence++
			next.Effects[id] = claimed
			p.states[key] = next
			p.applied, p.highWater = next.StreamSequence, next.StreamSequence
			p.mu.Unlock()
			client := &pb.ClientPresence{SessionId: ref.GetSession().Id, ConnectionId: connection, NodeId: "node-a", Cid: "111111", Kind: pb.ClientPresence_EUROSCOPE, ConnectedAt: timestamppb.Now()}
			putTestPresence(p, client)
			var sends int
			fanout := &SessionFanout{NC: m.owner.NC, Projection: p, NodeID: "node-a", sockets: map[string]*socketEntry{connection: {
				presence: client, delivered: map[string]bool{}, socket: LocalSessionSocket{OnEffect: func(*pb.EffectRecord) error { sends++; return nil }},
			}}}
			if first == "active" {
				require.True(t, m.Active(ref))
			} else if first == "read" {
				view, err := m.Read(ref)
				require.NoError(t, err)
				require.Equal(t, pb.EffectRecord_DISPATCH_CLAIMED, view.Effects[id].Status)
			}
			require.NoError(t, fanout.SendToCID(context.Background(), ref.GetSession().Id, claimed))
			require.Error(t, fanout.SendToCID(context.Background(), ref.GetSession().Id, claimed))
			require.Equal(t, 1, sends)
			view, err := m.Read(ref)
			require.NoError(t, err)
			require.Equal(t, pb.EffectRecord_DISPATCH_CLAIMED, view.Effects[id].Status)
			require.Equal(t, pb.EffectRecord_WAITING, old.Effects[id].Status, "published RAM views must remain immutable")
		})
	}
}

// This protocol stub supplies CONNECTED transport status only. It does not
// implement JetStream; persistence in these unit tests is the CAS memory store.
func asyncConnectedTransport(t *testing.T) *nats.Conn {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	go func() {
		c, e := listener.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		fmt.Fprint(c, "INFO {\"server_id\":\"unit\",\"version\":\"2.15.0\",\"proto\":1,\"max_payload\":1048576}\r\n")
		scan := bufio.NewScanner(c)
		for scan.Scan() {
			if scan.Text() == "PING" {
				fmt.Fprint(c, "PONG\r\n")
			}
		}
	}()
	nc, err := nats.Connect("nats://"+listener.Addr().String(), nats.NoReconnect(), nats.Timeout(time.Second))
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	return nc
}

type asyncReceiptStore struct {
	*memoryStore
	gate    <-chan struct{}
	started chan struct{}
	once    sync.Once
}

func (s *asyncReceiptStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	s.once.Do(func() { close(s.started) })
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-s.gate:
	}
	return s.memoryStore.Publish(ctx, subject, expected, data)
}
func (s *asyncReceiptStore) Committed(ctx context.Context, seq uint64) (AppliedEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[seq-1]
	e.Data = append([]byte(nil), e.Data...)
	return e, nil
}
func asyncOwnersFixture(t *testing.T) (*AsyncSessionOwners, *Projection, *pb.AggregateRef, chan struct{}, *asyncReceiptStore) {
	t.Helper()
	return asyncAggregateFixture(t, sessionRef(42))
}

func asyncAggregateFixture(t *testing.T, ref *pb.AggregateRef) (*AsyncSessionOwners, *Projection, *pb.AggregateRef, chan struct{}, *asyncReceiptStore) {
	t.Helper()
	subject, _ := Subject(ref)
	mem := &memoryStore{}
	event := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, err := proto.Marshal(event)
	require.NoError(t, err)
	_, err = mem.Publish(context.Background(), subject, 0, data)
	require.NoError(t, err)
	p := &Projection{states: map[string]*Aggregate{}, listeners: map[uint64]*projectionListener{}, lastSnapshot: map[string]time.Time{}, sinceSnapshot: map[string]uint64{}, started: true, checked: time.Now(), positionReady: true, presenceReady: true, highWater: 1}
	require.NoError(t, p.apply(mem.entries[0]))
	owner := &OwnerRuntime{NC: asyncConnectedTransport(t), Projection: p, NodeID: "node-a", lastRenew: map[string]time.Time{subject: time.Now()}, failed: map[string]bool{}}
	gate := make(chan struct{})
	store := &asyncReceiptStore{memoryStore: mem, gate: gate, started: make(chan struct{})}
	owners := NewAsyncSessionOwners(p, owner, store)
	p.Async = owners
	return owners, p, ref, gate, store
}

func TestAsyncAirportAndGlobalMutationsUseRAMAndPersistInOrder(t *testing.T) {
	for _, ref := range []*pb.AggregateRef{globalRef(), airportRef("EKCH")} {
		t.Run(mustAsyncSubject(ref), func(t *testing.T) {
			m, p, ref, gate, store := asyncAggregateFixture(t, ref)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			writer := Writer{Store: store, Projection: p, Lease: m.owner, NodeID: "node-a", Plan: func(context.Context, *pb.CommandRequest, *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
				return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
			}}
			request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "ecfmp.test", Value: &pb.EntityRecord{Value: &pb.EntityRecord_ProviderCheckpoint{ProviderCheckpoint: &pb.ProviderCheckpoint{Provider: "ecfmp", Resource: "test"}}}}}}}}
			first := writer.Execute(ctx, request)
			require.True(t, first.MemoryAccepted, "%v", first)
			require.Nil(t, first.StreamSequence)
			<-store.started
			request.CommandId = uuid.NewString()
			second := writer.Execute(ctx, request)
			require.True(t, second.MemoryAccepted, "%v", second)
			view, err := p.Read(ref)
			require.NoError(t, err)
			require.Equal(t, uint64(2), view.Revision)
			p.mu.RLock()
			require.Zero(t, p.states[mustAsyncSubject(ref)].Revision, "queued mutations must not appear durable")
			p.mu.RUnlock()
			close(gate)
			require.NoError(t, m.Drain(ctx))
			p.mu.RLock()
			require.Equal(t, uint64(2), p.states[mustAsyncSubject(ref)].Revision)
			p.mu.RUnlock()
		})
	}
}
func asyncDomainEvent(ref *pb.AggregateRef, revision uint64) *pb.StateEvent {
	id := uuid.NewString()
	actor := &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}
	return &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, AggregateRevision: revision, OwnerEpoch: 1, CommandId: &id, Actor: actor, Fact: &pb.StateEvent_DomainChanged{DomainChanged: &pb.DomainChange{Outcome: &pb.CommandOutcome{CommandId: id, Actor: actor, Aggregate: ref, AggregateRevision: revision, RequestSha256: strings.Repeat("a", 64), Status: pb.CommandOutcome_SUCCEEDED}}}}
}
func TestAsyncOwnerAcknowledgesRAMBeforePersistenceAndDrainsFIFO(t *testing.T) {
	m, p, ref, gate, store := asyncOwnersFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var ids []string
	for i := 1; i <= 2; i++ {
		require.NoError(t, m.Execute(ctx, ref, func(turn context.Context) error {
			base, err := m.Read(ref)
			if err != nil {
				return err
			}
			event := asyncDomainEvent(ref, uint64(i))
			ids = append(ids, event.GetCommandId())
			reply, err := m.AcceptState(turn, base, event)
			if err == nil {
				require.True(t, reply.MemoryAccepted)
				require.Nil(t, reply.StreamSequence)
				require.Zero(t, reply.Outcome.CommittedStreamSequence)
				require.Equal(t, uint64(i), reply.GetAggregateRevision())
			}
			return err
		}))
	}
	<-store.started
	raw, err := p.readOwnedDurable(ref, "node-a")
	require.NoError(t, err)
	require.Zero(t, raw.Revision, "pending RAM must not enter durable state or snapshots")
	ram, err := m.Read(ref)
	require.NoError(t, err)
	require.Equal(t, uint64(2), ram.Revision)
	require.Equal(t, uint64(1), ram.StreamSequence, "RAM read must expose only durable stream checkpoint")
	require.True(t, m.Pending(ref))
	close(gate)
	require.NoError(t, m.Drain(ctx))
	require.False(t, m.Pending(ref))
	raw, err = p.readOwnedDurable(ref, "node-a")
	require.NoError(t, err)
	require.Equal(t, uint64(2), raw.Revision)
	require.Equal(t, uint64(2), raw.Ledger[ids[0]].CommittedStreamSequence)
	require.Equal(t, uint64(3), raw.Ledger[ids[1]].CommittedStreamSequence)
	require.Equal(t, uint64(1), p.applied, "background local apply must not forge ordered replay")
	require.Error(t, m.Execute(ctx, ref, func(context.Context) error { return nil }), "drain must seal later admissions")
}
func TestAsyncOwnerCapacityCancellationAndShutdownWaitsForAcceptedJobs(t *testing.T) {
	m, _, ref, gate, _ := asyncOwnersFixture(t)
	ctx := context.Background()
	started := make(chan struct{})
	var count atomic.Int32
	var sequence []int
	require.NoError(t, m.Execute(ctx, ref, func(turn context.Context) error {
		for i := 0; i < asyncSessionCapacity; i++ {
			n := i
			err := m.Append(turn, ref, func(context.Context) error { count.Add(1); return nil }, func(context.Context) error {
				if n == 0 {
					close(started)
					<-gate
				}
				sequence = append(sequence, n)
				return nil
			})
			if err != nil {
				return err
			}
		}
		canceled, cancel := context.WithCancel(turn)
		cancel()
		returnErr := m.Append(canceled, ref, func(context.Context) error { count.Add(1); return nil }, func(context.Context) error { return nil })
		require.ErrorIs(t, returnErr, context.Canceled)
		return nil
	}))
	<-started
	require.Equal(t, int32(asyncSessionCapacity), count.Load())
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, m.Drain(short), context.DeadlineExceeded)
	close(gate)
	flush, cancelFlush := context.WithTimeout(ctx, time.Second)
	defer cancelFlush()
	require.NoError(t, m.Drain(flush))
	require.Len(t, sequence, asyncSessionCapacity)
	for i, n := range sequence {
		require.Equal(t, i, n)
	}
}
func TestAsyncOwnerReentrancyAndFailureFreeze(t *testing.T) {
	m, _, ref, gate, _ := asyncOwnersFixture(t)
	close(gate)
	fault := errors.New("foreign owner generation")
	require.NoError(t, m.Execute(context.Background(), ref, func(turn context.Context) error {
		require.NoError(t, m.Execute(turn, ref, func(context.Context) error { return nil }))
		require.Error(t, m.Execute(turn, sessionRef(43), func(context.Context) error { return nil }))
		canceled, cancel := context.WithCancel(turn)
		cancel()
		require.ErrorIs(t, m.Append(canceled, ref, func(context.Context) error { t.Fatal("canceled admission mutated RAM"); return nil }, func(context.Context) error { return nil }), context.Canceled)
		return nil
	}))
	require.NotNil(t, m.Control(ref))
	m.Invalidate(ref, fault)
	require.Nil(t, m.Control(ref))
	require.ErrorIs(t, m.Err(), fault)
	require.False(t, m.Active(ref))
	require.ErrorIs(t, m.Drain(context.Background()), fault)
}

func TestAsyncOwnerKeepsPendingReceiptOnlyInRAM(t *testing.T) {
	m, p, ref, gate, store := asyncOwnersFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	event := asyncDomainEvent(ref, 1)
	require.NoError(t, m.Execute(ctx, ref, func(turn context.Context) error {
		base, err := m.Read(ref)
		if err != nil {
			return err
		}
		_, err = m.AcceptState(turn, base, event)
		return err
	}))
	<-store.started
	ram, err := m.Read(ref)
	require.NoError(t, err)
	require.NotNil(t, ram.Ledger[event.GetCommandId()])
	require.Zero(t, ram.Ledger[event.GetCommandId()].CommittedStreamSequence)
	p.mu.RLock()
	require.Nil(t, p.states[mustAsyncSubject(ref)].Ledger[event.GetCommandId()])
	p.mu.RUnlock()
	close(gate)
	require.NoError(t, m.Drain(ctx))
}

func TestAsyncOwnerAdmissionAllocationsDoNotScaleWithFleetSize(t *testing.T) {
	measure := func(fleet int) float64 {
		m, p, ref, gate, _ := asyncOwnersFixture(t)
		close(gate)
		p.mu.Lock()
		state := p.states[mustAsyncSubject(ref)]
		for i := 0; i < fleet; i++ {
			key := fmt.Sprintf("SAS%d", i)
			state.Entities[key] = &pb.EntitySnapshot{Key: key, Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: key, Bay: "CLEARED"}}}}
		}
		p.mu.Unlock()
		run := func() {
			if err := m.Execute(context.Background(), ref, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
		}
		run() // Initial detached baseline is intentionally proportional to its fleet.
		allocations := testing.AllocsPerRun(30, run)
		require.NoError(t, m.Drain(context.Background()))
		return allocations
	}
	small, large := measure(8), measure(2048)
	t.Logf("owner turn allocations fleet8=%.0f fleet2048=%.0f", small, large)
	require.LessOrEqual(t, large, small+10, "unchanged owner admission must not clone the complete fleet")
}

func TestAsyncOwnerInvalidationCannotRepublishCapturedControl(t *testing.T) {
	m, _, ref, gate, _ := asyncOwnersFixture(t)
	close(gate)
	require.NoError(t, m.Execute(context.Background(), ref, func(context.Context) error { return nil }))
	require.NotNil(t, m.Control(ref))
	// A worker may have captured a valid durable checkpoint just before a
	// different session invalidates the shared runtime. Resume that worker's
	// final control publication only after invalidation has cleared all views.
	captured := make(chan struct{})
	resume := make(chan struct{})
	done := make(chan struct{})
	go func() {
		m.mu.Lock()
		s := m.sessions[mustAsyncSubject(ref)]
		m.mu.Unlock()
		close(captured)
		<-resume
		m.mu.Lock()
		m.publishControlLocked(s)
		m.mu.Unlock()
		close(done)
	}()
	<-captured
	m.Invalidate(ref, errors.New("position replay generation changed"))
	close(resume)
	<-done
	require.Nil(t, m.Control(ref), "a worker completion must not revive invalidated RAM authority")
	require.Error(t, m.Drain(context.Background()))
}

func reclaimAsyncFixtureEpoch(t *testing.T, p *Projection, ref *pb.AggregateRef) {
	t.Helper()
	subject := mustAsyncSubject(ref)
	p.mu.RLock()
	old := p.states[subject]
	seq := old.StreamSequence + 1
	at := old.Owner.LeaseUntil.AsTime().Add(time.Millisecond)
	p.mu.RUnlock()
	claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, AggregateRevision: old.Revision, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 2}}}
	data, err := proto.Marshal(claim)
	require.NoError(t, err)
	require.NoError(t, p.apply(AppliedEvent{Subject: subject, StreamSequence: seq, SubjectSequence: seq, ServerTime: at, Data: data}))
}
func TestAsyncOwnerIdleReclaimedEpochRecoversReadAdmissionAndControl(t *testing.T) {
	for _, first := range []string{"read", "active", "execute"} {
		t.Run(first, func(t *testing.T) {
			m, p, ref, gate, _ := asyncOwnersFixture(t)
			close(gate)
			require.NoError(t, m.Execute(context.Background(), ref, func(context.Context) error { return nil }))
			previous := m.Control(ref)
			require.Equal(t, uint64(1), previous.Owner.Epoch)
			reclaimAsyncFixtureEpoch(t, p, ref)
			switch first {
			case "read":
				a, err := m.Read(ref)
				require.NoError(t, err)
				require.Equal(t, uint64(2), a.Owner.Epoch)
			case "active":
				require.True(t, m.Active(ref))
			case "execute":
				require.NoError(t, m.Execute(context.Background(), ref, func(context.Context) error { return nil }))
			}
			require.True(t, m.Active(ref))
			require.Equal(t, uint64(2), m.Control(ref).Owner.Epoch)
			require.Equal(t, uint64(1), previous.Owner.Epoch, "old published control must remain immutable")
			a, err := m.Read(ref)
			require.NoError(t, err)
			require.Equal(t, uint64(2), a.StreamSequence)
			require.NoError(t, m.Err())
			require.False(t, m.Pending(ref))
			require.NoError(t, m.Drain(context.Background()))
		})
	}
}
func TestAsyncOwnerReclaimedEpochFreezesUnpersistedOldTail(t *testing.T) {
	m, p, ref, gate, _ := asyncOwnersFixture(t)
	started := make(chan struct{})
	finished := make(chan struct{})
	var later atomic.Int32
	require.NoError(t, m.Execute(context.Background(), ref, func(turn context.Context) error {
		if err := m.Append(turn, ref, func(context.Context) error { return nil }, func(ctx context.Context) error { close(started); <-ctx.Done(); close(finished); return ctx.Err() }); err != nil {
			return err
		}
		return m.Append(turn, ref, func(context.Context) error { return nil }, func(context.Context) error { later.Add(1); return nil })
	}))
	<-started
	reclaimAsyncFixtureEpoch(t, p, ref)
	require.False(t, m.Active(ref))
	require.ErrorContains(t, m.Err(), "pending tail")
	require.Nil(t, m.Control(ref))
	<-finished
	close(gate)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.Error(t, m.Drain(ctx))
	require.Zero(t, later.Load(), "new owner term must never persist the old queued tail")
}
