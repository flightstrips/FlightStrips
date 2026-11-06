package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	pkg "FlightStrips/pkg/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestFlightPlanDurationSurvivesReloadAndHonorsSourcePrecedence(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	require.NoError(t, q.InsertAirport(ctx, "EKBI"))
	id, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: "EKBI", Name: "LIVE"})
	require.NoError(t, err)
	repo := NewStripRepository(pool)
	seen := time.Now().UTC()
	row := &models.Strip{Callsign: "EET1", Session: id, Origin: "EKBI", Destination: "EKYT", Bay: "ACTIVE", FlightPlanEET: "0130", EuroscopeSeenAt: &seen, BoardState: &models.StripBoardState{Annotations: pkg.StripAnnotations{Notes: "paper note"}}}
	require.NoError(t, repo.Create(ctx, row))
	read := func() *models.Strip { r, err := repo.GetByCallsign(ctx, id, "EET1"); require.NoError(t, err); return r }
	require.Equal(t, "0130", read().FlightPlanEET)
	for _, tc := range []struct {
		at          time.Time
		value, want string
	}{{seen.Add(-time.Second), "0200", "0130"}, {seen.Add(time.Second), "0210", "0210"}, {seen.Add(2 * time.Second), "", "0210"}} {
		_, err = repo.UpdateVatsimSource(ctx, id, "EET1", models.VatsimStripSource{CID: "1", Revision: 1, SeenAt: tc.at, Origin: "EKBI", Destination: "EKYT", FlightPlanEET: tc.value})
		require.NoError(t, err)
		current := read()
		require.Equal(t, tc.want, current.FlightPlanEET)
		require.Equal(t, "paper note", current.BoardState.Annotations.Notes)
	}
}
