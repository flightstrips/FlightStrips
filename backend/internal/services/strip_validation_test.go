package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcknowledgeValidationStatus_AllowsPdcValidationForNonOwnerPosition(t *testing.T) {
	t.Parallel()

	var acknowledged bool
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			assert.Equal(t, "SAS123", callsign)
			return &models.Strip{
				Callsign: "SAS123",
				ValidationStatus: &models.ValidationStatus{
					IssueType:      pdcInvalidValidationIssueType,
					OwningPosition: "EKCH_DEL",
					Active:         true,
					ActivationKey:  "activation-key",
				},
			}, nil
		},
		AcknowledgeValidationStatusFn: func(_ context.Context, _ int32, callsign string, activationKey string) (int64, error) {
			assert.Equal(t, "SAS123", callsign)
			assert.Equal(t, "activation-key", activationKey)
			acknowledged = true
			return 1, nil
		},
	}

	svc := NewStripService(repo)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})

	require.NoError(t, svc.AcknowledgeValidationStatus(context.Background(), 1, "SAS123", "activation-key", "EKCH_GND"))
	assert.True(t, acknowledged)
}

func TestAcknowledgeValidationStatus_RejectsNonOwnerForNonPdcValidation(t *testing.T) {
	t.Parallel()

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS123",
				ValidationStatus: &models.ValidationStatus{
					IssueType:      "RUNWAY TYPE",
					OwningPosition: "EKCH_DEL",
					Active:         true,
					ActivationKey:  "activation-key",
				},
			}, nil
		},
	}

	svc := NewStripService(repo)

	err := svc.AcknowledgeValidationStatus(context.Background(), 1, "SAS123", "activation-key", "EKCH_GND")
	require.Error(t, err)
	assert.EqualError(t, err, "acknowledge_validation_status: requesting position is not the owning position")
}
