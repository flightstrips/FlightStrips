package cluster

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	euroscope "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The result crosses the real three-peer Core NATS cluster. The state store is
// fault injectable so the old claimed generation and new result socket can be
// observed independently without waiting for a live EuroScope client.
func TestEffectResultResendAcrossThreeNodes(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	base := 4222
	if value := os.Getenv("NATS_TEST_PORT_BASE"); value != "" {
		var err error
		base, err = strconv.Atoi(value)
		if err != nil {
			t.Fatal(err)
		}
	}
	connect := func(port int) *nats.Conn {
		nc, err := nats.Connect(fmt.Sprintf("nats://backend:backend-local-only@127.0.0.1:%d", port))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(nc.Close)
		return nc
	}
	ownerNC, socketNC, thirdNC := connect(base), connect(base+1), connect(base+2)
	if thirdNC.Status() != nats.CONNECTED {
		t.Fatal("third peer disconnected")
	}
	ownerID, socketID, commandID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	oldGeneration, newGeneration := uuid.NewString(), uuid.NewString()
	ref := sessionRef(941)
	subject, _ := Subject(ref)
	store := &memoryStore{}
	state := NewAggregate(ref)
	appendEvent := func(event *pb.StateEvent) {
		data, err := proto.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Publish(context.Background(), subject, state.SubjectSequence, data); err != nil {
			t.Fatal(err)
		}
		entry := store.entries[len(store.entries)-1]
		if _, err := state.Apply(entry); err != nil {
			t.Fatal(err)
		}
	}
	appendEvent(&pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: ownerID},
		Fact:  &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: ownerID, Epoch: 1}}})
	request := &pb.EffectRecord{CommandId: commandID, TargetCid: "controller-1", OwnerEpoch: 1,
		Status: pb.EffectRecord_WAITING, DispatchDeadline: timestamppb.New(time.Now().Add(effectDispatchWindow)),
		Payload: &pb.EffectRecord_Pdc{Pdc: &pb.PdcEffect{Callsign: "SAS123", Action: "ISSUE"}}}
	actor := &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "controller-1"}
	appendEvent(&pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), CommandId: &commandID,
		Aggregate: ref, AggregateRevision: 1, OwnerEpoch: 1, Actor: actor,
		Fact: &pb.StateEvent_DomainChanged{DomainChanged: &pb.DomainChange{Effects: []*pb.EffectRecord{request},
			Outcome: &pb.CommandOutcome{CommandId: commandID, RequestSha256: fmt.Sprintf("%064x", 1), Actor: actor,
				Status: pb.CommandOutcome_ACCEPTED, Aggregate: ref, AggregateRevision: 1}}}})
	claim := proto.Clone(state.Effects[commandID]).(*pb.EffectRecord)
	claim.Status, claim.DispatchConnectionId = pb.EffectRecord_DISPATCH_CLAIMED, &oldGeneration
	appendEvent(&pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), CommandId: &commandID,
		Aggregate: ref, AggregateRevision: 2, OwnerEpoch: 1, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: ownerID},
		Fact: &pb.StateEvent_EffectChanged{EffectChanged: claim}})
	makeProjection := func() *Projection {
		copy, err := cloneAggregate(state)
		if err != nil {
			t.Fatal(err)
		}
		p := &Projection{states: map[string]*Aggregate{subject: copy}, listeners: map[uint64]*projectionListener{},
			lastSnapshot: map[string]time.Time{subject: time.Now()}, sinceSnapshot: map[string]uint64{},
			presence: map[string]KVPresence{}, positions: map[string]KVPosition{}, syncFresh: map[string]bool{},
			started: true, startedAt: time.Now().Add(-time.Minute), checked: time.Now(),
			positionReady: true, presenceReady: true, applied: state.StreamSequence, highWater: state.StreamSequence}
		now := time.Now()
		for _, id := range []string{ownerID, socketID} {
			p.presence["node."+id] = KVPresence{Observed: now, Value: &pb.PresenceValue{SchemaVersion: 1,
				Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{NodeId: id, Ready: true}}}}
		}
		p.presence["client."+newGeneration] = KVPresence{Observed: now, Value: &pb.PresenceValue{SchemaVersion: 1,
			Present: &pb.PresenceValue_Client{Client: &pb.ClientPresence{ConnectionId: newGeneration,
				NodeId: socketID, SessionId: 941, Cid: "controller-1", Kind: pb.ClientPresence_EUROSCOPE,
				ConnectedAt: timestamppb.New(now)}}}}
		return p
	}
	ownerProjection, socketProjection := makeProjection(), makeProjection()
	owner := &OwnerRuntime{NC: ownerNC, Projection: ownerProjection, Store: store, NodeID: ownerID,
		tracked: map[string]*pb.AggregateRef{subject: ref}, lastRenew: map[string]time.Time{}, failed: map[string]bool{}}
	owner.mark(subject, true)
	socket := &OwnerRuntime{NC: socketNC, Projection: socketProjection, Store: store, NodeID: socketID,
		tracked: map[string]*pb.AggregateRef{subject: ref}, lastRenew: map[string]time.Time{}, failed: map[string]bool{}}
	ownerEffects := Effects{Owner: owner, Fanout: &SessionFanout{NC: ownerNC, Projection: ownerProjection, NodeID: ownerID}}
	socketEffects := Effects{Owner: socket, Fanout: &SessionFanout{NC: socketNC, Projection: socketProjection, NodeID: socketID}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serverDone := make(chan error, 1)
	go func() { serverDone <- ownerEffects.ServeResults(ctx) }()
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				entries, _ := store.Replay(ctx, subject)
				for _, p := range []*Projection{ownerProjection, socketProjection} {
					for _, entry := range entries {
						p.mu.RLock()
						applied := p.applied
						p.mu.RUnlock()
						if entry.StreamSequence > applied {
							_ = p.apply(entry)
						}
					}
					p.mu.Lock()
					p.highWater, p.checked = p.applied, time.Now()
					p.mu.Unlock()
				}
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	frame := &euroscope.Envelope{CommandId: commandID, SessionId: 941, OwnerEpoch: 1,
		Event: &euroscope.Envelope_CommandResult{CommandResult: &euroscope.CommandResultEvent{
			CommandId: commandID, Status: euroscope.CommandResultEvent_EXECUTED,
			Reason: euroscope.CommandResultEvent_OK}}}
	for i := 0; i < 2; i++ {
		if err := socketEffects.RecordResult(ctx, 941, newGeneration, "controller-1", frame); err != nil {
			t.Fatalf("plugin outbox resend %d: %v", i, err)
		}
	}
	result, err := ownerProjection.Read(ref)
	if err != nil || result.Effects[commandID].Status != pb.EffectRecord_EXECUTED ||
		result.Ledger[commandID].Status != pb.CommandOutcome_SUCCEEDED || store.commits != 4 {
		t.Fatalf("result not persisted exactly once: state=%v commits=%d err=%v", result, store.commits, err)
	}
	cancel()
	<-serverDone
}
