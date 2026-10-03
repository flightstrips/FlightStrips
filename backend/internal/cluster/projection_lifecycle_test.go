package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

func lifecyclePlanningFixture() (*Projection, *Aggregate) {
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: 42}}}
	subject, _ := Subject(ref)
	state := NewAggregate(ref)
	state.Revision, state.StreamSequence, state.SubjectSequence = 8, 8, 8
	state.Owner = &pb.OwnerTerm{NodeId: "node", Epoch: 2}
	state.Master = &pb.MasterTerm{ConnectionId: "connection", Cid: "123", Epoch: 2, OwnerEpoch: 2}
	state.Sync = &pb.SessionSync{ConnectionId: "connection", MasterEpoch: 2}
	for i := 0; i < 200; i++ {
		key := fmt.Sprintf("SAS%03d", i)
		state.Entities[key] = &pb.EntitySnapshot{Key: key, Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: key, Stand: "A1", Destination: "EKCH", Bay: "FINAL"}}}}
	}
	for i := 0; i < 512; i++ {
		key := fmt.Sprintf("command-%03d", i)
		state.Ledger[key] = &pb.CommandOutcome{CommandId: key, Status: pb.CommandOutcome_SUCCEEDED, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "lifecycle"}}
		state.Effects[key] = &pb.EffectRecord{CommandId: key, Status: pb.EffectRecord_EXECUTED}
	}
	state.Workflows["warning"] = &pb.WorkflowRecord{WorkflowId: "warning", Step: "wrong-stand", Status: pb.WorkflowRecord_COMPLETED}
	state.rebuildIndexes()
	p := &Projection{states: map[string]*Aggregate{subject: state}, started: true, checked: time.Now(), positionReady: true, presenceReady: true, applied: 8, highWater: 8}
	return p, state
}

func TestLifecyclePlanningPreservesDetachedInputsAndArchivedWorkflows(t *testing.T) {
	p, state := lifecyclePlanningFixture()
	for i := 0; i < 1024; i++ {
		key := fmt.Sprintf("workflow-%04d", i)
		state.Workflows[key] = &pb.WorkflowRecord{WorkflowId: key, Step: "prefile-consumed", Status: pb.WorkflowRecord_COMPLETED}
	}

	read, err := p.ReadLifecyclePlanning(state.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Entities) != len(state.Entities) || read.Revision != state.Revision || read.StreamSequence != state.StreamSequence || !proto.Equal(read.Owner, state.Owner) || !proto.Equal(read.Master, state.Master) || !proto.Equal(read.Sync, state.Sync) {
		t.Fatal("planning snapshot lost coherent owner/entity checkpoint")
	}
	for key, value := range state.Entities {
		if !proto.Equal(value, read.Entities[key]) {
			t.Fatalf("lost entity %s", key)
		}
	}
	for i := 0; i < 1024; i++ {
		key := fmt.Sprintf("workflow-%04d", i)
		got, err := read.LookupWorkflow(key)
		if err != nil || got == nil || got.Step != "prefile-consumed" {
			t.Fatalf("lost consumed input %s: %v %v", key, got, err)
		}
	}
	read.Entities["SAS000"].Value.GetStrip().Stand = "A2"
	read.Owner.Epoch++
	for _, value := range read.Workflows {
		value.Step = "mutated"
	}
	if state.Entities["SAS000"].Value.GetStrip().Stand != "A1" || state.Owner.Epoch != 2 {
		t.Fatal("planning mutation reached published projection")
	}
	for _, value := range state.Workflows {
		if value.Step == "mutated" {
			t.Fatal("planning workflow mutation reached published projection")
		}
	}
	p.healthErr = errors.New("quorum lost")
	if _, err := p.ReadLifecyclePlanning(state.Ref); err == nil {
		t.Fatal("planning snapshot bypassed fail-closed health barrier")
	}
}

func BenchmarkLifecyclePlanningSnapshot(b *testing.B) {
	for _, lean := range []bool{false, true} {
		name := "complete"
		if lean {
			name = "lifecycle"
		}
		b.Run(name, func(b *testing.B) {
			p, state := lifecyclePlanningFixture()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				p.checked = time.Now()
				var err error
				if lean {
					_, err = p.ReadLifecyclePlanning(state.Ref)
				} else {
					_, err = p.Read(state.Ref)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestLifecyclePlanningCacheRefreshesCommittedNeighborsAndFences(t *testing.T) {
	p, state := lifecyclePlanningFixture()
	subject, _ := Subject(state.Ref)
	first, err := p.ReadLifecyclePlanning(state.Ref)
	if err != nil {
		t.Fatal(err)
	}
	same, err := p.ReadLifecyclePlanningCached(state.Ref, first)
	if err != nil || same != first {
		t.Fatalf("unchanged detached input was not reused: %v", err)
	}
	committed := copyAggregateForApply(state)
	committed.Revision++
	committed.StreamSequence++
	committed.SubjectSequence++
	committed.Entities["SAS000"] = proto.Clone(state.Entities["SAS000"]).(*pb.EntitySnapshot)
	committed.Entities["SAS000"].Value.GetStrip().Stand = "A2"
	committed.Workflows["allocation"] = &pb.WorkflowRecord{WorkflowId: "allocation", Step: "stand-allocated", Status: pb.WorkflowRecord_PENDING}
	committed.Owner.Epoch++
	committed.rebuildIndexes()
	p.states[subject] = committed
	fresh, err := p.ReadLifecyclePlanningCached(state.Ref, first)
	if err != nil || fresh == first || fresh.Entities["SAS000"].Value.GetStrip().Stand != "A2" || fresh.Workflows["allocation"] == nil || fresh.Owner.Epoch != committed.Owner.Epoch {
		t.Fatalf("later planner missed committed neighbor/terms/workflows: %v", err)
	}
	if first.Entities["SAS000"].Value.GetStrip().Stand != "A1" || first.Owner.Epoch != 2 {
		t.Fatal("later commit mutated earlier planning inputs")
	}
	// An ignored stale-owner event may advance stream checkpoints without an
	// aggregate revision change. Its checkpoint must invalidate reuse as well.
	checkpoint := copyAggregateForApply(committed)
	checkpoint.StreamSequence++
	checkpoint.SubjectSequence++
	p.states[subject] = checkpoint
	advanced, err := p.ReadLifecyclePlanningCached(state.Ref, fresh)
	if err != nil || advanced == fresh || advanced.StreamSequence != checkpoint.StreamSequence {
		t.Fatalf("stream checkpoint did not invalidate reuse: %v", err)
	}
	otherRef := &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: 43}}}
	other := NewAggregate(otherRef)
	other.Revision, other.StreamSequence, other.SubjectSequence = advanced.Revision, advanced.StreamSequence, advanced.SubjectSequence
	otherSubject, _ := Subject(otherRef)
	p.states[otherSubject] = other
	otherRead, err := p.ReadLifecyclePlanningCached(otherRef, advanced)
	if err != nil || otherRead == advanced || !proto.Equal(otherRead.Ref, otherRef) || len(otherRead.Entities) != 0 {
		t.Fatalf("cache crossed aggregate boundaries: %v", err)
	}
	p.healthErr = errors.New("quorum lost")
	if _, err := p.ReadLifecyclePlanningCached(state.Ref, advanced); err == nil {
		t.Fatal("unchanged cache bypassed failed health fence")
	}
}

func BenchmarkLifecyclePlanningCachedSnapshot(b *testing.B) {
	p, state := lifecyclePlanningFixture()
	previous, err := p.ReadLifecyclePlanning(state.Ref)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.checked = time.Now()
		if _, err := p.ReadLifecyclePlanningCached(state.Ref, previous); err != nil {
			b.Fatal(err)
		}
	}
}
