package frontend

import (
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveOwnersUpdateNextDisplay_ComputesWhenBroadcastOmitsIt(t *testing.T) {
	strip := &internalModels.Strip{Callsign: "SAS123"}

	hub := &Hub{
		server: &testutil.MockServer{
			StripRepoVal: &testutil.MockStripRepository{
				GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*internalModels.Strip, error) {
					assert.Equal(t, int32(42), session)
					assert.Equal(t, "SAS123", callsign)
					return strip, nil
				},
			},
			ComputeNextDisplayForStripContextFn: func(_ context.Context, loaded *internalModels.Strip, session int32) (*internalModels.NextDisplay, error) {
				assert.Same(t, strip, loaded)
				assert.Equal(t, int32(42), session)
				return &internalModels.NextDisplay{Label: "K", Frequency: "124.980"}, nil
			},
		},
	}

	nextDisplay := hub.resolveOwnersUpdateNextDisplay(42, "SAS123", nil)

	require.NotNil(t, nextDisplay)
	assert.Equal(t, "K", nextDisplay.Label)
	assert.Equal(t, "124.980", nextDisplay.Frequency)
	require.NotNil(t, strip.NextDisplay)
	assert.Equal(t, "K", strip.NextDisplay.Label)
}

func TestControllerPayload_EnrichesBlankMetadataFromSectorOwnership(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_W_APP", Frequency: "119.805", Section: "APP"},
	}))

	hub := &Hub{
		server: &testutil.MockServer{
			SectorRepoVal: &testutil.MockSectorOwnerRepository{
				ListBySessionFn: func(_ context.Context, session int32) ([]*internalModels.SectorOwner, error) {
					assert.Equal(t, int32(42), session)
					return []*internalModels.SectorOwner{
						{
							Session:    42,
							Position:   "119.805",
							Identifier: "K",
							Sector:     []string{"K_DEP"},
						},
					}, nil
				},
			},
		},
	}

	controller := hub.controllerPayload(42, "EKCH_W_APP", "119.805", "", nil)

	assert.Equal(t, "EKCH_W_APP", controller.Callsign)
	assert.Equal(t, "119.805", controller.Position)
	assert.Equal(t, "APP", controller.Section)
	assert.Equal(t, "K", controller.Identifier)
	assert.Equal(t, []string{"K_DEP"}, controller.OwnedSectors)
}
