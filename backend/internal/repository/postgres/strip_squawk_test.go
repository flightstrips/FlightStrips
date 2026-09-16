package postgres

import (
	"FlightStrips/internal/pdc/testdata"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUpdateSquawkCapturesPreviousCode(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, q, "SQUAWK_PREVIOUS", nil)
	oldActual, oldAssigned := "4231", "4232"
	testdata.SeedTestStripWithSquawks(t, q, session, "TEST1", &oldActual, &oldAssigned)
	r := NewStripRepository(pool)
	ctx := context.Background()
	before, err := r.GetByCallsign(ctx, session, "TEST1")
	require.NoError(t, err)
	for _, assigned := range []bool{false, true} {
		previous, count, err := r.UpdateSquawkWithPrevious(ctx, session, "TEST1", "4233", assigned)
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
		if assigned {
			require.Equal(t, &oldAssigned, previous)
		} else {
			require.Equal(t, &oldActual, previous)
		}
	}
	after, err := r.GetByCallsign(ctx, session, "TEST1")
	require.NoError(t, err)
	require.Equal(t, before.Version+2, after.Version)
	require.Equal(t, "4233", *after.Squawk)
	require.Equal(t, "4233", *after.AssignedSquawk)
	_, count, err := r.UpdateSquawkWithPrevious(ctx, session, "MISSING", "4233", false)
	require.NoError(t, err)
	require.Zero(t, count)
}
