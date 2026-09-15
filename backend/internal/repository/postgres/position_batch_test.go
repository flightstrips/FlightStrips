package postgres

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestPositionBatchSQLSnapshotAndConflictIsolation(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, q, "BATCH_SQL", nil)
	ctx := context.Background()
	for _, name := range []string{"BATCH1", "BATCH2"} {
		testdata.SeedTestStrip(t, q, session, name)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, NewStandAssignmentRepository(pool).CreateAssignment(ctx, &models.StandAssignment{
		SessionID: session, Callsign: "BATCH1", Stand: "A1", Direction: "DEPARTURE", Stage: "RESERVED", Source: "automatic", Version: 1, AssignedAt: &now,
	}))
	counter := &positionQueryCounter{}
	cfg := pool.Config()
	cfg.ConnConfig.Tracer = counter
	traced, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer traced.Close()
	r := NewStripRepository(traced)
	var reads []*batchCall
	for _, name := range []string{"BATCH1", "BATCH2", "MISSING"} {
		reads = append(reads, &batchCall{ctx: ctx, input: positionRead{r, session, name}, result: make(chan batchResult, 1)})
	}
	batchPositionReads(reads)
	require.Equal(t, int32(1), counter.n.Load())
	snapshots := make([]*models.PositionSnapshot, 2)
	for i := 0; i < 2; i++ {
		result := <-reads[i].result
		require.NoError(t, result.err)
		snapshots[i] = result.value.(*models.PositionSnapshot)
	}
	require.ErrorIs(t, (<-reads[2].result).err, pgx.ErrNoRows)
	require.Equal(t, "A1", snapshots[0].Assignment.Stand)
	require.True(t, snapshots[0].Assignment.AssignedAt.Equal(now))
	require.Nil(t, snapshots[1].Assignment)
	_, err = pool.Exec(ctx, "UPDATE strips SET version = version + 1, heading = 123 WHERE session = $1 AND callsign = 'BATCH1'", session)
	require.NoError(t, err)
	lat, lon, alt := 55.7, 12.7, int32(1234)
	var writes []*batchCall
	for _, s := range snapshots {
		writes = append(writes, &batchCall{ctx: ctx, input: positionWrite{r, session, s.Strip.Callsign, &lat, &lon, &alt, s.Strip.Bay, 0, s.Strip.Version}, result: make(chan batchResult, 1)})
	}
	batchPositionWrites(writes)
	require.Equal(t, int32(4), counter.n.Load(), "one bulk read plus BEGIN, bulk write and COMMIT")
	for i, expected := range []int64{0, 1} {
		result := <-writes[i].result
		require.NoError(t, result.err)
		require.Equal(t, expected, result.value)
	}
	a, err := r.GetPositionSnapshot(ctx, session, "BATCH1")
	require.NoError(t, err)
	require.Equal(t, int32(123), *a.Strip.Heading)
	b, err := r.GetPositionSnapshot(ctx, session, "BATCH2")
	require.NoError(t, err)
	require.Equal(t, lat, *b.Strip.PositionLatitude)
	require.NotNil(t, b.Strip.EuroscopeSeenAt)
	_, err = pool.Exec(ctx, "UPDATE strips SET cdm_data = 'true'::jsonb WHERE session = $1 AND callsign = 'BATCH1'", session)
	require.NoError(t, err)
	batchPositionReads(reads[:2])
	require.Error(t, (<-reads[0].result).err, "bad operational JSON must fail only its own report")
	require.NoError(t, (<-reads[1].result).err)
}

func TestPositionBatchRetryUsesFreshSnapshot(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, q, "BATCH_RETRY", nil)
	testdata.SeedTestStrip(t, q, session, "RETRY1")
	r := NewStripRepository(pool)
	scope := NewPositionBatch(2)
	ctx := scope.Context(context.Background(), 0)
	scope.Done(1) // e.g. invalid master or a job which took the transition path
	defer scope.Done(0)
	a, err := r.GetPositionSnapshot(ctx, session, "RETRY1")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "UPDATE strips SET version = version + 1 WHERE session = $1 AND callsign = 'RETRY1'", session)
	require.NoError(t, err)
	n, err := r.UpdateAircraftPositionAndBay(ctx, session, "RETRY1", nil, nil, nil, a.Strip.Bay, 0, a.Strip.Version)
	require.NoError(t, err)
	require.Zero(t, n)
	b, err := r.GetPositionSnapshot(ctx, session, "RETRY1")
	require.NoError(t, err)
	require.Equal(t, a.Strip.Version+1, b.Strip.Version)
	n, err = r.UpdateAircraftPositionAndBay(ctx, session, "RETRY1", nil, nil, nil, b.Strip.Bay, 0, b.Strip.Version)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
}

func TestPositionBatchDoesNotWaitIndefinitelyForBlockedParticipant(t *testing.T) {
	scope := NewPositionBatch(2)
	ctx := scope.Context(context.Background(), 0)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = joinPositionBatch(ctx, "snapshot", nil, func(calls []*batchCall) {
			for _, call := range calls {
				call.result <- batchResult{}
			}
		})
		scope.Done(0)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("batch deadlocked behind absent/master-fenced participant")
	}
	// A late participant must use the individual path; closed batches cannot be reused.
	_, _, joined := joinPositionBatch(scope.Context(context.Background(), 1), "snapshot", nil, nil)
	require.False(t, joined)
	scope.Done(1)
}

func TestPositionBatchCancelledWriteDoesNotCommitAnyRow(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, q, "BATCH_CANCEL", nil)
	for _, name := range []string{"CANCEL1", "CANCEL2"} {
		testdata.SeedTestStrip(t, q, session, name)
	}
	r := NewStripRepository(pool)
	_, err := pool.Exec(context.Background(), "UPDATE strips SET euroscope_seen_at = NULL WHERE session = $1", session)
	require.NoError(t, err)
	lock, err := pool.Begin(context.Background())
	require.NoError(t, err)
	defer lock.Rollback(context.Background())
	_, err = lock.Exec(context.Background(), "SELECT id FROM strips WHERE session = $1 FOR UPDATE", session)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	lat := 10.
	var writes []*batchCall
	for _, name := range []string{"CANCEL1", "CANCEL2"} {
		s, err := r.GetByCallsign(context.Background(), session, name)
		require.NoError(t, err)
		writes = append(writes, &batchCall{ctx: ctx, input: positionWrite{r, session, name, &lat, nil, nil, s.Bay, 0, s.Version}, result: make(chan batchResult, 1)})
	}
	batchPositionWrites(writes)
	for _, call := range writes {
		require.ErrorIs(t, (<-call.result).err, context.DeadlineExceeded)
	}
	require.NoError(t, lock.Rollback(context.Background()))
	for _, name := range []string{"CANCEL1", "CANCEL2"} {
		s, err := r.GetByCallsign(context.Background(), session, name)
		require.NoError(t, err)
		require.Nil(t, s.EuroscopeSeenAt)
	}
}
