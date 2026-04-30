package services

import (
	"FlightStrips/internal/testutil"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateHeadingSendsStripUpdateForClxValidationRefresh(t *testing.T) {
	const session = int32(7)
	const callsign = "SAS123"
	const heading = int32(40)

	stripRepo := &testutil.MockStripRepository{
		UpdateHeadingFn: func(_ context.Context, gotSession int32, gotCallsign string, gotHeading *int32, version *int32) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			require.NotNil(t, gotHeading)
			assert.Equal(t, heading, *gotHeading)
			assert.Nil(t, version)
			return 1, nil
		},
	}
	frontendHub := &testutil.MockFrontendHub{}
	service := NewStripService(stripRepo)
	service.SetFrontendHub(frontendHub)

	err := service.UpdateHeading(context.Background(), session, callsign, heading)
	require.NoError(t, err)

	assert.Equal(t, []testutil.StripUpdateCall{{Session: session, Callsign: callsign}}, frontendHub.StripUpdates)
}

func TestUpdateRequestedAltitudeSendsStripUpdateForClxValidationRefresh(t *testing.T) {
	const session = int32(7)
	const callsign = "SAS123"
	const altitude = int32(28000)

	stripRepo := &testutil.MockStripRepository{
		UpdateRequestedAltitudeFn: func(_ context.Context, gotSession int32, gotCallsign string, gotAltitude *int32, version *int32) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			require.NotNil(t, gotAltitude)
			assert.Equal(t, altitude, *gotAltitude)
			assert.Nil(t, version)
			return 1, nil
		},
	}
	frontendHub := &testutil.MockFrontendHub{}
	service := NewStripService(stripRepo)
	service.SetFrontendHub(frontendHub)

	err := service.UpdateRequestedAltitude(context.Background(), session, callsign, altitude)
	require.NoError(t, err)

	assert.Equal(t, []testutil.StripUpdateCall{{Session: session, Callsign: callsign}}, frontendHub.StripUpdates)
}
