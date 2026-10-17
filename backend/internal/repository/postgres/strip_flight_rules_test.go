package postgres

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpdateVatsimSourceBackfillsUnknownFlightRules(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, queries, "FLIGHT-RULES", nil)
	testdata.SeedTestStrip(t, queries, session, "SAS123")
	repo := NewStripRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	later := now.Add(time.Minute)

	for _, tc := range []struct {
		name     string
		rules    string
		incoming string
		modified []string
		seenAt   time.Time
		want     string
	}{
		{name: "older plugin", incoming: "V", seenAt: later, want: "V"},
		{name: "known newer EuroScope rules", rules: "I", incoming: "V", seenAt: later, want: "I"},
		{name: "controller protected", incoming: "V", modified: []string{"flight_rules"}, seenAt: later, want: ""},
		{name: "newer VATSIM rules", rules: "I", incoming: "V", seenAt: now.Add(-time.Minute), want: "V"},
		{name: "missing VATSIM rules", rules: "V", seenAt: now.Add(-time.Minute), want: "V"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modified := tc.modified
			if modified == nil {
				modified = []string{}
			}
			_, err := pool.Exec(ctx, `UPDATE strips SET flight_rules = $1, euroscope_seen_at = $2,
				controller_modified_fields = $3, origin = 'EKCH' WHERE session = $4 AND callsign = 'SAS123'`,
				tc.rules, tc.seenAt, modified, session)
			require.NoError(t, err)
			count, err := repo.UpdateVatsimSource(ctx, session, "SAS123", models.VatsimStripSource{
				FlightRules: tc.incoming, SeenAt: now, Origin: "ESSA",
			})
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			strip, err := repo.GetByCallsign(ctx, session, "SAS123")
			require.NoError(t, err)
			require.Equal(t, tc.want, strip.FlightRules)
			if tc.seenAt.After(now) {
				require.Equal(t, "EKCH", strip.Origin)
			}
		})
	}
}
