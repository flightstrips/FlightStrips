package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"FlightStrips/internal/database"
	"FlightStrips/internal/pdc/testdata"
	"github.com/stretchr/testify/require"
)

func TestAcknowledgeCdmMilestonePreservesConcurrentFlagsAndOtherData(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, queries, "CDM-ACK", nil)
	testdata.SeedTestStrip(t, queries, session, "SAS123")
	repo := NewStripRepository(pool)
	ctx := context.Background()
	initial := `{"aobt":"1200","aobtViffPending":true,"atot":"1210","atotViffPending":true,"ctot":"1215","tobt":"1150","futureField":{"preserve":true}}`
	_, err := pool.Exec(ctx, "UPDATE strips SET cdm_data=$1::jsonb WHERE session=$2 AND callsign=$3", initial, session, "SAS123")
	require.NoError(t, err)
	type result struct {
		rows int64
		err  error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for milestone, value := range map[string]string{"AOBT": "1200", "ATOT": "1210"} {
		go func(milestone, value string) {
			<-start
			rows, err := repo.AcknowledgeCdmMilestone(ctx, session, "SAS123", milestone, value)
			results <- result{rows, err}
		}(milestone, value)
	}
	close(start)
	for range 2 {
		r := <-results
		require.NoError(t, r.err)
		require.Equal(t, int64(1), r.rows)
	}
	strip, err := queries.GetStrip(ctx, database.GetStripParams{Session: session, Callsign: "SAS123"})
	require.NoError(t, err)
	require.JSONEq(t, `{"aobt":"1200","aobtViffPending":false,"atot":"1210","atotViffPending":false,"ctot":"1215","tobt":"1150","futureField":{"preserve":true}}`, string(strip.CdmData))
	rows, err := repo.AcknowledgeCdmMilestone(ctx, session, "SAS123", "AOBT", "1200")
	require.NoError(t, err)
	require.Zero(t, rows, "an already acknowledged milestone must not be written again")
	rows, err = repo.AcknowledgeCdmMilestone(ctx, session, "MISSING", "AOBT", "1200")
	require.NoError(t, err)
	require.Zero(t, rows)
}

func TestAcknowledgeCdmMilestoneRejectsObsoleteValue(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, queries, "CDM-ACK-NEW", nil)
	testdata.SeedTestStrip(t, queries, session, "SAS123")
	repo := NewStripRepository(pool)
	ctx := context.Background()
	_, err := pool.Exec(ctx, "UPDATE strips SET cdm_data=$1::jsonb WHERE session=$2 AND callsign=$3", `{"aobt":"1201","aobtViffPending":true}`, session, "SAS123")
	require.NoError(t, err)
	rows, err := repo.AcknowledgeCdmMilestone(ctx, session, "SAS123", "AOBT", "1200")
	require.NoError(t, err)
	require.Zero(t, rows)
	strip, err := queries.GetStrip(ctx, database.GetStripParams{Session: session, Callsign: "SAS123"})
	require.NoError(t, err)
	var data map[string]any
	require.NoError(t, json.Unmarshal(strip.CdmData, &data))
	require.Equal(t, true, data["aobtViffPending"])
	rows, err = repo.AcknowledgeCdmMilestone(ctx, session, "SAS123", "AOBT", "1201")
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
}

func TestAcknowledgeCdmMilestoneRejectsUnknownMilestone(t *testing.T) {
	_, err := (&stripRepository{}).AcknowledgeCdmMilestone(context.Background(), 7, "SAS123", "TOBT", "1200")
	require.ErrorContains(t, err, "unsupported CDM milestone")
}
