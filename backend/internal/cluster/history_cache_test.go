package cluster

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func testHistoryCache(t *testing.T) *historyCache {
	t.Helper()
	t.Setenv("NATS_HISTORY_CACHE_DIR", t.TempDir())
	cache, err := newHistoryCache()
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.close() })
	return cache
}
func historyOutcome(ref *pb.AggregateRef, id string, sequence uint64) *pb.CommandOutcome {
	return &pb.CommandOutcome{CommandId: id, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, RequestSha256: strings.Repeat("a", 64), Status: pb.CommandOutcome_SUCCEEDED, AggregateRevision: sequence, CommittedStreamSequence: sequence}
}

func TestHistoryCacheCleansStoppedCachesAndPreservesLiveCaches(t *testing.T) {
	t.Setenv("NATS_HISTORY_CACHE_DIR", t.TempDir())
	stopped, err := newHistoryCache()
	require.NoError(t, err)
	require.NoError(t, stopped.db.Close()) // model a process that could not unlink
	live, err := newHistoryCache()
	require.NoError(t, err)
	t.Cleanup(func() { _ = live.close() })
	_, err = os.Stat(stopped.path)
	require.True(t, os.IsNotExist(err))
	other, err := newHistoryCache()
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.close() })
	require.NoError(t, live.check())
	_, err = os.Stat(live.path)
	require.NoError(t, err)
}

func TestHistoryCachePreservesSnapshotAndCheckpointVersions(t *testing.T) {
	state := NewAggregate(globalRef())
	state.history = testHistoryCache(t)
	for i := 1; i <= 2048; i++ {
		id := uuid.NewString()
		state.Ledger[id] = historyOutcome(state.Ref, id, uint64(i))
		state.Workflows[id] = &pb.WorkflowRecord{WorkflowId: id, Source: state.Ref, Destination: state.Ref, Step: "external/test", DerivedCommandId: id, Status: pb.WorkflowRecord_COMPLETED}
		state.Effects[id] = &pb.EffectRecord{CommandId: id, Status: pb.EffectRecord_EXPIRED}
	}
	state.Revision = 2048
	state.StreamSequence = 2048
	state.SubjectSequence = 2048
	before, err := state.Snapshot()
	require.NoError(t, err)
	require.NoError(t, state.boundHistory())
	require.Len(t, state.Ledger, historyWorkingSet)
	require.Len(t, state.Workflows, historyWorkingSet)
	require.Len(t, state.Effects, historyWorkingSet)
	after, err := state.Snapshot()
	require.NoError(t, err)
	require.True(t, proto.Equal(before, after), "cold materialization changed the canonical snapshot")
	rebuilt, err := aggregateFromSnapshot(after)
	require.NoError(t, err)
	require.Len(t, rebuilt.Ledger, 2048)
	var coldID string
	for _, outcome := range before.Outcomes {
		if state.Ledger[outcome.CommandId] == nil {
			coldID = outcome.CommandId
			break
		}
	}
	old, err := state.LookupOutcome(coldID)
	require.NoError(t, err)
	require.NotNil(t, old)
	// A later cache version cannot leak into an earlier published checkpoint.
	newer := copyAggregateForApply(state)
	newer.StreamSequence++
	updated := proto.Clone(old).(*pb.CommandOutcome)
	updated.Detail = "newer"
	updated.CommittedStreamSequence = newer.StreamSequence
	newer.Ledger[coldID] = updated
	for i := 0; i < historyWorkingSet; i++ {
		id := uuid.NewString()
		newer.Ledger[id] = historyOutcome(newer.Ref, id, newer.StreamSequence+1)
	}
	require.NoError(t, newer.boundHistory())
	stillOld, err := state.LookupOutcome(coldID)
	require.NoError(t, err)
	require.Equal(t, old.Detail, stillOld.Detail)
}

func TestHistoryCacheOldRetryAndFailureRemainFenced(t *testing.T) {
	store, writer, ref := fixture(t)
	request := command(ref, "Copenhagen", 0)
	original := writer.Execute(context.Background(), request)
	require.Equal(t, pb.CommandReply_COMMITTED, original.Status)
	state := NewAggregate(ref)
	entries, err := store.Replay(context.Background(), "fs.v1.state.global")
	require.NoError(t, err)
	for _, entry := range entries {
		_, err := state.Apply(entry)
		require.NoError(t, err)
	}
	state.history = testHistoryCache(t)
	for i := 3; i <= 2048; i++ {
		id := uuid.NewString()
		state.Ledger[id] = historyOutcome(ref, id, uint64(i))
	}
	state.StreamSequence = 2048
	state.SubjectSequence = 2048
	require.NoError(t, state.boundHistory())
	require.Nil(t, state.Ledger[request.CommandId])
	projection := &Projection{states: map[string]*Aggregate{"fs.v1.state.global": state}, started: true, checked: time.Now(), positionReady: true, presenceReady: true, applied: 2048, highWater: 2048}
	writer.Projection = projection
	commits := store.commits
	retry := writer.Execute(context.Background(), request)
	require.Equal(t, original.Outcome, retry.Outcome)
	require.Equal(t, commits, store.commits)
	changed := proto.Clone(request).(*pb.CommandRequest)
	changed.GetSystem().GetCreateSession().Name = "Changed"
	require.Equal(t, pb.CommandReply_INVALID_ARGUMENT, writer.Execute(context.Background(), changed).Status)
	changed = proto.Clone(request).(*pb.CommandRequest)
	changed.Actor.Id = "other"
	require.Equal(t, pb.CommandReply_UNAUTHORIZED, writer.Execute(context.Background(), changed).Status)
	require.NoError(t, state.history.db.Close())
	projection.history = state.history
	require.ErrorContains(t, projection.Ready(), "history cache unavailable")
	require.Equal(t, pb.CommandReply_UNAVAILABLE, writer.Execute(context.Background(), request).Status)
	require.Equal(t, pb.CommandReply_UNAVAILABLE, writer.Execute(context.Background(), command(ref, "New", 1)).Status)
	require.Equal(t, commits, store.commits)
}

func TestProjectionHistoryMemoryPlateaus(t *testing.T) {
	if testing.Short() {
		t.Skip("retained-history growth qualification")
	}
	cache := testHistoryCache(t)
	ref := globalRef()
	subject, _ := Subject(ref)
	at := time.Now()
	p := &Projection{history: cache, states: map[string]*Aggregate{}, lastSnapshot: map[string]time.Time{}, sinceSnapshot: map[string]uint64{}, listeners: map[uint64]*projectionListener{}}
	owner := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, err := proto.Marshal(owner)
	require.NoError(t, err)
	require.NoError(t, p.apply(AppliedEvent{Subject: subject, StreamSequence: 1, SubjectSequence: 1, ServerTime: at, Data: data}))
	var baseline uint64
	for i := 1; i <= 50000; i++ {
		id := uuid.NewString()
		outcome := historyOutcome(ref, id, 0)
		outcome.AggregateRevision = 0
		event := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), CommandId: &id, Aggregate: ref, AggregateRevision: uint64(i), OwnerEpoch: 1, Actor: outcome.Actor, Fact: &pb.StateEvent_OutcomeRecorded{OutcomeRecorded: outcome}}
		data, err := proto.Marshal(event)
		require.NoError(t, err)
		// All facts use accepted server times inside the same live owner term.
		require.NoError(t, p.apply(AppliedEvent{Subject: subject, StreamSequence: uint64(i + 1), SubjectSequence: uint64(i + 1), ServerTime: at.Add(time.Second), Data: data}))
		p.sinceSnapshot[subject] = 0 // snapshot behavior has separate bounded tests
		if i == 5000 || i == 25000 || i == 50000 {
			runtime.GC()
			var memory runtime.MemStats
			runtime.ReadMemStats(&memory)
			state := p.states[subject]
			require.Len(t, state.Ledger, historyWorkingSet)
			info, err := os.Stat(cache.path)
			require.NoError(t, err)
			t.Logf("HISTORY_MEMORY events=%d hot_outcomes=%d heap_bytes=%d disk_index_bytes=%d", i, len(state.Ledger), memory.HeapAlloc, info.Size())
			if i == 5000 {
				baseline = memory.HeapAlloc
			} else {
				require.Less(t, int64(memory.HeapAlloc)-int64(baseline), int64(8<<20), fmt.Sprintf("heap must plateau while history grows to %d", i))
			}
		}
	}
}

func TestHistoryCachePinsPendingWorkAndBoundsSnapshotAllocation(t *testing.T) {
	state := NewAggregate(globalRef())
	state.history = testHistoryCache(t)
	state.StreamSequence = 4096
	state.SubjectSequence = 4096
	state.Revision = 4096
	for i := 1; i <= 2048; i++ {
		id := uuid.NewString()
		state.Ledger[id] = historyOutcome(state.Ref, id, uint64(i))
		state.Workflows[id] = &pb.WorkflowRecord{WorkflowId: id, Status: pb.WorkflowRecord_PENDING}
		state.Effects[id] = &pb.EffectRecord{CommandId: id, Status: pb.EffectRecord_WAITING}
	}
	require.NoError(t, state.boundHistory())
	require.Len(t, state.Ledger, 2048)
	require.Len(t, state.Workflows, 2048)
	require.Len(t, state.Effects, 2048)
	for _, effect := range state.Effects {
		effect.Status = pb.EffectRecord_EXPIRED
	}
	for _, workflow := range state.Workflows {
		workflow.Status = pb.WorkflowRecord_COMPLETED
	}
	require.NoError(t, state.boundHistory())
	require.Len(t, state.Ledger, historyWorkingSet)
	require.Len(t, state.Workflows, historyWorkingSet)
	require.Len(t, state.Effects, historyWorkingSet)
	// Cold records exceed the 32 MiB snapshot cap. Check size before decoding
	// more cold protobufs, while retaining lookup/replay authority on disk/NATS.
	for i := 2049; i <= 4096; i++ {
		id := uuid.NewString()
		outcome := historyOutcome(state.Ref, id, uint64(i))
		outcome.Detail = strings.Repeat("x", 32<<10)
		state.Ledger[id] = outcome
	}
	require.NoError(t, state.boundHistory())
	_, err := state.Snapshot()
	require.ErrorIs(t, err, ErrSnapshotTooLarge)
}

func TestHistoryCachePreservesLatestTransceiverCheckpoint(t *testing.T) {
	state := NewAggregate(globalRef())
	state.history = testHistoryCache(t)
	state.StreamSequence = 2048
	for i := 0; i < 1024; i++ {
		id := uuid.NewString()
		state.Workflows[id] = &pb.WorkflowRecord{WorkflowId: id, Step: "other", Status: pb.WorkflowRecord_COMPLETED}
	}
	id := "00000000-0000-4000-8000-000000000001"
	revision := uint64(123)
	state.Workflows[id] = &pb.WorkflowRecord{WorkflowId: id, Step: transceiverSectorStep + "hash", SourceRevision: &revision, Status: pb.WorkflowRecord_COMPLETED}
	require.NoError(t, state.boundHistory())
	require.Len(t, state.Workflows, historyWorkingSet+1)
	require.Equal(t, revision, transceiverAppliedRevision(state))
}
