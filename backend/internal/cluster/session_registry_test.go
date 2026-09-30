package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

func registryFixture(t *testing.T, sessionIDs ...int32) (*memoryStore, SessionRegistry) {
	t.Helper()
	store := &memoryStore{}
	for _, ref := range append([]*pb.AggregateRef{globalRef()}, sessionRefs(sessionIDs...)...) {
		subject, _ := Subject(ref)
		claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref,
			Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"},
			Fact:  &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
		data, err := proto.Marshal(claim)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Publish(context.Background(), subject, 0, data); err != nil {
			t.Fatal(err)
		}
	}
	reader := Writer{Store: store}
	writer := Writer{Store: store, NodeID: "node-a", Plan: SessionLifecyclePlanner(func(ctx context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
		subject, err := Subject(ref)
		if err != nil {
			return nil, err
		}
		return reader.load(ctx, subject, ref)
	})}
	return store, SessionRegistry{Store: LocalLifecycleStore{Writer: writer}}
}
func sessionRefs(ids ...int32) []*pb.AggregateRef {
	refs := make([]*pb.AggregateRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, sessionRef(id))
	}
	return refs
}
func registryState(t *testing.T, r SessionRegistry) *Aggregate {
	t.Helper()
	a, err := r.read(context.Background(), globalRef())
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSessionRegistryConcurrentGetOrCreateAndReplay(t *testing.T) {
	store, registry := registryFixture(t, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	const callers = 24
	ids := make(chan int32, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session, err := registry.GetOrCreateSession(ctx, "ekch", "LIVE")
			if err != nil {
				errs <- err
			} else {
				ids <- session.Id
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	close(ids)
	for id := range ids {
		if id != 1 {
			t.Fatalf("concurrent create returned ID %d", id)
		}
	}
	state := registryState(t, registry)
	if len(state.EntitiesByKind(pb.EntityKind_SESSION_REGISTRY)) != 1 || state.Entities["1"].GetValue().GetSessionRegistry().State != pb.SessionRegistry_ACTIVE {
		t.Fatalf("registry did not converge: %v", state.Entities)
	}
	// Two independent backend projections consume the same stream and derive
	// the same index. No local cache or separately committed name index exists.
	projections := []*Projection{
		{states: map[string]*Aggregate{}, listeners: map[uint64]*projectionListener{}, lastSnapshot: map[string]time.Time{}, sinceSnapshot: map[string]uint64{}},
		{states: map[string]*Aggregate{}, listeners: map[uint64]*projectionListener{}, lastSnapshot: map[string]time.Time{}, sinceSnapshot: map[string]uint64{}},
	}
	store.mu.Lock()
	entries := append([]AppliedEvent(nil), store.entries...)
	store.mu.Unlock()
	for _, projection := range projections {
		for _, entry := range entries {
			if err := projection.apply(entry); err != nil {
				t.Fatal(err)
			}
		}
	}
	otherState := projections[1].states["fs.v1.state.global"]
	firstState := projections[0].states["fs.v1.state.global"]
	firstSnapshot, err := state.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot, err := otherState.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	projectedSnapshot, err := firstState.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(firstSnapshot, secondSnapshot) || !proto.Equal(firstSnapshot, projectedSnapshot) || registryByName(otherState, "EKCH", "LIVE").Id != 1 {
		t.Fatal("independent registry projections differ")
	}
}

func TestSessionRegistryRecoveryWaitAndDeletion(t *testing.T) {
	_, registry := registryFixture(t, 1, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	workflow := uuid.NewString()
	allocate := lifecycleRequest(globalRef(), workflow, &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Airport: "EKCH", Name: "LIVE", WorkflowId: workflow}})
	if err := registry.execute(ctx, allocate); err != nil {
		t.Fatal(err)
	}
	// A connection must not observe an initializing record as active.
	waitCtx, stopWait := context.WithTimeout(ctx, 80*time.Millisecond)
	defer stopWait()
	if _, err := registry.WaitActive(waitCtx, "EKCH", "LIVE"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("initializing session was visible: %v", err)
	}
	// The first process dies after global allocation. A fresh adapter resumes
	// the same workflow and seeds the aggregate exactly once.
	restarted := registry
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	seed, err := restarted.WaitActive(ctx, "EKCH", "LIVE")
	if err != nil || seed.Id != 1 || seed.NextStripId != 1 || seed.Sync != nil {
		t.Fatalf("bad recovered seed: %v %v", seed, err)
	}
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if got := registryState(t, restarted).Entities["1"].GetValue().GetSessionRegistry(); got.WorkflowId != workflow || got.State != pb.SessionRegistry_ACTIVE {
		t.Fatalf("workflow changed on retry: %v", got)
	}
	if active, err := restarted.ActiveSessions(ctx); err != nil || len(active) != 1 || active[0].Id != 1 {
		t.Fatalf("active registry list is wrong: %v, %v", active, err)
	}
	if err := restarted.FinalizeDeletion(ctx, 1); err == nil {
		t.Fatal("name released without a session tombstone")
	}
	if err := restarted.TombstoneSession(ctx, 1); err != nil {
		t.Fatal(err)
	}
	// Crash before the global deletion step: recovery must release the index.
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if got := registryState(t, restarted).Entities["1"].GetValue().GetSessionRegistry(); got.State != pb.SessionRegistry_DELETED {
		t.Fatalf("incomplete deletion after restart: %v", got)
	}
	if err := restarted.FinalizeDeletion(ctx, 1); err != nil {
		t.Fatalf("deletion retry should be idempotent: %v", err)
	}
	second, err := restarted.GetOrCreateSession(ctx, "EKCH", "LIVE")
	if err != nil || second.Id != 2 {
		t.Fatalf("name reuse returned %v, %v", second, err)
	}
	old, err := restarted.read(ctx, sessionRef(1))
	if err != nil || !old.Entities["1"].GetValue().GetSession().Tombstoned {
		t.Fatalf("old ID revived: %v", err)
	}
	if len(registryState(t, restarted).EntitiesByKind(pb.EntityKind_SESSION_REGISTRY)) != 2 {
		t.Fatal("deleted ID was discarded from history")
	}
	if active, err := restarted.ActiveSessions(ctx); err != nil || len(active) != 1 || active[0].Id != 2 {
		t.Fatalf("active list retained deleted ID: %v, %v", active, err)
	}
}

func TestSessionRegistryUncertainAllocationResumesOriginalWorkflow(t *testing.T) {
	store, registry := registryFixture(t, 1)
	store.loseAck, store.failAfterAck = true, true
	ctx := context.Background()
	if _, err := registry.GetOrCreateSession(ctx, "EKCH", "LIVE"); err == nil {
		t.Fatal("lost acknowledgment was reported as confirmed")
	}
	allocated := registryState(t, registry).Entities["1"].GetValue().GetSessionRegistry()
	if allocated.State != pb.SessionRegistry_INITIALIZING {
		t.Fatalf("allocation did not persist: %v", allocated)
	}
	seed, err := registry.GetOrCreateSession(ctx, "EKCH", "LIVE")
	if err != nil || seed.Id != 1 {
		t.Fatalf("allocation retry failed: %v, %v", seed, err)
	}
	if got := registryState(t, registry).Entities["1"].GetValue().GetSessionRegistry(); got.WorkflowId != allocated.WorkflowId {
		t.Fatal("uncertain allocation generated a second workflow")
	}
}

func TestSessionRegistryResumeAfterDeletingAndRejectTombstoneRevival(t *testing.T) {
	_, registry := registryFixture(t, 1)
	ctx := context.Background()
	if _, err := registry.GetOrCreateSession(ctx, "EKCH", "LIVE"); err != nil {
		t.Fatal(err)
	}
	if err := registry.TombstoneSession(ctx, 1); err != nil {
		t.Fatal(err)
	}
	entry := registryState(t, registry).Entities["1"].GetValue().GetSessionRegistry()
	if err := registry.execute(ctx, lifecycleRequest(globalRef(), lifecycleID(entry.WorkflowId, "deleting"), &pb.SystemCommand_DeleteSession{DeleteSession: &pb.DeleteSession{Id: 1, WorkflowId: entry.WorkflowId}})); err != nil {
		t.Fatal(err)
	}
	if err := registry.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if state := registryState(t, registry).Entities["1"].GetValue().GetSessionRegistry().State; state != pb.SessionRegistry_DELETED {
		t.Fatalf("deleting workflow did not finish: %v", state)
	}
	request := lifecycleRequest(sessionRef(1), uuid.NewString(), &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Id: 1, Airport: "EKCH", Name: "LIVE", WorkflowId: entry.WorkflowId}})
	if reply := registry.Store.Execute(ctx, request); reply.Status != pb.CommandReply_COMMITTED || reply.Outcome.Status != pb.CommandOutcome_FAILED {
		t.Fatalf("reviving tombstone should fail durably: %v", reply)
	}
	if err := registry.execute(ctx, request); err == nil {
		t.Fatal("a committed failed lifecycle outcome was reported as success")
	}
}

func TestSessionDeletionBarrierDoesNotRetainFailedOutcome(t *testing.T) {
	_, registry := registryFixture(t, 1)
	ctx := context.Background()
	if _, err := registry.GetOrCreateSession(ctx, "EKCH", "LIVE"); err != nil {
		t.Fatal(err)
	}
	entry := registryState(t, registry).Entities["1"].GetValue().GetSessionRegistry()
	request := lifecycleRequest(sessionRef(1), lifecycleID(entry.WorkflowId, "tombstone"), &pb.SystemCommand_DeleteSession{DeleteSession: &pb.DeleteSession{Id: 1, WorkflowId: entry.WorkflowId}})
	store := registry.Store.(LocalLifecycleStore)
	next := store.Writer.Plan
	pending := true
	store.Writer.Plan = func(ctx context.Context, req *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if pending {
			state.Effects["pending"] = &pb.EffectRecord{Status: pb.EffectRecord_DISPATCH_CLAIMED}
		}
		return next(ctx, req, state)
	}
	if reply := store.Execute(ctx, request); reply.Status != pb.CommandReply_UNAVAILABLE || reply.Outcome != nil {
		t.Fatalf("pending effects must defer deletion without retaining failure: %v", reply)
	}
	pending = false
	if reply := store.Execute(ctx, request); reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("the same durable deletion identity must succeed after effects finish: %v", reply)
	}
}
