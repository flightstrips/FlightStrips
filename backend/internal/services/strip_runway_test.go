package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunwayClearance_RejectsActiveValidation(t *testing.T) {
	t.Parallel()

	active := true
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
			assert.Equal(t, int32(1), session)
			assert.Equal(t, "SAS123", callsign)
			return &models.Strip{
				Callsign: "SAS123",
				ValidationStatus: &models.ValidationStatus{
					IssueType:      wrongSquawkValidationIssueType,
					Active:         active,
					OwningPosition: "121.630",
				},
			}, nil
		},
	}

	svc := NewStripService(repo)
	err := svc.RunwayClearance(context.Background(), 1, "SAS123", "1001", "EKCH")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locked by an active validation")
}

func TestRunwayConfirmation_RejectsActiveValidation(t *testing.T) {
	t.Parallel()

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
			assert.Equal(t, int32(1), session)
			assert.Equal(t, "SAS123", callsign)
			return &models.Strip{
				Callsign: "SAS123",
				ValidationStatus: &models.ValidationStatus{
					IssueType:      wrongSquawkValidationIssueType,
					Active:         true,
					OwningPosition: "121.630",
				},
			}, nil
		},
	}

	svc := NewStripService(repo)
	err := svc.RunwayConfirmation(context.Background(), 1, "SAS123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locked by an active validation")
}

func TestRunwayClearance_TaxiLwrAssignsEndSequenceInDepartBay(t *testing.T) {
	t.Parallel()

	const session = int32(1)
	const callsign = "SAS123"

	var updatedBay string
	var updatedSequence int32

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign: callsign,
				Bay:      shared.BAY_TAXI_LWR,
				Origin:   "EKCH",
			}, nil
		},
		UpdateRunwayClearanceFn: func(_ context.Context, gotSession int32, gotCallsign string) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, gotSession int32, gotCallsign string, bay string, sequence int32) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			updatedBay = bay
			updatedSequence = sequence
			return 1, nil
		},
		UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, state *string, bay string, _ *int32) (int64, error) {
			require.NotNil(t, state)
			assert.Equal(t, shared.BAY_DEPART, bay)
			return 1, nil
		},
	}

	tacticalRepo := &testutil.MockTacticalStripRepository{
		GetMaxSequenceInBayFn: func(_ context.Context, gotSession int32, bay string) (int32, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, shared.BAY_DEPART, bay)
			return 2400, nil
		},
	}

	svc := NewStripService(stripRepo)
	svc.SetTacticalStripRepo(tacticalRepo)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})

	err := svc.RunwayClearance(context.Background(), session, callsign, "CID123", "EKCH")
	require.NoError(t, err)
	assert.Equal(t, shared.BAY_DEPART, updatedBay)
	assert.Equal(t, int32(2400+InitialOrderSpacing), updatedSequence)
}

func TestRunwayClearance_FinalAssignsEndSequenceInRwyArrBay(t *testing.T) {
	t.Parallel()

	const session = int32(1)
	const callsign = "SAS789"

	var updatedBay string
	var updatedSequence int32

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign: callsign,
				Bay:      shared.BAY_FINAL,
				Origin:   "ESSA",
			}, nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, gotSession int32, bay string) (int32, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, shared.BAY_RWY_ARR, bay)
			return 600, nil
		},
		UpdateRunwayClearanceFn: func(_ context.Context, gotSession int32, gotCallsign string) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, gotSession int32, gotCallsign string, bay string, sequence int32) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			updatedBay = bay
			updatedSequence = sequence
			return 1, nil
		},
	}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})

	err := svc.RunwayClearance(context.Background(), session, callsign, "CID123", "EKCH")
	require.NoError(t, err)
	assert.Equal(t, shared.BAY_RWY_ARR, updatedBay)
	assert.Equal(t, int32(600+InitialOrderSpacing), updatedSequence)
}
