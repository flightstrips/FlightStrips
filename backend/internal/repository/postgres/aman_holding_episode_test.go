package postgres

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAMANRepositoryRetainsHoldingEpisodeAndRebuildReservation(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	repo := NewAMANRepository(pool)
	state := amanState(1, "holding", "TEST")
	f := &state.Flights[0]
	hold := "LUGAS-HOLD"
	f.SelectedHolding = &hold
	started := state.GeneratedAt.Add(-15 * time.Minute)
	level := int32(10000)
	f.HoldingClearance = &aman.HoldingClearance{Hold: "LUGAS", HoldType: aman.HoldingClearanceEnroute, ClearedAltitude: &level, ObservedAt: started.Add(-time.Hour)}
	f.HoldingEpisode = &aman.HoldingEpisode{ID: "episode", HoldingID: hold, Fix: "LUGAS", StartedAt: &started, Source: "fix_crossing"}
	f.HoldingReleaseReservation = &aman.HoldingReleaseReservation{CommandID: "rebuild", EpisodeID: "episode", HoldingID: hold, Level: 10000, Rank: 1}
	_, err := repo.Commit(context.Background(), aman.StateCommit{State: state})
	require.NoError(t, err)
	restored, err := NewAMANRepository(pool).LoadAirportState(context.Background(), state.Airport)
	require.NoError(t, err)
	require.Equal(t, state.Flights[0].HoldingEpisode, restored.Flights[0].HoldingEpisode)
	require.Equal(t, state.Flights[0].HoldingReleaseReservation, restored.Flights[0].HoldingReleaseReservation)
	require.NotNil(t, aman.ValidHoldingReleaseReservation(restored.Flights[0]))
}
