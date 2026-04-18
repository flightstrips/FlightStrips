package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/testutil"
	pkgModels "FlightStrips/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPdcInvalidValidationFixture(stripRepo *testutil.MockStripRepository, departureRunways ...string) (*StripService, *testutil.MockFrontendHub) {
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{
		SessionRepoVal: &testutil.MockSessionRepository{
			GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
				return &models.Session{
					ID: id,
					ActiveRunways: pkgModels.ActiveRunways{
						DepartureRunways: departureRunways,
					},
				}, nil
			},
		},
	})

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
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
	assert.Contains(t, persisted.Message, "Open DCL menu to correct the SID or runway.")
	assert.NotEmpty(t, persisted.ActivationKey)
}

func TestReevaluatePdcInvalidValidation_ClearsWhenFaultsNoLongerExist(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	runway := "22R"
	cleared := false

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS123",
				Owner:    &owner,
				Runway:   &runway,
				PdcState: "REQUESTED_WITH_FAULTS",
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

func TestReevaluatePdcInvalidValidation_ClearsWhenOwnerLeavesDelivery(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	runway := "22L"
	cleared := false

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS123",
				Owner:    &owner,
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
		ClearValidationStatusFn: func(_ context.Context, _ int32, _ string) error {
			cleared = true
			return nil
		},
	}

	svc, _ := newPdcInvalidValidationFixture(repo, "22R")
	require.NoError(t, svc.ReevaluatePdcInvalidValidation(context.Background(), 1, "SAS123", false, false))
	assert.True(t, cleared)
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
