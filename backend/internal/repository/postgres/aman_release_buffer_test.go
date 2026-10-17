package postgres

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/pdc/testdata"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAMANReleaseReservationAndBufferSurviveSessionRestart(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	session := testdata.SeedTestSession(t, queries)
	ctx := aman.WithSession(t.Context(), session)
	state := amanState(1, "unused", "SAS123")
	state.SessionID = session
	state.Flights[0].SequenceDisposition = aman.SequenceDispositionDesequenced
	state.Flights[0].Expedited = true
	state.Flights[0].Prediction.RemainingRouteFixes = []string{"NEKSO", "FINAL"}
	state.ReleaseBuffer = &aman.ReleaseBufferSummary{Status: "ready", Target: 10, Reserved: 1}
	state.Flights[0].ApproachRelease = &aman.ApproachReleaseReservation{
		Episode: "hold/arrival-1", HoldingID: "OLPIB", STARFamily: "MONAK", FeederFix: "NEKSO",
		RunwayGroupID: state.Flights[0].Slot.RunwayGroupID, ReleaseAt: state.GeneratedAt,
		Transit: 12 * time.Minute, AdditionalApproachTime: 5 * time.Minute, RouteDigest: "physical", PolicyVersion: "release-v1",
	}
	_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{State: state})
	require.NoError(t, err)
	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, state, restored)
	require.Nil(t, restored.Flights[0].ApproachRelease.ReleasedAt)
}
