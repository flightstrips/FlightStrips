package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncStripPublishesPersistedHoldingClearance(t *testing.T) {
	const session = int32(1)
	cid := "123456"
	altitude := int32(11000)
	existing := &models.Strip{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", VatsimCID: &cid,
		ClearedAltitude: &altitude,
	}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) { return existing, nil },
		UpdateFn:        func(_ context.Context, _ *models.Strip) (int64, error) { return 1, nil },
	}
	service, _, _ := newSyncTestFixture(t, existing, repo)
	observer := &holdingObserverSpy{}
	service.SetHoldingClearanceObserver(observer)

	require.NoError(t, service.SyncStrip(context.Background(), session, "", euroscope.Strip{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", HoldSupported: true,
		Hold: "OLPIB", HoldType: "enroute", HoldEat: "1422", ClearedAltitude: altitude,
	}, "EKCH"))
	require.Len(t, observer.strips, 1)
	require.Equal(t, "OLPIB", observer.strips[0].Hold)
	require.Equal(t, "enroute", observer.strips[0].HoldType)
	require.Equal(t, "1422", observer.strips[0].HoldEat)
	require.Equal(t, altitude, *observer.strips[0].ClearedAltitude)
}

type holdingObserverSpy struct{ strips []*models.Strip }

func (s *holdingObserverSpy) ObserveHoldingClearance(_ context.Context, strip *models.Strip) error {
	s.strips = append(s.strips, strip)
	return nil
}
