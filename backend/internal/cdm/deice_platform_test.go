package cdm

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleDeicePlatformUpdate_PersistsNormalizedSelection(t *testing.T) {
	stored := (&models.CdmData{}).Normalize()
	setCount := 0
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS123"}, nil
		},
		GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
			return stored.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			setCount++
			stored = data.Clone()
			return 1, nil
		},
	}
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, nil, nil)

	require.NoError(t, service.HandleDeicePlatformUpdate(context.Background(), 7, "SAS123", " b "))
	require.NotNil(t, stored.DeicePlatform)
	assert.Equal(t, "B", *stored.DeicePlatform)
	assert.False(t, stored.DeicePlatformAcknowledged)
	assert.True(t, stored.NeedsLocalRecalculation())
	assert.Equal(t, 1, setCount)

	stored.DeicePlatformAcknowledged = true
	require.NoError(t, service.HandleDeicePlatformUpdate(context.Background(), 7, "SAS123", "B"))
	assert.False(t, stored.DeicePlatformAcknowledged)
	assert.Equal(t, 2, setCount)

	require.NoError(t, service.AcknowledgeDeicePlatform(context.Background(), 7, "SAS123"))
	assert.True(t, stored.DeicePlatformAcknowledged)
	assert.Equal(t, 3, setCount)

	require.NoError(t, service.HandleDeicePlatformUpdate(context.Background(), 7, "SAS123", ""))
	assert.Nil(t, stored.DeicePlatform)
	assert.False(t, stored.DeicePlatformAcknowledged)
	assert.Equal(t, 4, setCount)
}

func TestHandleDeicePlatformUpdate_RejectsUnknownPlatform(t *testing.T) {
	service := newTestCdmService(newTestClientWithAirportMasters(nil), &testutil.MockStripRepository{}, nil, nil)

	err := service.HandleDeicePlatformUpdate(context.Background(), 7, "SAS123", "X")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid de-icing platform")
}
