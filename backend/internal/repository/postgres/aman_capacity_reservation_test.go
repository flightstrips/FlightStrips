package postgres

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAMANRepositoryRestartsWithCapacityReservationsOutsideFlights(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-RESERVATION", "SAS101")
	state.Flights = []aman.AMANFlight{}
	state.RunwayGroups[0].CapacityReservations = []aman.RunwayCapacityReservation{{
		ID: "command-42", Start: amanTestTime.Add(time.Hour), End: amanTestTime.Add(63 * time.Minute),
		Label: "FLIGHT", CreatedAt: amanTestTime, CreatedBy: "controller-1",
	}}

	_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state, restored)
	require.Empty(t, restored.Flights, "capacity reservations must never be reconstructed as AMAN flights")

	var stored []byte
	require.NoError(t, pool.QueryRow(ctx, "SELECT runway_groups FROM aman_airport_states WHERE airport = $1", state.Airport).Scan(&stored))
	var groups []aman.RunwayGroupPolicy
	require.NoError(t, json.Unmarshal(stored, &groups))
	require.Equal(t, state.RunwayGroups[0].CapacityReservations, groups[0].CapacityReservations)
}
