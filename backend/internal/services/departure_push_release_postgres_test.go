package services

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type standReleaseQueries struct {
	mu      sync.Mutex
	queries []string
}

func (q *standReleaseQueries) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queries = append(q.queries, data.SQL)
	return ctx
}
func (*standReleaseQueries) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (q *standReleaseQueries) take() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	queries := q.queries
	q.queries = nil
	return queries
}

func TestRepeatedDepartureStandReleaseDoesNotWaitForSessionLock(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	counter := &standReleaseQueries{}
	cfg := pool.Config()
	cfg.ConnConfig.Tracer = counter
	traced, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer traced.Close()
	lifecycle, allocations, session, assignments, _, _ := departureLifecycleFixture(t, traced, queries, "", "", nil)
	const callsign = "RELEASE1"
	testdata.SeedTestStrip(t, queries, session, callsign)
	require.NoError(t, lifecycle.ReleaseDepartureStand(ctx, session, callsign))
	counter.take()

	// Reproduce another allocator/AMAN transaction holding the shared session
	// row. The aircraft was already released; another radar tick has no write.
	blocker, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer blocker.Rollback(ctx)
	_, err = blocker.Exec(ctx, "SELECT id FROM sessions WHERE id = $1 FOR UPDATE", session)
	require.NoError(t, err)
	deadline, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = lifecycle.ReleaseDepartureStand(deadline, session, callsign)
	require.NoError(t, err, "an already completed release must not contend for the session write lock")
	t.Logf("repeated release completed in %s with session row locked", time.Since(start))
	statements := counter.take()
	require.Len(t, statements, 1, "a repeated release needs only one fresh database snapshot")
	require.True(t, strings.HasPrefix(statements[0], "SELECT"))
	require.NoError(t, blocker.Rollback(ctx))

	// The marker is not a cross-message cache: resetting it starts a fresh
	// lifecycle, and an unexpected new departure assignment must still be freed.
	require.NoError(t, lifecycle.ResetDepartureStandRelease(ctx, session, callsign))
	require.NoError(t, lifecycle.ReleaseDepartureStand(ctx, session, callsign))
	var marked bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM departure_stand_releases r JOIN strips s ON s.id = r.strip_id WHERE s.session = $1 AND s.callsign = $2)`, session, callsign).Scan(&marked))
	require.True(t, marked)
	var published []StandAllocationResult
	allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
		published = append(published, result)
		return nil
	})
	for _, direction := range []string{"DEPARTURE", "ARRIVAL"} {
		require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{SessionID: session, Callsign: callsign, Stand: "A1", Direction: direction, Stage: StageReserved, Source: "AUTOMATIC", Version: 1}))
		require.NoError(t, lifecycle.ReleaseDepartureStand(ctx, session, callsign))
		assignment, err := assignments.GetAssignment(ctx, session, callsign)
		if direction == "DEPARTURE" {
			require.ErrorIs(t, err, pgx.ErrNoRows)
			require.Len(t, published, 1)
			require.True(t, published[0].Removed)
		} else {
			require.NoError(t, err)
			require.Equal(t, direction, assignment.Direction)
			require.Len(t, published, 1, "an arrival assignment must remain untouched")
		}
	}
}
