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

type precedenceValidationFixture struct {
	svc       *StripService
	strip     *models.Strip
	duplicate *models.Strip
	persisted *models.ValidationStatus
}

func newDuplicateNoStandPrecedenceFixture() *precedenceValidationFixture {
	owner := "EKCH_A_GND"
	assignedSquawk := "4231"
	strip := &models.Strip{
		Callsign:       "SAS123",
		Owner:          &owner,
		Bay:            shared.BAY_CLEARED,
		AssignedSquawk: &assignedSquawk,
		Version:        7,
	}
	duplicate := &models.Strip{
		Callsign: "DLH456",
		Squawk:   &assignedSquawk,
	}

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{strip, duplicate}, nil
		},
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			if callsign != strip.Callsign {
				return nil, nil
			}
			return strip, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, callsign string, status *models.ValidationStatus) error {
			if callsign == strip.Callsign {
				clone := *status
				strip.ValidationStatus = &clone
			}
			return nil
		},
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			if callsign == strip.Callsign {
				strip.ValidationStatus = nil
			}
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, callsign string, owner *string, version int32) (int64, error) {
			if callsign == strip.Callsign {
				strip.Owner = owner
				strip.Version = version
			}
			return 1, nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, callsign string, bay string, sequence int32) (int64, error) {
			if callsign == strip.Callsign {
				strip.Bay = bay
				strip.Sequence = &sequence
			}
			return 1, nil
		},
	}

	return &precedenceValidationFixture{
		svc:       NewStripService(repo),
		strip:     strip,
		duplicate: duplicate,
	}
}

func TestApplyNoStandValidation_ActivatesForRelevantOwnerAndBay(t *testing.T) {
	owner := "EKCH_C_TWR"
	var persisted *models.ValidationStatus

	svc := NewStripService(&testutil.MockStripRepository{
		SetValidationStatusFn: func(_ context.Context, session int32, callsign string, status *models.ValidationStatus) error {
			assert.Equal(t, int32(1), session)
			assert.Equal(t, "SAS123", callsign)
			persisted = status
			return nil
		},
	})

	require.NoError(t, svc.applyNoStandValidation(context.Background(), 1, &models.Strip{
		Callsign: "SAS123",
		Owner:    &owner,
		Bay:      shared.BAY_TWY_ARR,
	}, false, false))

	require.NotNil(t, persisted)
	assert.Equal(t, noStandValidationIssueType, persisted.IssueType)
	assert.Equal(t, noStandValidationMessage, persisted.Message)
	assert.Equal(t, owner, persisted.OwningPosition)
	assert.True(t, persisted.Active)
	require.NotNil(t, persisted.CustomAction)
	assert.Equal(t, noStandValidationActionKind, persisted.CustomAction.ActionKind)
	assert.Equal(t, noStandValidationActionLabel, persisted.CustomAction.Label)
	assert.NotEmpty(t, persisted.ActivationKey)
}

func TestApplyNoStandValidation_ClearsWhenStandAssigned(t *testing.T) {
	owner := "EKCH_A_GND"
	stand := "A12"
	cleared := false

	svc := NewStripService(&testutil.MockStripRepository{
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			assert.Equal(t, "SAS123", callsign)
			cleared = true
			return nil
		},
	})

	require.NoError(t, svc.applyNoStandValidation(context.Background(), 1, &models.Strip{
		Callsign: "SAS123",
		Owner:    &owner,
		Bay:      shared.BAY_TAXI,
		Stand:    &stand,
		ValidationStatus: &models.ValidationStatus{
			IssueType:      noStandValidationIssueType,
			Message:        noStandValidationMessage,
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "old-key",
			CustomAction:   noStandValidationAction(),
		},
	}, false, false))

	assert.True(t, cleared)
}

func TestApplyNoStandValidation_ClearsForNonApplicableOwner(t *testing.T) {
	owner := "EKCH_DEL"
	cleared := false

	svc := NewStripService(&testutil.MockStripRepository{
		ClearValidationStatusFn: func(_ context.Context, _ int32, _ string) error {
			cleared = true
			return nil
		},
	})

	require.NoError(t, svc.applyNoStandValidation(context.Background(), 1, &models.Strip{
		Callsign: "SAS123",
		Owner:    &owner,
		Bay:      shared.BAY_CLEARED,
		ValidationStatus: &models.ValidationStatus{
			IssueType:      noStandValidationIssueType,
			Message:        noStandValidationMessage,
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "old-key",
			CustomAction:   noStandValidationAction(),
		},
	}, false, false))

	assert.True(t, cleared)
}

func TestReevaluateNoStandValidation_ReactivatesOnOwnerChange(t *testing.T) {
	oldOwner := "EKCH_A_GND"
	newOwner := "EKCH_C_TWR"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS123",
				Owner:    &newOwner,
				Bay:      shared.BAY_TWY_ARR,
				ValidationStatus: &models.ValidationStatus{
					IssueType:      noStandValidationIssueType,
					Message:        noStandValidationMessage,
					OwningPosition: oldOwner,
					Active:         false,
					ActivationKey:  "old-key",
					CustomAction:   noStandValidationAction(),
				},
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.ReevaluateNoStandValidation(context.Background(), 1, "SAS123", false, true))

	require.NotNil(t, persisted)
	assert.True(t, persisted.Active)
	assert.Equal(t, newOwner, persisted.OwningPosition)
	assert.NotEqual(t, "old-key", persisted.ActivationKey)
}

func TestUpdateStand_ReevaluatesNoStandValidation(t *testing.T) {
	owner := "EKCH_A_GND"
	updatedStand := "A12"
	cleared := false

	repo := &testutil.MockStripRepository{
		UpdateStandFn: func(_ context.Context, _ int32, callsign string, stand *string, _ *int32) (int64, error) {
			assert.Equal(t, "SAS123", callsign)
			require.NotNil(t, stand)
			assert.Equal(t, updatedStand, *stand)
			return 1, nil
		},
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS123",
				Owner:    &owner,
				Bay:      shared.BAY_TAXI,
				Stand:    &updatedStand,
				ValidationStatus: &models.ValidationStatus{
					IssueType:      noStandValidationIssueType,
					Message:        noStandValidationMessage,
					OwningPosition: owner,
					Active:         true,
					ActivationKey:  "old-key",
					CustomAction:   noStandValidationAction(),
				},
			}, nil
		},
		ClearValidationStatusFn: func(_ context.Context, _ int32, _ string) error {
			cleared = true
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{})
	svc := NewStripService(repo)
	svc.SetFrontendHub(hub)

	require.NoError(t, svc.UpdateStand(context.Background(), 1, "SAS123", updatedStand))
	assert.True(t, cleared)
}

func TestMoveToBay_ReevaluatesNoStandValidationOnActivationEdge(t *testing.T) {
	owner := "EKCH_C_TWR"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, bay string) (int32, error) {
			assert.Equal(t, shared.BAY_TWY_ARR, bay)
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, callsign string, bay string, sequence int32) (int64, error) {
			assert.Equal(t, "SAS123", callsign)
			assert.Equal(t, shared.BAY_TWY_ARR, bay)
			assert.Equal(t, int32(1000), sequence)
			return 1, nil
		},
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{{Callsign: "SAS123", Owner: &owner, Bay: shared.BAY_TWY_ARR}}, nil
		},
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS123", Owner: &owner, Bay: shared.BAY_TWY_ARR}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.MoveToBay(context.Background(), 1, "SAS123", shared.BAY_TWY_ARR, false))

	require.NotNil(t, persisted)
	assert.Equal(t, noStandValidationIssueType, persisted.IssueType)
}

func TestSetOwnerAndReevaluateDuplicateSquawkValidation_ActivatesNoStandValidation(t *testing.T) {
	newOwner := "EKCH_A_GND"
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		SetOwnerFn: func(_ context.Context, _ int32, callsign string, owner *string, version int32) (int64, error) {
			assert.Equal(t, "SAS123", callsign)
			require.NotNil(t, owner)
			assert.Equal(t, newOwner, *owner)
			assert.Equal(t, int32(7), version)
			return 1, nil
		},
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS123", Owner: &newOwner, Bay: shared.BAY_CLEARED}, nil
		},
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{{Callsign: "SAS123", Owner: &newOwner, Bay: shared.BAY_CLEARED}}, nil
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
	assert.Equal(t, noStandValidationIssueType, persisted.IssueType)
	assert.Equal(t, newOwner, persisted.OwningPosition)
}

func TestMoveToBay_DuplicateSquawkOverridesNoStandWithDeterministicPrecedence(t *testing.T) {
	fixture := newDuplicateNoStandPrecedenceFixture()
	require.NoError(t, fixture.svc.MoveToBay(context.Background(), 1, fixture.strip.Callsign, shared.BAY_TAXI, false))

	require.NotNil(t, fixture.strip.ValidationStatus)
	assert.Equal(t, duplicateSquawkValidationIssueType, fixture.strip.ValidationStatus.IssueType)
	assert.Equal(t, duplicateSquawkValidationMessage, fixture.strip.ValidationStatus.Message)
}

func TestSetOwnerAndReevaluateDuplicateSquawkValidation_DuplicateSquawkOverridesExistingNoStand(t *testing.T) {
	fixture := newDuplicateNoStandPrecedenceFixture()
	fixture.strip.ValidationStatus = &models.ValidationStatus{
		IssueType:      noStandValidationIssueType,
		Message:        noStandValidationMessage,
		OwningPosition: *fixture.strip.Owner,
		Active:         true,
		ActivationKey:  "no-stand-key",
		CustomAction:   noStandValidationAction(),
	}

	newOwner := "EKCH_A_GND"
	count, err := fixture.svc.setOwnerAndReevaluateDuplicateSquawkValidation(context.Background(), 1, fixture.strip.Callsign, &newOwner, fixture.strip.Version)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	require.NotNil(t, fixture.strip.ValidationStatus)
	assert.Equal(t, duplicateSquawkValidationIssueType, fixture.strip.ValidationStatus.IssueType)
	assert.Equal(t, duplicateSquawkValidationMessage, fixture.strip.ValidationStatus.Message)
	assert.Equal(t, newOwner, fixture.strip.ValidationStatus.OwningPosition)
}

func TestReevaluateSquawkValidationsForSession_FallsBackToNoStandAfterDuplicateClears(t *testing.T) {
	owner := "EKCH_A_GND"
	assignedSquawk := "4231"
	strip := &models.Strip{
		Callsign:       "SAS123",
		Owner:          &owner,
		Bay:            shared.BAY_CLEARED,
		AssignedSquawk: &assignedSquawk,
		ValidationStatus: &models.ValidationStatus{
			IssueType:      duplicateSquawkValidationIssueType,
			Message:        duplicateSquawkValidationMessage,
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "dup-key",
			CustomAction:   duplicateSquawkValidationAction(),
		},
	}

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{strip}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, callsign string, status *models.ValidationStatus) error {
			if callsign == strip.Callsign {
				clone := *status
				strip.ValidationStatus = &clone
			}
			return nil
		},
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			if callsign == strip.Callsign {
				strip.ValidationStatus = nil
			}
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.reevaluateSquawkValidationsForSession(context.Background(), 1, false))

	require.NotNil(t, strip.ValidationStatus)
	assert.Equal(t, noStandValidationIssueType, strip.ValidationStatus.IssueType)
	assert.Equal(t, noStandValidationMessage, strip.ValidationStatus.Message)
	assert.Equal(t, owner, strip.ValidationStatus.OwningPosition)
}

func TestReevaluateCtotValidationsForSession_FallsBackToNoStandAfterCtotClears(t *testing.T) {
	now := ctotValidationNow
	ctotValidationNow = func() time.Time {
		return time.Date(2024, time.January, 1, 10, 1, 0, 0, time.UTC)
	}
	t.Cleanup(func() {
		ctotValidationNow = now
	})

	owner := "EKCH_A_TWR"
	ctot := "1010"
	strip := &models.Strip{
		Callsign: "SAS123",
		Owner:    &owner,
		Bay:      shared.BAY_TAXI_LWR,
		CdmData:  &models.CdmData{Ctot: &ctot},
		ValidationStatus: &models.ValidationStatus{
			IssueType:      ctotValidationIssueType,
			Message:        ctotValidationMessage,
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "ctot-key",
			CustomAction:   ctotValidationAction(),
		},
	}

	repo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return []*models.Strip{strip}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, callsign string, status *models.ValidationStatus) error {
			if callsign == strip.Callsign {
				clone := *status
				strip.ValidationStatus = &clone
			}
			return nil
		},
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			if callsign == strip.Callsign {
				strip.ValidationStatus = nil
			}
			return nil
		},
	}

	svc := NewStripService(repo)
	require.NoError(t, svc.ReevaluateCtotValidationsForSession(context.Background(), 1, false))

	require.NotNil(t, strip.ValidationStatus)
	assert.Equal(t, noStandValidationIssueType, strip.ValidationStatus.IssueType)
	assert.Equal(t, noStandValidationMessage, strip.ValidationStatus.Message)
	assert.Equal(t, owner, strip.ValidationStatus.OwningPosition)
}
