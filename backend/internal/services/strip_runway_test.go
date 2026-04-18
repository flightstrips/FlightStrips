package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
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
