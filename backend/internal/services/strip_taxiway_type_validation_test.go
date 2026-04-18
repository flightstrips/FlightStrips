package services

import (
	"context"
	"testing"

	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTaxiwayTypeValidationTestConfig(t *testing.T) {
	t.Helper()

	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_DEL", Section: "DEL"},
		{Name: "EKCH_A_GND", Section: "GND"},
		{Name: "EKCH_A_TWR", Section: "TWR"},
		{Name: "EKCH_C_TWR", Section: "TWR"},
		{Name: "TEST_GW_GND", Section: "GND"},
	}))

	t.Cleanup(config.SetLayoutsForTest(map[string][]config.LayoutVariant{
		"EKCH_A_GND":  {{Layout: "AA"}},
		"EKCH_A_TWR":  {{Layout: "TWTE"}},
		"EKCH_C_TWR":  {{Layout: "GEGW"}},
		"TEST_GW_GND": {{Layout: "GEGW"}},
	}))

	t.Cleanup(config.SetTaxiwayTypeValidationConfigForTest(config.TaxiwayTypeValidationConfig{
		Apron: config.TaxiwayTypeValidationScopeConfig{
			Categories: map[string][]string{
				"E": {"L/Y", "Y/L", "K/J", "K1"},
				"F": {"L/Y", "Y/L", "K/J", "K1", "C/22L", "G2/30", "A3", "A4", "A5", "B2", "C/30", "A1", "A2", "E1", "B4"},
			},
		},
		Tower: config.TaxiwayTypeValidationScopeConfig{
			Categories: map[string][]string{
				"D": {"C/22L"},
				"E": {"C/22L", "G2/30", "A3", "A4", "A5"},
				"F": {"B2", "C/30", "A1", "A2", "A3", "A4", "A5", "E1", "B4"},
			},
			AircraftTypes: map[string][]string{
				"A359": {"B2"},
				"B78X": {"B2"},
				"A35K": {"B2", "C/30"},
				"B773": {"B2", "C/30"},
				"B77W": {"B2", "C/30"},
				"A345": {"B2", "C/30"},
				"A346": {"B2", "C/30"},
				"B778": {"B2", "C/30", "A1", "A2", "A3", "A4", "A5", "E1", "B4"},
				"B779": {"B2", "C/30", "A1", "A2", "A3", "A4", "A5", "E1", "B4"},
			},
		},
	}))
}

func TestApplyTaxiwayTypeValidation_ActivatesForApronCategoryRestriction(t *testing.T) {
	setupTaxiwayTypeValidationTestConfig(t)

	owner := "EKCH_A_GND"
	category := "E"
	releasePoint := "K1"
	var persisted *models.ValidationStatus

	svc := NewStripService(&testutil.MockStripRepository{
		SetValidationStatusFn: func(_ context.Context, session int32, callsign string, status *models.ValidationStatus) error {
			assert.Equal(t, int32(1), session)
			assert.Equal(t, "SAS123", callsign)
			persisted = status
			return nil
		},
	})

	require.NoError(t, svc.applyTaxiwayTypeValidation(context.Background(), 1, &models.Strip{
		Callsign:         "SAS123",
		Owner:            &owner,
		AircraftCategory: &category,
		ReleasePoint:     &releasePoint,
		Bay:              shared.BAY_TAXI,
	}, false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, taxiwayTypeValidationIssueType, persisted.IssueType)
	assert.Equal(t, owner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
	require.NotNil(t, persisted.CustomAction)
	assert.Equal(t, taxiwayTypeValidationActionKind, persisted.CustomAction.ActionKind)
	assert.Equal(t, taxiwayTypeValidationActionLabel, persisted.CustomAction.Label)
	assert.Contains(t, persisted.Message, "holding point K1")
	assert.Contains(t, persisted.Message, "aircraft category E")
	assert.NotEmpty(t, persisted.ActivationKey)
}

func TestApplyTaxiwayTypeValidation_ActivatesForSpecificAircraftRestriction(t *testing.T) {
	setupTaxiwayTypeValidationTestConfig(t)

	owner := "EKCH_A_TWR"
	category := "E"
	aircraftType := "A359"
	releasePoint := "B2"
	var persisted *models.ValidationStatus

	svc := NewStripService(&testutil.MockStripRepository{
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	})

	require.NoError(t, svc.applyTaxiwayTypeValidation(context.Background(), 1, &models.Strip{
		Callsign:         "SAS123",
		Owner:            &owner,
		AircraftCategory: &category,
		AircraftType:     &aircraftType,
		ReleasePoint:     &releasePoint,
		Bay:              shared.BAY_TAXI_LWR,
	}, false, false))

	require.NotNil(t, persisted)
	assert.Contains(t, persisted.Message, "holding point B2")
	assert.Contains(t, persisted.Message, "aircraft type A359")
}

func TestApplyTaxiwayTypeValidation_UsesGegwScopeForGroundPositionMappedToTowerGroundLayout(t *testing.T) {
	setupTaxiwayTypeValidationTestConfig(t)

	owner := "TEST_GW_GND"
	category := "E"

	t.Run("uses shared gwge restriction matrix", func(t *testing.T) {
		releasePoint := "A3"
		var persisted *models.ValidationStatus

		svc := NewStripService(&testutil.MockStripRepository{
			SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
				persisted = status
				return nil
			},
		})

		require.NoError(t, svc.applyTaxiwayTypeValidation(context.Background(), 1, &models.Strip{
			Callsign:         "SAS123",
			Owner:            &owner,
			AircraftCategory: &category,
			ReleasePoint:     &releasePoint,
			Bay:              shared.BAY_TAXI_LWR,
		}, false, false))

		require.NotNil(t, persisted)
		assert.Contains(t, persisted.Message, "holding point A3")
		assert.Contains(t, persisted.Message, "aircraft category E")
	})

	t.Run("does not fall back to apron-only restrictions", func(t *testing.T) {
		releasePoint := "K1"
		setCalled := false

		svc := NewStripService(&testutil.MockStripRepository{
			SetValidationStatusFn: func(context.Context, int32, string, *models.ValidationStatus) error {
				setCalled = true
				return nil
			},
		})

		require.NoError(t, svc.applyTaxiwayTypeValidation(context.Background(), 1, &models.Strip{
			Callsign:         "SAS123",
			Owner:            &owner,
			AircraftCategory: &category,
			ReleasePoint:     &releasePoint,
			Bay:              shared.BAY_TAXI_LWR,
		}, false, false))

		assert.False(t, setCalled)
	})
}

func TestApplyTaxiwayTypeValidation_DoesNotActivateForCompatibleAssignment(t *testing.T) {
	setupTaxiwayTypeValidationTestConfig(t)

	owner := "EKCH_A_TWR"
	category := "D"
	releasePoint := "B2"
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

	require.NoError(t, svc.applyTaxiwayTypeValidation(context.Background(), 1, &models.Strip{
		Callsign:         "SAS123",
		Owner:            &owner,
		AircraftCategory: &category,
		ReleasePoint:     &releasePoint,
		Bay:              shared.BAY_TAXI_LWR,
	}, false, false))

	assert.False(t, setCalled)
	assert.False(t, clearCalled)
}

func TestUpdateReleasePoint_ReevaluatesTaxiwayTypeValidation(t *testing.T) {
	setupTaxiwayTypeValidationTestConfig(t)

	owner := "EKCH_A_TWR"
	category := "E"
	invalidReleasePoint := "A3"
	compatibleReleasePoint := "B1"
	strip := &models.Strip{
		Callsign:         "SAS123",
		Owner:            &owner,
		AircraftCategory: &category,
		ReleasePoint:     &invalidReleasePoint,
		Bay:              shared.BAY_TAXI_LWR,
		ValidationStatus: &models.ValidationStatus{
			IssueType:      taxiwayTypeValidationIssueType,
			Message:        "old",
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "old-key",
			CustomAction:   taxiwayTypeValidationAction(),
		},
	}
	cleared := false

	repo := &testutil.MockStripRepository{
		UpdateReleasePointFn: func(_ context.Context, _ int32, _ string, releasePoint *string) (int64, error) {
			strip.ReleasePoint = releasePoint
			return 1, nil
		},
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			assert.Equal(t, "SAS123", callsign)
			cleared = true
			return nil
		},
	}

	svc := NewStripService(repo)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})

	require.NoError(t, svc.UpdateReleasePoint(context.Background(), 1, "SAS123", compatibleReleasePoint))
	assert.True(t, cleared)
}

func TestSetOwnerAndReevaluateDuplicateSquawkValidation_ReactivatesTaxiwayTypeValidationOnOwnerChange(t *testing.T) {
	setupTaxiwayTypeValidationTestConfig(t)

	oldOwner := "EKCH_DEL"
	newOwner := "EKCH_A_TWR"
	category := "E"
	releasePoint := "A3"
	strip := &models.Strip{
		Callsign:         "SAS123",
		Owner:            &oldOwner,
		AircraftCategory: &category,
		ReleasePoint:     &releasePoint,
		Bay:              shared.BAY_TAXI_LWR,
		ValidationStatus: &models.ValidationStatus{
			IssueType:      taxiwayTypeValidationIssueType,
			Message:        "Assigned holding point A3 is incompatible with aircraft category E.",
			OwningPosition: oldOwner,
			Active:         false,
			ActivationKey:  "old-key",
			CustomAction:   taxiwayTypeValidationAction(),
		},
	}
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		SetOwnerFn: func(_ context.Context, _ int32, _ string, owner *string, _ int32) (int64, error) {
			strip.Owner = owner
			return 1, nil
		},
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{strip}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)

	count, err := svc.setOwnerAndReevaluateDuplicateSquawkValidation(context.Background(), 1, "SAS123", &newOwner, 7)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	require.NotNil(t, persisted)
	assert.Equal(t, newOwner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
	assert.NotEqual(t, "old-key", persisted.ActivationKey)
}

func TestMoveToBay_ReevaluatesTaxiwayTypeValidationOnActivationEdge(t *testing.T) {
	setupTaxiwayTypeValidationTestConfig(t)

	owner := "EKCH_A_TWR"
	category := "E"
	releasePoint := "A3"
	strip := &models.Strip{
		Callsign:         "SAS123",
		Owner:            &owner,
		AircraftCategory: &category,
		ReleasePoint:     &releasePoint,
		Bay:              shared.BAY_CLEARED,
	}
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			strip.Bay = bay
			return 1, nil
		},
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{strip}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.MoveToBay(context.Background(), 1, "SAS123", shared.BAY_TAXI_LWR, false))

	require.NotNil(t, persisted)
	assert.Equal(t, taxiwayTypeValidationIssueType, persisted.IssueType)
}

func TestSyncEuroscopeStrip_ReevaluatesTaxiwayTypeValidationOnAircraftCategoryChange(t *testing.T) {
	setupTaxiwayTypeValidationTestConfig(t)

	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS123"
	owner := "EKCH_A_TWR"
	initialCategory := "D"
	updatedCategory := "E"
	releasePoint := "A3"
	currentStrip := &models.Strip{
		Callsign:         callsign,
		Origin:           "EKCH",
		Owner:            &owner,
		AircraftCategory: &initialCategory,
		ReleasePoint:     &releasePoint,
		Bay:              shared.BAY_TAXI_LWR,
	}
	var persisted *models.ValidationStatus

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return currentStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			strip.ReleasePoint = currentStrip.ReleasePoint
			currentStrip = strip
			return 1, nil
		},
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{currentStrip}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, currentStrip, stripRepo)
	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:         callsign,
		Origin:           "EKCH",
		AircraftCategory: updatedCategory,
		GroundState:      euroscope.GroundStateTaxi,
	}, "EKCH")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	assert.Equal(t, taxiwayTypeValidationIssueType, persisted.IssueType)
	assert.Contains(t, persisted.Message, "aircraft category E")
}
