package cluster

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
)

type routedCoordinationStore struct{ router *CommandRouter }

func coordinationTestPort(t *testing.T) int {
	t.Helper()
	if value := os.Getenv("NATS_TEST_PORT_BASE"); value != "" {
		base, err := strconv.Atoi(value)
		if err != nil {
			t.Fatal(err)
		}
		return base
	}
	return 4222
}

func coordinationTestURL(base, offset int, user, password string) string {
	return fmt.Sprintf("nats://%s:%s@127.0.0.1:%d", user, password, base+offset)
}

func (s routedCoordinationStore) Execute(ctx context.Context, req *pb.CommandRequest) *pb.CommandReply {
	return s.router.Route(ctx, req)
}
func (s routedCoordinationStore) Read(_ context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
	return s.router.Projection.Read(ref)
}

func TestNATSCoordinationAcrossBackendNodes(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	base := coordinationTestPort(t)
	cfg := natsresources.Config{URLs: []string{coordinationTestURL(base, 0, "bootstrap", "bootstrap-local-only"), coordinationTestURL(base, 1, "bootstrap", "bootstrap-local-only"), coordinationTestURL(base, 2, "bootstrap", "bootstrap-local-only")}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
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
	appCfg := cfg
	appCfg.URLs = []string{coordinationTestURL(base, 0, "backend", "backend-local-only"), coordinationTestURL(base, 1, "backend", "backend-local-only")}
	random := uuid.New()
	id := int32(100000 + binary.BigEndian.Uint32(random[:4])%1000000000)
	ref := sessionRef(id)
	type node struct {
		router *CommandRouter
		stop   context.CancelFunc
		close  func()
	}
	nodes := make([]node, 2)
	for i := range nodes {
		nc, err := natsresources.Connect(appCfg)
		if err != nil {
			t.Fatal(err)
		}
		projection := startProjection(t, ctx, nc, appCfg)
		store := NATSStore{JS: projection.JS}
		owner, err := NewOwnerRuntime(nc, projection, store)
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Track(ref); err != nil {
			t.Fatal(err)
		}
		runCtx, stop := context.WithCancel(ctx)
		router := &CommandRouter{NC: nc, Projection: projection, Lease: owner, Writer: Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner, Plan: PlanStrip}}
		nodes[i] = node{router, stop, nc.Close}
		go func() { _ = owner.Run(runCtx) }()
		go func() { _ = router.Serve(runCtx) }()
	}
	defer func() {
		for _, n := range nodes {
			n.stop()
			n.close()
		}
	}()
	for ctx.Err() == nil {
		state, err := nodes[0].router.Projection.Read(ref)
		if err == nil && state.Owner != nil && state.Owner.Epoch > 0 && nodes[1].router.Projection.Ready() == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal("session owner was not claimed")
	}
	storeA, storeB := routedCoordinationStore{nodes[0].router}, routedCoordinationStore{nodes[1].router}
	zero := uint64(0)
	seed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: fmt.Sprint(id), Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: id, Airport: "EKCH", Name: "LIVE", NextStripId: 1, NextCoordinationId: 1}}}}}}}}
	if reply := nodes[0].router.Route(ctx, seed); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("seed: %v", reply)
	}
	controller := ControllerSector{Store: storeA}
	for _, cid := range []string{"cid-a", "cid-b", "cid-c"} {
		if reply, err := controller.PutController(ctx, id, &pb.Controller{Cid: cid, Callsign: "EKCH_" + cid, Position: cid}, 0); err != nil {
			t.Fatalf("controller: %v %v", reply, err)
		}
	}
	strips := StripState{Store: storeA}
	if reply, err := strips.Put(ctx, id, &pb.Strip{Callsign: "SAS101", Bay: "CLEARED", OwnerCid: "cid-a", NextControllers: []string{"cid-b", "cid-c"}}, 0); err != nil {
		t.Fatalf("strip: %v %v", reply, err)
	}
	strip, _, _ := strips.ByCallsign(ctx, id, "SAS101")
	adapters := []CoordinationState{{Store: storeA}, {Store: storeB}}
	var wg sync.WaitGroup
	replies := make(chan string, 2)
	for _, cid := range []string{"cid-b", "cid-c"} {
		wg.Add(1)
		go func(cid string) {
			defer wg.Done()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCoordinationBackendProcess$")
			child.Env = append(os.Environ(), "COORDINATION_CHILD_SESSION="+strconv.Itoa(int(id)), "COORDINATION_CHILD_CID="+cid, "COORDINATION_CHILD_REVISION="+strconv.FormatUint(strip.Revision, 10))
			out, err := child.CombinedOutput()
			if err != nil {
				replies <- fmt.Sprintf("ERROR %v %s", err, out)
				return
			}
			replies <- string(out)
		}(cid)
	}
	wg.Wait()
	close(replies)
	won, conflict := 0, 0
	for output := range replies {
		switch {
		case strings.Contains(output, "COORDINATION_RESULT=SUCCEEDED"):
			won++
		case strings.Contains(output, "COORDINATION_RESULT=REVISION_CONFLICT"):
			conflict++
		default:
			t.Fatalf("conflicting backend reply: %s", output)
		}
	}
	if won != 1 || conflict != 1 {
		t.Fatalf("winners=%d conflicts=%d", won, conflict)
	}
	strip, _, _ = strips.ByCallsign(ctx, id, "SAS101")
	target := "cid-b"
	if strip.OwnerCid == target {
		target = "cid-c"
	}
	commandID := uuid.NewString()
	first, err := adapters[0].Action(ctx, id, strip.OwnerCid, commandID, strip.Revision, actTransfer(target))
	if err != nil {
		t.Fatalf("transfer: %v %v", first, err)
	}
	second, err := adapters[1].Action(ctx, id, strip.OwnerCid, commandID, strip.Revision, actTransfer(target))
	if err != nil || first.GetStreamSequence() != second.GetStreamSequence() {
		t.Fatalf("cross-node retry: %v %v", second, err)
	}
	list, _, err := adapters[1].List(ctx, id)
	if err != nil || len(list) != 1 || list[0].ToCid != target {
		t.Fatalf("projection coordination: %v %v", list, err)
	}
}

// The parent starts two copies of this test binary. Each process has its own
// NATS connection and projection and routes the same strip revision to the
// aggregate owner through Core NATS.
func TestCoordinationBackendProcess(t *testing.T) {
	value := os.Getenv("COORDINATION_CHILD_SESSION")
	if value == "" {
		t.Skip("subprocess helper")
	}
	id, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := strconv.ParseUint(os.Getenv("COORDINATION_CHILD_REVISION"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	base := coordinationTestPort(t)
	cfg := natsresources.Config{URLs: []string{coordinationTestURL(base, 0, "backend", "backend-local-only"), coordinationTestURL(base, 1, "backend", "backend-local-only")}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	nc, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	projection := startProjection(t, ctx, nc, cfg)
	owner, err := NewOwnerRuntime(nc, projection, NATSStore{JS: projection.JS})
	if err != nil {
		t.Fatal(err)
	}
	router := &CommandRouter{NC: nc, Projection: projection, Lease: owner}
	adapter := CoordinationState{Store: routedCoordinationStore{router}}
	reply, _ := adapter.Action(ctx, int32(id), os.Getenv("COORDINATION_CHILD_CID"), uuid.NewString(), revision, actForce("cid-a"))
	if reply == nil {
		t.Fatal("missing reply")
	}
	if reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
		fmt.Println("COORDINATION_RESULT=SUCCEEDED")
		return
	}
	if reply.GetOutcome().GetReasonCode() == "REVISION_CONFLICT" {
		fmt.Println("COORDINATION_RESULT=REVISION_CONFLICT")
		return
	}
	t.Fatalf("unexpected routed reply: %v", reply)
}
