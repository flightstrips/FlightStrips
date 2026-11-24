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

	svc := newWorkflowTestService(repo)
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

func TestAcceptCoordinationValidatesCommittedOwnerFromRequestCaches(t *testing.T) {
	for _, cache := range []string{"sync", "message", "publication", "combined"} {
		t.Run(cache, func(t *testing.T) {
			oldOwner, newOwner, squawk, sector := "EKCH_A_GND", "EKCH_B_GND", "4231", "OLD"
			original := &models.Strip{ID: 70, Session: 1, Callsign: "SAS123", Owner: &oldOwner, Version: 2,
				Bay: shared.BAY_TAXI, AssignedSquawk: &squawk, NextControllerSector: &sector,
				NextDisplay: &models.NextDisplay{Label: "OLD", Frequency: "121.730"}}
			peer := &models.Strip{Callsign: "DLH456", Squawk: &squawk}
			persisted := *original
			var validation *models.ValidationStatus
			repo := &testutil.MockStripRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { copy := persisted; return &copy, nil },
				ListFn: func(context.Context, int32) ([]*models.Strip, error) {
					copy := persisted
					return []*models.Strip{&copy, peer}, nil
				},
				CommitStripAssumptionFn: func(_ context.Context, _ int32, _ *models.Strip, _ *models.Coordination, owner string, _, _ []string) (bool, error) {
					persisted.Owner = &owner
					persisted.Version++
					return true, nil
				},
				SetValidationStatusFn: func(_ context.Context, _ int32, callsign string, status *models.ValidationStatus) error {
					require.Equal(t, original.Callsign, callsign)
					validation = status
					return nil
				},
			}
			syncState := &shared.SyncState{ExistingStrips: map[string]*models.Strip{original.Callsign: original, peer.Callsign: peer}}
			messageState := &shared.WebsocketMessageState{ExistingStrips: map[string]*models.Strip{original.Callsign: original, peer.Callsign: peer}, StripList: []*models.Strip{original, peer}}
			publication := &models.StripPublicationSnapshot{Strip: original, Session: &models.Session{ID: 1}}
			ctx := t.Context()
			if cache == "sync" || cache == "combined" {
				ctx = shared.WithSyncState(ctx, syncState)
			}
			if cache == "message" || cache == "combined" {
				ctx = shared.WithWebsocketMessageState(ctx, messageState)
			}
			if cache == "publication" || cache == "combined" {
				ctx = shared.WithStripPublication(ctx, publication)
			}
			svc := newWorkflowTestService(repo)
			svc.SetFrontendHub(&testutil.MockFrontendHub{})
			svc.SetCoordinationRepo(&testutil.MockCoordinationRepository{GetByStripIDFn: func(context.Context, int32, int32) (*models.Coordination, error) {
				return &models.Coordination{ID: 7, Session: 1, StripID: original.ID, FromPosition: oldOwner, ToPosition: newOwner}, nil
			}})
			require.NoError(t, svc.AcceptCoordination(ctx, 1, original.Callsign, newOwner))
			require.NotNil(t, validation)
			require.Equal(t, newOwner, validation.OwningPosition)
			if cache == "sync" || cache == "combined" {
				require.Equal(t, newOwner, *syncState.ExistingStrips[original.Callsign].Owner)
				require.Nil(t, syncState.ExistingStrips[original.Callsign].NextControllerSector)
				require.Nil(t, syncState.ExistingStrips[original.Callsign].NextDisplay)
				require.Equal(t, original.Version+1, syncState.ExistingStrips[original.Callsign].Version)
			}
			if cache == "message" || cache == "combined" {
				cached := messageState.ExistingStrips[original.Callsign]
				require.Equal(t, newOwner, *cached.Owner)
				for _, entry := range messageState.StripList {
					if entry.Callsign == original.Callsign {
						require.Same(t, cached, entry)
					}
				}
			}
			if cache == "publication" || cache == "combined" {
				require.Equal(t, newOwner, *publication.Strip.Owner)
			}
			// A caller may still hold this original for a before/after sync diff.
			require.Equal(t, oldOwner, *original.Owner)
			require.Equal(t, &sector, original.NextControllerSector)
			changed := *original
			changed.Owner = &newOwner
			require.True(t, syncStripChanged(original, &changed))
		})
	}
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

	svc := newWorkflowTestService(repo)
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

	svc := newWorkflowTestService(repo)
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

	svc := newWorkflowTestService(repo)
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

	svc := newWorkflowTestService(repo)
	require.NoError(t, svc.reevaluateDuplicateSquawkValidation(context.Background(), 1, "SAS123", false, false))
	assert.True(t, cleared)
}

func TestReevaluateDepartureValidation_RestoresPdcAfterDuplicateClears(t *testing.T) {
	t.Parallel()

	owner := "EKCH_A_GND"
	assigned := "4231"
	runway := "22L"
	strip := &models.Strip{
		Callsign:       "SAS124",
		Owner:          &owner,
		Bay:            shared.BAY_NOT_CLEARED,
		AssignedSquawk: &assigned,
		Runway:         &runway,
		PdcState:       "REQUESTED_WITH_FAULTS",
		ValidationStatus: &models.ValidationStatus{
			IssueType:      duplicateSquawkValidationIssueType,
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  "duplicate-key",
			CustomAction:   duplicateSquawkValidationAction(),
		},
	}
	var persisted *models.ValidationStatus
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
	require.NoError(t, svc.reevaluateDepartureValidation(context.Background(), 1, strip.Callsign, false, false))
	require.NotNil(t, persisted)
	assert.Equal(t, pdcInvalidValidationIssueType, persisted.IssueType)
	assert.Equal(t, persisted, strip.ValidationStatus)
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

	svc := newWorkflowTestService(repo)
	require.NoError(t, svc.reevaluateDuplicateSquawkValidationsForSession(context.Background(), 1, false))
	assert.Equal(t, map[string]bool{"SAS123": true, "DLH456": true}, changed)
}
