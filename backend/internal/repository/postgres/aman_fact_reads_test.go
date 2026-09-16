package postgres

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAMANFactReadsMatchPersistedFlight(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	repo := NewAMANRepository(pool)
	ctx := context.Background()
	state := amanState(1, "1234567", "SAS123")
	state.Flights[0].HoldingClearance = &aman.HoldingClearance{Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute, ObservedAt: amanTestTime}
	_, err := repo.Commit(ctx, aman.StateCommit{State: state})
	require.NoError(t, err)
	id, err := repo.FindActiveFactFlight(ctx, state.Airport, " sas123 ")
	require.NoError(t, err)
	require.Equal(t, state.Flights[0].ID, id)
	facts, err := repo.LoadHoldingFactSnapshots(ctx, state.Airport, []aman.FlightID{id, "absent"})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, state.Flights[0].HoldingClearance, facts[0].Clearance)
	require.Equal(t, "1234567", facts[0].VATSIMCID)
	_, err = repo.FindActiveFactFlight(ctx, "XXXX", "SAS123")
	require.Error(t, err)
	_, err = pool.Exec(ctx, "UPDATE aman_flights SET state='landed' WHERE flight_id=$1", id)
	require.NoError(t, err)
	_, err = repo.FindActiveFactFlight(ctx, state.Airport, "SAS123")
	var domain *aman.DomainError
	require.ErrorAs(t, err, &domain)
	require.Equal(t, aman.ErrorNotFound, domain.Class)
	// Holdings preserve existing behavior for landed rows and do not use the
	// route-fact active-flight filter.
	facts, err = repo.LoadHoldingFactSnapshots(ctx, state.Airport, []aman.FlightID{id})
	require.NoError(t, err)
	require.Len(t, facts, 1)
}
