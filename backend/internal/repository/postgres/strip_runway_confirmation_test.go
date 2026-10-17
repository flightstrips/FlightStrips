package postgres

import (
	"FlightStrips/internal/pdc/testdata"
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateArrivalRunwayConfirmationSerializesPerSession(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, queries, "RWY-ARR-CONFIRMATION", nil)
	testdata.SeedTestStrip(t, queries, session, "SAS123")
	testdata.SeedTestStrip(t, queries, session, "NAX456")
	_, err := pool.Exec(context.Background(), `
		UPDATE strips SET bay = 'RWY_ARR', runway_cleared = true, runway_confirmed = false
		WHERE session = $1 AND callsign IN ('SAS123', 'NAX456')`, session)
	require.NoError(t, err)

	repo := NewStripRepository(pool)
	start := make(chan struct{})
	type result struct {
		affected int64
		err      error
	}
	results := make(chan result, 2)
	var workers sync.WaitGroup
	for _, callsign := range []string{"SAS123", "NAX456"} {
		workers.Add(1)
		go func(callsign string) {
			defer workers.Done()
			<-start
			affected, err := repo.UpdateArrivalRunwayConfirmation(context.Background(), session, callsign)
			results <- result{affected: affected, err: err}
		}(callsign)
	}
	close(start)
	workers.Wait()
	close(results)

	var confirmed int64
	for result := range results {
		require.NoError(t, result.err)
		confirmed += result.affected
	}
	require.EqualValues(t, 1, confirmed, "only one concurrent arrival confirmation can succeed")

	var greenCount int64
	err = pool.QueryRow(context.Background(), `
		SELECT count(*) FROM strips
		WHERE session = $1 AND bay = 'RWY_ARR' AND runway_cleared AND runway_confirmed`, session).Scan(&greenCount)
	require.NoError(t, err)
	require.EqualValues(t, 1, greenCount)
}
