package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleTrackingControllerChanged_ReleasePreservesStripOwnershipAndCoordination(t *testing.T) {
	owner := "118.105"
	strip := &models.Strip{
		ID: 1, Callsign: "SAS123", Owner: &owner,
		Bay: shared.BAY_AIRBORNE, TrackingController: "EKCH_D_TWR",
	}
	trackingUpdated := false
	svc := NewStripService(&testutil.MockStripRepository{
		UpdateTrackingControllerFn: func(_ context.Context, session int32, callsign string, controller string) (int64, error) {
			assert.Equal(t, int32(1), session)
			assert.Equal(t, strip.Callsign, callsign)
			assert.Empty(t, controller)
			strip.TrackingController = controller
			trackingUpdated = true
			return 1, nil
		},
	})
	svc.SetControllerRepo(&testutil.MockControllerRepository{})
	svc.SetCoordinationRepo(&testutil.MockCoordinationRepository{})

	require.NoError(t, svc.HandleTrackingControllerChanged(context.Background(), 1, strip.Callsign, ""))
	assert.True(t, trackingUpdated)
	assert.Empty(t, strip.TrackingController)
	assert.Equal(t, owner, *strip.Owner)
	assert.Equal(t, shared.BAY_AIRBORNE, strip.Bay)
}
