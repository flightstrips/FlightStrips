package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReaRequestTobtResetsTobtOnRepeatedRea(t *testing.T) {
	now := time.Date(2026, 10, 12, 14, 50, 0, 0, time.UTC)

	for _, currentTobt := range []string{"1453", ""} {
		t.Run(currentTobt, func(t *testing.T) {
			status := "REA"
			data := &models.CdmData{Status: &status}
			if currentTobt != "" {
				data.Tobt = &currentTobt
			}

			tobt, change := reaRequestTobt(data, now)

			assert.True(t, change)
			assert.Equal(t, "1450", tobt)
		})
	}
}

func TestReaRequestTobtSixMinuteBoundary(t *testing.T) {
	now := time.Date(2026, 10, 12, 14, 50, 0, 0, time.UTC)
	for _, tc := range []struct {
		tobt   string
		change bool
	}{
		{"1455", false},
		{"1456", false},
		{"145600", false},
		{"145601", true},
		{"1457", true},
		{"1450", false},
		{"1445", false},
		{"1444", false},
		{"144400", false},
		{"144359", true},
		{"1443", true},
		{"1440", true},
		{"", false},
		{"2500", false},
	} {
		t.Run(tc.tobt, func(t *testing.T) {
			tobt, change := reaRequestTobt(&models.CdmData{Tobt: stringPtr(tc.tobt)}, now)
			assert.Equal(t, tc.change, change)
			if tc.change {
				assert.Equal(t, "1450", tobt)
			} else {
				assert.Equal(t, tc.tobt, tobt)
			}
		})
	}
	_, change := reaRequestTobt(nil, now)
	assert.False(t, change)
}

func TestReaRequestTobtAcrossMidnight(t *testing.T) {
	now := time.Date(2026, 10, 12, 23, 58, 0, 0, time.UTC)
	tobt, change := reaRequestTobt(&models.CdmData{Tobt: stringPtr("0005")}, now)
	assert.True(t, change)
	assert.Equal(t, "2358", tobt)
	_, change = reaRequestTobt(&models.CdmData{Tobt: stringPtr("0004")}, now)
	assert.False(t, change)

	now = time.Date(2026, 10, 13, 0, 2, 0, 0, time.UTC)
	tobt, change = reaRequestTobt(&models.CdmData{Tobt: stringPtr("2355")}, now)
	assert.True(t, change)
	assert.Equal(t, "0002", tobt)
	_, change = reaRequestTobt(&models.CdmData{Tobt: stringPtr("2356")}, now)
	assert.False(t, change)
}

func TestHandleReaRequestPersistsStatusAndChangesTobtOutsideSixMinuteWindow(t *testing.T) {
	for _, offset := range []time.Duration{-10 * time.Minute, -5 * time.Minute, 5 * time.Minute, 15 * time.Minute} {
		t.Run(offset.String(), func(t *testing.T) {
			initialTobt := time.Now().UTC().Add(offset).Format("1504")
			var persisted *models.CdmData
			service := newTestCdmService(
				newTestClientWithAirportMasters(nil),
				&testutil.MockStripRepository{
					GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
						return &models.Strip{Callsign: "SAS123", Origin: "EKCH"}, nil
					},
					GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
						if persisted != nil {
							return persisted.Clone(), nil
						}
						return (&models.CdmData{Tobt: stringPtr(initialTobt)}).Normalize(), nil
					},
					SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
						persisted = data.Clone()
						return 1, nil
					},
				},
				&testutil.MockSessionRepository{},
				&testutil.MockControllerRepository{},
			)
			service.client.isValid = false
			publisher := &testutil.MockFrontendHub{}
			setTestCdmFrontend(service, publisher)
			before := time.Now().UTC().Format("1504")
			require.NoError(t, service.HandleReaRequest(context.Background(), 42, "SAS123", "119.905", "ATC"))
			after := time.Now().UTC().Format("1504")
			require.NotNil(t, persisted)
			require.NotNil(t, persisted.Status)
			assert.Equal(t, "REA", *persisted.Status)
			require.NotNil(t, persisted.Tobt)
			if offset > 6*time.Minute || offset < -6*time.Minute {
				assert.Contains(t, []string{before, after}, *persisted.Tobt)
			} else {
				assert.Equal(t, initialTobt, *persisted.Tobt)
			}
			require.NotEmpty(t, publisher.CdmUpdates)
			assert.Equal(t, "REA", publisher.CdmUpdates[len(publisher.CdmUpdates)-1].Event.Status)
		})
	}
}
