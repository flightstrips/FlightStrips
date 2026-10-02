package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncStripPublishesPersistedHoldingClearanceWithoutCID(t *testing.T) {
	const session = int32(1)
	altitude := int32(11000)
	existing := &models.Strip{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH",
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
	require.Nil(t, observer.strips[0].VatsimCID)
}

func TestSyncHoldingClearancesAreSnapshottedAndFlushedAsOneBatch(t *testing.T) {
	observer := &holdingBatchObserverSpy{}
	service := &StripService{holdingObserver: observer}
	state := &shared.SyncState{}
	ctx := shared.WithSyncState(context.Background(), state)
	cid := "1234567"
	altitude := int32(11000)
	strip := &models.Strip{Callsign: "SAS123", VatsimCID: &cid, Hold: "OLPIB", ClearedAltitude: &altitude}
	require.NoError(t, service.observeHoldingClearance(ctx, strip))
	strip.Hold = "TIDVU"
	require.NoError(t, service.observeHoldingClearance(ctx, strip))
	require.NoError(t, service.observeHoldingClearance(ctx, &models.Strip{Callsign: "SAS456", Hold: "OLPIB"}))
	strip.Hold, cid, altitude = "", "changed", 9000
	require.Empty(t, observer.batches)
	require.Empty(t, observer.strips)
	queuedAt := state.HoldingClearanceStrips["SAS123"].ObservedAt

	observer.err = errors.New("temporary failure")
	require.Error(t, service.FlushHoldingClearances(ctx))
	require.Len(t, state.HoldingClearanceStrips, 2, "failed work must remain available for retry")
	observer.err = nil
	require.NoError(t, service.FlushHoldingClearances(ctx))
	require.Len(t, observer.batches, 1)
	require.Len(t, observer.batches[0], 2)
	require.Equal(t, "TIDVU", observer.batches[0][0].Strip.Hold)
	require.Equal(t, "1234567", *observer.batches[0][0].Strip.VatsimCID)
	require.Equal(t, int32(11000), *observer.batches[0][0].Strip.ClearedAltitude)
	require.Equal(t, queuedAt, observer.batches[0][0].ObservedAt)
	require.Empty(t, state.HoldingClearanceStrips)
	require.NoError(t, service.FlushHoldingClearances(ctx))
	require.Len(t, observer.batches, 1)
}

type holdingBatchObserverSpy struct {
	holdingObserverSpy
	batches [][]shared.HoldingClearanceObservation
	err     error
}

func (s *holdingBatchObserverSpy) ObserveHoldingClearances(_ context.Context, observations []shared.HoldingClearanceObservation) error {
	if s.err != nil {
		return s.err
	}
	s.batches = append(s.batches, observations)
	return nil
}

type holdingObserverSpy struct{ strips []*models.Strip }

func TestStripSyncOnlyAcceptsHoldingFieldsFromPersistedTrackingController(t *testing.T) {
	for _, sender := range []string{"EKCH_A_APP", "EKCH_TWR", ""} {
		t.Run(sender, func(t *testing.T) {
			cid := "1234567"
			existing := &models.Strip{
				Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", VatsimCID: &cid,
				TrackingController: "EKCH_A_APP", Hold: "OLPIB", HoldType: "enroute", HoldEat: "1422",
			}
			var persisted *models.Strip
			repo := &testutil.MockStripRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return existing, nil },
				UpdateFn:        func(_ context.Context, strip *models.Strip) (int64, error) { persisted = strip; return 1, nil },
			}
			service, _, _ := newSyncTestFixture(t, existing, repo)
			amanObserver := &amanStripObserverSpy{}
			service.SetEuroScopeAMANStripObserver(amanObserver)
			ctx := shared.WithStripSyncController(context.Background(), sender)
			// Claiming tracking ownership in the incoming snapshot must not
			// authorize its holding fields against a different persisted owner.
			err := service.SyncStrip(ctx, 1, "", euroscope.Strip{
				Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", TrackingController: sender,
				HoldSupported: true, Hold: "TIDVU", HoldType: "enroute", HoldEat: "1450", Remarks: "changed",
			}, "EKCH")
			require.NoError(t, err)
			require.NotNil(t, persisted)
			require.Len(t, amanObserver.strips, 1, "flight-plan observation must not depend on holding authority")
			if sender == "EKCH_A_APP" {
				require.Equal(t, "TIDVU", persisted.Hold)
				require.Equal(t, "1450", persisted.HoldEat)
			} else {
				require.Equal(t, "OLPIB", persisted.Hold)
				require.Equal(t, "1422", persisted.HoldEat)
			}
		})
	}
}

func (s *holdingObserverSpy) ObserveHoldingClearance(_ context.Context, strip *models.Strip) error {
	s.strips = append(s.strips, strip)
	return nil
}

type amanStripObserverSpy struct{ strips []*models.Strip }

func (s *amanStripObserverSpy) ObserveEuroScopeStrip(_ context.Context, strip *models.Strip) error {
	s.strips = append(s.strips, strip)
	return nil
}
