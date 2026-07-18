package vatsim

import (
	"context"
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
