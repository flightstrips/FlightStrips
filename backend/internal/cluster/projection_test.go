package cluster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

func TestProjectionInitialThenLiveDeltaAndBarrier(t *testing.T) {
	store, writer, ref := fixture(t)
	p := &Projection{states: map[string]*Aggregate{}, listeners: map[uint64]*projectionListener{}, lastSnapshot: map[string]time.Time{}, sinceSnapshot: map[string]uint64{}, started: true, checked: time.Now(), positionReady: true, presenceReady: true, highWater: 1}
	subject, _ := Subject(ref)
	entries, _ := store.Replay(context.Background(), subject)
	if err := p.apply(entries[0]); err != nil {
		t.Fatal(err)
	}
	initial, deltas, closeFn, err := p.SubscribeInitial(ref)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	if initial.Revision != 0 || len(initial.Entities) != 0 {
		t.Fatal("initial snapshot crossed live event")
	}
	reply := writer.Execute(context.Background(), command(ref, "Copenhagen", 0))
	if reply.Status != pb.CommandReply_COMMITTED {
		t.Fatal(reply)
	}
	entries, _ = store.Replay(context.Background(), subject)
	if err := p.apply(entries[1]); err != nil {
		t.Fatal(err)
	}
	select {
	case delta := <-deltas:
		if delta.AggregateRevision != 1 || delta.StreamSequence != entries[1].StreamSequence || len(delta.Changes) != 1 {
			t.Fatalf("bad live delta: %v", delta)
		}
	default:
		t.Fatal("live delta lost after initial snapshot")
	}
	p.mu.Lock()
	p.highWater = entries[1].StreamSequence
	p.mu.Unlock()
	if err := p.WaitApplied(context.Background(), entries[1].StreamSequence); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := p.WaitApplied(ctx, entries[1].StreamSequence+1); err == nil {
		t.Fatal("stalled consumer crossed PubAck barrier")
	}
}

func TestNATSWriterRequiresLocalProjectionBarrier(t *testing.T) {
	_, _, ref := fixture(t)
	w := Writer{Store: NATSStore{}, NodeID: "node-a"}
	if reply := w.Execute(context.Background(), command(ref, "test", 0)); reply.Status != pb.CommandReply_UNAVAILABLE {
		t.Fatalf("NATS write bypassed local projection: %v", reply)
	}
}

func TestProjectionReadinessRejectsLagStaleMetadataAndUnknownVersion(t *testing.T) {
	store, _, ref := fixture(t)
	p := &Projection{states: map[string]*Aggregate{}, listeners: map[uint64]*projectionListener{}, lastSnapshot: map[string]time.Time{}, sinceSnapshot: map[string]uint64{}, started: true, checked: time.Now(), positionReady: true, presenceReady: true, highWater: 2}
	subject, _ := Subject(ref)
	entries, _ := store.Replay(context.Background(), subject)
	if err := p.apply(entries[0]); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	p.Readyz(w, request)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("lagging replay reported ready")
	}
	p.mu.Lock()
	p.highWater = 1
	p.checked = time.Now().Add(-3 * time.Second)
	p.mu.Unlock()
	w = httptest.NewRecorder()
	p.Readyz(w, request)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("stale metadata reported ready")
	}
	p.mu.Lock()
	p.checked = time.Now()
	p.mu.Unlock()
	w = httptest.NewRecorder()
	p.Readyz(w, request)
	if w.Code != http.StatusOK {
		t.Fatal("caught-up projection was unready")
	}
	bad := &pb.StateEvent{SchemaVersion: 2, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerRenewed{OwnerRenewed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, _ := proto.Marshal(bad)
	if err := p.apply(AppliedEvent{Subject: subject, StreamSequence: 2, ServerTime: time.Now(), Data: data}); err == nil {
		t.Fatal("unknown event version accepted")
	} else {
		p.fail(err)
	}
	w = httptest.NewRecorder()
	p.Readyz(w, request)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("unknown event version reported ready")
	}
}

func TestOperationalSyncNeedsRenewedMasterPresence(t *testing.T) {
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: 42}}}
	subject, _ := Subject(ref)
	now := time.Now()
	state := NewAggregate(ref)
	state.Master = &pb.MasterTerm{ConnectionId: "connection", Cid: "123", Epoch: 2}
	state.Sync = &pb.SessionSync{ConnectionId: "connection", MasterEpoch: 2}
	p := &Projection{states: map[string]*Aggregate{subject: state}, presence: map[string]KVPresence{}, started: true, startedAt: now.Add(-500 * time.Millisecond), checked: now, positionReady: true, presenceReady: true}
	p.presence["client.connection"] = KVPresence{Value: &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Client{Client: &pb.ClientPresence{ConnectionId: "connection", NodeId: "node", SessionId: 42, Cid: "123", Kind: pb.ClientPresence_EUROSCOPE}}}, Observed: now.Add(-time.Second)}
	p.presence["node.node"] = KVPresence{Value: &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{NodeId: "node", Ready: true}}}, Observed: now.Add(-time.Second)}
	if got, err := p.OperationalSync(ref); err != nil || got != nil {
		t.Fatalf("pre-restart presence validated durable sync: %v %v", got, err)
	}
	for key, item := range p.presence {
		item.Observed = now
		p.presence[key] = item
	}
	if got, err := p.OperationalSync(ref); err != nil || got == nil {
		t.Fatalf("renewed live master was not accepted: %v %v", got, err)
	}
}
