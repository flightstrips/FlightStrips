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

func TestReevaluateSquawkValidation_ActivatesWrongSquawkForGroundOwner(t *testing.T) {
	t.Parallel()

	owner := "121.630"
	assigned := "4231"
	observed := "5231"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, session int32) ([]*models.Strip, error) {
			assert.Equal(t, int32(1), session)
			return []*models.Strip{
				{
					Callsign:       "SAS123",
					Owner:          &owner,
					Bay:            shared.BAY_TAXI,
					AssignedSquawk: &assigned,
					Squawk:         &observed,
				},
			}, nil
		},
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
			assert.Equal(t, int32(1), session)
			assert.Equal(t, "SAS123", callsign)
			return &models.Strip{
				Callsign:       "SAS123",
				Owner:          &owner,
				Bay:            shared.BAY_TAXI,
				AssignedSquawk: &assigned,
				Squawk:         &observed,
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
	require.NoError(t, svc.reevaluateSquawkValidation(context.Background(), 1, "SAS123", false, false))
	require.NotNil(t, persisted)
	assert.Equal(t, wrongSquawkValidationIssueType, persisted.IssueType)
	assert.Equal(t, "Pilot is transmitting squawk 5231 but assigned squawk is 4231.", persisted.Message)
	assert.Equal(t, owner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
	assert.Nil(t, persisted.CustomAction)
	assert.NotEmpty(t, persisted.ActivationKey)
}

func TestSquawkValidationApplies_RecognizesFrequencyOwner(t *testing.T) {
	t.Parallel()

	owner := "121.630"
	assert.True(t, squawkValidationApplies(&models.Strip{
		Owner: &owner,
		Bay:   shared.BAY_TAXI,
	}))
}

func TestWrongSquawkValidationApplies_DoesNotApplyInClearedBay(t *testing.T) {
	t.Parallel()

	owner := "121.630"
	assert.False(t, wrongSquawkValidationApplies(&models.Strip{
		Owner: &owner,
		Bay:   shared.BAY_CLEARED,
	}))
}

func TestReevaluateSquawkValidation_ClearsWrongSquawkForNonApplicableOwner(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	assigned := "4231"
	observed := "5231"
	cleared := false

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{
					Callsign:       "SAS123",
					Owner:          &owner,
					Bay:            shared.BAY_CLEARED,
					AssignedSquawk: &assigned,
					Squawk:         &observed,
					ValidationStatus: &models.ValidationStatus{
						IssueType:      wrongSquawkValidationIssueType,
						OwningPosition: owner,
						Active:         true,
						ActivationKey:  "old-key",
					},
				},
			}, nil
		},
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:       "SAS123",
				Owner:          &owner,
				Bay:            shared.BAY_CLEARED,
				AssignedSquawk: &assigned,
				Squawk:         &observed,
				ValidationStatus: &models.ValidationStatus{
					IssueType:      wrongSquawkValidationIssueType,
					OwningPosition: owner,
					Active:         true,
					ActivationKey:  "old-key",
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
	require.NoError(t, svc.reevaluateSquawkValidation(context.Background(), 1, "SAS123", false, false))
	assert.True(t, cleared)
}

func TestReevaluateSquawkValidation_ClearsWrongSquawkInClearedBay(t *testing.T) {
	t.Parallel()

	owner := "121.630"
	assigned := "4231"
	observed := "5231"
	cleared := false

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{
					Callsign:       "SAS123",
					Owner:          &owner,
					Bay:            shared.BAY_CLEARED,
					AssignedSquawk: &assigned,
					Squawk:         &observed,
					ValidationStatus: &models.ValidationStatus{
						IssueType:      wrongSquawkValidationIssueType,
						OwningPosition: owner,
						Active:         true,
						ActivationKey:  "old-key",
					},
				},
			}, nil
		},
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:       "SAS123",
				Owner:          &owner,
				Bay:            shared.BAY_CLEARED,
				AssignedSquawk: &assigned,
				Squawk:         &observed,
				ValidationStatus: &models.ValidationStatus{
					IssueType:      wrongSquawkValidationIssueType,
					OwningPosition: owner,
					Active:         true,
					ActivationKey:  "old-key",
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
	require.NoError(t, svc.reevaluateSquawkValidation(context.Background(), 1, "SAS123", false, false))
	assert.True(t, cleared)
}

func TestApplyWrongSquawkValidation_PreservesAcknowledgedStateForSameMessage(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	assigned := "4231"
	observed := "5231"
	current := &models.ValidationStatus{
		IssueType:      wrongSquawkValidationIssueType,
		Message:        "Pilot is transmitting squawk 5231 but assigned squawk is 4231.",
		OwningPosition: owner,
		Active:         false,
		ActivationKey:  "existing-key",
	}

	svc := NewStripService(&testutil.MockStripRepository{})
	require.NoError(t, svc.applyWrongSquawkValidation(context.Background(), 1, &models.Strip{
		Callsign:         "SAS123",
		Owner:            &owner,
		Bay:              shared.BAY_TAXI,
		AssignedSquawk:   &assigned,
		Squawk:           &observed,
		ValidationStatus: current,
	}, false, false))
}

func TestApplyWrongSquawkValidation_ReactivatesOnSquawkChange(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	assigned := "4231"
	oldObserved := "5231"
	newObserved := "6231"
	current := &models.ValidationStatus{
		IssueType:      wrongSquawkValidationIssueType,
		Message:        "Pilot is transmitting squawk 5231 but assigned squawk is 4231.",
		OwningPosition: owner,
		Active:         false,
		ActivationKey:  "old-key",
	}
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.applyWrongSquawkValidation(context.Background(), 1, &models.Strip{
		Callsign:       "SAS123",
		Owner:          &owner,
		Bay:            shared.BAY_TAXI,
		AssignedSquawk: &assigned,
		Squawk:         &newObserved,
		ValidationStatus: &models.ValidationStatus{
			IssueType:      current.IssueType,
			Message:        current.Message,
			OwningPosition: current.OwningPosition,
			Active:         current.Active,
			ActivationKey:  current.ActivationKey,
		},
	}, false, false))
	require.NotNil(t, persisted)
	assert.Equal(t, wrongSquawkValidationIssueType, persisted.IssueType)
	assert.Equal(t, "Pilot is transmitting squawk 6231 but assigned squawk is 4231.", persisted.Message)
	assert.True(t, persisted.Active)
	assert.NotEqual(t, "old-key", persisted.ActivationKey)
	assert.NotEqual(t, oldObserved, newObserved)
}

func TestReevaluateSquawkValidation_DuplicateTakesPrecedenceOverWrongSquawk(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	assigned := "4231"
	observed := "5231"
	otherObserved := "4231"
	statusByCallsign := map[string]*models.ValidationStatus{}

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{
					Callsign:         "SAS123",
					Owner:            &owner,
					Bay:              shared.BAY_TAXI,
					AssignedSquawk:   &assigned,
					Squawk:           &observed,
					ValidationStatus: statusByCallsign["SAS123"],
				},
				{
					Callsign: "DLH456",
					Squawk:   &otherObserved,
				},
			}, nil
		},
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:         callsign,
				Owner:            &owner,
				Bay:              shared.BAY_TAXI,
				AssignedSquawk:   &assigned,
				Squawk:           &observed,
				ValidationStatus: statusByCallsign[callsign],
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, callsign string, status *models.ValidationStatus) error {
			statusByCallsign[callsign] = status
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.reevaluateSquawkValidation(context.Background(), 1, "SAS123", false, false))
	require.NotNil(t, statusByCallsign["SAS123"])
	assert.Equal(t, duplicateSquawkValidationIssueType, statusByCallsign["SAS123"].IssueType)
	require.NotNil(t, statusByCallsign["SAS123"].CustomAction)
	assert.Equal(t, duplicateSquawkValidationActionKind, statusByCallsign["SAS123"].CustomAction.ActionKind)
}
