package cluster

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This is intentionally two independent backend connections and projections.
// It exercises the real FS_STATE CAS, FS_OBJECTS, lease handoff and replay.
func TestEcfmpGlobalTwoReplicaNATS(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	basePort := 4222
	if value := os.Getenv("NATS_TEST_PORT_BASE"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1024 || parsed > 65533 {
			t.Fatalf("invalid NATS_TEST_PORT_BASE %q", value)
		}
		basePort = parsed
	}
	url := func(user string, port int) string {
		return fmt.Sprintf("nats://%s:%s-local-only@127.0.0.1:%d", user, user, port)
	}
	cfg := natsresources.Config{URLs: []string{url("bootstrap", basePort), url("bootstrap", basePort+1), url("bootstrap", basePort+2)},
		ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
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
	cfg.URLs = []string{url("backend", basePort), url("backend", basePort+1), url("backend", basePort+2)}
	type replica struct {
		nc         *nats.Conn
		projection *Projection
		owner      *OwnerRuntime
		nav        NavigationWeather
		stop       context.CancelFunc
	}
	var nodes [2]replica
	ref := globalRef()
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
		if err := owner.Track(ref); err != nil {
			t.Fatal(err)
		}
		objects, err := projection.JS.ObjectStore("FS_OBJECTS")
		if err != nil {
			t.Fatal(err)
		}
		runCtx, stop := context.WithCancel(ctx)
		nodes[i] = replica{nc: nc, projection: projection, owner: owner,
			nav: NavigationWeather{Writer: Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}, Objects: NATSObjects{Store: objects}}, stop: stop}
		go func() { _ = owner.Run(runCtx) }()
	}
	defer func() {
		for _, node := range nodes {
			node.stop()
			node.nc.Close()
		}
	}()
	waitOwner := func(exclude string) int {
		t.Helper()
		for ctx.Err() == nil {
			for i := range nodes {
				if nodes[i].owner.NodeID != exclude && nodes[i].owner.CanWrite(ref) {
					return i
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("global owner did not become writable")
		return -1
	}
	first := waitOwner("")
	second := 1 - first
	makePage := func() *pb.ProviderPage {
		return &pb.ProviderPage{Provider: "ecfmp", Resource: "flow-measure/active", Parsed: &pb.ProviderPage_Ecfmp{Ecfmp: &pb.EcfmpPage{FetchedAt: timestamppb.Now(),
			Measures: []*pb.EcfmpMeasure{{Id: 42, Kind: "mandatory_route", StartTime: timestamppb.New(time.Now().Add(-time.Minute)), EndTime: timestamppb.New(time.Now().Add(time.Hour)), Routes: []string{"DCT ABC"}}}}}}
	}
	var calls atomic.Int32
	fetch := func(context.Context, *pb.ProviderCheckpoint, *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
		calls.Add(1)
		return makePage(), &pb.ProviderCheckpoint{Provider: "ecfmp", Resource: "flow-measure/active"}, nil
	}
	id := uuid.NewString()
	if sent, err := nodes[second].nav.FetchProviderPageFor(ctx, ExternalCallWorker{Writer: nodes[second].nav.Writer}, id, ref, "ecfmp", "flow-measure/active", fetch); err == nil || sent || calls.Load() != 0 {
		t.Fatalf("non-owner fetched: %v %v calls=%d", sent, err, calls.Load())
	}
	if sent, err := nodes[first].nav.FetchProviderPageFor(ctx, ExternalCallWorker{Writer: nodes[first].nav.Writer}, id, ref, "ecfmp", "flow-measure/active", fetch); err != nil || !sent || calls.Load() != 1 {
		t.Fatalf("owner fetch: %v %v calls=%d", sent, err, calls.Load())
	}
	checkpoint, page, checkpointRevision, err := nodes[second].nav.CheckpointRevisionFor(ctx, ref, "ecfmp", "flow-measure/active")
	if err != nil || checkpoint == nil || checkpoint.Sha256 == "" || page.GetEcfmp() == nil {
		t.Fatalf("second replica cannot read typed result: %v %v %v", checkpoint, page, err)
	}
	// Pick a session whose first owner is the same process as the global
	// owner, so the same failure exercises both takeover paths.
	var sessionID int32
	var session *pb.AggregateRef
	baseSessionID := int32(100000 + int(uuid.New()[0])*1000)
	for candidate := baseSessionID; candidate < baseSessionID+1000; candidate++ {
		ref := sessionRef(candidate)
		subject, _ := Subject(ref)
		if RendezvousRank(subject, []string{nodes[0].owner.NodeID, nodes[1].owner.NodeID})[0] == nodes[first].owner.NodeID {
			sessionID, session = candidate, ref
			break
		}
	}
	if session == nil {
		t.Fatal("could not select session owner")
	}
	sessionSubject, _ := Subject(session)
	if err := nodes[first].owner.Track(session); err != nil {
		t.Fatal(err)
	}
	for !nodes[first].owner.CanWrite(session) && ctx.Err() == nil {
		time.Sleep(50 * time.Millisecond)
	}
	if ctx.Err() != nil {
		state, _ := nodes[first].projection.Read(session)
		t.Fatalf("session owner did not become writable: projected=%v ready=%v", state.Owner, nodes[first].owner.Ready())
	}
	if err := nodes[second].owner.Track(session); err != nil {
		t.Fatal(err)
	}
	seedWriter := nodes[first].nav.Writer
	seedWriter.Plan = func(context.Context, *pb.CommandRequest, *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		return &pb.DomainChange{Changes: []*pb.EntityChange{
			{Key: strconv.FormatInt(int64(sessionID), 10), Revision: 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: sessionID, Airport: "EKCH", Name: "LIVE", NextStripId: 2}}}}},
			{Key: "SAS123", Revision: 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Id: 1, Callsign: "SAS123", Revision: 1, Bay: "CLEARED", Sequence: 1000, Departure: "EKCH", Destination: "EDDF"}}}}},
		}}, pb.CommandReply_COMMITTED, 0, nil
	}
	seed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: session,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "ecfmp-test"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: strconv.FormatInt(int64(sessionID), 10), Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: sessionID, Airport: "EKCH", Name: "LIVE"}}}}}}}}
	if reply := seedWriter.Execute(ctx, seed); reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("seed session: %v", reply)
	}
	application := EcfmpSessionAdapter{Writer: nodes[first].nav.Writer}
	strips, err := application.Strips(ctx, sessionID)
	if err != nil || len(strips) != 1 {
		t.Fatalf("session strips: %v %v", strips, err)
	}
	sourceVersion := fmt.Sprintf("%020d:%s", checkpointRevision, checkpoint.Sha256)
	if reply := (EcfmpSessionAdapter{Writer: nodes[second].nav.Writer}).Apply(ctx, sessionID, strips[0], sourceVersion, nil); reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("nonowner applied ECFMP result: %v", reply)
	}
	applicationResult := application.Apply(ctx, sessionID, strips[0], sourceVersion, []*pb.EcfmpRestriction{{MeasureId: 42, Kind: "mandatory_route", Routes: []string{"DCT ABC"}}})
	if applicationResult.Status != pb.CommandReply_COMMITTED || applicationResult.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("session application: %v", applicationResult)
	}
	// One intent is committed but never dispatched. Another provider call
	// returns just as the owner dies, before its result can be committed.
	pendingID := uuid.NewString()
	pendingStep, _ := AmanIntentID(pendingID, "external/provider/ecfmp/flow-measure/active")
	pending := &pb.WorkflowRecord{WorkflowId: pendingID, Source: ref, Destination: ref,
		Step: "external/provider/ecfmp/flow-measure/active", DerivedCommandId: pendingStep, Status: pb.WorkflowRecord_PENDING}
	if reply, fresh := (ExternalCallWorker{Writer: nodes[first].nav.Writer}).advance(ctx, pending, "intent"); reply == nil || !fresh || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("pre-call intent: %v fresh=%v", reply, fresh)
	}
	uncertainID := uuid.NewString()
	uncertainFetch := func(context.Context, *pb.ProviderCheckpoint, *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
		calls.Add(1)
		nodes[first].stop()
		nodes[first].nc.Close()
		return makePage(), &pb.ProviderCheckpoint{Provider: "ecfmp", Resource: "flow-measure/active"}, nil
	}
	if sent, err := nodes[first].nav.FetchProviderPageFor(ctx, ExternalCallWorker{Writer: nodes[first].nav.Writer}, uncertainID, ref, "ecfmp", "flow-measure/active", uncertainFetch); !sent || err == nil {
		t.Fatalf("post-call owner failure was not uncertain: %v %v", sent, err)
	}
	// The new owner proves the first result from its ledger. Both unresolved
	// workflows terminate uncertain without a second provider request.
	nodes[first].stop()
	nodes[first].nc.Close()
	survivor := waitOwner(nodes[first].owner.NodeID)
	if survivor != second {
		t.Fatal("unexpected takeover")
	}
	for !nodes[second].owner.CanWrite(session) && ctx.Err() == nil {
		time.Sleep(50 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal("session owner takeover timed out")
	}
	readSession, err := nodes[second].nav.Writer.load(ctx, sessionSubject, session)
	if err != nil {
		t.Fatal(err)
	}
	result := readSession.Indexes[pb.EntityKind_ECFMP_STATE]["SAS123"].GetValue().GetEcfmpState()
	if result == nil || len(result.Restrictions) != 1 || !proto.Equal(result.Restrictions[0], &pb.EcfmpRestriction{MeasureId: 42, Kind: "mandatory_route", Routes: []string{"DCT ABC"}}) {
		t.Fatalf("session result not replayed: %v %v", readSession, err)
	}
	if reply := (EcfmpSessionAdapter{Writer: nodes[second].nav.Writer}).Apply(ctx, sessionID, strips[0], sourceVersion, []*pb.EcfmpRestriction{{MeasureId: 42, Kind: "mandatory_route", Routes: []string{"DCT ABC"}}}); reply.GetStreamSequence() != applicationResult.GetStreamSequence() || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("session result duplicated after takeover: %v", reply)
	}
	if err := (ExternalCallWorker{Writer: nodes[second].nav.Writer}).Resume(ctx, ref); err != nil {
		t.Fatal(err)
	}
	state, err := nodes[second].nav.read(ctx, ref)
	if err != nil || state.Workflows[id].Status != pb.WorkflowRecord_COMPLETED || state.Workflows[pendingID].ReasonCode != "CALL_UNCERTAIN" || state.Workflows[uncertainID].ReasonCode != "CALL_UNCERTAIN" {
		t.Fatalf("takeover recovery: %v %v", state.Workflows, err)
	}
	if sent, err := nodes[second].nav.FetchProviderPageFor(ctx, ExternalCallWorker{Writer: nodes[second].nav.Writer}, id, ref, "ecfmp", "flow-measure/active", fetch); err != nil || sent || calls.Load() != 2 {
		t.Fatalf("takeover refetched: %v %v calls=%d", sent, err, calls.Load())
	}
	for _, replay := range []string{pendingID, uncertainID} {
		if sent, err := nodes[second].nav.FetchProviderPageFor(ctx, ExternalCallWorker{Writer: nodes[second].nav.Writer}, replay, ref, "ecfmp", "flow-measure/active", fetch); err != nil || sent || calls.Load() != 2 {
			t.Fatalf("uncertain call repeated: %v %v calls=%d", sent, err, calls.Load())
		}
	}
}
