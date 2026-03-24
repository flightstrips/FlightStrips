package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncEuroscopeStrip_NewLocalDepartureWithoutPositionStartsInNotCleared(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS123"

	var createdStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, strip *models.Strip) error {
			createdStrip = strip
			return nil
		},
	}

	svc, _ := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, euroscope.Strip{
		Callsign: callsign,
		Origin:   "EKCH",
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, createdStrip)
	assert.Equal(t, callsign, createdStrip.Callsign)
	assert.Equal(t, shared.BAY_NOT_CLEARED, createdStrip.Bay)
}

func TestSyncEuroscopeStrip_ExistingHiddenNonArrivalBecomesArrivalStartsInArrHidden(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "DLH8LL"

	existingStrip := &models.Strip{
		Callsign:    callsign,
		Origin:      "EDDF",
		Destination: "ESSA",
		Bay:         shared.BAY_HIDDEN,
	}

	var updatedStrip *models.Strip
	var movedToBay string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			movedToBay = bay
			return 1, nil
		},
	}

	svc, hub := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, euroscope.Strip{
		Callsign:    callsign,
		Origin:      "EDDF",
		Destination: "EKCH",
	}, "EKCH")
	require.NoError(t, err)

	require.NotNil(t, updatedStrip)
	assert.Equal(t, "EKCH", updatedStrip.Destination)
	assert.Equal(t, shared.BAY_ARR_HIDDEN, updatedStrip.Bay)
	assert.Equal(t, shared.BAY_ARR_HIDDEN, movedToBay)
	require.Len(t, hub.StripUpdates, 1)
	assert.Equal(t, callsign, hub.StripUpdates[0].Callsign)
}

func TestSyncEuroscopeStrip_ExistingArrivalAutoHiddenRemainsHidden(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS432"

	existingStrip := &models.Strip{
		Callsign:    callsign,
		Origin:      "ESSA",
		Destination: "EKCH",
		Bay:         shared.BAY_HIDDEN,
	}

	var updatedStrip *models.Strip
	var movedToBay string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			movedToBay = bay
			return 1, nil
		},
	}

	svc, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, euroscope.Strip{
		Callsign:    callsign,
		Origin:      "ESSA",
		Destination: "EKCH",
	}, "EKCH")
	require.NoError(t, err)

	require.NotNil(t, updatedStrip)
	assert.Equal(t, shared.BAY_HIDDEN, updatedStrip.Bay)
	assert.Equal(t, shared.BAY_HIDDEN, movedToBay)
}
