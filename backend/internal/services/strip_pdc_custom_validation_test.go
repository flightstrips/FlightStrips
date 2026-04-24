package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReevaluatePdcCustomValidation_ActivatesForDeliveryOwnerWithRemarks(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	remarks := "REQ VOICE CONFIRMATION"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			assert.Equal(t, "SAS123", callsign)
			return &models.Strip{
				Callsign:          "SAS123",
				Owner:             &owner,
				PdcState:          "REQUESTED",
				PdcRequestRemarks: &remarks,
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcCustomValidation(context.Background(), 1, "SAS123", false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, pdcCustomValidationIssueType, persisted.IssueType)
	assert.Equal(t, owner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
	require.NotNil(t, persisted.CustomAction)
	assert.Equal(t, pdcCustomValidationActionKind, persisted.CustomAction.ActionKind)
	assert.Equal(t, pdcCustomValidationActionLabel, persisted.CustomAction.Label)
	assert.Contains(t, persisted.Message, remarks)
	assert.Contains(t, persisted.Message, "manual handling")
	assert.NotEmpty(t, persisted.ActivationKey)
}

func TestReevaluatePdcCustomValidation_ActivatesWithoutOwner(t *testing.T) {
	t.Parallel()

	remarks := "REQ VOICE CONFIRMATION"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			assert.Equal(t, "SAS123", callsign)
			return &models.Strip{
				Callsign:          "SAS123",
				PdcState:          "REQUESTED",
				PdcRequestRemarks: &remarks,
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcCustomValidation(context.Background(), 1, "SAS123", false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, pdcCustomValidationIssueType, persisted.IssueType)
	assert.Equal(t, "", persisted.OwningPosition)
	assert.True(t, persisted.Active)
}

func TestReevaluatePdcCustomValidation_ClearsWhenRemarksRemoved(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	cleared := false

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS123",
				Owner:    &owner,
				PdcState: "REQUESTED",
				ValidationStatus: &models.ValidationStatus{
					IssueType:      pdcCustomValidationIssueType,
					Message:        "old",
					OwningPosition: owner,
					Active:         true,
					ActivationKey:  "old-key",
					CustomAction:   pdcCustomValidationAction(),
				},
			}, nil
		},
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			assert.Equal(t, "SAS123", callsign)
			cleared = true
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcCustomValidation(context.Background(), 1, "SAS123", false, false))
	assert.True(t, cleared)
}

func TestReevaluatePdcRequestValidationsForStrip_ClearsCustomWhenStateNoLongerRequested(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	cleared := false

	repo := &testutil.MockStripRepository{
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			assert.Equal(t, "SAS123", callsign)
			cleared = true
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	strip := &models.Strip{
		Callsign: "SAS123",
		Owner:    &owner,
		PdcState: "CLEARED",
		ValidationStatus: &models.ValidationStatus{
			IssueType:      pdcCustomValidationIssueType,
			Message:        "old custom",
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "old-key",
			CustomAction:   pdcCustomValidationAction(),
		},
	}

	require.NoError(t, svc.ReevaluatePdcRequestValidationsForStrip(context.Background(), 1, strip, []string{"22R"}, false, false))

	assert.True(t, cleared)
	assert.Nil(t, strip.ValidationStatus)
}

func TestReevaluatePdcRequestValidationsForStrip_TransitionsFromInvalidToCustom(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	remarks := "NEEDS MANUAL REVIEW"
	var persisted *models.ValidationStatus
	cleared := false

	repo := &testutil.MockStripRepository{
		ClearValidationStatusFn: func(_ context.Context, _ int32, _ string) error {
			cleared = true
			return nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	strip := &models.Strip{
		Callsign:          "SAS123",
		Owner:             &owner,
		PdcState:          "REQUESTED",
		PdcRequestRemarks: &remarks,
		ValidationStatus: &models.ValidationStatus{
			IssueType:      pdcInvalidValidationIssueType,
			Message:        "old invalid",
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "old-key",
			CustomAction:   pdcInvalidValidationAction(),
		},
	}

	require.NoError(t, svc.ReevaluatePdcRequestValidationsForStrip(context.Background(), 1, strip, []string{"22R"}, false, false))

	assert.True(t, cleared)
	require.NotNil(t, persisted)
	assert.Equal(t, pdcCustomValidationIssueType, persisted.IssueType)
	assert.Contains(t, persisted.Message, remarks)
}

func TestReevaluatePdcRequestValidationsForStrip_TransitionsFromCustomToInvalid(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	remarks := "MANUAL"
	runway := "22L"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	strip := &models.Strip{
		Callsign:          "SAS123",
		Owner:             &owner,
		Runway:            &runway,
		PdcState:          "REQUESTED_WITH_FAULTS",
		PdcRequestRemarks: &remarks,
		ValidationStatus: &models.ValidationStatus{
			IssueType:      pdcCustomValidationIssueType,
			Message:        "old custom",
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "old-key",
			CustomAction:   pdcCustomValidationAction(),
		},
	}

	require.NoError(t, svc.ReevaluatePdcRequestValidationsForStrip(context.Background(), 1, strip, []string{"22R"}, false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, pdcInvalidValidationIssueType, persisted.IssueType)
	assert.Contains(t, persisted.Message, "Runway 22L is not an active departure runway")
}
