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

func TestApplyRunwayTypeValidation_ActivatesForGroundOwnerInDepartureFlow(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	aircraftType := "A388"
	runway := "22R"
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
	require.NoError(t, svc.applyRunwayTypeValidation(context.Background(), 1, &models.Strip{
		Callsign:     "SAS123",
		Owner:        &owner,
		Bay:          shared.BAY_TAXI,
		AircraftType: &aircraftType,
		Runway:       &runway,
	}, false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, runwayTypeValidationIssueType, persisted.IssueType)
	assert.Equal(t, owner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
	assert.Contains(t, persisted.Message, "Aircraft type A388 is not allowed on runway 22R")
	require.NotNil(t, persisted.CustomAction)
	assert.Equal(t, pdcInvalidValidationActionKind, persisted.CustomAction.ActionKind)
	assert.Equal(t, pdcInvalidValidationActionLabel, persisted.CustomAction.Label)
}

func TestApplyRunwayTypeValidation_ActivatesForTowerOwnerAtDepartureBay(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_TWR"
	aircraftType := "B748"
	runway := "04L"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.applyRunwayTypeValidation(context.Background(), 1, &models.Strip{
		Callsign:     "SAS123",
		Owner:        &owner,
		Bay:          shared.BAY_DEPART,
		AircraftType: &aircraftType,
		Runway:       &runway,
	}, false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, owner, persisted.OwningPosition)
}

func TestApplyRunwayTypeValidation_ClearsWhenRunwayBecomesCompatible(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	aircraftType := "A388"
	runway := "22L"
	cleared := false

	repo := &testutil.MockStripRepository{
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			assert.Equal(t, "SAS123", callsign)
			cleared = true
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.applyRunwayTypeValidation(context.Background(), 1, &models.Strip{
		Callsign:     "SAS123",
		Owner:        &owner,
		Bay:          shared.BAY_TAXI,
		AircraftType: &aircraftType,
		Runway:       &runway,
		ValidationStatus: &models.ValidationStatus{
			IssueType:      runwayTypeValidationIssueType,
			Message:        "old",
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "old-key",
			CustomAction:   pdcInvalidValidationAction(),
		},
	}, false, false))

	assert.True(t, cleared)
}

func TestApplyRunwayTypeValidation_ClearsForNonApplicableOwner(t *testing.T) {
	t.Parallel()

	owner := "EKCH_DEL"
	aircraftType := "AN225"
	runway := "30"
	cleared := false

	repo := &testutil.MockStripRepository{
		ClearValidationStatusFn: func(_ context.Context, _ int32, _ string) error {
			cleared = true
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.applyRunwayTypeValidation(context.Background(), 1, &models.Strip{
		Callsign:     "SAS123",
		Owner:        &owner,
		Bay:          shared.BAY_CLEARED,
		AircraftType: &aircraftType,
		Runway:       &runway,
		ValidationStatus: &models.ValidationStatus{
			IssueType:      runwayTypeValidationIssueType,
			Message:        "old",
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "old-key",
			CustomAction:   pdcInvalidValidationAction(),
		},
	}, false, false))

	assert.True(t, cleared)
}

func TestApplyRunwayTypeValidation_ReactivatesOnOwnerChange(t *testing.T) {
	t.Parallel()

	oldOwner := "EKCH_A_GND"
	newOwner := "EKCH_A_TWR"
	aircraftType := "A388"
	runway := "22R"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.applyRunwayTypeValidation(context.Background(), 1, &models.Strip{
		Callsign:     "SAS123",
		Owner:        &newOwner,
		Bay:          shared.BAY_DEPART,
		AircraftType: &aircraftType,
		Runway:       &runway,
		ValidationStatus: &models.ValidationStatus{
			IssueType:      runwayTypeValidationIssueType,
			Message:        "old",
			OwningPosition: oldOwner,
			Active:         false,
			ActivationKey:  "old-key",
			CustomAction:   pdcInvalidValidationAction(),
		},
	}, false, true))

	require.NotNil(t, persisted)
	assert.Equal(t, newOwner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
	assert.NotEqual(t, "old-key", persisted.ActivationKey)
}

func TestReevaluateDepartureValidation_TransitionsFromRunwayTypeToCtotAfterCorrection(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, time.January, 1, 10, 0, 0, 0, time.UTC)
	owner := "EKCH_A_TWR"
	aircraftType := "A388"
	runway := "22L"
	ctot := "1015"
	cleared := false
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		ListFn: func(context.Context, int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{
					Callsign:     "SAS123",
					Owner:        &owner,
					Bay:          shared.BAY_DEPART,
					AircraftType: &aircraftType,
					Runway:       &runway,
					CdmData:      &models.CdmData{Ctot: &ctot},
					ValidationStatus: &models.ValidationStatus{
						IssueType:      runwayTypeValidationIssueType,
						Message:        "old",
						OwningPosition: owner,
						Active:         true,
						ActivationKey:  "runway-key",
						CustomAction:   pdcInvalidValidationAction(),
					},
				},
			}, nil
		},
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:     "SAS123",
				Owner:        &owner,
				Bay:          shared.BAY_DEPART,
				AircraftType: &aircraftType,
				Runway:       &runway,
				CdmData:      &models.CdmData{Ctot: &ctot},
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

	require.NoError(t, svc.reevaluateDepartureValidation(context.Background(), 1, "SAS123", false, false))
	assert.True(t, cleared)
	require.NotNil(t, persisted)
	assert.Equal(t, ctotValidationIssueType, persisted.IssueType)
}
