package cluster

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSessionWorkerTwoNodeDeadlineTakeover(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	port := 4222
	if value := os.Getenv("NATS_TEST_PORT_BASE"); value != "" {
		var err error
		port, err = strconv.Atoi(value)
		if err != nil {
			t.Fatal(err)
		}
	}
	url := func(user, password string, offset int) string {
		return fmt.Sprintf("nats://%s:%s@127.0.0.1:%d", user, password, port+offset)
	}
	cfg := natsresources.Config{URLs: []string{url("bootstrap", "bootstrap-local-only", 0), url("bootstrap", "bootstrap-local-only", 1), url("bootstrap", "bootstrap-local-only", 2)}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	admin, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err := natscluster.WaitForQuorum(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := natsresources.Bootstrap(ctx, admin, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.URLs = []string{url("backend", "backend-local-only", 0), url("backend", "backend-local-only", 1), url("backend", "backend-local-only", 2)}
	type node struct {
		nc         *nats.Conn
		projection *Projection
		owner      *OwnerRuntime
		router     *CommandRouter
		worker     *SessionWork
		stop       context.CancelFunc
	}
	nodes := make([]node, 2)
	for i := range nodes {
		nc, err := natsresources.Connect(cfg)
		if err != nil {
			t.Fatal(err)
		}
		projection := startProjection(t, ctx, nc, cfg)
		store := NATSStore{JS: projection.JS}
		owner, err := NewOwnerRuntime(nc, projection, store)
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Track(globalRef()); err != nil {
			t.Fatal(err)
		}
		base := SessionLifecyclePlanner(func(_ context.Context, ref *pb.AggregateRef) (*Aggregate, error) { return projection.Read(ref) })
		planner := SessionWorkerPlanner{Next: base, Projection: projection}
		router := &CommandRouter{NC: nc, Projection: projection, Lease: owner, Writer: Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner, Plan: planner.Plan}}
		adapter := RoutedLifecycleStore{Router: router, Projection: projection}
		worker := &SessionWork{Registry: SessionRegistry{Store: adapter}, Store: adapter, Projection: projection, Owner: owner, SessionUpdate: func(context.Context, int32) error { return nil }}
		runCtx, stop := context.WithCancel(ctx)
		nodes[i] = node{nc, projection, owner, router, worker, stop}
		go func() { _ = owner.Run(runCtx) }()
		go func() { _ = router.Serve(runCtx) }()
	}
	defer func() {
		for _, n := range nodes {
			n.stop()
			n.nc.Close()
		}
	}()
	await := func(what string, check func() bool) {
		t.Helper()
		for ctx.Err() == nil {
			if check() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	await("global owner", func() bool { return nodes[0].owner.CanWrite(globalRef()) || nodes[1].owner.CanWrite(globalRef()) })
	name, workflow := "WORKER-"+uuid.NewString(), uuid.NewString()
	globalCreate := lifecycleRequest(globalRef(), workflow, &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Airport: "EKCH", Name: name, WorkflowId: workflow}})
	if reply := nodes[0].router.Route(ctx, globalCreate); reply.Status != pb.CommandReply_COMMITTED {
		t.Fatal(reply)
	}
	var id int32
	await("allocated session", func() bool {
		state, err := nodes[0].projection.Read(globalRef())
		if err != nil {
			return false
		}
		if entry := registryByName(state, "EKCH", name); entry != nil {
			id = entry.Id
			return true
		}
		return false
	})
	ref := sessionRef(id)
	for i := range nodes {
		if err := nodes[i].owner.Track(ref); err != nil {
			t.Fatal(err)
		}
	}
	await("session owner", func() bool { return nodes[0].owner.CanWrite(ref) || nodes[1].owner.CanWrite(ref) })
	if _, err := nodes[0].worker.Registry.GetOrCreateSession(ctx, "EKCH", name); err != nil {
		t.Fatal(err)
	}
	ownerIndex := func() int {
		for i := range nodes {
			if nodes[i].owner.CanWrite(ref) {
				return i
			}
		}
		return -1
	}
	first := ownerIndex()
	if first < 0 {
		t.Fatal("session owner disappeared")
	}
	addDeadline := func(key string, delay time.Duration) {
		state, err := nodes[first].projection.Read(ref)
		if err != nil {
			t.Fatal(err)
		}
		seed := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(id))]
		d := &pb.SessionDeadline{Id: key, Kind: "session-update", DueAt: timestamppb.New(time.Now().Add(delay)), SourceRevision: seed.Revision}
		if err := nodes[first].worker.ScheduleDeadline(ctx, id, d); err != nil {
			t.Fatal(err)
		}
	}
	keyA, keyB := "session-update."+uuid.NewString(), "session-update."+uuid.NewString()
	addDeadline(keyA, time.Second)
	await("first deadline replicated", func() bool {
		for _, n := range nodes {
			state, err := n.projection.Read(ref)
			if err != nil || state.Indexes[pb.EntityKind_SESSION_DEADLINE][keyA] == nil {
				return false
			}
		}
		return true
	})
	await("first deadline", func() bool {
		for i := range nodes {
			_ = nodes[i].worker.Step(ctx)
		}
		state, err := nodes[0].projection.Read(ref)
		return err == nil && state.Indexes[pb.EntityKind_SESSION_DEADLINE][keyA] == nil
	})
	addDeadline(keyB, 2*time.Second)
	await("second deadline replicated", func() bool {
		for _, n := range nodes {
			state, err := n.projection.Read(ref)
			if err != nil || state.Indexes[pb.EntityKind_SESSION_DEADLINE][keyB] == nil {
				return false
			}
		}
		return true
	})
	// Kill the accepted owner before the second deadline. The survivor must
	// rearm from FS_STATE and consume it once under the next owner epoch.
	nodes[first].stop()
	nodes[first].nc.Close()
	second := 1 - first
	await("takeover", func() bool { return nodes[second].owner.CanWrite(ref) })
	await("takeover deadline", func() bool {
		_ = nodes[second].worker.Step(ctx)
		state, err := nodes[second].projection.Read(ref)
		return err == nil && state.Indexes[pb.EntityKind_SESSION_DEADLINE][keyB] == nil
	})
	state, err := nodes[second].projection.Read(ref)
	if err != nil {
		t.Fatal(err)
	}
	if state.Indexes[pb.EntityKind_SESSION_DEADLINE][keyA] != nil || state.Indexes[pb.EntityKind_SESSION_DEADLINE][keyB] != nil {
		t.Fatal("deadline replay duplicated or lost a result")
	}
	before := state.Revision
	if err := nodes[second].worker.Step(ctx); err != nil {
		t.Fatal(err)
	}
	state, err = nodes[second].projection.Read(ref)
	if err != nil || state.Revision != before {
		t.Fatalf("completed deadline fired after takeover: %v", err)
	}
}
