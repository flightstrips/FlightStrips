package cluster

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

func TestOwnerFailoverAndLostCoreReply(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	basePort := 4222
	if value := os.Getenv("NATS_TEST_PORT_BASE"); value != "" {
		var err error
		basePort, err = strconv.Atoi(value)
		if err != nil {
			t.Fatal(err)
		}
	}
	url := func(user, password string, offset int) string {
		return fmt.Sprintf("nats://%s:%s@127.0.0.1:%d", user, password, basePort+offset)
	}
	cfg := natsresources.Config{URLs: []string{
		url("bootstrap", "bootstrap-local-only", 0),
		url("bootstrap", "bootstrap-local-only", 1),
		url("bootstrap", "bootstrap-local-only", 2),
	}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
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
	id := uuid.New()
	icao := string([]byte{'A' + id[0]%26, 'A' + id[1]%26, 'A' + id[2]%26, 'A' + id[3]%26})
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: icao}}}
	subject, _ := Subject(ref)
	type node struct {
		nc         *nats.Conn
		projection *Projection
		owner      *OwnerRuntime
		router     *CommandRouter
		stop       context.CancelFunc
	}
	nodes := make([]node, 3)
	appCfg := cfg
	appCfg.URLs = []string{url("backend", "backend-local-only", 0), url("backend", "backend-local-only", 1), url("backend", "backend-local-only", 2)}
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
		router := &CommandRouter{NC: nc, Projection: projection, Lease: owner,
			Writer: Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}}
		nodes[i] = node{nc, projection, owner, router, stop}
		go func() { _ = owner.Run(runCtx) }()
		go func() { _ = router.Serve(runCtx) }()
	}
	defer func() {
		for _, n := range nodes {
			n.stop()
			n.nc.Close()
		}
	}()
	var first *pb.OwnerTerm
	for ctx.Err() == nil {
		state, err := nodes[0].projection.Read(ref)
		if err == nil && state.Owner != nil && state.Owner.Epoch == 1 {
			first = state.Owner
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if first == nil {
		t.Fatal("initial owner was not claimed")
	}
	var dead int
	for i, n := range nodes {
		if n.owner.NodeID == first.NodeId {
			dead = i
			break
		}
	}
	var source int
	for i := range nodes {
		if i != dead {
			source = i
			break
		}
	}
	zero := uint64(0)
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "owner-test"}, ExpectedEntityRevision: &zero,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{
			UpdateEntity: &pb.UpdateEntity{Key: icao, Value: &pb.EntityRecord{Value: &pb.EntityRecord_AirportPolicy{
				AirportPolicy: &pb.AirportPolicy{Airport: icao},
			}}},
		}}},
	}
	// Send to an inbox with no subscriber. The owner can commit even though
	// the caller loses the Core NATS reply; Route must reuse the same ID.
	data, _ := proto.Marshal(request)
	redirect := nodes[source].router.handle(ctx, data)
	if redirect.Status != pb.CommandReply_NOT_OWNER || redirect.GetCurrentOwner().GetNodeId() != first.NodeId {
		t.Fatalf("non-owner did not redirect to projected owner: %v", redirect)
	}
	if err := nodes[source].nc.PublishRequest("fs.v1.command."+first.NodeId, nats.NewInbox(), data); err != nil {
		t.Fatal(err)
	}
	var committed uint64
	for ctx.Err() == nil {
		state, err := nodes[source].projection.Read(ref)
		if err == nil && state.Ledger[request.CommandId] != nil {
			committed = state.Ledger[request.CommandId].CommittedStreamSequence
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if committed == 0 {
		t.Fatal("lost-reply request did not commit")
	}
	if reply := nodes[source].router.Route(ctx, request); reply.Status != pb.CommandReply_COMMITTED || reply.GetStreamSequence() != committed {
		t.Fatalf("retry did not return stored outcome: %v", reply)
	}
	raw, err := nodes[source].nc.RequestWithContext(ctx, "fs.v1.command."+first.NodeId, data)
	if err != nil {
		t.Fatalf("Core NATS reply unavailable: %v", err)
	}
	remoteReply := &pb.CommandReply{}
	if err := pb.UnmarshalStrict(raw.Data, remoteReply); err != nil || remoteReply.Status != pb.CommandReply_COMMITTED {
		t.Fatalf("invalid owner reply: %v, %v", remoteReply, err)
	}
	second := proto.Clone(request).(*pb.CommandRequest)
	second.CommandId = uuid.NewString()
	second.ExpectedEntityRevision = proto.Uint64(1)
	second.GetSystem().GetUpdateEntity().Value.GetAirportPolicy().CdmConfigurationVersion = "routed"
	if reply := nodes[source].router.Route(ctx, second); reply.Status != pb.CommandReply_COMMITTED || reply.GetStreamSequence() <= committed {
		t.Fatalf("Core NATS routing did not commit: %v", reply)
	}
	nodes[dead].stop()
	nodes[dead].nc.Close()
	var takeover *Aggregate
	for ctx.Err() == nil {
		state, err := nodes[source].projection.Read(ref)
		if err == nil && state.Owner != nil && state.Owner.Epoch == 2 && state.Owner.NodeId != first.NodeId {
			takeover = state
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if takeover == nil {
		t.Fatal("standbys did not take over")
	}
	alive := []string{}
	for i, n := range nodes {
		if i != dead {
			alive = append(alive, n.owner.NodeID)
		}
	}
	if rank := RendezvousRank(subject, alive); takeover.Owner.NodeId != rank[0] {
		t.Fatalf("takeover owner %s lost rendezvous rank to %s", takeover.Owner.NodeId, rank[0])
	}
	// A stale publisher can still win a later subject CAS. Its old epoch
	// must not change an entity, ledger, or acknowledged command result.
	staleID := uuid.NewString()
	stale := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), CommandId: &staleID,
		Aggregate: ref, AggregateRevision: takeover.Revision + 1, OwnerEpoch: first.Epoch,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: first.NodeId},
		Fact:  &pb.StateEvent_DomainChanged{DomainChanged: &pb.DomainChange{}}}
	staleData, _ := proto.Marshal(stale)
	var seq uint64
	for ctx.Err() == nil {
		current, err := nodes[source].projection.Read(ref)
		if err != nil {
			t.Fatal(err)
		}
		seq, err = (NATSStore{JS: nodes[source].projection.JS}).Publish(ctx, subject, current.SubjectSequence, staleData)
		if err == nil {
			break
		}
		if err != ErrCAS {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if seq == 0 {
		t.Fatal("stale publisher never won subject CAS")
	}
	if err := nodes[source].projection.WaitApplied(ctx, seq); err != nil {
		t.Fatal(err)
	}
	latest, err := nodes[source].projection.Read(ref)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Revision != takeover.Revision || latest.Owner.Epoch != 2 || latest.Ledger[staleID] != nil {
		t.Fatalf("stale epoch became effective: %+v", latest)
	}
	entries, err := (NATSStore{JS: nodes[source].projection.JS}).Replay(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	state := NewAggregate(ref)
	var effectiveTakeovers int
	for _, entry := range entries {
		e := &pb.StateEvent{}
		if err := pb.UnmarshalStrict(entry.Data, e); err != nil {
			t.Fatal(err)
		}
		effective, err := state.Apply(entry)
		if err != nil {
			t.Fatal(err)
		}
		if effective && e.GetOwnerClaimed() != nil && e.GetOwnerClaimed().Epoch == 2 {
			effectiveTakeovers++
		}
	}
	if effectiveTakeovers != 1 {
		t.Fatalf("effective epoch-two claims = %d", effectiveTakeovers)
	}
	if project := os.Getenv("NATS_FAULT_PROJECT"); project != "" {
		// Only an explicitly named disposable fixture is stopped. The normal
		// integration run never changes another developer's NATS processes.
		stopNode := func(number int) {
			name := fmt.Sprintf("%s-nats-%d-1", project, number)
			command := exec.CommandContext(ctx, "docker", "stop", name)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("stop %s: %v: %s", name, err, output)
			}
			t.Cleanup(func() {
				startCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				command := exec.CommandContext(startCtx, "docker", "start", name)
				if output, err := command.CombinedOutput(); err != nil {
					t.Errorf("restart %s: %v: %s", name, err, output)
				}
			})
		}
		stopNode(3)
		third := proto.Clone(second).(*pb.CommandRequest)
		third.CommandId = uuid.NewString()
		third.ExpectedEntityRevision = proto.Uint64(2)
		third.GetSystem().GetUpdateEntity().Value.GetAirportPolicy().CdmConfigurationVersion = "one-peer-down"
		var write *pb.CommandReply
		for ctx.Err() == nil {
			write = nodes[source].router.Route(ctx, third)
			if write.Status == pb.CommandReply_COMMITTED {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if write == nil || write.Status != pb.CommandReply_COMMITTED {
			t.Fatalf("write failed with one NATS node down: %v", write)
		}
		stopNode(2)
		other := 0
		for i := range nodes {
			if i != dead && i != source {
				other = i
			}
		}
		for ctx.Err() == nil {
			if nodes[source].projection.Ready() != nil && nodes[other].projection.Ready() != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if nodes[source].projection.Ready() == nil || nodes[other].projection.Ready() == nil {
			t.Fatal("quorum loss did not fence both projections")
		}
		if reply := nodes[source].router.Route(ctx, third); reply.Status != pb.CommandReply_UNAVAILABLE {
			t.Fatalf("quorum loss accepted command: %v", reply)
		}
	}
}
