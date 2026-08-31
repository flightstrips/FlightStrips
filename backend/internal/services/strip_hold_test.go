package services

import (
	"context"
	"errors"
	"testing"

	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateHold_NotifiesFrontend(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS123"

	var got struct {
		hold, holdType, holdEat string
	}

	stripRepo := &testutil.MockStripRepository{
		UpdateHoldFn: func(_ context.Context, s int32, cs string, hold string, holdType string, holdEat string, _ *int32) (int64, error) {
			assert.Equal(t, session, s)
			assert.Equal(t, callsign, cs)
			got.hold, got.holdType, got.holdEat = hold, holdType, holdEat
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	require.NoError(t, svc.UpdateHold(ctx, session, callsign, "OLPIB", "enroute", "1422"))

	assert.Equal(t, "OLPIB", got.hold)
	assert.Equal(t, "enroute", got.holdType)
	assert.Equal(t, "1422", got.holdEat)

	require.Len(t, hub.HoldEvents, 1)
	assert.Equal(t, callsign, hub.HoldEvents[0].Callsign)
	assert.Equal(t, "OLPIB", hub.HoldEvents[0].Hold)
	assert.Equal(t, "enroute", hub.HoldEvents[0].HoldType)
	assert.Equal(t, "1422", hub.HoldEvents[0].HoldEat)
}

// A cancellation is an empty hold and must still reach the frontend.
func TestUpdateHold_CancellationIsBroadcast(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		UpdateHoldFn: func(_ context.Context, _ int32, _ string, _ string, _ string, _ string, _ *int32) (int64, error) {
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	require.NoError(t, svc.UpdateHold(ctx, 1, "SAS123", "", "", ""))

	require.Len(t, hub.HoldEvents, 1)
	assert.Empty(t, hub.HoldEvents[0].Hold)
}

// Zero rows means the hold was already what we were told.
func TestUpdateHold_UnchangedHoldIsNotBroadcast(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		UpdateHoldFn: func(_ context.Context, _ int32, _ string, _ string, _ string, _ string, _ *int32) (int64, error) {
			return 0, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	require.NoError(t, svc.UpdateHold(ctx, 1, "SAS123", "OLPIB", "enroute", ""))
	assert.Empty(t, hub.HoldEvents)
}

func TestUpdateHold_RepositoryError(t *testing.T) {
	ctx := context.Background()
	wanted := errors.New("boom")

	stripRepo := &testutil.MockStripRepository{
		UpdateHoldFn: func(_ context.Context, _ int32, _ string, _ string, _ string, _ string, _ *int32) (int64, error) {
			return 0, wanted
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	assert.ErrorIs(t, svc.UpdateHold(ctx, 1, "SAS123", "OLPIB", "enroute", ""), wanted)
	assert.Empty(t, hub.HoldEvents)
}
