package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func seedCoordinationAcceptance(t *testing.T) (*pgxpool.Pool, *stripRepository, *models.Strip, *models.Coordination, *models.Coordination) {
	t.Helper()
	pool, queries := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, queries, "COORDINATION", nil)
	testdata.SeedTestStrip(t, queries, session, "SAS123")
	_, err := pool.Exec(t.Context(), `UPDATE strips SET owner='A',next_owners='["P","Q"]',previous_owners='["X"]' WHERE session=$1 AND callsign='SAS123'`, session)
	require.NoError(t, err)
	repo := NewStripRepository(pool)
	strip, err := repo.GetByCallsign(t.Context(), session, "SAS123")
	require.NoError(t, err)
	incoming, err := queries.CreateCoordination(t.Context(), database.CreateCoordinationParams{Session: session, StripID: strip.ID, FromPosition: "A", ToPosition: "P"})
	require.NoError(t, err)
	outgoing, err := queries.CreateCoordination(t.Context(), database.CreateCoordinationParams{Session: session, StripID: strip.ID, FromPosition: "P", ToPosition: "Q"})
	require.NoError(t, err)
	return pool, repo, strip, coordinationToModel(incoming), coordinationToModel(outgoing)
}

func TestAcceptCoordinationConsumesExactRowOnce(t *testing.T) {
	pool, repo, strip, incoming, outgoing := seedCoordinationAcceptance(t)
	start := make(chan struct{})
	type result struct {
		accepted bool
		err      error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			<-start
			accepted, err := repo.AcceptCoordination(t.Context(), strip.Session, strip, incoming, "P", []string{"Q"}, []string{"X", "A"})
			results <- result{accepted, err}
		}()
	}
	close(start)
	acceptedCount := 0
	for range 2 {
		r := <-results
		require.NoError(t, r.err)
		if r.accepted {
			acceptedCount++
		}
	}
	require.Equal(t, 1, acceptedCount)
	current, err := repo.GetByCallsign(t.Context(), strip.Session, strip.Callsign)
	require.NoError(t, err)
	require.Equal(t, "P", *current.Owner)
	require.Equal(t, strip.Version+1, current.Version)
	require.Equal(t, []string{"X", "A"}, current.PreviousOwners)
	require.Equal(t, []string{"Q"}, current.NextOwners)
	var remaining []int32
	rows, err := pool.Query(t.Context(), "SELECT id FROM coordinations WHERE strip_id=$1 ORDER BY id", strip.ID)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var id int32
		require.NoError(t, rows.Scan(&id))
		remaining = append(remaining, id)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []int32{outgoing.ID}, remaining, "a replacement outgoing transfer must survive")
}

func TestAcceptCoordinationRejectsConsumedOrCancelledRow(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "consumed and transferred again", true: "cancelled"}[cancelled], func(t *testing.T) {
			pool, repo, strip, incoming, outgoing := seedCoordinationAcceptance(t)
			if cancelled {
				_, err := pool.Exec(t.Context(), "DELETE FROM coordinations WHERE id=$1", incoming.ID)
				require.NoError(t, err)
			} else {
				accepted, err := repo.AcceptCoordination(t.Context(), strip.Session, strip, incoming, "P", []string{"Q"}, []string{"X", "A"})
				require.NoError(t, err)
				require.True(t, accepted)
				current, err := repo.GetByCallsign(t.Context(), strip.Session, strip.Callsign)
				require.NoError(t, err)
				accepted, err = repo.AcceptCoordination(t.Context(), strip.Session, current, outgoing, "Q", nil, []string{"X", "A", "P"})
				require.NoError(t, err)
				require.True(t, accepted)
			}
			// The delayed service rereads the latest strip version, but retains
			// the obsolete incoming coordination. Its row must still be consumable.
			before, err := repo.GetByCallsign(t.Context(), strip.Session, strip.Callsign)
			require.NoError(t, err)
			accepted, err := repo.AcceptCoordination(t.Context(), strip.Session, before, incoming, "P", nil, []string{"A"})
			require.NoError(t, err)
			require.False(t, accepted)
			after, err := repo.GetByCallsign(t.Context(), strip.Session, strip.Callsign)
			require.NoError(t, err)
			require.Equal(t, before, after, "stale acceptance cannot alter owner, history or revision")
		})
	}
}

func TestAcceptCoordinationRechecksVersionAfterLockWait(t *testing.T) {
	pool, repo, strip, incoming, _ := seedCoordinationAcceptance(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	lock, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer lock.Rollback(context.Background())
	_, err = lock.Exec(ctx, "SELECT id FROM strips WHERE id=$1 FOR UPDATE", strip.ID)
	require.NoError(t, err)
	type acceptanceResult struct {
		accepted bool
		err      error
	}
	result := make(chan acceptanceResult, 1)
	go func() {
		accepted, err := repo.AcceptCoordination(ctx, strip.Session, strip, incoming, "P", nil, nil)
		result <- acceptanceResult{accepted, err}
	}()
	require.Eventually(t, func() bool {
		var waiting bool
		err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
			WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT version FROM strips%')`).Scan(&waiting)
		return err == nil && waiting
	}, 3*time.Second, 10*time.Millisecond)
	_, err = lock.Exec(ctx, "UPDATE strips SET owner='Q',version=version+1 WHERE id=$1", strip.ID)
	require.NoError(t, err)
	require.NoError(t, lock.Commit(ctx))
	completed := <-result
	require.NoError(t, completed.err)
	require.False(t, completed.accepted)
	current, err := repo.GetByCallsign(ctx, strip.Session, strip.Callsign)
	require.NoError(t, err)
	require.Equal(t, "Q", *current.Owner)
	var remaining bool
	require.NoError(t, pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM coordinations WHERE id=$1)", incoming.ID).Scan(&remaining))
	require.True(t, remaining, "a failed version check must not consume the coordination")
}

func TestAcceptCoordinationRollsBackConsumptionWhenOwnerWriteFails(t *testing.T) {
	pool, repo, strip, incoming, _ := seedCoordinationAcceptance(t)
	_, err := pool.Exec(t.Context(), `CREATE FUNCTION reject_coordination_owner() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN RAISE EXCEPTION 'owner write failed'; END $$;
	CREATE TRIGGER reject_owner BEFORE UPDATE ON strips FOR EACH ROW EXECUTE FUNCTION reject_coordination_owner()`)
	require.NoError(t, err)
	accepted, err := repo.AcceptCoordination(t.Context(), strip.Session, strip, incoming, "P", nil, nil)
	require.ErrorContains(t, err, "owner write failed")
	require.False(t, accepted)
	current, err := repo.GetByCallsign(t.Context(), strip.Session, strip.Callsign)
	require.NoError(t, err)
	require.Equal(t, strip, current)
	var remaining bool
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT EXISTS(SELECT 1 FROM coordinations WHERE id=$1)", incoming.ID).Scan(&remaining))
	require.True(t, remaining, "owner failure must roll back the selected coordination's deletion")
}
