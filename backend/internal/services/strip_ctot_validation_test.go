package services

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCtotMoreThanThresholdAhead_HandlesMidnightRollover(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, time.January, 1, 23, 55, 0, 0, time.UTC)
	assert.True(t, ctotMoreThanThresholdAhead("0008", now))
	assert.False(t, ctotMoreThanThresholdAhead("0005", now))
}

func TestApplyCtotValidation_ActivatesForTowerOwnerInDepartureFlow(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, time.January, 1, 10, 0, 0, 0, time.UTC)
	owner := "EKCH_A_TWR"
	ctot := "1011"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		SetValidationStatusFn: func(_ context.Context, session int32, callsign string, status *models.ValidationStatus) error {
			assert.Equal(t, int32(1), session)
			assert.Equal(t, "SAS123", callsign)
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.applyCtotValidation(context.Background(), 1, &models.Strip{
		Callsign: "SAS123",
		Owner:    &owner,
		Bay:      shared.BAY_TAXI_LWR,
		CdmData:  &models.CdmData{Ctot: &ctot},
	}, now, false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, ctotValidationIssueType, persisted.IssueType)
	assert.Equal(t, ctotValidationMessage, persisted.Message)
	assert.Equal(t, owner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
	require.NotNil(t, persisted.CustomAction)
	assert.Equal(t, ctotValidationActionKind, persisted.CustomAction.ActionKind)
	assert.Equal(t, ctotValidationActionLabel, persisted.CustomAction.Label)
	assert.NotEmpty(t, persisted.ActivationKey)
}

func TestApplyCtotValidation_ClearsWhenThresholdNoLongerApplies(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, time.January, 1, 10, 1, 0, 0, time.UTC)
	owner := "EKCH_A_TWR"
	ctot := "1010"
	cleared := false

	repo := &testutil.MockStripRepository{
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			assert.Equal(t, "SAS123", callsign)
			cleared = true
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.applyCtotValidation(context.Background(), 1, &models.Strip{
		Callsign: "SAS123",
		Owner:    &owner,
		Bay:      shared.BAY_DEPART,
		CdmData:  &models.CdmData{Ctot: &ctot},
		ValidationStatus: &models.ValidationStatus{
			IssueType:      ctotValidationIssueType,
			Message:        ctotValidationMessage,
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "existing-key",
			CustomAction:   ctotValidationAction(),
		},
	}, now, false, false))

	assert.True(t, cleared)
}

func TestApplyCtotValidation_PreservesAcknowledgedStateForSameOwner(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, time.January, 1, 10, 0, 0, 0, time.UTC)
	owner := "EKCH_A_TWR"
	ctot := "1015"
	setCalled := false
	clearCalled := false

	svc := NewStripService(&testutil.MockStripRepository{
		SetValidationStatusFn: func(context.Context, int32, string, *models.ValidationStatus) error {
			setCalled = true
			return nil
		},
		ClearValidationStatusFn: func(context.Context, int32, string) error {
			clearCalled = true
			return nil
		},
	})
	require.NoError(t, svc.applyCtotValidation(context.Background(), 1, &models.Strip{
		Callsign: "SAS123",
		Owner:    &owner,
		Bay:      shared.BAY_DEPART,
		CdmData:  &models.CdmData{Ctot: &ctot},
		ValidationStatus: &models.ValidationStatus{
			IssueType:      ctotValidationIssueType,
			Message:        ctotValidationMessage,
			OwningPosition: owner,
			Active:         false,
			ActivationKey:  "existing-key",
			CustomAction:   ctotValidationAction(),
		},
	}, now, false, false))
	assert.False(t, setCalled)
	assert.False(t, clearCalled)
}

func TestApplyCtotValidation_ReactivatesOnOwnerChange(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, time.January, 1, 10, 0, 0, 0, time.UTC)
	oldOwner := "EKCH_C_TWR"
	newOwner := "EKCH_A_TWR"
	ctot := "1015"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.applyCtotValidation(context.Background(), 1, &models.Strip{
		Callsign: "SAS123",
		Owner:    &newOwner,
		Bay:      shared.BAY_DEPART,
		CdmData:  &models.CdmData{Ctot: &ctot},
		ValidationStatus: &models.ValidationStatus{
			IssueType:      ctotValidationIssueType,
			Message:        ctotValidationMessage,
			OwningPosition: oldOwner,
			Active:         false,
			ActivationKey:  "old-key",
			CustomAction:   ctotValidationAction(),
		},
	}, now, false, true))

	require.NotNil(t, persisted)
	assert.True(t, persisted.Active)
	assert.Equal(t, newOwner, persisted.OwningPosition)
	assert.NotEqual(t, "old-key", persisted.ActivationKey)
}

func TestReevaluateSquawkValidation_TransitionsFromWrongSquawkToCtot(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, time.January, 1, 10, 0, 0, 0, time.UTC)
	owner := "EKCH_A_TWR"
	assignedSquawk := "4231"
	cleared := false
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		ListFn: func(context.Context, int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{
					Callsign:       "SAS123",
					Owner:          &owner,
					Bay:            shared.BAY_DEPART,
					AssignedSquawk: &assignedSquawk,
					Squawk:         &assignedSquawk,
					CdmData:        &models.CdmData{Ctot: func() *string { value := "1015"; return &value }()},
					ValidationStatus: &models.ValidationStatus{
						IssueType:      wrongSquawkValidationIssueType,
						Message:        wrongSquawkValidationMessage(&models.Strip{AssignedSquawk: &assignedSquawk, Squawk: func() *string { value := "5231"; return &value }()}),
						OwningPosition: owner,
						Active:         true,
						ActivationKey:  "wrong-key",
					},
				},
			}, nil
		},
		ClearValidationStatusFn: func(context.Context, int32, string) error {
			cleared = true
			return nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)
	originalNow := ctotValidationNow
	ctotValidationNow = func() time.Time { return now }
	t.Cleanup(func() {
		ctotValidationNow = originalNow
	})

	require.NoError(t, svc.reevaluateSquawkValidation(context.Background(), 1, "SAS123", false, false))
	assert.True(t, cleared)
	require.NotNil(t, persisted)
	assert.Equal(t, ctotValidationIssueType, persisted.IssueType)
}
