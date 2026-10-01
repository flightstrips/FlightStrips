package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"os"
	"testing"
	"time"
)

// An opt-in local snapshot permits profiling retained-state cost without
// modifying the running cluster or committing its operational records.
func BenchmarkProjectionRetainedStateCopy(b *testing.B) {
	state := retainedProfileState(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy, err := cloneAggregate(state)
		if err != nil || len(copy.Ledger) != len(state.Ledger) {
			b.Fatal("copy failed", err)
		}
	}
}

func retainedProfileState(b *testing.B) *Aggregate {
	b.Helper()
	path := os.Getenv("NATS_PROFILE_SNAPSHOT")
	if path == "" {
		b.Skip("set NATS_PROFILE_SNAPSHOT to a private verified snapshot")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	snapshot := &pb.Snapshot{}
	if err := pb.UnmarshalStrict(data, snapshot); err != nil {
		b.Fatal(err)
	}
	state, err := aggregateFromSnapshot(snapshot)
	if err != nil {
		b.Fatal(err)
	}
	b.Logf("entities=%d outcomes=%d workflows=%d bytes=%d", len(state.Entities), len(state.Ledger), len(state.Workflows), len(data))
	return state
}

func BenchmarkProjectionOwnerCheckpoint(b *testing.B) {
	state := retainedProfileState(b)
	subject, err := Subject(state.Ref)
	if err != nil {
		b.Fatal(err)
	}
	p := &Projection{started: true, positionReady: true, presenceReady: true, states: map[string]*Aggregate{subject: state}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.checked = time.Now()
		view, err := p.readOwner(state.Ref)
		if err != nil || view.SubjectSequence != state.SubjectSequence {
			b.Fatal("owner checkpoint failed", err)
		}
	}
}

func BenchmarkProjectionApplyCopy(b *testing.B) {
	state := retainedProfileState(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy := copyAggregateForApply(state)
		if len(copy.Ledger) != len(state.Ledger) {
			b.Fatal("apply copy failed")
		}
	}
}
