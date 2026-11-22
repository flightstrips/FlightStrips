package postgres

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"github.com/stretchr/testify/require"
)

func TestArrivalETARevisionTracksEveryMutationAndDeletion(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := t.Context()
	session := testdata.SeedTestSessionNamedWithSectors(t, queries, t.Name(), nil)
	other := testdata.SeedTestSessionNamedWithSectors(t, queries, t.Name()+" other", nil)
	store := NewStripRepository(pool)
	read := func() int64 {
		t.Helper()
		revision, err := store.ArrivalETARevision(ctx, session)
		require.NoError(t, err)
		return revision
	}
	require.Zero(t, read())
	testdata.SeedTestStrip(t, queries, session, "SAS1")
	revision := read()
	require.Positive(t, revision)
	testdata.SeedTestStrip(t, queries, other, "SAS2")
	require.Equal(t, revision, read(), "another session cannot invalidate this pass")
	eta := models.ArrivalETA{Time: time.Now().UTC().Add(time.Hour), Source: "aman", CalculatedAt: time.Now().UTC()}
	_, err := store.UpdateArrivalETA(ctx, session, "SAS1", eta)
	require.NoError(t, err)
	require.Greater(t, read(), revision)
	revision = read()
	_, err = store.UpdateArrivalETA(ctx, session, "SAS1", eta)
	require.NoError(t, err)
	require.Equal(t, revision, read(), "unchanged ETA writes need no new generation")
	_, err = store.ClearArrivalETA(ctx, session, "SAS1")
	require.NoError(t, err)
	require.Greater(t, read(), revision)
	revision = read()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, "UPDATE strips SET arrival_eta=$1::jsonb WHERE session=$2", `{"source":"FILED","time":"2026-07-12T12:00:00Z"}`, session)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback(ctx))
	require.Equal(t, revision, read(), "revision updates roll back with the ETA")
	require.NoError(t, store.Delete(ctx, session, "SAS1"))
	require.Greater(t, read(), revision, "deleting a strip invalidates a competing pass")
}
