package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- UpdateGroundStateForMove ----

// TestUpdateGroundStateForMove_ArrivalToStand_SendsParkedToES verifies that moving
// an arrival strip to BAY_STAND updates the ground state to PARK and notifies EuroScope.
func TestUpdateGroundStateForMove_ArrivalToStand_SendsParkedToES(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS123"
	const cid = "1234567"
	const airport = "EKCH"

	var savedState *string
	var savedBay string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:    callsign,
				Destination: airport,
				Bay:         shared.BAY_TWY_ARR,
			}, nil
		},
		UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, state *string, bay string, _ *int32) (int64, error) {
			savedState = state
			savedBay = bay
			return 1, nil
		},
	}

	esHub := &testutil.MockEuroscopeHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})
	svc.SetEuroscopeHub(esHub)

	err := svc.UpdateGroundStateForMove(ctx, session, callsign, shared.BAY_STAND, cid, airport)
	require.NoError(t, err)

	assert.Equal(t, shared.BAY_STAND, savedBay)
	require.NotNil(t, savedState)
	assert.Equal(t, euroscope.GroundStateParked, *savedState)

	require.Len(t, esHub.GroundStates, 1)
	assert.Equal(t, euroscope.GroundStateParked, esHub.GroundStates[0].GroundState)
	assert.Equal(t, callsign, esHub.GroundStates[0].Callsign)
	assert.Equal(t, cid, esHub.GroundStates[0].Cid)
}

// TestUpdateGroundStateForMove_ArrivalToNonStandBay_NoESUpdate verifies that moving
// an arrival to a non-STAND bay does not send a ground state update to EuroScope.
func TestUpdateGroundStateForMove_ArrivalToNonStandBay_NoESUpdate(t *testing.T) {
	ctx := context.Background()
	const airport = "EKCH"

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:    "SAS456",
				Destination: airport,
				Bay:         shared.BAY_FINAL,
			}, nil
		},
		UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, _ *string, _ string, _ *int32) (int64, error) {
			return 1, nil
		},
	}

	esHub := &testutil.MockEuroscopeHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})
	svc.SetEuroscopeHub(esHub)

	err := svc.UpdateGroundStateForMove(ctx, 1, "SAS456", shared.BAY_RWY_ARR, "1234567", airport)
	require.NoError(t, err)
	assert.Empty(t, esHub.GroundStates, "no ground state update expected for arrival to non-STAND bay")
}

// TestUpdateGroundStateForMove_DepartureToStand_NoESUpdate verifies that moving a
// departure strip to BAY_STAND (unusual, but possible) does not send a ground state
// update since STAND is not a departure tracking bay.
func TestUpdateGroundStateForMove_DepartureToStand_NoESUpdate(t *testing.T) {
	ctx := context.Background()
	const airport = "EKCH"

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "EZY789",
				Origin:   airport,
				Bay:      shared.BAY_NOT_CLEARED,
			}, nil
		},
		UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, _ *string, _ string, _ *int32) (int64, error) {
			return 1, nil
		},
	}

	esHub := &testutil.MockEuroscopeHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})
	svc.SetEuroscopeHub(esHub)

	err := svc.UpdateGroundStateForMove(ctx, 1, "EZY789", shared.BAY_STAND, "1234567", airport)
	require.NoError(t, err)
	assert.Empty(t, esHub.GroundStates, "departure moved to STAND should not update ground state")
}

// TestUpdateGroundStateForMove_DepartureToPush_SendsPushToES verifies the existing
// departure ground state update still works correctly after the arrival changes.
func TestUpdateGroundStateForMove_DepartureToPush_SendsPushToES(t *testing.T) {
	ctx := context.Background()
	const airport = "EKCH"

	var savedState *string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "DLH100",
				Origin:   airport,
				Bay:      shared.BAY_CLEARED,
			}, nil
		},
		UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, state *string, _ string, _ *int32) (int64, error) {
			savedState = state
			return 1, nil
		},
	}

	esHub := &testutil.MockEuroscopeHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})
	svc.SetEuroscopeHub(esHub)

	err := svc.UpdateGroundStateForMove(ctx, 1, "DLH100", shared.BAY_PUSH, "1234567", airport)
	require.NoError(t, err)
	require.NotNil(t, savedState)
	assert.Equal(t, euroscope.GroundStatePush, *savedState)
	require.Len(t, esHub.GroundStates, 1)
	assert.Equal(t, euroscope.GroundStatePush, esHub.GroundStates[0].GroundState)
}
