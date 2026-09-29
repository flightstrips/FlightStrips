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

type memoryStore struct {
	mu           sync.Mutex
	entries      []AppliedEvent
	loseAck      bool
	failAfterAck bool
	failReplay   bool
	commits      int
}

func (s *memoryStore) Replay(_ context.Context, subject string) ([]AppliedEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failReplay {
		s.failReplay = false
		return nil, errors.New("request/reply timeout")
	}
	out := []AppliedEvent{}
	for _, e := range s.entries {
		if e.Subject == subject {
			e.Data = append([]byte(nil), e.Data...)
			out = append(out, e)
		}
	}
	return out, nil
}
func (s *memoryStore) Publish(_ context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var last uint64
	for _, e := range s.entries {
		if e.Subject == subject {
			last = e.SubjectSequence
		}
	}
	if expected != last {
		return 0, ErrCAS
	}
	seq := uint64(len(s.entries) + 1)
	s.entries = append(s.entries, AppliedEvent{Subject: subject, StreamSequence: seq, SubjectSequence: seq, ServerTime: time.Now(), Data: append([]byte(nil), data...)})
	s.commits++
	if s.loseAck {
		s.loseAck = false
		if s.failAfterAck {
			s.failReplay = true
			s.failAfterAck = false
		}
		return 0, errors.New("lost PubAck")
	}
	return seq, nil
}

func fixture(t *testing.T) (*memoryStore, Writer, *pb.AggregateRef) {
	t.Helper()
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
	s := &memoryStore{}
	id := uuid.NewString()
	e := &pb.StateEvent{SchemaVersion: 1, EventId: id, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	b, err := proto.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(context.Background(), "fs.v1.state.global", 0, b); err != nil {
		t.Fatal(err)
	}
	plan := func(_ context.Context, r *pb.CommandRequest, a *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		current := uint64(0)
		if old := a.Entities["EKCH"]; old != nil {
			current = old.Revision
		}
		if r.ExpectedEntityRevision != nil && *r.ExpectedEntityRevision != current {
			return nil, pb.CommandReply_REVISION_CONFLICT, current, errors.New("stale entity revision")
		}
		name := r.GetSystem().GetCreateSession().GetName()
		return &pb.DomainChange{Changes: []*pb.EntityChange{{Key: "EKCH", Revision: current + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_AirportRegistry{AirportRegistry: &pb.AirportRegistry{Icao: "EKCH", DisplayName: name}}}}}}}, pb.CommandReply_COMMITTED, current, nil
	}
	return s, Writer{Store: s, NodeID: "node-a", Plan: plan}, ref
}

func command(ref *pb.AggregateRef, name string, revision uint64) *pb.CommandRequest {
	return &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &revision, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Id: 1, Airport: "EKCH", Name: name}}}}}
}

func TestLostPubAckAndChangedPayload(t *testing.T) {
	s, w, ref := fixture(t)
	r := command(ref, "Copenhagen", 0)
	s.loseAck = true
	first := w.Execute(context.Background(), r)
	if first.Status != pb.CommandReply_COMMITTED || first.Outcome.CommittedStreamSequence != 2 {
		t.Fatalf("lost PubAck did not resolve: %v", first)
	}
	again := w.Execute(context.Background(), r)
	if again.Status != pb.CommandReply_COMMITTED || again.GetStreamSequence() != first.GetStreamSequence() || s.commits != 2 {
		t.Fatalf("retry appended another event: %v", again)
	}
	changed := proto.Clone(r).(*pb.CommandRequest)
	changed.GetSystem().GetCreateSession().Name = "Different"
	if reply := w.Execute(context.Background(), changed); reply.Status != pb.CommandReply_INVALID_ARGUMENT || s.commits != 2 {
		t.Fatalf("changed payload accepted: %v", reply)
	}
	other := proto.Clone(r).(*pb.CommandRequest)
	other.Actor.Id = "other"
	if reply := w.Execute(context.Background(), other); reply.Status != pb.CommandReply_UNAUTHORIZED {
		t.Fatalf("foreign actor read outcome: %v", reply)
	}
	if reply := w.Outcome(context.Background(), ref, r.CommandId, other.Actor); reply.Status != pb.CommandReply_UNAUTHORIZED {
		t.Fatalf("foreign actor queried outcome: %v", reply)
	}
}

func TestCompetingWritersRevalidateAfterCAS(t *testing.T) {
	s, w, ref := fixture(t)
	one, two := command(ref, "One", 0), command(ref, "Two", 0)
	basePlan := w.Plan
	gate := make(chan struct{})
	var planned int
	var mu sync.Mutex
	w.Plan = func(ctx context.Context, r *pb.CommandRequest, a *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		mu.Lock()
		planned++
		if planned == 2 {
			close(gate)
		}
		mu.Unlock()
		<-gate
		return basePlan(ctx, r, a)
	}
	var wait sync.WaitGroup
	wait.Add(2)
	replies := make([]*pb.CommandReply, 2)
	go func() { defer wait.Done(); replies[0] = w.Execute(context.Background(), one) }()
	go func() { defer wait.Done(); replies[1] = w.Execute(context.Background(), two) }()
	wait.Wait()
	var success, conflict int
	for _, r := range replies {
		if r.Status != pb.CommandReply_COMMITTED {
			t.Fatalf("unexpected transport result: %v", r)
		}
		if r.Outcome.Status == pb.CommandOutcome_SUCCEEDED {
			success++
		} else if r.Outcome.ReasonCode == pb.CommandReply_REVISION_CONFLICT.String() {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("same entity revision committed twice: %v", replies)
	}
	state, err := w.load(context.Background(), "fs.v1.state.global", ref)
	if err != nil {
		t.Fatal(err)
	}
	if state.Entities["EKCH"].Revision != 1 || len(state.Ledger) != 2 || s.commits != 3 {
		t.Fatalf("bad replay: %+v", state)
	}
}

func TestStaleOwnerEventIsDeterministicNoOp(t *testing.T) {
	s, _, ref := fixture(t)
	e := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, AggregateRevision: 0, OwnerEpoch: 0, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "old"}, Fact: &pb.StateEvent_OwnerRenewed{OwnerRenewed: &pb.OwnerTerm{NodeId: "old", Epoch: 0}}}
	b, _ := proto.Marshal(e)
	if _, err := s.Publish(context.Background(), "fs.v1.state.global", 1, b); err != nil {
		t.Fatal(err)
	}
	a, bstate := NewAggregate(ref), NewAggregate(ref)
	entries, _ := s.Replay(context.Background(), "fs.v1.state.global")
	for _, v := range entries {
		if _, err := a.Apply(v); err != nil {
			t.Fatal(err)
		}
		if _, err := bstate.Apply(v); err != nil {
			t.Fatal(err)
		}
	}
	if a.Owner.NodeId != "node-a" || !proto.Equal(a.Owner, bstate.Owner) || a.SubjectSequence != 2 {
		t.Fatalf("stale control changed owner: %+v", a)
	}
	one, err := a.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	two, err := bstate.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	oneBytes, _ := (proto.MarshalOptions{Deterministic: true}).Marshal(one)
	twoBytes, _ := (proto.MarshalOptions{Deterministic: true}).Marshal(two)
	if string(oneBytes) != string(twoBytes) {
		t.Fatal("same replay produced different snapshot bytes")
	}
}

func TestStaleDomainEventDoesNotEnterLedger(t *testing.T) {
	s, _, ref := fixture(t)
	id := uuid.NewString()
	e := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), CommandId: &id, Aggregate: ref, AggregateRevision: 1, OwnerEpoch: 0, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "old"}, Fact: &pb.StateEvent_DomainChanged{DomainChanged: &pb.DomainChange{}}}
	b, _ := proto.Marshal(e)
	if _, err := s.Publish(context.Background(), "fs.v1.state.global", 1, b); err != nil {
		t.Fatal(err)
	}
	a := NewAggregate(ref)
	entries, _ := s.Replay(context.Background(), "fs.v1.state.global")
	for _, entry := range entries {
		if _, err := a.Apply(entry); err != nil {
			t.Fatal(err)
		}
	}
	if a.Revision != 0 || a.SubjectSequence != 2 || len(a.Ledger) != 0 {
		t.Fatalf("stale event changed state: %+v", a)
	}
}

func TestUncertainReplyTimeoutRetriedWithSameID(t *testing.T) {
	s, w, ref := fixture(t)
	r := command(ref, "Committed", 0)
	s.loseAck, s.failAfterAck = true, true
	first := w.Execute(context.Background(), r)
	if first.Status != pb.CommandReply_UNAVAILABLE {
		t.Fatalf("uncertain acknowledgment reported success: %v", first)
	}
	second := w.Execute(context.Background(), r)
	if second.Status != pb.CommandReply_COMMITTED || second.GetStreamSequence() != 2 || s.commits != 2 {
		t.Fatalf("retry did not recover committed outcome: %v", second)
	}
}

func TestTypedSystemReplacementAndNumericKey(t *testing.T) {
	s, _, ref := fixture(t)
	w := Writer{Store: s, NodeID: "node-a"}
	zero := uint64(0)
	r := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_SessionRegistry{SessionRegistry: &pb.SessionRegistry{Id: 1, Airport: "EKCH", Name: "Session", State: pb.SessionRegistry_ACTIVE}}}}}}}}
	if reply := w.Execute(context.Background(), r); reply.Status != pb.CommandReply_COMMITTED {
		t.Fatalf("typed replacement failed: %v", reply)
	}
	state, err := w.load(context.Background(), "fs.v1.state.global", ref)
	if err != nil {
		t.Fatal(err)
	}
	if state.Entities["1"].GetValue().GetSessionRegistry().Id != 1 {
		t.Fatal("numeric key was not projected")
	}
}

func TestTypedCommandCodecAndNormalization(t *testing.T) {
	_, _, ref := fixture(t)
	r := command(ref, "Codec", 0)
	encoded, err := EncodeCommandRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCommandRequest(encoded)
	if err != nil || !proto.Equal(r, decoded) {
		t.Fatalf("typed request round trip: %v", err)
	}
	first, err := RequestHash(r)
	if err != nil {
		t.Fatal(err)
	}
	copy := proto.Clone(r).(*pb.CommandRequest)
	copy.CommandId = uuid.NewString()
	second, err := RequestHash(copy)
	if err != nil || first != second {
		t.Fatal("command transport ID changed payload digest")
	}
	reply := &pb.CommandReply{ProtocolRevision: 1, CommandId: r.CommandId, Status: pb.CommandReply_COMMITTED}
	b, err := EncodeCommandReply(reply)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCommandReply(b); err != nil {
		t.Fatal(err)
	}
}
