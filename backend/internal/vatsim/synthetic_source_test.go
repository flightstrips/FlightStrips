package vatsim

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyntheticSourceSnapshotRefreshesReadTime(t *testing.T) {
	source := NewSyntheticSource()
	before := time.Now().UTC()

	snapshot := source.Snapshot()
	after := time.Now().UTC()

	assert.False(t, snapshot.Timestamp.Before(before))
	assert.False(t, snapshot.Timestamp.After(after))
}

func TestSyntheticSourceSupportsOfflineScenarioControl(t *testing.T) {
	source := NewSyntheticSource()
	source.Upsert(Flight{
		CID: "990000001", Callsign: " tst101 ", State: FlightStatePrefile,
		FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EGLL", Revision: 1},
	})

	flight, ok := source.Snapshot().FlightByCallsign("TST101")
	require.True(t, ok)
	assert.True(t, flight.Prefile())
	owned, err := source.VerifyPilotOwnsCallsign(context.Background(), "990000001", "TST101")
	require.NoError(t, err)
	assert.False(t, owned, "prefiles must not satisfy live CID verification")

	flight.State = FlightStateOnline
	source.Upsert(flight)
	owned, err = source.VerifyPilotOwnsCallsign(context.Background(), "990000001", "TST101")
	require.NoError(t, err)
	assert.True(t, owned)
	callsign, found, err := source.GetCallsignByCID(context.Background(), "990000001")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "TST101", callsign)

	source.Remove("TST101")
	assert.Empty(t, source.Snapshot().Flights())
	assert.False(t, source.Snapshot().Timestamp.IsZero())
}

func TestSyntheticSourceMergesValidatedReplayAndResetsIt(t *testing.T) {
	source := NewSyntheticSource()
	received := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	payload := `{"general":{"update_timestamp":"2026-08-03T11:59:00Z"},"pilots":[{"cid":123,"callsign":"SAS123","latitude":55.6,"longitude":12.6,"altitude":12000,"groundspeed":220,"last_updated":"2026-08-03T11:59:00Z"}]}`
	require.NoError(t, source.LoadReplay(strings.NewReader(payload), received))
	snapshot := source.Snapshot()
	assert.Equal(t, time.Date(2026, 8, 3, 11, 59, 0, 0, time.UTC), snapshot.Timestamp)
	assert.Equal(t, "SAS123", snapshot.Flights()[0].Callsign)
	source.ResetReplay()
	assert.Empty(t, source.Snapshot().Flights())
}
