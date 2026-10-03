package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestRetryWindowSurvivesSnapshotAndPinsPendingWork(t *testing.T) {
	state := NewAggregate(globalRef())
	state.Revision, state.StreamSequence, state.SubjectSequence = retryWindow+100, retryWindow+100, retryWindow+100
	ids := make([]string, retryWindow+100)
	for i := range ids {
		id := uuid.NewString()
		ids[i] = id
		state.Ledger[id] = &pb.CommandOutcome{CommandId: id, Aggregate: state.Ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, RequestSha256: strings.Repeat("a", 64), Status: pb.CommandOutcome_SUCCEEDED, CommittedStreamSequence: uint64(i + 1), AggregateRevision: uint64(i + 1)}
	}
	state.Effects[ids[0]] = &pb.EffectRecord{CommandId: ids[0], Status: pb.EffectRecord_WAITING}
	state.Workflows[ids[1]] = &pb.WorkflowRecord{WorkflowId: ids[1], DerivedCommandId: ids[1], Status: pb.WorkflowRecord_PENDING}
	// Loading a legacy full-history snapshot now materializes bounded state.
	legacy, err := state.snapshotCurrent()
	require.NoError(t, err)
	rebuilt, err := aggregateFromSnapshot(legacy)
	require.NoError(t, err)
	require.Len(t, rebuilt.Ledger, retryWindow+2)
	require.NotNil(t, rebuilt.Ledger[ids[0]])
	require.NotNil(t, rebuilt.Ledger[ids[1]])
	require.Nil(t, rebuilt.Ledger[ids[2]])
	require.NotNil(t, rebuilt.Ledger[ids[len(ids)-1]])
	require.Len(t, state.Ledger, len(ids), "trimming must not mutate a published older state")
	snapshot, err := rebuilt.Snapshot()
	require.NoError(t, err)
	again, err := aggregateFromSnapshot(snapshot)
	require.NoError(t, err)
	second, err := again.Snapshot()
	require.NoError(t, err)
	require.True(t, proto.Equal(snapshot, second), "snapshot/recovery must agree on the retry window")
}

func TestRetryWindowNeverEvictsUnpersistedReceipt(t *testing.T) {
	state := NewAggregate(globalRef())
	for i := 0; i < retryWindow+1; i++ {
		id := fmt.Sprint(i)
		state.Ledger[id] = &pb.CommandOutcome{CommittedStreamSequence: uint64(i + 1)}
	}
	state.Ledger["pending"] = &pb.CommandOutcome{}
	state.trimRecords()
	require.Len(t, state.Ledger, retryWindow+1)
	require.NotNil(t, state.Ledger["pending"])
}

func TestProviderReadCheckpointRetainsPageAcrossFailureWithoutWorkflow(t *testing.T) {
	ctx := context.Background()
	store, _, nav := navFixture(t)
	worker := ExternalCallWorker{Writer: nav.Writer}
	page := &pb.ProviderPage{Provider: "airacnet", Resource: "airport/EKCH", Parsed: &pb.ProviderPage_Airac{Airac: &pb.AiracPage{Fragments: []*pb.NavData{testNav("2610", "Copenhagen")}}}}
	firstID := uuid.NewString()
	sent, err := nav.FetchProviderPageFenced(ctx, worker, firstID, "EKCH", "airacnet", "airport/EKCH", func(context.Context, *pb.ProviderCheckpoint, *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
		return page, &pb.ProviderCheckpoint{Provider: page.Provider, Resource: page.Resource}, nil
	})
	require.NoError(t, err)
	require.True(t, sent)
	prior, _, revision, err := nav.CheckpointRevisionFor(ctx, airportRef("EKCH"), page.Provider, page.Resource)
	require.NoError(t, err)
	require.Equal(t, uint64(1), revision)
	failureID := uuid.NewString()
	calls := 0
	fetch := func(context.Context, *pb.ProviderCheckpoint, *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
		calls++
		pending, _, current, err := nav.CheckpointRevisionFor(ctx, airportRef("EKCH"), page.Provider, page.Resource)
		require.NoError(t, err)
		require.Equal(t, revision, current)
		require.Equal(t, prior.Sha256, pending.Sha256)
		require.Equal(t, pb.ProviderCheckpoint_PENDING, pending.AttemptStatus)
		return nil, nil, errors.New("provider timed out")
	}
	sent, err = nav.FetchProviderPageFenced(ctx, worker, failureID, "EKCH", page.Provider, page.Resource, fetch)
	require.Error(t, err)
	require.True(t, sent)
	sent, err = nav.FetchProviderPageFenced(ctx, worker, failureID, "EKCH", page.Provider, page.Resource, fetch)
	require.NoError(t, err)
	require.False(t, sent)
	require.Equal(t, 1, calls)
	checkpoint, restored, current, err := nav.CheckpointRevisionFor(ctx, airportRef("EKCH"), page.Provider, page.Resource)
	require.NoError(t, err)
	require.Equal(t, revision, current)
	require.Equal(t, pb.ProviderCheckpoint_FAILED, checkpoint.AttemptStatus)
	require.True(t, proto.Equal(page, restored))
	state, err := nav.Writer.load(ctx, mustAsyncSubject(airportRef("EKCH")), airportRef("EKCH"))
	require.NoError(t, err)
	require.Empty(t, state.Workflows)
	require.NotZero(t, store.commits)
}

func TestProviderAttemptFieldsRejectPartialIdentity(t *testing.T) {
	for _, checkpoint := range []*pb.ProviderCheckpoint{
		{AttemptId: uuid.NewString()},
		{AttemptStatus: pb.ProviderCheckpoint_PENDING},
		{AttemptId: "invalid", AttemptStatus: pb.ProviderCheckpoint_FAILED},
		{AcceptedRevision: 1},
	} {
		require.Error(t, validateTyped(checkpoint.ProtoReflect()))
	}
	require.NoError(t, validateTyped((&pb.ProviderCheckpoint{}).ProtoReflect()))
	require.NoError(t, validateTyped((&pb.ProviderCheckpoint{AttemptId: uuid.NewString(), AttemptStatus: pb.ProviderCheckpoint_PENDING}).ProtoReflect()))
}

func TestVatsimCursorFencesGenerationWithoutReceipt(t *testing.T) {
	board := AmanBoard{Observations: []*pb.VatsimObservation{{Callsign: "SAS123", ProviderId: "EKCH/00000000000000000002/SAS123/digest"}}}
	id := uuid.NewString()
	reply := retainedVatsimObservation(board, board.Observations[0].ProviderId, id)
	require.Equal(t, pb.CommandReply_COMMITTED, reply.Status)
	require.Equal(t, id, reply.CommandId)
	require.Nil(t, retainedVatsimObservation(board, "EKCH/00000000000000000003/SAS123/new-digest", uuid.NewString()))
}
