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

func TestVatsimSessionTwoReplicaNATS(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	port := 4222
	if raw := os.Getenv("NATS_TEST_PORT_BASE"); raw != "" {
		var err error
		port, err = strconv.Atoi(raw)
		if err != nil || port < 1024 || port > 65533 {
			t.Fatalf("invalid NATS_TEST_PORT_BASE %q", raw)
		}
	}
	url := func(user string, offset int) string {
		return fmt.Sprintf("nats://%s:%s-local-only@127.0.0.1:%d", user, user, port+offset)
	}
	cfg := natsresources.Config{URLs: []string{url("bootstrap", 0), url("bootstrap", 1), url("bootstrap", 2)}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
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
	cfg.URLs = []string{url("backend", 0), url("backend", 1), url("backend", 2)}
	type replica struct {
		nc    *nats.Conn
		owner *OwnerRuntime
		nav   NavigationWeather
		stop  context.CancelFunc
	}
	var nodes [2]replica
	global := globalRef()
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
		if err := owner.Track(global); err != nil {
			t.Fatal(err)
		}
		objects, err := projection.JS.ObjectStore("FS_OBJECTS")
		if err != nil {
			t.Fatal(err)
		}
		runCtx, stop := context.WithCancel(ctx)
		nodes[i] = replica{nc, owner, NavigationWeather{Writer: Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}, Objects: NATSObjects{Store: objects}}, stop}
		go func() { _ = owner.Run(runCtx) }()
	}
	defer func() {
		for _, node := range nodes {
			node.stop()
			node.nc.Close()
		}
	}()
	waitOwner := func(ref *pb.AggregateRef, exclude string) int {
		t.Helper()
		for ctx.Err() == nil {
			for i := range nodes {
				if nodes[i].owner.NodeID != exclude && nodes[i].owner.CanWrite(ref) {
					return i
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("owner takeover timed out")
		return -1
	}
	globalOwner := waitOwner(global, "")
	page := &pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: &pb.VatsimPage{
		SnapshotAt: timestamppb.Now(), Flights: []*pb.VatsimFlight{{Cid: "12345", Callsign: "SAS123", State: "prefile", FlightPlan: &pb.VatsimFlightPlan{Origin: "EKCH", Destination: "EDDF", Route: "DCT", Revision: 1}}},
	}}}
	name, sha, err := nodes[globalOwner].nav.PublishProvider(page)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: name, Sha256: sha}
	if reply, err := nodes[globalOwner].nav.PutCheckpointFor(ctx, global, uuid.NewString(), checkpoint); err != nil || reply.Status != pb.CommandReply_COMMITTED {
		t.Fatalf("global checkpoint: %v %v", reply, err)
	}
	var sessionID int32
	var session *pb.AggregateRef
	for candidate := int32(200000); candidate < 201000; candidate++ {
		ref := sessionRef(candidate)
		subject, _ := Subject(ref)
		if RendezvousRank(subject, []string{nodes[0].owner.NodeID, nodes[1].owner.NodeID})[0] == nodes[globalOwner].owner.NodeID {
			sessionID, session = candidate, ref
			break
		}
	}
	if session == nil {
		t.Fatal("session owner candidate unavailable")
	}
	for i := range nodes {
		if err := nodes[i].owner.Track(session); err != nil {
			t.Fatal(err)
		}
	}
	first := waitOwner(session, "")
	if first != globalOwner {
		t.Fatalf("unexpected session owner %d", first)
	}
	second := 1 - first
	seedWriter := nodes[first].nav.Writer
	seedWriter.Plan = func(context.Context, *pb.CommandRequest, *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		return &pb.DomainChange{Changes: []*pb.EntityChange{{Key: strconv.FormatInt(int64(sessionID), 10), Revision: 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: sessionID, Airport: "EKCH", Name: "LIVE", NextStripId: 1}}}}}}}, pb.CommandReply_COMMITTED, 0, nil
	}
	seed := &pb.CommandRequest{
		ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: session,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "vatsim-test"},
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{
			UpdateEntity: &pb.UpdateEntity{Key: strconv.FormatInt(int64(sessionID), 10), Value: &pb.EntityRecord{
				Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: sessionID}},
			}},
		}}},
	}
	if reply := seedWriter.Execute(ctx, seed); reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("seed session: %v", reply)
	}
	if reply := (VatsimSessionAdapter{Source: nodes[second].nav, Writer: nodes[second].nav.Writer}).Reconcile(ctx, sessionID); reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("nonowner reconciled: %v", reply)
	}
	firstReply := (VatsimSessionAdapter{Source: nodes[first].nav, Writer: nodes[first].nav.Writer}).Reconcile(ctx, sessionID)
	if firstReply.Status != pb.CommandReply_COMMITTED || firstReply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("owner reconciliation: %v", firstReply)
	}
	nodes[first].stop()
	nodes[first].nc.Close()
	if newOwner := waitOwner(session, nodes[first].owner.NodeID); newOwner != second {
		t.Fatal("wrong takeover owner")
	}
	state, err := nodes[second].nav.Writer.load(ctx, fmt.Sprintf("fs.v1.state.session.%d", sessionID), session)
	if err != nil {
		t.Fatal(err)
	}
	strip := state.Indexes[pb.EntityKind_STRIP]["SAS123"]
	if strip == nil || strip.GetValue().GetStrip().VatsimCid != "12345" || state.Indexes[pb.EntityKind_VATSIM_SESSION_CURSOR]["vatsim"] == nil {
		t.Fatalf("takeover lost VATSIM result: %v", state.Indexes)
	}
	sequence := state.StreamSequence
	replay := (VatsimSessionAdapter{Source: nodes[second].nav, Writer: nodes[second].nav.Writer}).Reconcile(ctx, sessionID)
	if replay.Status != pb.CommandReply_COMMITTED || replay.GetStreamSequence() != firstReply.GetStreamSequence() {
		t.Fatalf("takeover replay duplicated: %v", replay)
	}
	state, err = nodes[second].nav.Writer.load(ctx, fmt.Sprintf("fs.v1.state.session.%d", sessionID), session)
	if err != nil || state.StreamSequence != sequence {
		t.Fatalf("takeover wrote duplicate event: %v %v", state.StreamSequence, err)
	}
}
