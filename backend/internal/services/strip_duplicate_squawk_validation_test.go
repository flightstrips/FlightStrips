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

func TestReevaluateDuplicateSquawkValidation_ActivatesForGroundOwnerInDepartureFlow(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	targetCode := "4231"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, session int32) ([]*models.Strip, error) {
			assert.Equal(t, int32(1), session)
			return []*models.Strip{
				{
					Callsign:       "SAS123",
					Owner:          &owner,
					Bay:            shared.BAY_TAXI,
					AssignedSquawk: &targetCode,
				},
				{
					Callsign: "DLH456",
					Squawk:   &targetCode,
				},
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, session int32, callsign string, status *models.ValidationStatus) error {
			assert.Equal(t, int32(1), session)
			assert.Equal(t, "SAS123", callsign)
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.reevaluateDuplicateSquawkValidation(context.Background(), 1, "SAS123", false, false))
	require.NotNil(t, persisted)
	assert.Equal(t, duplicateSquawkValidationIssueType, persisted.IssueType)
	assert.Equal(t, duplicateSquawkValidationMessage, persisted.Message)
	assert.Equal(t, owner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
	require.NotNil(t, persisted.CustomAction)
	assert.Equal(t, duplicateSquawkValidationActionLabel, persisted.CustomAction.Label)
	assert.Equal(t, duplicateSquawkValidationActionKind, persisted.CustomAction.ActionKind)
	assert.NotEmpty(t, persisted.ActivationKey)
}

func TestReevaluateDuplicateSquawkValidation_ClearsForNonApplicableOwner(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	targetCode := "4231"
	cleared := false

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{
					Callsign:       "SAS123",
					Owner:          &owner,
					Bay:            shared.BAY_CLEARED,
					AssignedSquawk: &targetCode,
					ValidationStatus: &models.ValidationStatus{
						IssueType:      duplicateSquawkValidationIssueType,
						OwningPosition: owner,
						Active:         true,
						ActivationKey:  "old-key",
						CustomAction:   duplicateSquawkValidationAction(),
					},
				},
				{
					Callsign:       "DLH456",
					AssignedSquawk: &targetCode,
				},
			}, nil
		},
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			assert.Equal(t, "SAS123", callsign)
			cleared = true
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.reevaluateDuplicateSquawkValidation(context.Background(), 1, "SAS123", false, false))
	assert.True(t, cleared)
}

func TestReevaluateDuplicateSquawkValidation_PreservesAcknowledgedStateForSameOwner(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	targetCode := "4231"
	current := &models.ValidationStatus{
		IssueType:      duplicateSquawkValidationIssueType,
		Message:        duplicateSquawkValidationMessage,
		OwningPosition: owner,
		Active:         false,
		ActivationKey:  "existing-key",
		CustomAction:   duplicateSquawkValidationAction(),
	}

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{
					Callsign:         "SAS123",
					Owner:            &owner,
					Bay:              shared.BAY_TAXI,
					AssignedSquawk:   &targetCode,
					ValidationStatus: current,
				},
				{
					Callsign: "DLH456",
					Squawk:   &targetCode,
				},
			}, nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.reevaluateDuplicateSquawkValidation(context.Background(), 1, "SAS123", false, false))
}

func TestReevaluateDuplicateSquawkValidation_ReactivatesOnOwnerChange(t *testing.T) {
	t.Parallel()

	oldOwner := "EKCH_A_GND"
	newOwner := "EKCH_A_TWR"
	targetCode := "4231"
	current := &models.ValidationStatus{
		IssueType:      duplicateSquawkValidationIssueType,
		Message:        duplicateSquawkValidationMessage,
		OwningPosition: oldOwner,
		Active:         false,
		ActivationKey:  "old-key",
		CustomAction:   duplicateSquawkValidationAction(),
	}
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{
					Callsign:         "SAS123",
					Owner:            &newOwner,
					Bay:              shared.BAY_DEPART,
					AssignedSquawk:   &targetCode,
					ValidationStatus: current,
				},
				{
					Callsign: "DLH456",
					Squawk:   &targetCode,
				},
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.reevaluateDuplicateSquawkValidation(context.Background(), 1, "SAS123", false, true))
	require.NotNil(t, persisted)
	assert.True(t, persisted.Active)
	assert.Equal(t, newOwner, persisted.OwningPosition)
	assert.NotEqual(t, "old-key", persisted.ActivationKey)
}

func TestReevaluateDuplicateSquawkValidation_ClearsWhenDuplicateDisappears(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	targetCode := "4231"
	cleared := false

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{
					Callsign:       "SAS123",
					Owner:          &owner,
					Bay:            shared.BAY_TAXI,
					AssignedSquawk: &targetCode,
					ValidationStatus: &models.ValidationStatus{
						IssueType:      duplicateSquawkValidationIssueType,
						OwningPosition: owner,
						Active:         true,
						ActivationKey:  "old-key",
						CustomAction:   duplicateSquawkValidationAction(),
					},
				},
				{
					Callsign: "DLH456",
					AssignedSquawk: func() *string {
						value := "5231"
						return &value
					}(),
				},
			}, nil
		},
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			assert.Equal(t, "SAS123", callsign)
			cleared = true
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.reevaluateDuplicateSquawkValidation(context.Background(), 1, "SAS123", false, false))
	assert.True(t, cleared)
}

func TestReevaluateDuplicateSquawkValidationsForSession_UpdatesOtherAffectedStrips(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	targetCode := "4231"
	changed := make(map[string]bool)

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{
					Callsign:       "SAS123",
					Owner:          &owner,
					Bay:            shared.BAY_TAXI,
					AssignedSquawk: &targetCode,
				},
				{
					Callsign:       "DLH456",
					Owner:          &owner,
					Bay:            shared.BAY_PUSH,
					AssignedSquawk: &targetCode,
				},
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, callsign string, _ *models.ValidationStatus) error {
			changed[callsign] = true
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.reevaluateDuplicateSquawkValidationsForSession(context.Background(), 1, false))
	assert.Equal(t, map[string]bool{"SAS123": true, "DLH456": true}, changed)
}
