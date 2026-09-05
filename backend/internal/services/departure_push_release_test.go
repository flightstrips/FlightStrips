package services

import (
	"context"
	"errors"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"github.com/stretchr/testify/require"
)

type pushReleaseObserver struct {
	calls     int
	err       error
	onRelease func()
}

func (o *pushReleaseObserver) ObserveDeparturePosition(context.Context, int32, *models.Strip, float64, float64) error {
	return nil
}

func (o *pushReleaseObserver) ReleaseDepartureStand(_ context.Context, _ int32, _ string) error {
	o.calls++
	if o.onRelease != nil {
		o.onRelease()
	}
	return o.err
}

func TestUpdateGroundState_CompletesDownstreamWorkBeforePushReleaseFailure(t *testing.T) {
	state := ""
	strip := &models.Strip{Callsign: "SAS123", Origin: "EKCH", State: &state, Bay: shared.BAY_CLEARED}
	order := make([]string, 0, 3)
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			return strip, nil
		},
		UpdateGroundStateFn: func(context.Context, int32, string, *string, string, *int32) (int64, error) {
			return 1, nil
		},
		GetMaxSequenceInBayFn: func(context.Context, int32, string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(context.Context, int32, string, string, int32) (int64, error) {
			order = append(order, "move")
			return 1, nil
		},
	}
	cdmService := &spyStripCdmService{syncAsatFn: func() { order = append(order, "asat") }}
	releaseErr := errors.New("stand release unavailable")
	observer := &pushReleaseObserver{
		err:       releaseErr,
		onRelease: func() { order = append(order, "release") },
	}
	service := NewStripService(stripRepo)
	service.SetFrontendHub(&testutil.MockFrontendHub{})
	service.SetCdmService(cdmService)
	service.SetDeparturePositionObserver(observer)

	err := service.UpdateGroundState(context.Background(), 1, strip.Callsign, shared.BAY_PUSH, "EKCH")
	require.ErrorIs(t, err, releaseErr)
	require.Equal(t, []string{"move", "asat", "release"}, order)
}

func TestReleaseDepartureStandOnPush(t *testing.T) {
	observer := &pushReleaseObserver{}
	service := &StripService{departureObserver: observer}
	strip := &models.Strip{Callsign: "SAS123", Origin: "EKCH"}

	require.NoError(t, service.releaseDepartureStandOnPush(context.Background(), 1, strip, "PUSH", "EKCH"))
	require.Equal(t, 1, observer.calls)

	require.NoError(t, service.releaseDepartureStandOnPush(context.Background(), 1, strip, "TAXI", "EKCH"))
	require.NoError(t, service.releaseDepartureStandOnPush(context.Background(), 1, strip, shared.BAY_PUSH, "EKBI"))
	require.Equal(t, 1, observer.calls)
}
