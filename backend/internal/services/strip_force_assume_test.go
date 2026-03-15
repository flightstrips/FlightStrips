package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestForceAssumeStrip_AlreadyOwned verifies that ForceAssumeStrip rejects a strip that already has an owner.
func TestForceAssumeStrip_AlreadyOwned(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:       1,
		Callsign: "SAS123",
		Owner:    &owner,
		Version:  1,
	}

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	})
	svc.SetFrontendHub(&testutil.MockFrontendHub{})

	err := svc.ForceAssumeStrip(context.Background(), 1, "SAS123", "EKCH_D_TWR")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has an owner")
}

// TestForceAssumeStrip_UnownedNoCoordination verifies that an unowned strip with no coordination is assumed successfully.
func TestForceAssumeStrip_UnownedNoCoordination(t *testing.T) {
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          nil,
		NextOwners:     []string{},
		PreviousOwners: []string{},
		Version:        2,
	}

	setNextAndPreviousOwnersCalled := false
	setOwnerCalled := false
	hub := &testutil.MockFrontendHub{}

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, nextOwners []string, _ []string) error {
			setNextAndPreviousOwnersCalled = true
			assert.Empty(t, nextOwners)
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, owner *string, version int32) (int64, error) {
			setOwnerCalled = true
			require.NotNil(t, owner)
			assert.Equal(t, "EKCH_A_TWR", *owner)
			assert.Equal(t, int32(2), version)
			return 1, nil
		},
	})
	svc.SetFrontendHub(hub)
	// No coordRepo set — ForceAssumeStrip should handle nil coordRepo gracefully.

	err := svc.ForceAssumeStrip(context.Background(), 1, "SAS123", "EKCH_A_TWR")
	require.NoError(t, err)
	assert.True(t, setNextAndPreviousOwnersCalled)
	assert.True(t, setOwnerCalled)
	require.Len(t, hub.CoordinationAssumes, 1)
	assert.Equal(t, "SAS123", hub.CoordinationAssumes[0].Callsign)
	assert.Equal(t, "EKCH_A_TWR", hub.CoordinationAssumes[0].Position)
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, "EKCH_A_TWR", hub.OwnersUpdates[0].Owner)
}

// TestForceAssumeStrip_UnownedWithStaleCoordination verifies that a stale coordination is deleted before assuming.
func TestForceAssumeStrip_UnownedWithStaleCoordination(t *testing.T) {
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS456",
		Owner:          nil,
		NextOwners:     []string{"EKCH_A_TWR"},
		PreviousOwners: []string{},
		Version:        3,
	}

	staleCoord := &models.Coordination{ID: 99, ToPosition: "SOME_OTHER"}
	deletedCoordID := int32(0)
	hub := &testutil.MockFrontendHub{}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			return staleCoord, nil
		},
		DeleteFn: func(_ context.Context, id int32) error {
			deletedCoordID = id
			return nil
		},
	}

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _ []string, _ []string) error {
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, _ *string, _ int32) (int64, error) {
			return 1, nil
		},
	})
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)

	err := svc.ForceAssumeStrip(context.Background(), 1, "SAS456", "EKCH_A_TWR")
	require.NoError(t, err)
	assert.Equal(t, int32(99), deletedCoordID, "stale coordination should have been deleted")
}

// TestForceAssumeStrip_NotFound verifies that an error from the strip repository is propagated.
func TestForceAssumeStrip_NotFound(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
	})
	svc.SetFrontendHub(&testutil.MockFrontendHub{})

	err := svc.ForceAssumeStrip(context.Background(), 1, "MISSING", "EKCH_A_TWR")
	require.Error(t, err)
	assert.ErrorIs(t, err, pgx.ErrNoRows)
}

// TestForceAssumeStrip_SetOwnerFails verifies that a DB failure from SetOwner is propagated.
func TestForceAssumeStrip_SetOwnerFails(t *testing.T) {
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS789",
		Owner:          nil,
		NextOwners:     []string{},
		PreviousOwners: []string{},
		Version:        1,
	}

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _ []string, _ []string) error {
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, _ *string, _ int32) (int64, error) {
			return 0, errors.New("db error")
		},
	})
	svc.SetFrontendHub(&testutil.MockFrontendHub{})

	err := svc.ForceAssumeStrip(context.Background(), 1, "SAS789", "EKCH_A_TWR")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}
