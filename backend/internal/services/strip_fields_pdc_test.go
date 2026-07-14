package services

import (
	"context"
	"testing"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateClearedFlag_EuroscopeClearConfirmsStalePdcAsVoiceClearance(t *testing.T) {
	const (
		session  = int32(1)
		callsign = "EZY13EJ"
	)

	strip := &internalModels.Strip{
		Callsign: callsign,
		Bay:      shared.BAY_NOT_CLEARED,
		Cleared:  false,
		PdcState: "NO_RESPONSE",
	}
	var updatedCleared bool
	var updatedBay string

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*internalModels.Strip, error) {
			return strip, nil
		},
		UpdateClearedFlagFn: func(_ context.Context, _ int32, _ string, cleared bool, bay string, _ *int32) (int64, error) {
			updatedCleared = cleared
			updatedBay = bay
			return 1, nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			updatedBay = bay
			return 1, nil
		},
	}
	pdcSpy := &frontendMovePdcSpy{}
	svc := NewStripService(repo, WithPdcService(pdcSpy))
	svc.SetFrontendHub(&testutil.MockFrontendHub{})

	err := svc.UpdateClearedFlag(context.Background(), session, callsign, true)

	require.NoError(t, err)
	require.Len(t, pdcSpy.confirmCalls, 1)
	assert.Equal(t, callsign, pdcSpy.confirmCalls[0].callsign)
	assert.Equal(t, session, pdcSpy.confirmCalls[0].session)
	assert.True(t, updatedCleared)
	assert.Equal(t, shared.BAY_CLEARED, updatedBay)
}

func TestUpdateClearedFlag_EuroscopeClearConfirmsPendingPdcAsVoiceClearance(t *testing.T) {
	strip := &internalModels.Strip{
		Callsign: "SAS123",
		Bay:      shared.BAY_NOT_CLEARED,
		Cleared:  false,
		PdcState: "CLEARED",
	}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*internalModels.Strip, error) {
			return strip, nil
		},
		UpdateClearedFlagFn: func(_ context.Context, _ int32, _ string, _ bool, _ string, _ *int32) (int64, error) {
			return 1, nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			return 1, nil
		},
	}
	pdcSpy := &frontendMovePdcSpy{}
	svc := NewStripService(repo, WithPdcService(pdcSpy))
	svc.SetFrontendHub(&testutil.MockFrontendHub{})

	err := svc.UpdateClearedFlag(context.Background(), 1, strip.Callsign, true)

	require.NoError(t, err)
	require.Len(t, pdcSpy.confirmCalls, 1)
	assert.Equal(t, strip.Callsign, pdcSpy.confirmCalls[0].callsign)
}

func TestUpdateClearedFlag_EuroscopeClearReconcilesStalePdcWhenFlagAlreadySet(t *testing.T) {
	strip := &internalModels.Strip{
		Callsign: "SAS456",
		Bay:      shared.BAY_CLEARED,
		Cleared:  true,
		PdcState: "FAILED",
	}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*internalModels.Strip, error) {
			return strip, nil
		},
	}
	pdcSpy := &frontendMovePdcSpy{}
	svc := NewStripService(repo, WithPdcService(pdcSpy))

	err := svc.UpdateClearedFlag(context.Background(), 1, strip.Callsign, true)

	require.NoError(t, err)
	require.Len(t, pdcSpy.confirmCalls, 1)
	assert.Equal(t, strip.Callsign, pdcSpy.confirmCalls[0].callsign)
}
