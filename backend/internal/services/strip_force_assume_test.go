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

// TestForceAssumeStrip_OwnedStrip verifies that ForceAssumeStrip succeeds even when the strip already has an owner.
func TestForceAssumeStrip_OwnedStrip(t *testing.T) {
	existingOwner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &existingOwner,
		NextOwners:     []string{},
		PreviousOwners: []string{},
		Version:        1,
	}

	setOwnerCalled := false
	hub := &testutil.MockFrontendHub{}

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _ []string, _ []string) error {
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, owner *string, _ int32) (int64, error) {
			setOwnerCalled = true
			require.NotNil(t, owner)
			assert.Equal(t, "EKCH_D_TWR", *owner)
			return 1, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.ForceAssumeStrip(context.Background(), 1, "SAS123", "EKCH_D_TWR")
	require.NoError(t, err)
	assert.True(t, setOwnerCalled)
	require.Len(t, hub.CoordinationAssumes, 1)
	assert.Equal(t, "EKCH_D_TWR", hub.CoordinationAssumes[0].Position)
}

// TestForceAssumeStrip_DisplacedOwnerRemovedFromPrevious verifies that the displaced owner is
// removed from previous controllers after a force assume.
func TestForceAssumeStrip_DisplacedOwnerRemovedFromPrevious(t *testing.T) {
	existingOwner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &existingOwner,
		NextOwners:     []string{},
		PreviousOwners: []string{"EKCH_APP", "EKCH_A_TWR"},
		Version:        1,
	}

	var savedPreviousOwners []string
	hub := &testutil.MockFrontendHub{}

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _ []string, prevOwners []string) error {
			savedPreviousOwners = prevOwners
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, _ *string, _ int32) (int64, error) {
			return 1, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.ForceAssumeStrip(context.Background(), 1, "SAS123", "EKCH_D_TWR")
	require.NoError(t, err)
	assert.Equal(t, []string{"EKCH_APP"}, savedPreviousOwners, "displaced owner must be removed from previous controllers")
}

// TestForceAssumeStrip_RouteRecalculated verifies that UpdateRouteForStrip is called on the server
// after a force assume so the route starts from the new owner.
func TestForceAssumeStrip_RouteRecalculated(t *testing.T) {
	existingOwner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &existingOwner,
		NextOwners:     []string{},
		PreviousOwners: []string{},
		Version:        1,
	}

	routeRecalculated := false
	mockServer := &testutil.MockServer{
		UpdateRouteForStripFn: func(callsign string, sessionId int32, sendUpdate bool) error {
			assert.Equal(t, "SAS123", callsign)
			assert.Equal(t, int32(1), sessionId)
			assert.False(t, sendUpdate, "route recalculation must not send its own update")
			routeRecalculated = true
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)

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

	err := svc.ForceAssumeStrip(context.Background(), 1, "SAS123", "EKCH_D_TWR")
	require.NoError(t, err)
	assert.True(t, routeRecalculated, "route must be recalculated after force assume")
}

// TestForceAssumeStrip_NextOwnersFilteredFromPrevious verifies that any controller appearing in the
// recalculated next owners is removed from previous controllers.
func TestForceAssumeStrip_NextOwnersFilteredFromPrevious(t *testing.T) {
	existingOwner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &existingOwner,
		NextOwners:     []string{},
		PreviousOwners: []string{"EKCH_APP", "EKCH_CTR"},
		Version:        1,
	}

	// Simulate route recalculation putting EKCH_APP back in NextOwners.
	recalculatedStrip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &existingOwner,
		NextOwners:     []string{"EKCH_APP"},
		PreviousOwners: []string{"EKCH_APP", "EKCH_CTR"},
		Version:        1,
	}

	callCount := 0
	var savedPreviousOwners []string
	hub := &testutil.MockFrontendHub{}

	mockServer := &testutil.MockServer{
		UpdateRouteForStripFn: func(_ string, _ int32, _ bool) error { return nil },
	}
	hub.SetServer(mockServer)

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			callCount++
			if callCount == 1 {
				return strip, nil
			}
			return recalculatedStrip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _ []string, _ []string) error {
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, _ *string, _ int32) (int64, error) {
			return 1, nil
		},
		SetPreviousOwnersFn: func(_ context.Context, _ int32, _ string, prevOwners []string) error {
			savedPreviousOwners = prevOwners
			return nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.ForceAssumeStrip(context.Background(), 1, "SAS123", "EKCH_D_TWR")
	require.NoError(t, err)

	// EKCH_APP is now in NextOwners so it must be removed from PreviousOwners.
	// EKCH_CTR is not in NextOwners so it stays. EKCH_A_TWR (displaced owner) was already removed.
	assert.Equal(t, []string{"EKCH_CTR"}, savedPreviousOwners)

	// The owners update notification must reflect the recalculated next owners and cleaned previous owners.
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, []string{"EKCH_APP"}, hub.OwnersUpdates[0].NextOwners)
	assert.Equal(t, []string{"EKCH_CTR"}, hub.OwnersUpdates[0].PreviousOwners)
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

// TestForceAssumeStrip_AssumeInNextList_OwnerAppendedToPrevious verifies that when the assuming
// controller was already in NextOwners, the displaced owner is appended to PreviousOwners (expected
// handoff path). The existing previous owners must be preserved.
func TestForceAssumeStrip_AssumeInNextList_OwnerAppendedToPrevious(t *testing.T) {
	existingOwner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &existingOwner,
		NextOwners:     []string{"EKCH_D_TWR", "EKCH_APP"},
		PreviousOwners: []string{"EKCH_DEL"},
		Version:        1,
	}

	var savedPreviousOwners []string
	hub := &testutil.MockFrontendHub{}

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _ []string, prevOwners []string) error {
			savedPreviousOwners = prevOwners
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, _ *string, _ int32) (int64, error) {
			return 1, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.ForceAssumeStrip(context.Background(), 1, "SAS123", "EKCH_D_TWR")
	require.NoError(t, err)
	// Existing previous owners preserved + displaced owner appended.
	assert.Equal(t, []string{"EKCH_DEL", "EKCH_A_TWR"}, savedPreviousOwners,
		"displaced owner must be appended when assuming controller was in next list")
}

// TestForceAssumeStrip_AssumeNotInNextList_OwnerNotAppendedToPrevious verifies that when the
// assuming controller was NOT in NextOwners (unexpected force-assume), the displaced owner is
// removed from previous controllers and not re-appended.
func TestForceAssumeStrip_AssumeNotInNextList_OwnerNotAppendedToPrevious(t *testing.T) {
	existingOwner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &existingOwner,
		NextOwners:     []string{"EKCH_APP"}, // EKCH_D_TWR not in list
		PreviousOwners: []string{"EKCH_DEL", "EKCH_A_TWR"},
		Version:        1,
	}

	var savedPreviousOwners []string
	hub := &testutil.MockFrontendHub{}

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _ []string, prevOwners []string) error {
			savedPreviousOwners = prevOwners
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, _ *string, _ int32) (int64, error) {
			return 1, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.ForceAssumeStrip(context.Background(), 1, "SAS123", "EKCH_D_TWR")
	require.NoError(t, err)
	// Displaced owner removed; not re-appended because this was an unexpected force-assume.
	assert.Equal(t, []string{"EKCH_DEL"}, savedPreviousOwners,
		"displaced owner must not be appended when force-assuming outside of next list")
}
