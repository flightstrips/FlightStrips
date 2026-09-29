package cluster

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	euroscope "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type writableLease bool

func (w writableLease) CanWrite(*pb.AggregateRef) bool { return bool(w) }

type electionTestRouter struct {
	writer   Writer
	store    *memoryStore
	replicas []*Projection
}

func (r electionTestRouter) Route(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	reply := r.writer.Execute(ctx, request)
	subject, _ := Subject(request.Aggregate)
	entries, _ := r.store.Replay(ctx, subject)
	for _, p := range r.replicas {
		for _, event := range entries {
			if event.StreamSequence > p.applied {
				if err := p.apply(event); err != nil {
					panic(err)
				}
			}
		}
		p.mu.Lock()
		p.highWater = p.applied
		p.checked = time.Now()
		p.mu.Unlock()
	}
	return reply
}

func testSessionReplicas(t *testing.T, sessionID int32) (*memoryStore, *Projection, *Projection) {
	t.Helper()
	ctx := context.Background()
	store := &memoryStore{}
	ref := sessionRef(sessionID)
	subject, _ := Subject(ref)
	claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"},
		Fact:  &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, _ := proto.Marshal(claim)
	if _, err := store.Publish(ctx, subject, 0, data); err != nil {
		t.Fatal(err)
	}
	zero := uint64(0)
	session := &pb.Session{Id: sessionID, Airport: "EKCH", Name: "LIVE",
		NextStripId: 1, NextCoordinationId: 1, NextTacticalId: 1, NextMessageId: 1}
	seed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{
			UpdateEntity: &pb.UpdateEntity{Key: fmt.Sprint(sessionID), Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}}}}}}
	if reply := (Writer{Store: store, NodeID: "node-a", Plan: PlanSystemEntity}).Execute(ctx, seed); reply.Status != pb.CommandReply_COMMITTED {
		t.Fatalf("seed: %v", reply)
	}
	makeProjection := func() *Projection {
		p := &Projection{states: map[string]*Aggregate{}, listeners: map[uint64]*projectionListener{},
			lastSnapshot: map[string]time.Time{}, sinceSnapshot: map[string]uint64{},
			presence: map[string]KVPresence{}, positions: map[string]KVPosition{}, syncFresh: map[string]bool{},
			started: true, startedAt: time.Now().Add(-time.Minute), checked: time.Now(),
			positionReady: true, presenceReady: true}
		entries, _ := store.Replay(ctx, subject)
		for _, entry := range entries {
			if err := p.apply(entry); err != nil {
				t.Fatal(err)
			}
		}
		p.highWater = p.applied
		return p
	}
	return store, makeProjection(), makeProjection()
}

func putTestPresence(p *Projection, clients ...*pb.ClientPresence) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for _, node := range []string{"node-a", "node-b"} {
		p.presence["node."+node] = KVPresence{Value: &pb.PresenceValue{SchemaVersion: 1,
			Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{NodeId: node, Ready: true}}}, Observed: now}
	}
	for _, client := range clients {
		p.presence["client."+client.ConnectionId] = KVPresence{Value: &pb.PresenceValue{SchemaVersion: 1,
			Present: &pb.PresenceValue_Client{Client: client}}, Observed: now}
	}
}

func TestMasterElectionSplitNodesTakeoverAndReconnectGeneration(t *testing.T) {
	const id int32 = 42
	store, first, second := testSessionReplicas(t, id)
	now := time.Now()
	tower := &pb.ClientPresence{ConnectionId: uuid.NewString(), NodeId: "node-a", SessionId: id,
		Cid: "tower", Callsign: "EKCH_A_TWR", Kind: pb.ClientPresence_EUROSCOPE,
		ConnectedAt: timestamppb.New(now.Add(-time.Minute))}
	fmp := &pb.ClientPresence{ConnectionId: uuid.NewString(), NodeId: "node-b", SessionId: id,
		Cid: "fmp", Callsign: "EKDK_FMP", Kind: pb.ClientPresence_EUROSCOPE,
		ConnectedAt: timestamppb.New(now)}
	for _, p := range []*Projection{first, second} {
		putTestPresence(p, tower, fmp)
	}
	writer := Writer{Store: store, NodeID: "node-a", Plan: MasterElectionPlanner(first, PlanSessionObservations)}
	router := electionTestRouter{writer: writer, store: store, replicas: []*Projection{first, second}}
	election := MasterElection{Projection: first, Router: router, Lease: writableLease(true)}
	master, err := election.Reconcile(context.Background(), id)
	if err != nil || master.ConnectionId != fmp.ConnectionId || master.Epoch != 1 {
		t.Fatalf("split election: %v %v", master, err)
	}
	if state, _ := second.Read(sessionRef(id)); state.Master.ConnectionId != fmp.ConnectionId {
		t.Fatal("other backend did not see committed master")
	}
	sync := &pb.SessionSync{ConnectionId: fmp.ConnectionId, MasterEpoch: 1, CompletedAt: timestamppb.Now()}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(id),
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-sync"},
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_RecordSync{
			RecordSync: &pb.RecordSessionSync{Sync: sync}}}}}
	if reply := router.Route(context.Background(), request); reply.Status != pb.CommandReply_COMMITTED {
		t.Fatalf("master sync: %v", reply)
	}
	if err := second.RequireMasterInbound(id, fmp.ConnectionId, fmp.Cid, 1, true); err != nil {
		t.Fatalf("remote master rejected after sync: %v", err)
	}
	if err := first.RequireMasterInbound(id, tower.ConnectionId, tower.Cid, 1, true); err == nil {
		t.Fatal("non-master accepted")
	}
	for _, p := range []*Projection{first, second} {
		p.mu.Lock()
		delete(p.presence, "client."+fmp.ConnectionId)
		p.mu.Unlock()
	}
	master, err = election.Reconcile(context.Background(), id)
	if err != nil || master.ConnectionId != tower.ConnectionId || master.Epoch != 2 {
		t.Fatalf("takeover: %v %v", master, err)
	}
	if err := second.RequireMasterInbound(id, fmp.ConnectionId, fmp.Cid, 1, true); err == nil {
		t.Fatal("stale master accepted after takeover")
	}
	staleSync := proto.Clone(request).(*pb.CommandRequest)
	staleSync.CommandId = uuid.NewString()
	staleSync.GetSystem().GetRecordSync().Sync.CompletedAt = timestamppb.Now()
	if reply := router.Route(context.Background(), staleSync); reply.Status != pb.CommandReply_UNAUTHORIZED {
		t.Fatalf("stale master sync was accepted: %v", reply)
	}
	if err := second.RequireMasterInbound(id, tower.ConnectionId, tower.Cid, 2, true); err == nil {
		t.Fatal("takeover inherited old sync")
	}
	newTower := proto.Clone(tower).(*pb.ClientPresence)
	newTower.ConnectionId = uuid.NewString()
	newTower.ConnectedAt = timestamppb.Now()
	for _, p := range []*Projection{first, second} {
		p.mu.Lock()
		delete(p.presence, "client."+tower.ConnectionId)
		p.mu.Unlock()
		putTestPresence(p, newTower)
	}
	master, err = election.Reconcile(context.Background(), id)
	if err != nil || master.ConnectionId != newTower.ConnectionId || master.Epoch != 3 {
		t.Fatalf("reconnect generation: %v %v", master, err)
	}
	if err := second.RequireMasterInbound(id, tower.ConnectionId, tower.Cid, 2, false); err == nil {
		t.Fatal("old socket generation accepted")
	}
	if err := second.RequireMasterInbound(id, newTower.ConnectionId, newTower.Cid, 3, true); err == nil {
		t.Fatal("new socket inherited sync")
	}
	frame := &euroscope.Envelope{SessionId: id, OwnerEpoch: 1, MasterEpoch: 2,
		Event: &euroscope.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &euroscope.AircraftPositionUpdateEvent{}}}
	if err := second.ValidateEuroScopeInbound(id, newTower.ConnectionId, newTower.Cid, frame); err == nil {
		t.Fatal("old master epoch frame accepted")
	}
}

func TestSessionFanoutBuffersInitialDeltaWithoutDuplicate(t *testing.T) {
	const id int32 = 77
	store, projection, _ := testSessionReplicas(t, id)
	kv := &presenceKVTest{values: map[string][]byte{}}
	lease, err := NewSocketPresenceLease(kv, "node-a", id, "123", "EKCH_A_TWR", "TWR", false, pb.ClientPresence_EUROSCOPE)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var revisions []uint64
	initialRevision := uint64(0)
	received := make(chan struct{}, 2)
	writer := Writer{Store: store, NodeID: "node-a", Plan: PlanSystemEntity}
	socket := LocalSessionSocket{
		OnInitial: func(session, airport *Aggregate) error {
			initialRevision = session.Revision
			record := session.Entities[fmt.Sprint(id)]
			copy := proto.Clone(record.GetValue().GetSession()).(*pb.Session)
			copy.LayoutId = "new-layout"
			expected := record.Revision
			request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(id),
				Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &expected,
				Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{
					UpdateEntity: &pb.UpdateEntity{Key: fmt.Sprint(id), Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: copy}}}}}}}
			if reply := writer.Execute(context.Background(), request); reply.Status != pb.CommandReply_COMMITTED {
				return fmt.Errorf("update: %v", reply)
			}
			subject, _ := Subject(sessionRef(id))
			entries, _ := store.Replay(context.Background(), subject)
			if err := projection.apply(entries[len(entries)-1]); err != nil {
				return err
			}
			projection.mu.Lock()
			projection.highWater = projection.applied
			projection.mu.Unlock()
			return nil
		},
		OnDelta: func(delta *pb.FrontendDelta) error {
			mu.Lock()
			revisions = append(revisions, delta.AggregateRevision)
			mu.Unlock()
			received <- struct{}{}
			return nil
		},
		OnRole: func(string, uint64, uint64) error { return nil },
	}
	fanout := &SessionFanout{Projection: projection, NodeID: "node-a"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closeSocket, err := fanout.Attach(ctx, lease, socket)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSocket()
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("delta during initial was lost")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(revisions) != 1 || revisions[0] != initialRevision+1 {
		t.Fatalf("gap or duplicate after initial: initial %d, deltas %v", initialRevision, revisions)
	}
}

func TestCIDTargetResolvesOnlyClaimedLiveGeneration(t *testing.T) {
	const id int32 = 91
	_, projection, _ := testSessionReplicas(t, id)
	old := &pb.ClientPresence{ConnectionId: uuid.NewString(), NodeId: "node-a", SessionId: id,
		Cid: "shared-cid", Kind: pb.ClientPresence_EUROSCOPE,
		ConnectedAt: timestamppb.New(time.Now().Add(-time.Minute))}
	reconnected := proto.Clone(old).(*pb.ClientPresence)
	reconnected.ConnectionId, reconnected.NodeId, reconnected.ConnectedAt = uuid.NewString(), "node-b", timestamppb.Now()
	putTestPresence(projection, old, reconnected)
	fanout := &SessionFanout{Projection: projection, NodeID: "node-a"}
	effect := &pb.EffectRecord{CommandId: uuid.NewString(), TargetCid: old.Cid,
		Status: pb.EffectRecord_DISPATCH_CLAIMED, DispatchConnectionId: &reconnected.ConnectionId}
	target, err := fanout.resolveCID(id, effect)
	if err != nil || target.ConnectionId != reconnected.ConnectionId || target.NodeId != "node-b" {
		t.Fatalf("claimed generation not selected: %v %v", target, err)
	}
	projection.mu.Lock()
	delete(projection.presence, "client."+reconnected.ConnectionId)
	projection.mu.Unlock()
	if _, err := fanout.resolveCID(id, effect); err == nil {
		t.Fatal("old generation received a claim for the reconnected socket")
	}
}

func TestNATSTargetedCIDAcrossBackendNodes(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned NATS fixture")
	}
	const id int32 = 92
	_, first, second := testSessionReplicas(t, id)
	origin := &pb.ClientPresence{ConnectionId: uuid.NewString(), NodeId: "node-a", SessionId: id,
		Cid: "origin", Kind: pb.ClientPresence_EUROSCOPE, ConnectedAt: timestamppb.Now()}
	target := &pb.ClientPresence{ConnectionId: uuid.NewString(), NodeId: "node-b", SessionId: id,
		Cid: "target", Kind: pb.ClientPresence_EUROSCOPE, ConnectedAt: timestamppb.Now()}
	for _, p := range []*Projection{first, second} {
		putTestPresence(p, origin, target)
	}
	effect := &pb.EffectRecord{CommandId: uuid.NewString(), TargetCid: target.Cid,
		DispatchConnectionId: &target.ConnectionId, OwnerEpoch: 1,
		Status: pb.EffectRecord_DISPATCH_CLAIMED, ResultDeadline: timestamppb.New(time.Now().Add(time.Minute)),
		Payload: &pb.EffectRecord_Pdc{Pdc: &pb.PdcEffect{Callsign: "SAS123", Action: "ISSUE"}}}
	subject, _ := Subject(sessionRef(id))
	for _, p := range []*Projection{first, second} {
		p.mu.Lock()
		p.states[subject].Effects[effect.CommandId] = proto.Clone(effect).(*pb.EffectRecord)
		p.mu.Unlock()
	}
	// The shared local fixture may have been started from an older checkout
	// whose backend credential predates delivery replies.
	url := "nats://bootstrap:bootstrap-local-only@127.0.0.1:4222"
	left, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer left.Close()
	right, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer right.Close()
	got := make(chan string, 2)
	fanoutRight := &SessionFanout{NC: right, Projection: second, NodeID: "node-b",
		sockets: map[string]*socketEntry{target.ConnectionId: {presence: target,
			socket: LocalSessionSocket{OnEffect: func(effect *pb.EffectRecord) error {
				got <- effect.CommandId
				return nil
			}}, delivered: map[string]bool{}}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- fanoutRight.ServeTargeted(ctx) }()
	time.Sleep(50 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("remote delivery subscriber stopped: %v", err)
	default:
	}
	fanoutLeft := &SessionFanout{NC: left, Projection: first, NodeID: "node-a"}
	if err := fanoutLeft.SendToCID(ctx, id, effect); err != nil {
		t.Fatal(err)
	}
	select {
	case command := <-got:
		if command != effect.CommandId {
			t.Fatalf("wrong CID received command %s", command)
		}
	case <-time.After(time.Second):
		t.Fatal("remote target did not receive command")
	}
	if err := fanoutLeft.SendToCID(ctx, id, effect); err == nil {
		t.Fatal("claimed command was delivered twice")
	}
	select {
	case <-got:
		t.Fatal("target received a duplicate")
	default:
	}
	cancel()
	<-done
}
