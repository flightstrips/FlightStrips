package cluster

import (
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestObservedInitialTagsStalePreviousEpochUntilFreshSync(t *testing.T) {
	now := time.Now()
	ref := sessionRef(42)
	subject, _ := Subject(ref)
	session := &pb.Session{Id: 42, Airport: "EKCH", Name: "LIVE", Master: &pb.MasterTerm{ConnectionId: "new-connection", Cid: "123", Epoch: 2, OwnerEpoch: 2},
		Sync: &pb.SessionSync{ConnectionId: "new-connection", MasterEpoch: 2, CompletedAt: timestamppb.New(now)}}
	state := NewAggregate(ref)
	state.Owner = &pb.OwnerTerm{NodeId: "node-2", Epoch: 2, LeaseUntil: timestamppb.New(now.Add(time.Minute))}
	state.Master = session.Master
	state.Sync = session.Sync
	state.Revision, state.StreamSequence = 5, 10
	state.Entities["42"] = &pb.EntitySnapshot{Key: "42", Revision: 2, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}}
	prior := &pb.PositionValue{SchemaVersion: 1, SessionId: 42, AircraftKey: "SAS101", OwnerEpoch: 1, SourceConnectionId: "old-connection",
		ObservedAt: timestamppb.New(now.Add(-time.Minute)), Observation: &pb.PositionValue_Position{Position: &pb.AircraftPosition{Latitude: 55, Longitude: 12}}}
	p := &Projection{states: map[string]*Aggregate{subject: state}, listeners: map[uint64]*projectionListener{},
		syncFresh: map[string]bool{},
		positions: map[string]KVPosition{positionKey(42, "SAS101", 1): {Value: prior, Revision: 7, Observed: now.Add(-time.Minute)}},
		presence:  map[string]KVPresence{}, started: true, startedAt: now, checked: now, positionReady: true, presenceReady: true, applied: 10, highWater: 10}
	initial, _, _, closeFirst, err := p.SubscribeObservedInitial(42)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Writable || initial.AggregateRevision != 5 || len(initial.TaggedObservations) != 1 || !initial.TaggedObservations[0].Stale || initial.TaggedObservations[0].SourceRevision != 7 {
		t.Fatalf("restart exposed old master observation as fresh: %v", initial)
	}
	closeFirst()
	p.startedAt = now.Add(-time.Second)
	p.syncFresh[subject] = true
	p.positions[positionKey(42, "SAS101", 2)] = KVPosition{Value: &pb.PositionValue{SchemaVersion: 1, SessionId: 42, AircraftKey: "SAS101", OwnerEpoch: 2,
		SourceConnectionId: "new-connection", ObservedAt: timestamppb.New(now), Observation: &pb.PositionValue_Position{Position: &pb.AircraftPosition{Latitude: 56, Longitude: 13}}},
		Revision: 11, Observed: now}
	p.presence["client.new-connection"] = KVPresence{Value: &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Client{Client: &pb.ClientPresence{
		ConnectionId: "new-connection", NodeId: "node-2", SessionId: 42, Cid: "123", Kind: pb.ClientPresence_EUROSCOPE}}}, Revision: 2, Observed: now}
	p.presence["node.node-2"] = KVPresence{Value: &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{
		NodeId: "node-2", Ready: true}}}, Revision: 3, Observed: now}
	initial, _, observations, closeSecond, err := p.SubscribeObservedInitial(42)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSecond()
	if !initial.Writable || len(initial.Positions) != 1 || initial.Positions[0].OwnerEpoch != 2 || len(initial.TaggedObservations) != 3 || initial.TaggedObservations[0].Stale {
		t.Fatalf("fresh sync did not replace prior epoch: %v", initial)
	}
	p.mu.Lock()
	p.publishObservationLocked(42, positionObservation(p.positions[positionKey(42, "SAS101", 2)], false, false))
	p.mu.Unlock()
	select {
	case live := <-observations:
		if live.SourceRevision != 11 || live.Stale {
			t.Fatalf("bad live observation tag: %v", live)
		}
	default:
		t.Fatal("registered listener missed live observation")
	}
}
