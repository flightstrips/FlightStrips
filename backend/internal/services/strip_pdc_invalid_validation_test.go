package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	pkgModels "FlightStrips/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPdcInvalidValidationFixture(stripRepo *testutil.MockStripRepository, departureRunways ...string) (*StripService, *testutil.MockFrontendHub) {
	hub := &testutil.MockFrontendHub{}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{
				ID: id,
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: departureRunways,
				},
			}, nil
		},
	}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetSessionRepo(sessionRepo)
	return svc, hub
}

func TestReevaluatePdcInvalidValidation_ActivatesForDeliveryOwnerWithRelevantFaults(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	sid := "BETUD"
	runway := "22L"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			assert.Equal(t, "SAS123", callsign)
			return &models.Strip{
				Callsign: "SAS123",
				Owner:    &owner,
				Bay:      shared.BAY_NOT_CLEARED,
				Sid:      &sid,
				Runway:   &runway,
				PdcState: "REQUESTED_WITH_FAULTS",
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcInvalidValidation(context.Background(), 1, "SAS123", false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, pdcInvalidValidationIssueType, persisted.IssueType)
	assert.Equal(t, owner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
	require.NotNil(t, persisted.CustomAction)
	assert.Equal(t, pdcInvalidValidationActionKind, persisted.CustomAction.ActionKind)
	assert.Equal(t, pdcInvalidValidationActionLabel, persisted.CustomAction.Label)
	assert.Contains(t, persisted.Message, "SID BETUD is not available via PDC")
	assert.Contains(t, persisted.Message, "Runway 22L is not an active departure runway")
	assert.Contains(t, persisted.Message, "Open DCL menu to review the request and correct the issue.")
	assert.NotEmpty(t, persisted.ActivationKey)
}

func TestReevaluatePdcInvalidValidation_ActivatesWithoutOwner(t *testing.T) {
	t.Parallel()

	sid := "BETUD"
	runway := "22L"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			assert.Equal(t, "SAS123", callsign)
			return &models.Strip{
				Callsign: "SAS123",
				Bay:      shared.BAY_NOT_CLEARED,
				Sid:      &sid,
				Runway:   &runway,
				PdcState: "REQUESTED_WITH_FAULTS",
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcInvalidValidation(context.Background(), 1, "SAS123", false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, pdcInvalidValidationIssueType, persisted.IssueType)
	assert.Equal(t, "", persisted.OwningPosition)
	assert.True(t, persisted.Active)
}

func TestReevaluatePdcInvalidValidation_MissingSquawkUsesExistingDclFlow(t *testing.T) {
	t.Parallel()

	sid := "VEMBO2E"
	runway := "22R"
	var persisted *models.ValidationStatus
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS482",
				Bay:      shared.BAY_NOT_CLEARED,
				Sid:      &sid,
				Runway:   &runway,
				PdcState: "REQUESTED_WITH_FAULTS",
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcInvalidValidation(context.Background(), 1, "SAS482", false, false))

	require.NotNil(t, persisted)
	assert.Contains(t, persisted.Message, "No valid assigned squawk")
	require.NotNil(t, persisted.CustomAction)
	assert.Equal(t, pdcInvalidValidationActionKind, persisted.CustomAction.ActionKind)
	assert.Equal(t, pdcInvalidValidationActionLabel, persisted.CustomAction.Label)
}

func TestSetPdcAutoIssueFailureValidation_UsesExistingDclFlow(t *testing.T) {
	t.Parallel()

	var persisted *models.ValidationStatus
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS483", Bay: shared.BAY_NOT_CLEARED}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.SetPdcAutoIssueFailureValidation(context.Background(), 1, "SAS483", false))

	require.NotNil(t, persisted)
	assert.Equal(t, pdcInvalidValidationIssueType, persisted.IssueType)
	assert.Contains(t, persisted.Message, "automatic issuance failed")
	require.NotNil(t, persisted.CustomAction)
	assert.Equal(t, pdcInvalidValidationActionKind, persisted.CustomAction.ActionKind)
}

func TestReevaluatePdcInvalidValidation_ActivatesInNotClearedBay(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	runway := "22L"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS123",
				Owner:    &owner,
				Bay:      shared.BAY_NOT_CLEARED,
				Runway:   &runway,
				PdcState: "REQUESTED_WITH_FAULTS",
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcInvalidValidation(context.Background(), 1, "SAS123", false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, pdcInvalidValidationIssueType, persisted.IssueType)
	assert.Contains(t, persisted.Message, "Runway 22L is not an active departure runway")
}

func TestReevaluatePdcInvalidValidation_DoesNotActivateInClearedBay(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	runway := "22L"
	setCalled := false

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS123",
				Owner:    &owner,
				Bay:      shared.BAY_CLEARED,
				Runway:   &runway,
				PdcState: "REQUESTED_WITH_FAULTS",
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, _ *models.ValidationStatus) error {
			setCalled = true
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcInvalidValidation(context.Background(), 1, "SAS123", false, false))
	assert.False(t, setCalled)
}

func TestReevaluatePdcInvalidValidation_ClearsWhenFaultsNoLongerExist(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	runway := "22R"
	sid := "VEMBO2E"
	assignedSquawk := "2401"
	cleared := false

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:       "SAS123",
				Owner:          &owner,
				Bay:            shared.BAY_NOT_CLEARED,
				Runway:         &runway,
				Sid:            &sid,
				AssignedSquawk: &assignedSquawk,
				PdcState:       "REQUESTED_WITH_FAULTS",
				ValidationStatus: &models.ValidationStatus{
					IssueType:      pdcInvalidValidationIssueType,
					Message:        "old",
					OwningPosition: owner,
					Active:         true,
					ActivationKey:  "old-key",
					CustomAction:   pdcInvalidValidationAction(),
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
	require.NoError(t, svc.ReevaluatePdcInvalidValidation(context.Background(), 1, "SAS123", false, false))
	assert.True(t, cleared)
}

func TestReevaluatePdcInvalidValidationForStrip_RestoresLowerPriorityValidation(t *testing.T) {
	t.Parallel()

	owner := "121.630"
	runway := "22R"
	sid := "VEMBO2E"
	assigned := "4231"
	observed := "5231"
	var persisted *models.ValidationStatus
	strip := &models.Strip{
		Callsign:       "SAS124",
		Owner:          &owner,
		Bay:            shared.BAY_NOT_CLEARED,
		Runway:         &runway,
		Sid:            &sid,
		AssignedSquawk: &assigned,
		Squawk:         &observed,
		PdcState:       "REQUESTED_WITH_FAULTS",
		ValidationStatus: &models.ValidationStatus{
			IssueType:      pdcInvalidValidationIssueType,
			Message:        "old invalid",
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "old-key",
			CustomAction:   pdcInvalidValidationAction(),
		},
	}
	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{strip}, nil
		},
		ClearValidationStatusFn: func(_ context.Context, _ int32, _ string) error {
			return nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcInvalidValidationForStrip(context.Background(), 1, strip, []string{"22R"}, false, false))
	require.NotNil(t, persisted)
	assert.Equal(t, wrongSquawkValidationIssueType, persisted.IssueType)
	assert.Equal(t, persisted, strip.ValidationStatus)
}

func TestReevaluatePdcRequestValidationsForStrip_ClearsInvalidAfterLeavingStartupBay(t *testing.T) {
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
		Bay:      shared.BAY_PUSH,
		PdcState: "REQUESTED_WITH_FAULTS",
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
	assert.Nil(t, strip.ValidationStatus)
}

func TestReevaluatePdcInvalidValidation_RemainsActiveForNonDeliveryOwner(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	runway := "22L"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS123",
				Owner:    &owner,
				Bay:      shared.BAY_NOT_CLEARED,
				Runway:   &runway,
				PdcState: "REQUESTED_WITH_FAULTS",
				ValidationStatus: &models.ValidationStatus{
					IssueType:      pdcInvalidValidationIssueType,
					Message:        "old",
					OwningPosition: "EKCH_DEL",
					Active:         true,
					ActivationKey:  "old-key",
					CustomAction:   pdcInvalidValidationAction(),
				},
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcInvalidValidation(context.Background(), 1, "SAS123", false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, owner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
}

func TestReevaluatePdcInvalidValidation_ReactivatesOnOwnerChange(t *testing.T) {
	t.Parallel()

	oldOwner := "119.905"
	newOwner := "EKCH_DEL"
	runway := "22L"
	currentMessage := pdcInvalidValidationMessage([]pdc.FlightPlanValidationFault{{
		Kind:    pdc.FlightPlanValidationFaultKindRunway,
		Message: "Runway 22L is not an active departure runway",
	}})
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS123",
				Owner:    &newOwner,
				Bay:      shared.BAY_NOT_CLEARED,
				Runway:   &runway,
				PdcState: "REQUESTED_WITH_FAULTS",
				ValidationStatus: &models.ValidationStatus{
					IssueType:      pdcInvalidValidationIssueType,
					Message:        currentMessage,
					OwningPosition: oldOwner,
					Active:         false,
					ActivationKey:  "old-key",
					CustomAction:   pdcInvalidValidationAction(),
				},
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcInvalidValidation(context.Background(), 1, "SAS123", false, true))

	require.NotNil(t, persisted)
	assert.Equal(t, newOwner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
	assert.NotEqual(t, "old-key", persisted.ActivationKey)
}

func TestReevaluatePdcInvalidValidation_DoesNotActivateForEobtOnly(t *testing.T) {
	t.Parallel()

	eobt := "2359"
	sid := "VEMBO2E"
	runway := "22R"
	assignedSquawk := "2401"
	setCalled := false

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:       "SAS123",
				Bay:            shared.BAY_NOT_CLEARED,
				PdcState:       "REQUESTED_WITH_FAULTS",
				Sid:            &sid,
				Runway:         &runway,
				AssignedSquawk: &assignedSquawk,
				CdmData:        (&models.CdmData{Eobt: &eobt}).Normalize(),
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			setCalled = true
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcInvalidValidation(context.Background(), 1, "SAS123", false, false))

	assert.False(t, setCalled)
}
