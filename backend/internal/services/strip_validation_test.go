package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type validationStoreFake struct {
	getByCallsignFn               func(ctx context.Context, session int32, callsign string) (*models.Strip, error)
	setValidationStatusFn         func(ctx context.Context, session int32, callsign string, status *models.ValidationStatus) error
	acknowledgeValidationStatusFn func(ctx context.Context, session int32, callsign string, activationKey string) (int64, error)
	clearValidationStatusFn       func(ctx context.Context, session int32, callsign string) error
}

func (f *validationStoreFake) GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	if f.getByCallsignFn == nil {
		panic("unexpected call to validationStoreFake.GetByCallsign")
	}
	return f.getByCallsignFn(ctx, session, callsign)
}

func (f *validationStoreFake) SetValidationStatus(ctx context.Context, session int32, callsign string, status *models.ValidationStatus) error {
	if f.setValidationStatusFn == nil {
		panic("unexpected call to validationStoreFake.SetValidationStatus")
	}
	return f.setValidationStatusFn(ctx, session, callsign, status)
}

func (f *validationStoreFake) AcknowledgeValidationStatus(ctx context.Context, session int32, callsign string, activationKey string) (int64, error) {
	if f.acknowledgeValidationStatusFn == nil {
		panic("unexpected call to validationStoreFake.AcknowledgeValidationStatus")
	}
	return f.acknowledgeValidationStatusFn(ctx, session, callsign, activationKey)
}

func (f *validationStoreFake) ClearValidationStatus(ctx context.Context, session int32, callsign string) error {
	if f.clearValidationStatusFn == nil {
		panic("unexpected call to validationStoreFake.ClearValidationStatus")
	}
	return f.clearValidationStatusFn(ctx, session, callsign)
}

type readerOnlyStripFake struct{}

func (readerOnlyStripFake) GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	return &models.Strip{Callsign: callsign}, nil
}

func TestAcknowledgeValidationStatus_AllowsPdcValidationForNonOwnerPosition(t *testing.T) {
	t.Parallel()

	var acknowledged bool
	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
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
		acknowledgeValidationStatusFn: func(_ context.Context, _ int32, callsign string, activationKey string) (int64, error) {
			assert.Equal(t, "SAS123", callsign)
			assert.Equal(t, "activation-key", activationKey)
			acknowledged = true
			return 1, nil
		},
	}

	svc := newTestStripValidationService(repo, repo)
	svc.publisher = &testutil.MockFrontendHub{}

	require.NoError(t, svc.AcknowledgeValidationStatus(context.Background(), 1, "SAS123", "activation-key", "EKCH_GND"))
	assert.True(t, acknowledged)
}

func TestAcknowledgeValidationStatus_RejectsNonOwnerForNonPdcValidation(t *testing.T) {
	t.Parallel()

	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
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

	svc := newTestStripValidationService(repo, repo)

	err := svc.AcknowledgeValidationStatus(context.Background(), 1, "SAS123", "activation-key", "EKCH_GND")
	require.Error(t, err)
	assert.EqualError(t, err, "acknowledge_validation_status: requesting position is not the owning position")
}

func TestNewStripValidationServiceRejectsMissingStore(t *testing.T) {
	t.Parallel()

	_, err := NewStripValidationService(StripValidationDependencies{
		Strips: readerOnlyStripFake{}, Publisher: testStripValidationPublisher{},
	})

	require.Error(t, err)
	assert.EqualError(t, err, "strip validation service requires validation status store")
}

func TestReconcileStandAssignmentValidationActivatesBlockedAssignment(t *testing.T) {
	owner := "EKCH_APP"
	var persisted *models.ValidationStatus
	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS123", Owner: &owner}, nil
		},
		setValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}
	svc := newTestStripValidationService(repo, repo)

	require.NoError(t, svc.ReconcileStandAssignmentValidation(context.Background(), 1, "SAS123", []string{"A22"}, ""))
	require.NotNil(t, persisted)
	require.Equal(t, standAssignmentValidationIssueType, persisted.IssueType)
	require.Equal(t, "Assigned stand is blocked by A22.", persisted.Message)
	require.Equal(t, owner, persisted.OwningPosition)
	require.True(t, persisted.Active)
	require.Equal(t, "assign_stand", persisted.CustomAction.ActionKind)
}

func TestReconcileStandAssignmentValidationClearsResolvedSatIssueOnly(t *testing.T) {
	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS123", ValidationStatus: &models.ValidationStatus{IssueType: standAssignmentValidationIssueType}}, nil
		},
		clearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			require.Equal(t, "SAS123", callsign)
			return nil
		},
	}
	svc := newTestStripValidationService(repo, repo)

	require.NoError(t, svc.ReconcileStandAssignmentValidation(context.Background(), 1, "SAS123", nil, ""))
}

func TestNewStripValidationServiceRejectsMissingReader(t *testing.T) {
	t.Parallel()

	_, err := NewStripValidationService(StripValidationDependencies{})

	require.Error(t, err)
	assert.EqualError(t, err, "strip validation service requires strip reader")
}
