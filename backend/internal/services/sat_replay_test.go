package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSATPhaseOneReplay is a deterministic, tower-independent replay of the
// phase-one source-to-controller state machine. Repository reloads represent a
// frontend reconnect: the authoritative assignment must survive unchanged.
func TestSATPhaseOneReplay(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)

	const callsign = "SAS917"
	seedTestArrivalStrip(t, queries, session, callsign)
	eta := clock.current().Add(45 * time.Minute)
	setArrivalETA(t, strips, session, callsign, eta)
	reason := "deterministic replay closure"
	require.NoError(t, allocations.CreateManualBlock(ctx, "EKCH", &models.StandBlock{
		SessionID: session, Stand: "A2", BlockType: "CLOSURE", Source: "CONTROLLER", Reason: &reason, Manual: true,
	}))

	// Distant arrival / ETA reveal: the closure leaves only A1 available.
	require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, callsign), arrivalFlight(callsign, 1)))
	estimated, err := assignments.GetAssignment(ctx, session, callsign)
	require.NoError(t, err)
	assert.Equal(t, StageEstimated, estimated.Stage)
	assert.Equal(t, "A1", estimated.Stand)

	// EuroScope/live enrichment drives all remaining arrival stages.
	clock.set(eta.Add(-10 * time.Minute))
	flight := arrivalFlight(callsign, 2)
	flight.Online = true
	require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, callsign), flight))
	assigned, err := assignments.GetAssignment(ctx, session, callsign)
	require.NoError(t, err)
	assert.Equal(t, StageAssigned, assigned.Stage)

	clock.set(eta.Add(-2 * time.Minute))
	require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, callsign), flight))
	confirmed, err := assignments.GetAssignment(ctx, session, callsign)
	require.NoError(t, err)
	assert.Equal(t, StageConfirmed, confirmed.Stage)

	// A controller knowingly reallocates onto the blocked stand. The override
	// reason and durable version are what a reconnecting frontend receives.
	request := lifecycle.buildRequest(session, loadStrip(t, strips, session, callsign), flight, StageConfirmed, &eta, nil)
	request.Stand = "A2"
	request.ConflictReason = "controller operational decision"
	result, err := allocations.OverrideManually(ctx, request)
	require.NoError(t, err)
	assert.Equal(t, "A2", result.Assignment.Stand)
	assert.Equal(t, "MANUAL_OVERRIDE", result.Assignment.Source)

	reconnected, err := assignments.GetAssignment(ctx, session, callsign)
	require.NoError(t, err)
	assert.Equal(t, result.Assignment.Stand, reconnected.Stand)
	assert.Equal(t, result.Assignment.Version, reconnected.Version)
	assert.Equal(t, result.Assignment.ConflictReason, reconnected.ConflictReason)
}
