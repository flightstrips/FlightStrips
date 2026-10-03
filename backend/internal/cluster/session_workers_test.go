package cluster

import (
	"context"
	"fmt"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSessionPolicyRunsWithInitialAndSelfAcceptedRAMBacklog(t *testing.T) {
	for _, initialBacklog := range []bool{false, true} {
		t.Run(fmt.Sprintf("initial_backlog_%t", initialBacklog), func(t *testing.T) {
			owners, p, ref, gate, store := asyncOwnersFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			defer func() {
				close(gate)
				cleanup, stop := context.WithTimeout(context.Background(), time.Second)
				defer stop()
				_ = owners.Drain(cleanup)
			}()
			key, _ := Subject(ref)
			p.mu.Lock()
			seed := &pb.EntitySnapshot{Key: "42", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 42, Airport: "EKCH", Name: "LIVE", NextStripId: 1}}}}
			p.states[key].Entities["42"] = seed
			p.states[key].Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"42": seed}
			p.mu.Unlock()
			now := time.Now()
			setWorkerPresence(p, now, now.Add(-time.Hour), pb.ClientPresence_EUROSCOPE)
			p.presence["client.client-a"].Value.GetClient().SessionId = 42
			accept := func(run context.Context) error {
				return owners.Execute(run, ref, func(turn context.Context) error {
					base, err := owners.Read(ref)
					if err != nil {
						return err
					}
					_, err = owners.AcceptState(turn, base, asyncDomainEvent(ref, base.Revision+1))
					return err
				})
			}
			if initialBacklog {
				require.NoError(t, accept(ctx))
				<-store.started
				require.False(t, owners.owner.CanWrite(ref))
			}
			var calls []string
			work := &SessionWork{Projection: p, Owner: owners.owner, Store: LocalLifecycleStore{Writer: Writer{Store: store, Projection: p, Lease: owners.owner, NodeID: owners.owner.NodeID}},
				EuroScope: func(run context.Context, _ int32) error { calls = append(calls, "recover"); return accept(run) },
				Departure: func(context.Context, int32) error { calls = append(calls, "SAT"); return nil },
				Traffic:   func(context.Context, int32) error { calls = append(calls, "traffic"); return nil }}
			// ReconcileSession uses the same entry gate and callback sequence as
			// Step, without relying on a periodic timer's phase.
			require.NoError(t, work.ReconcileSession(ctx, 42))
			require.Equal(t, []string{"recover", "SAT", "traffic"}, calls)
			require.True(t, owners.Pending(ref))
			require.False(t, owners.owner.CanWrite(ref), "external durable work must still wait for persistence")
			require.True(t, work.canPlan(ref))
			policy, err := work.readPolicy(ctx, ref)
			require.NoError(t, err)
			require.Empty(t, policy.Ledger, "timer reads must not clone retained receipts")
			policy.Indexes[pb.EntityKind_SESSION]["42"].GetValue().GetSession().Name = "changed"
			seed, err = p.ReadEntity(ref, pb.EntityKind_SESSION, "42")
			require.NoError(t, err)
			require.Equal(t, "LIVE", seed.GetValue().GetSession().Name, "policy records must remain detached")
		})
	}
}

func TestSessionRAMPolicyKeepsOwnerHealthFences(t *testing.T) {
	for _, fault := range []string{"stale-renewal", "foreign-owner", "disconnected", "queue-error"} {
		t.Run(fault, func(t *testing.T) {
			owners, p, ref, gate, _ := asyncOwnersFixture(t)
			defer close(gate)
			key, _ := Subject(ref)
			switch fault {
			case "stale-renewal":
				owners.owner.lastRenew[key] = time.Now().Add(-ownerLease)
			case "foreign-owner":
				p.states[key].Owner = proto.Clone(p.states[key].Owner).(*pb.OwnerTerm)
				p.states[key].Owner.NodeId = "node-b"
				p.states[key].Owner.Epoch++
			case "disconnected":
				owners.owner.NC.Close()
			case "queue-error":
				owners.Invalidate(ref, fmt.Errorf("persistence failed"))
			}
			called := false
			work := &SessionWork{Projection: p, Owner: owners.owner, Store: LocalLifecycleStore{Writer: Writer{Store: owners.store}}, Traffic: func(context.Context, int32) error { called = true; return nil }}
			require.False(t, work.canPlan(ref))
			require.Error(t, work.ReconcileSession(context.Background(), 42))
			require.False(t, called)
		})
	}
}

func TestSessionRAMPolicyRejectsUnsupportedOwnerGate(t *testing.T) {
	owners, p, ref, gate, _ := asyncOwnersFixture(t)
	defer close(gate)
	owner := &workerLease{active: true}
	work := &SessionWork{Projection: p, Owner: owner}
	require.False(t, work.canPlan(ref), "a legacy writable gate cannot prove RAM owner eligibility")
	p.Async = nil
	require.True(t, work.canPlan(ref), "non-memory policy retains its legacy writable gate")
	owners.Invalidate(ref, fmt.Errorf("fixture complete"))
}

type workerLease struct{ active bool }

func (l *workerLease) CanWrite(*pb.AggregateRef) bool { return l.active }
func (*workerLease) Track(*pb.AggregateRef) error     { return nil }

func workerFixture(t *testing.T) (*memoryStore, *SessionWork, *SessionWorkerPlanner, *Projection, *time.Time, *workerLease) {
	t.Helper()
	store, registry := registryFixture(t, 1)
	if _, err := registry.GetOrCreateSession(context.Background(), "EKCH", "LIVE"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	p := &Projection{started: true, startedAt: now.Add(-time.Hour), checked: time.Now(), positionReady: true, presenceReady: true,
		presence: map[string]KVPresence{}, positions: map[string]KVPosition{}}
	setWorkerPresence(p, now, now.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	reader := Writer{Store: store}
	base := SessionLifecyclePlanner(func(ctx context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
		subject, err := Subject(ref)
		if err != nil {
			return nil, err
		}
		return reader.load(ctx, subject, ref)
	})
	planner := &SessionWorkerPlanner{Next: base, Projection: p, Now: func() time.Time { return now }}
	writer := Writer{Store: store, NodeID: "node-a", Plan: planner.Plan}
	adapter := LocalLifecycleStore{Writer: writer}
	lease := &workerLease{active: true}
	w := &SessionWork{Registry: SessionRegistry{Store: adapter}, Store: adapter, Projection: p, Owner: lease,
		Now: func() time.Time { return now }}
	return store, w, planner, p, &now, lease
}

func setWorkerPresence(p *Projection, observed, started time.Time, kind pb.ClientPresence_Kind) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.presence = map[string]KVPresence{"node.node-a": {Value: &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{NodeId: "node-a", Ready: true, StartedAt: timestamppb.New(started)}}}, Observed: observed}}
	if kind != pb.ClientPresence_KIND_UNSPECIFIED {
		p.presence["client.client-a"] = KVPresence{Value: &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Client{Client: &pb.ClientPresence{ConnectionId: "client-a", NodeId: "node-a", SessionId: 1, Cid: "cid-a", Callsign: "EKCH_A_TWR", Kind: kind, ConnectedAt: timestamppb.New(observed)}}}, Observed: observed}
	}
}

func workerState(t *testing.T, w *SessionWork) *Aggregate {
	t.Helper()
	state, err := w.Store.Read(context.Background(), sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestSessionCleanupRequiresFiveHealthyMinutesAndEuroScopePresence(t *testing.T) {
	_, w, _, p, now, _ := workerFixture(t)
	ctx := context.Background()
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	marker := workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt
	if marker == nil {
		t.Fatal("frontend-only socket kept session alive")
	}
	*now = now.Add(5*time.Minute + time.Second)
	setWorkerPresence(p, *now, marker.AsTime().Add(-time.Hour), pb.ClientPresence_EUROSCOPE)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().Tombstoned {
		t.Fatal("live EuroScope controller did not stop cleanup")
	}
	if workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt != nil {
		t.Fatal("returning controller did not clear marker")
	}
	setWorkerPresence(p, *now, marker.AsTime().Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(4*time.Minute + 59*time.Second)
	setWorkerPresence(p, *now, marker.AsTime().Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().Tombstoned {
		t.Fatal("cleanup fired before five minutes")
	}
	*now = now.Add(2 * time.Second)
	setWorkerPresence(p, *now, marker.AsTime().Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if !workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().Tombstoned {
		t.Fatal("due cleanup did not tombstone")
	}
}

func TestSessionCleanupStartsWhenLastEuroScopeClientDisconnects(t *testing.T) {
	_, w, _, p, now, _ := workerFixture(t)
	ctx := context.Background()
	setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_EUROSCOPE)
	p.mu.Lock()
	p.presence["client.client-b"] = KVPresence{Value: &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Client{Client: &pb.ClientPresence{
		ConnectionId: "client-b", NodeId: "node-a", SessionId: 1, Cid: "cid-b", Callsign: "EKCH_B_TWR",
		Kind: pb.ClientPresence_EUROSCOPE, ConnectedAt: timestamppb.New(*now),
	}}}, Observed: *now}
	p.mu.Unlock()
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt != nil {
		t.Fatal("session started expiring with two clients connected")
	}
	*now = now.Add(time.Minute)
	p.mu.Lock()
	delete(p.presence, "client.client-a")
	p.presence["node.node-a"] = KVPresence{Value: &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{
		NodeId: "node-a", Ready: true, StartedAt: timestamppb.New(now.Add(-time.Hour)),
	}}}, Observed: *now}
	remaining := p.presence["client.client-b"]
	remaining.Observed = *now
	p.presence["client.client-b"] = remaining
	p.mu.Unlock()
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt != nil {
		t.Fatal("first disconnect started expiration while another client remained")
	}
	*now = now.Add(time.Minute)
	setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	marker := workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt
	if marker == nil || !marker.AsTime().Equal(*now) {
		t.Fatalf("last disconnect did not start the five-minute window: %v", marker)
	}
}

func TestSessionCleanupPausesOnQuorumLossAndRestartsObservation(t *testing.T) {
	_, w, _, p, now, _ := workerFixture(t)
	ctx := context.Background()
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	old := workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt.AsTime()
	p.mu.Lock()
	p.observationErr = context.DeadlineExceeded
	p.mu.Unlock()
	if err := w.Step(ctx); err == nil {
		t.Fatal("unhealthy projection permitted work")
	}
	*now = now.Add(20 * time.Minute)
	p.mu.Lock()
	p.observationErr = nil
	p.mu.Unlock()
	setWorkerPresence(p, *now, old.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	s := workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession()
	if s.Tombstoned || !s.FirstNoControllerAt.AsTime().Equal(*now) {
		t.Fatalf("quorum recovery did not pause observation: marker=%v now=%v pause=%v", s.FirstNoControllerAt, *now, s.CleanupPausedAt)
	}
	// A full backend restart has no local unhealthy flag. New node
	// incarnations still force a fresh observation after state replay.
	*now = now.Add(20 * time.Minute)
	setWorkerPresence(p, *now, now.Add(-time.Second), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	s = workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession()
	if s.Tombstoned || !s.FirstNoControllerAt.AsTime().Equal(*now) {
		t.Fatal("full restart deleted session immediately")
	}
}

func TestSessionCleanupPreservesRemainingHealthyTime(t *testing.T) {
	_, w, _, p, now, _ := workerFixture(t)
	ctx := context.Background()
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	start := *now
	*now = now.Add(2 * time.Minute)
	setWorkerPresence(p, *now, start.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	p.mu.Lock()
	p.observationErr = context.DeadlineExceeded
	p.mu.Unlock()
	if err := w.Step(ctx); err == nil {
		t.Fatal("quorum failure was not seen")
	}
	*now = now.Add(10 * time.Minute)
	p.mu.Lock()
	p.observationErr = nil
	p.mu.Unlock()
	setWorkerPresence(p, *now, start.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	marker := workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt.AsTime()
	if !marker.Equal(start.Add(10 * time.Minute)) {
		t.Fatalf("outage was counted as healthy time: marker=%v", marker)
	}
	*now = now.Add(2*time.Minute + 59*time.Second)
	setWorkerPresence(p, *now, start.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().Tombstoned {
		t.Fatal("cleanup ran before remaining healthy time elapsed")
	}
	*now = now.Add(2 * time.Second)
	setWorkerPresence(p, *now, start.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if !workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().Tombstoned {
		t.Fatal("cleanup did not run after remaining healthy time")
	}
}

func TestSessionCleanupExtendsPauseWhenRecoveryFailsAgain(t *testing.T) {
	_, w, _, p, now, lease := workerFixture(t)
	ctx := context.Background()
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	start := *now
	*now = now.Add(2 * time.Minute)
	p.mu.Lock()
	p.observationErr = context.DeadlineExceeded
	p.mu.Unlock()
	if err := w.Step(ctx); err == nil {
		t.Fatal("unhealthy projection permitted work")
	}
	*now = now.Add(10 * time.Second)
	p.mu.Lock()
	p.observationErr = nil
	p.mu.Unlock()
	lease.active = false
	setWorkerPresence(p, *now, start.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	// Metadata can briefly recover before an accepted owner is available.
	// A renewed outage must not leave the short first recovery interval frozen.
	p.mu.Lock()
	p.observationErr = context.DeadlineExceeded
	p.mu.Unlock()
	if err := w.Step(ctx); err == nil {
		t.Fatal("renewed outage permitted work")
	}
	*now = now.Add(10 * time.Minute)
	p.mu.Lock()
	p.observationErr = nil
	p.mu.Unlock()
	lease.active = true
	setWorkerPresence(p, *now, start.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	marker := workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt.AsTime()
	if !marker.Equal(start.Add(10*time.Minute + 10*time.Second)) {
		t.Fatalf("renewed outage was counted as healthy time: marker=%v", marker)
	}
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().Tombstoned {
		t.Fatal("session deleted after incomplete recovery")
	}
}

func TestSessionCleanupExtendsOnlyUnrecordedPause(t *testing.T) {
	_, w, _, p, now, _ := workerFixture(t)
	ctx := context.Background()
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	start := *now
	unhealthy := start.Add(2 * time.Minute)
	*now = unhealthy.Add(10 * time.Second)
	setWorkerPresence(p, *now, start.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.stepSession(ctx, &pb.SessionRegistry{Id: 1}, unhealthy, *now, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(10 * time.Minute)
	setWorkerPresence(p, *now, start.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.stepSession(ctx, &pb.SessionRegistry{Id: 1}, unhealthy, *now, now.Sub(unhealthy)); err != nil {
		t.Fatal(err)
	}
	marker := workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt.AsTime()
	if !marker.Equal(start.Add(10*time.Minute + 10*time.Second)) {
		t.Fatalf("pause double counted: marker=%v", marker)
	}
	if err := w.stepSession(ctx, &pb.SessionRegistry{Id: 1}, unhealthy, *now, now.Sub(unhealthy)); err != nil {
		t.Fatal(err)
	}
	if !workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt.AsTime().Equal(marker) {
		t.Fatal("persisted pause was applied twice")
	}
}

func TestSessionDeadlineFiresOnceAfterTakeover(t *testing.T) {
	store, w, _, p, now, lease := workerFixture(t)
	ctx := context.Background()
	due := timestamppb.New(now.Add(-time.Second))
	d := &pb.SessionDeadline{Id: "session-update.1", Kind: "session-update", DueAt: due, SourceRevision: 1}
	if err := w.ScheduleDeadline(ctx, 1, d); err != nil {
		t.Fatal(err)
	}
	w.SessionUpdate = func(context.Context, int32) error { return nil }
	lease.active = false
	before := store.commits
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if store.commits != before {
		t.Fatal("non-owner fired deadline")
	}
	lease.active = true
	setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_EUROSCOPE)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	state := workerState(t, w)
	if state.Indexes[pb.EntityKind_SESSION_DEADLINE][d.Id] != nil {
		t.Fatal("takeover did not consume deadline")
	}
	after := store.commits
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if store.commits != after {
		t.Fatal("completed PDC deadline fired twice")
	}
}

func TestPDCDeadlineRechecksSourceRevisionAndDueTime(t *testing.T) {
	now := time.Now().UTC()
	due := timestamppb.New(now.Add(-time.Second))
	state := NewAggregate(sessionRef(1))
	sequence := &pb.EntitySnapshot{Key: "SAS101", Revision: 4, Value: &pb.EntityRecord{Value: &pb.EntityRecord_PdcSequence{PdcSequence: &pb.PdcSequence{Callsign: "SAS101", State: "CLEARED", Deadline: due}}}}
	deadline := &pb.EntitySnapshot{Key: "pdc.SAS101", Revision: 2, Value: &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{Id: "pdc.SAS101", Kind: "pdc-response", Callsign: "SAS101", DueAt: due, SourceRevision: 4}}}}
	state.Indexes[pb.EntityKind_PDC_SEQUENCE] = map[string]*pb.EntitySnapshot{"SAS101": sequence}
	state.Indexes[pb.EntityKind_SESSION_DEADLINE] = map[string]*pb.EntitySnapshot{deadline.Key: deadline}
	planner := SessionWorkerPlanner{Next: PlanSystemEntity, Now: func() time.Time { return now }}
	req := sessionWorkerRequest(1, uuid.NewString(), 2, &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: deadline.Key, Kind: pb.EntityKind_SESSION_DEADLINE}}})
	change, status, _, err := planner.Plan(context.Background(), req, state)
	var timedOut bool
	if change != nil {
		for _, item := range change.Changes {
			if item.GetUpsert().GetPdcSequence().GetState() == "NO_RESPONSE" {
				timedOut = true
			}
		}
	}
	if err != nil || status != pb.CommandReply_COMMITTED || len(change.Changes) != 2 || !timedOut {
		t.Fatalf("PDC deadline was not applied atomically: %v %v", change, err)
	}
	sequence.Revision++ // A pilot response won the race after the worker read.
	change, status, _, err = planner.Plan(context.Background(), req, state)
	if err != nil || status != pb.CommandReply_COMMITTED || len(change.Changes) != 1 {
		t.Fatalf("stale PDC deadline changed newer sequence: %v %v", change, err)
	}
	sequence.Revision--
	*req.ExpectedEntityRevision = 1 // The deadline itself was rearmed.
	if _, status, _, _ := planner.Plan(context.Background(), req, state); status != pb.CommandReply_REVISION_CONFLICT {
		t.Fatal("stale deadline revision was accepted")
	}
}

func TestStandSweepRearmsFromProjectionAfterOwnerSwitch(t *testing.T) {
	store, w, planner, p, now, oldOwner := workerFixture(t)
	ctx := context.Background()
	stand := StandState{Stands: standTestRegistry(t), Now: func() time.Time { return *now }}
	base := planner.Next
	planner.Next = func(ctx context.Context, req *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		remove := req.GetSystem().GetRemoveEntity()
		if req.GetClient().GetStand() != nil || remove != nil && (remove.Kind == pb.EntityKind_STAND_BLOCK || remove.Kind == pb.EntityKind_STAND_ASSIGNMENT) {
			return stand.PlanStand(ctx, req, state)
		}
		return base(ctx, req, state)
	}
	adapter := LocalLifecycleStore{Writer: Writer{Store: store, NodeID: "node-a", Plan: planner.Plan}}
	w.Store, w.Registry.Store = adapter, adapter
	writer := Writer{Store: store, NodeID: "node-a", Plan: stand.PlanStand}
	due := now.Add(time.Minute)
	create := standRequest("", "B1", testBlock, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1"}, 0)
	create.GetClient().GetStand().GetCreateBlock().ExpiresAt = timestamppb.New(due)
	if reply := writer.Execute(ctx, create); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(reply)
	}
	oldOwner.active = false
	*now = due.Add(time.Second)
	setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if workerState(t, w).Indexes[pb.EntityKind_STAND_BLOCK]["B1"] == nil {
		t.Fatal("stand expired without owner")
	}
	newOwner := &workerLease{active: true}
	w2 := &SessionWork{Registry: w.Registry, Store: w.Store, Projection: p, Owner: newOwner, Now: w.Now}
	if err := w2.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if workerState(t, w2).Indexes[pb.EntityKind_STAND_BLOCK]["B1"] != nil {
		t.Fatal("takeover did not expire stand")
	}
	commits := store.commits
	if err := w2.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if store.commits != commits {
		t.Fatal("stand expiry fired twice")
	}
}

func TestControllerOfflineDeadlineRechecksClusterPresence(t *testing.T) {
	_, _, _, p, now, _ := workerFixture(t)
	state := NewAggregate(sessionRef(1))
	controller := &pb.EntitySnapshot{Key: "cid-a", Revision: 3, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Controller{Controller: &pb.Controller{Cid: "cid-a", Callsign: "EKCH_A_TWR", Position: "118.100", Revision: 3}}}}
	deadline := &pb.EntitySnapshot{Key: "offline.EKCH_A_TWR", Revision: 2, Value: &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{Id: "offline.EKCH_A_TWR", Kind: "controller-offline", Callsign: "EKCH_A_TWR", DueAt: timestamppb.New(now.Add(-time.Second)), SourceRevision: 3}}}}
	state.Indexes[pb.EntityKind_CONTROLLER] = map[string]*pb.EntitySnapshot{controller.Key: controller}
	state.Indexes[pb.EntityKind_SESSION_DEADLINE] = map[string]*pb.EntitySnapshot{deadline.Key: deadline}
	state.Entities[controller.Key], state.Entities[deadline.Key] = controller, deadline
	planner := SessionWorkerPlanner{Next: PlanSystemEntity, Projection: p, Now: func() time.Time { return *now }}
	req := sessionWorkerRequest(1, uuid.NewString(), 2, &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: deadline.Key, Kind: pb.EntityKind_SESSION_DEADLINE}}})
	setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_EUROSCOPE)
	change, status, _, err := planner.Plan(context.Background(), req, state)
	if err != nil || status != pb.CommandReply_COMMITTED || len(change.Changes) != 1 {
		t.Fatalf("reconnected controller was deleted: %v %v", change, err)
	}
	setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	change, status, _, err = planner.Plan(context.Background(), req, state)
	if err != nil || status != pb.CommandReply_COMMITTED || len(change.Changes) != 2 {
		t.Fatalf("offline controller was not removed: %v %v", change, err)
	}
	controller.Revision++
	change, status, _, err = planner.Plan(context.Background(), req, state)
	if err != nil || status != pb.CommandReply_COMMITTED || len(change.Changes) != 1 {
		t.Fatalf("stale controller source was deleted: %v %v", change, err)
	}
}

func TestAircraftDisconnectRequiresCurrentTombstoneAndRetentionCheck(t *testing.T) {
	_, _, _, p, now, _ := workerFixture(t)
	setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_EUROSCOPE)
	state := NewAggregate(sessionRef(1))
	state.Owner = &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}
	state.Master = &pb.MasterTerm{ConnectionId: "client-a", Cid: "cid-a", Epoch: 1, OwnerEpoch: 1}
	state.Sync = &pb.SessionSync{ConnectionId: "client-a", MasterEpoch: 1, CompletedAt: timestamppb.New(*now)}
	seed := &pb.EntitySnapshot{Key: "1", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 1, Airport: "EKCH", Name: "LIVE"}}}}
	strip := &pb.EntitySnapshot{Key: "SAS101", Revision: 2, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Id: 1, Callsign: "SAS101", Revision: 2}}}}
	deadline := &pb.EntitySnapshot{Key: "disconnect.SAS101", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{Id: "disconnect.SAS101", Kind: "aircraft-disconnect", Callsign: "SAS101", DueAt: timestamppb.New(now.Add(-time.Second)), SourceRevision: 8}}}}
	for _, entity := range []*pb.EntitySnapshot{seed, strip, deadline} {
		state.Entities[entity.Key] = entity
	}
	state.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"1": seed}
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{"SAS101": strip}
	state.Indexes[pb.EntityKind_SESSION_DEADLINE] = map[string]*pb.EntitySnapshot{deadline.Key: deadline}
	p.mu.Lock()
	p.states = map[string]*Aggregate{"fs.v1.state.session.1": state}
	p.positions["1.SAS101.1"] = KVPosition{Value: &pb.PositionValue{SchemaVersion: 1, SessionId: 1, AircraftKey: "SAS101", OwnerEpoch: 1, SourceConnectionId: "client-a", Observation: &pb.PositionValue_Tombstone{Tombstone: &pb.PositionTombstone{}}}, Revision: 8, Observed: *now}
	p.mu.Unlock()
	planner := SessionWorkerPlanner{Next: PlanSystemEntity, Projection: p, Now: func() time.Time { return *now }, RetainedAircraft: func(*Aggregate, string) (bool, error) { return false, nil }}
	req := sessionWorkerRequest(1, uuid.NewString(), 1, &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: deadline.Key, Kind: pb.EntityKind_SESSION_DEADLINE}}})
	change, status, _, err := planner.Plan(context.Background(), req, state)
	if err != nil || status != pb.CommandReply_COMMITTED || len(change.Changes) != 2 {
		t.Fatalf("disconnect did not atomically remove strip and deadline: %v %v", change, err)
	}
	p.mu.Lock()
	p.positions["1.SAS101.1"] = KVPosition{Value: &pb.PositionValue{SchemaVersion: 1, SessionId: 1, AircraftKey: "SAS101", OwnerEpoch: 1, SourceConnectionId: "client-a", Observation: &pb.PositionValue_Position{Position: &pb.AircraftPosition{Latitude: 55.6, Longitude: 12.6}}}, Revision: 9, Observed: *now}
	p.mu.Unlock()
	change, status, _, err = planner.Plan(context.Background(), req, state)
	if err != nil || status != pb.CommandReply_COMMITTED || len(change.Changes) != 1 {
		t.Fatalf("reconnected aircraft was removed: %v %v", change, err)
	}
	// A VATSIM-retained aircraft keeps its strip after the EuroScope tombstone.
	p.mu.Lock()
	p.positions["1.SAS101.1"] = KVPosition{Value: &pb.PositionValue{SchemaVersion: 1, SessionId: 1, AircraftKey: "SAS101", OwnerEpoch: 1, SourceConnectionId: "client-a", Observation: &pb.PositionValue_Tombstone{Tombstone: &pb.PositionTombstone{}}}, Revision: 8, Observed: *now}
	p.mu.Unlock()
	planner.RetainedAircraft = func(*Aggregate, string) (bool, error) { return true, nil }
	change, status, _, err = planner.Plan(context.Background(), req, state)
	if err != nil || status != pb.CommandReply_COMMITTED || len(change.Changes) != 1 {
		t.Fatalf("retained aircraft was removed: %v %v", change, err)
	}
}

func TestStandBayAutoHideSurvivesWorkerTakeover(t *testing.T) {
	_, w, _, p, now, oldOwner := workerFixture(t)
	ctx := context.Background()
	strips := StripState{Store: w.Store}
	if reply, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "STAND", Departure: "ESSA", Destination: "EKCH"}, 0); err != nil {
		t.Fatalf("stand strip: %v %v", reply, err)
	}
	state := workerState(t, w)
	d := state.Indexes[pb.EntityKind_SESSION_DEADLINE]["strip-auto-hide.SAS101"]
	if d == nil || d.GetValue().GetSessionDeadline().SourceRevision != state.Indexes[pb.EntityKind_STRIP]["SAS101"].Revision {
		t.Fatal("stand arrival has no durable hide deadline")
	}
	oldOwner.active = false
	*now = d.GetValue().GetSessionDeadline().DueAt.AsTime().Add(time.Second)
	setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_EUROSCOPE)
	newOwner := &workerLease{active: true}
	w2 := &SessionWork{Registry: w.Registry, Store: w.Store, Projection: p, Owner: newOwner, Now: w.Now}
	if err := w2.Step(ctx); err != nil {
		t.Fatal(err)
	}
	state = workerState(t, w2)
	if state.Indexes[pb.EntityKind_STRIP]["SAS101"].GetValue().GetStrip().Bay != "HIDDEN" || state.Indexes[pb.EntityKind_SESSION_DEADLINE]["strip-auto-hide.SAS101"] != nil {
		t.Fatal("takeover did not hide stand strip once")
	}
}

func TestDepartureStandExpiryRechecksPhysicalOccupancy(t *testing.T) {
	now := time.Now().UTC()
	state := NewAggregate(sessionRef(1))
	seed := &pb.EntitySnapshot{Key: "1", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 1, Airport: "EKCH"}}}}
	strip := &pb.EntitySnapshot{Key: "SAS101", Revision: 2, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Id: 1, Callsign: "SAS101", Stand: "A1"}}}}
	assignment := &pb.EntitySnapshot{Key: "SAS101", Revision: 2, Value: &pb.EntityRecord{Value: &pb.EntityRecord_StandAssignment{StandAssignment: &pb.StandAssignment{Callsign: "SAS101", Stand: "A1", Direction: "DEPARTURE", Stage: "DEPARTURE_BLOCK", Revision: 2, ExpiresAt: timestamppb.New(now.Add(-time.Second))}}}}
	state.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"1": seed}
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{"SAS101": strip}
	state.Indexes[pb.EntityKind_STAND_ASSIGNMENT] = map[string]*pb.EntitySnapshot{"SAS101": assignment}
	stand := StandState{Now: func() time.Time { return now }}
	req := sessionWorkerRequest(1, uuid.NewString(), 2, &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: "SAS101", Kind: pb.EntityKind_STAND_ASSIGNMENT}}})
	change, status, _, err := stand.PlanStand(context.Background(), req, state)
	if err != nil || status != pb.CommandReply_COMMITTED || change.Changes[0].GetUpsert().GetStandAssignment().ExpiresAt != nil {
		t.Fatalf("occupied stand was released: %v %v", change, err)
	}
	strip.GetValue().GetStrip().Stand = "B1"
	change, status, _, err = stand.PlanStand(context.Background(), req, state)
	if err != nil || status != pb.CommandReply_COMMITTED || change.Changes[0].GetDelete() == nil {
		t.Fatalf("vacated stand was not released: %v %v", change, err)
	}
}

func TestSessionWorkerKeepsDeadlineRearmedByCallback(t *testing.T) {
	_, w, _, p, now, _ := workerFixture(t)
	ctx := context.Background()
	setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_EUROSCOPE)
	d := &pb.SessionDeadline{Id: "session-update.1", Kind: "session-update", DueAt: timestamppb.New(now.Add(-time.Second)), SourceRevision: 1}
	require.NoError(t, w.ScheduleDeadline(ctx, 1, d))
	w.SessionUpdate = func(ctx context.Context, id int32) error {
		next := proto.Clone(d).(*pb.SessionDeadline)
		next.DueAt = timestamppb.New(now.Add(time.Minute))
		return w.ScheduleDeadline(ctx, id, next)
	}
	require.NoError(t, w.Step(ctx))
	current := workerState(t, w).Indexes[pb.EntityKind_SESSION_DEADLINE][d.Id]
	require.NotNil(t, current)
	require.Equal(t, now.Add(time.Minute), current.GetValue().GetSessionDeadline().DueAt.AsTime())
}

func TestSessionWorkerSamplesPresenceAfterRecovery(t *testing.T) {
	_, w, _, p, now, _ := workerFixture(t)
	w.EuroScope = func(context.Context, int32) error {
		*now = now.Add(time.Second)
		setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_EUROSCOPE)
		return nil
	}
	require.NoError(t, w.Step(context.Background()))
	require.Nil(t, workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt)
}

func TestSessionWorkerRetriesPresenceChangeWithoutLosingOtherFailures(t *testing.T) {
	store, w, planner, p, now, _ := workerFixture(t)
	w.Store = LocalLifecycleStore{Writer: Writer{Store: store, NodeID: "node-a", Plan: func(ctx context.Context, req *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_EUROSCOPE)
		return planner.Plan(ctx, req, state)
	}}}
	require.NoError(t, w.Step(context.Background()))
	require.Nil(t, workerState(t, w).Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession().FirstNoControllerAt)
	setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_FRONTEND)
	w.EuroScope = func(context.Context, int32) error { return fmt.Errorf("genuine storage failure") }
	require.ErrorContains(t, w.Step(context.Background()), "genuine storage failure")
}
