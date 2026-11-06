package services

import (
	"context"
	"errors"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestUpdateClearedAltitudeImmediatelyNotifiesAMANFromPersistedStrip(t *testing.T) {
	for _, altitude := range []int32{9000, 0} {
		t.Run(map[int32]string{9000: "descent", 0: "clear level"}[altitude], func(t *testing.T) {
			previous := int32(11000)
			persisted := &models.Strip{
				Session: 12, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH",
				Hold: "OLPIB", HoldType: "enroute", HoldEat: "2022", ClearedAltitude: &previous,
			}
			written := false
			repo := &testutil.MockStripRepository{
				UpdateClearedAltitudeFn: func(_ context.Context, session int32, callsign string, level, version *int32) (int64, error) {
					require.Equal(t, persisted.Session, session)
					require.Equal(t, persisted.Callsign, callsign)
					require.Equal(t, altitude, *level)
					require.Nil(t, version)
					persisted.ClearedAltitude = level
					written = true
					return 1, nil
				},
				GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
					require.True(t, written, "AMAN must receive the persisted clearance after the field update")
					require.Equal(t, persisted.Session, session)
					require.Equal(t, persisted.Callsign, callsign)
					return persisted, nil
				},
			}
			observer := &holdingObserverSpy{}
			service := newWorkflowTestService(repo)
			service.SetFrontendHub(&testutil.MockFrontendHub{})
			service.SetHoldingClearanceObserver(observer)

			require.NoError(t, service.UpdateClearedAltitude(context.Background(), persisted.Session, persisted.Callsign, altitude))
			require.Len(t, observer.strips, 1, "a level change must reach AMAN without another hold event or full sync")
			require.Equal(t, persisted, observer.strips[0])
			require.Equal(t, altitude, *observer.strips[0].ClearedAltitude)
		})
	}
}

func TestUpdateClearedAltitudeOnlyObservesSuccessfulWrites(t *testing.T) {
	wanted := errors.New("write failed")
	for _, scenario := range []struct {
		name  string
		count int64
		err   error
	}{
		{name: "missing strip"},
		{name: "failed write", err: wanted},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			repo := &testutil.MockStripRepository{
				UpdateClearedAltitudeFn: func(context.Context, int32, string, *int32, *int32) (int64, error) {
					return scenario.count, scenario.err
				},
			}
			observer := &holdingObserverSpy{}
			service := newWorkflowTestService(repo)
			service.SetFrontendHub(&testutil.MockFrontendHub{})
			service.SetHoldingClearanceObserver(observer)

			require.ErrorIs(t, service.UpdateClearedAltitude(context.Background(), 12, "SAS123", 9000), scenario.err)
			require.Empty(t, observer.strips)
		})
	}
}

func TestUpdateClearedAltitudePropagatesHoldingObservationReadFailure(t *testing.T) {
	wanted := errors.New("read failed")
	repo := &testutil.MockStripRepository{
		UpdateClearedAltitudeFn: func(context.Context, int32, string, *int32, *int32) (int64, error) { return 1, nil },
		GetByCallsignFn:         func(context.Context, int32, string) (*models.Strip, error) { return nil, wanted },
	}
	service := newWorkflowTestService(repo)
	service.SetFrontendHub(&testutil.MockFrontendHub{})
	service.SetHoldingClearanceObserver(&holdingObserverSpy{})

	require.ErrorIs(t, service.UpdateClearedAltitude(context.Background(), 12, "SAS123", 9000), wanted)
}
