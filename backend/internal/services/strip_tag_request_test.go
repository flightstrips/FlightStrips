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

// ── CreateTagRequest ──────────────────────────────────────────────────────────

// TestCreateTagRequest_Success verifies that a tag request coordination is created and the
// broadcast event is sent when all preconditions are met.
func TestCreateTagRequest_Success(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:       1,
		Callsign: "SAS123",
		Owner:    &owner,
	}

	var createdCoord *models.Coordination
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return nil, pgx.ErrNoRows // no existing coordination
		},
		CreateFn: func(_ context.Context, coord *models.Coordination) error {
			createdCoord = coord
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	mockServer := &testutil.MockServer{CoordRepoVal: coordRepo}
	hub.SetServer(mockServer)

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.CreateTagRequest(context.Background(), 1, "SAS123", "EKCH_D_TWR")
	require.NoError(t, err)

	require.NotNil(t, createdCoord)
	assert.Equal(t, "EKCH_A_TWR", createdCoord.FromPosition)
	assert.Equal(t, "EKCH_D_TWR", createdCoord.ToPosition)
	assert.True(t, createdCoord.IsTagRequest)

	require.Len(t, hub.CoordinationTagRequests, 1)
	assert.Equal(t, "SAS123", hub.CoordinationTagRequests[0].Callsign)
	assert.Equal(t, "EKCH_A_TWR", hub.CoordinationTagRequests[0].From)
	assert.Equal(t, "EKCH_D_TWR", hub.CoordinationTagRequests[0].To)
}

// TestCreateTagRequest_UnownedStrip verifies that requesting a tag for an unowned strip is rejected.
func TestCreateTagRequest_UnownedStrip(t *testing.T) {
	strip := &models.Strip{ID: 1, Callsign: "SAS123", Owner: nil}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{})

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.CreateTagRequest(context.Background(), 1, "SAS123", "EKCH_D_TWR")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unowned")
}

// TestCreateTagRequest_AlreadyOwner verifies that the strip owner cannot request their own tag.
func TestCreateTagRequest_AlreadyOwner(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{ID: 1, Callsign: "SAS123", Owner: &owner}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{})

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.CreateTagRequest(context.Background(), 1, "SAS123", "EKCH_A_TWR")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already own")
}

// TestCreateTagRequest_ActiveCoordinationExists verifies that a tag request is rejected when
// another coordination is already active for the strip.
func TestCreateTagRequest_ActiveCoordinationExists(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{ID: 1, Callsign: "SAS123", Owner: &owner}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return &models.Coordination{ID: 5, FromPosition: "EKCH_A_TWR", ToPosition: "EKCH_D_TWR"}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.CreateTagRequest(context.Background(), 1, "SAS123", "EKCH_GND")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "active coordination")
}

// TestCreateTagRequest_StripNotFound verifies that a repository error is propagated.
func TestCreateTagRequest_StripNotFound(t *testing.T) {
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{})

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.CreateTagRequest(context.Background(), 1, "MISSING", "EKCH_D_TWR")
	require.Error(t, err)
	assert.ErrorIs(t, err, pgx.ErrNoRows)
}

// ── AcceptTagRequest ──────────────────────────────────────────────────────────

// TestAcceptTagRequest_Success verifies that accepting a tag request transfers ownership to the
// requester, sends the assume event, and sends the owners update.
func TestAcceptTagRequest_Success(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &owner,
		NextOwners:     []string{},
		PreviousOwners: []string{},
		Version:        1,
	}

	coord := &models.Coordination{
		ID:           7,
		FromPosition: "EKCH_A_TWR",
		ToPosition:   "EKCH_D_TWR",
		IsTagRequest: true,
	}

	var setOwnerPosition string
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return coord, nil
		},
		DeleteFn: func(_ context.Context, _ int32) error { return nil },
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _ []string, _ []string) error {
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, p *string, _ int32) (int64, error) {
			setOwnerPosition = *p
			return 1, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.AcceptTagRequest(context.Background(), 1, "SAS123", "EKCH_A_TWR")
	require.NoError(t, err)
	assert.Equal(t, "EKCH_D_TWR", setOwnerPosition)

	require.Len(t, hub.CoordinationAssumes, 1)
	assert.Equal(t, "EKCH_D_TWR", hub.CoordinationAssumes[0].Position)

	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, "EKCH_D_TWR", hub.OwnersUpdates[0].Owner)
}

// TestAcceptTagRequest_NotOwner verifies that a non-owner cannot accept a tag request.
func TestAcceptTagRequest_NotOwner(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{ID: 1, Callsign: "SAS123", Owner: &owner}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return &models.Coordination{ID: 1, IsTagRequest: true, ToPosition: "EKCH_D_TWR"}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.AcceptTagRequest(context.Background(), 1, "SAS123", "EKCH_D_TWR")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the strip owner")
}

// TestAcceptTagRequest_NotTagRequest verifies that a normal coordination cannot be accepted
// via AcceptTagRequest.
func TestAcceptTagRequest_NotTagRequest(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{ID: 1, Callsign: "SAS123", Owner: &owner}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return &models.Coordination{ID: 2, IsTagRequest: false, ToPosition: "EKCH_D_TWR"}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.AcceptTagRequest(context.Background(), 1, "SAS123", "EKCH_A_TWR")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no pending tag request")
}

// TestAcceptTagRequest_DisplacedOwnerRemovedFromPrevious verifies that the old owner is removed
// from previous controllers after a tag request is accepted (matches ForceAssumeStrip behaviour).
func TestAcceptTagRequest_DisplacedOwnerRemovedFromPrevious(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &owner,
		NextOwners:     []string{},
		PreviousOwners: []string{"EKCH_DEL", "EKCH_A_TWR"},
		Version:        1,
	}

	coord := &models.Coordination{
		ID:           3,
		FromPosition: "EKCH_A_TWR",
		ToPosition:   "EKCH_D_TWR",
		IsTagRequest: true,
	}

	var savedPrevious []string
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return coord, nil
		},
		DeleteFn: func(_ context.Context, _ int32) error { return nil },
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _ []string, prev []string) error {
			savedPrevious = prev
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, _ *string, _ int32) (int64, error) {
			return 1, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.AcceptTagRequest(context.Background(), 1, "SAS123", "EKCH_A_TWR")
	require.NoError(t, err)
	assert.Equal(t, []string{"EKCH_DEL"}, savedPrevious, "old owner must be removed from previous controllers")
}

// TestAcceptTagRequest_RouteRecalculated verifies that UpdateRouteForStrip is called after
// a tag request is accepted (matches ForceAssumeStrip behaviour).
func TestAcceptTagRequest_RouteRecalculated(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &owner,
		NextOwners:     []string{},
		PreviousOwners: []string{},
		Version:        1,
	}

	coord := &models.Coordination{
		ID:           4,
		FromPosition: "EKCH_A_TWR",
		ToPosition:   "EKCH_D_TWR",
		IsTagRequest: true,
	}

	routeRecalculated := false
	mockServer := &testutil.MockServer{
		CoordRepoVal: &testutil.MockCoordinationRepository{
			GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
				return coord, nil
			},
			DeleteFn: func(_ context.Context, _ int32) error { return nil },
		},
		UpdateRouteForStripFn: func(callsign string, sessionId int32, sendUpdate bool) error {
			assert.Equal(t, "SAS123", callsign)
			assert.Equal(t, int32(1), sessionId)
			assert.False(t, sendUpdate)
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

	err := svc.AcceptTagRequest(context.Background(), 1, "SAS123", "EKCH_A_TWR")
	require.NoError(t, err)
	assert.True(t, routeRecalculated, "route must be recalculated after accepting a tag request")
}

// TestAcceptTagRequest_NextOwnersFilteredFromPrevious verifies that any controller appearing in
// the recalculated next owners is removed from previous controllers (matches ForceAssumeStrip).
func TestAcceptTagRequest_NextOwnersFilteredFromPrevious(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &owner,
		NextOwners:     []string{},
		PreviousOwners: []string{"EKCH_DEL", "EKCH_CTR"},
		Version:        1,
	}

	// After route recalculation EKCH_DEL ends up back in NextOwners.
	recalculatedStrip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &owner,
		NextOwners:     []string{"EKCH_DEL"},
		PreviousOwners: []string{"EKCH_DEL", "EKCH_CTR"},
		Version:        1,
	}

	coord := &models.Coordination{
		ID:           5,
		FromPosition: "EKCH_A_TWR",
		ToPosition:   "EKCH_D_TWR",
		IsTagRequest: true,
	}

	callCount := 0
	var savedPrevious []string

	mockServer := &testutil.MockServer{
		CoordRepoVal: &testutil.MockCoordinationRepository{
			GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
				return coord, nil
			},
			DeleteFn: func(_ context.Context, _ int32) error { return nil },
		},
		UpdateRouteForStripFn: func(_ string, _ int32, _ bool) error { return nil },
	}

	hub := &testutil.MockFrontendHub{}
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
		SetPreviousOwnersFn: func(_ context.Context, _ int32, _ string, prev []string) error {
			savedPrevious = prev
			return nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.AcceptTagRequest(context.Background(), 1, "SAS123", "EKCH_A_TWR")
	require.NoError(t, err)

	// EKCH_DEL is in NextOwners → must be removed from PreviousOwners.
	// EKCH_CTR stays. EKCH_A_TWR (old owner) was already filtered out earlier.
	assert.Equal(t, []string{"EKCH_CTR"}, savedPrevious)

	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, []string{"EKCH_DEL"}, hub.OwnersUpdates[0].NextOwners)
	assert.Equal(t, []string{"EKCH_CTR"}, hub.OwnersUpdates[0].PreviousOwners)
}

// TestAcceptTagRequest_CoordinationDeleted verifies that the coordination record is deleted
// when the tag request is accepted.
func TestAcceptTagRequest_CoordinationDeleted(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &owner,
		NextOwners:     []string{},
		PreviousOwners: []string{},
		Version:        1,
	}

	coord := &models.Coordination{
		ID:           99,
		FromPosition: "EKCH_A_TWR",
		ToPosition:   "EKCH_D_TWR",
		IsTagRequest: true,
	}

	deletedID := int32(0)
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return coord, nil
		},
		DeleteFn: func(_ context.Context, id int32) error {
			deletedID = id
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})

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

	err := svc.AcceptTagRequest(context.Background(), 1, "SAS123", "EKCH_A_TWR")
	require.NoError(t, err)
	assert.Equal(t, int32(99), deletedID, "coordination must be deleted on accept")
}

// TestAcceptTagRequest_SetOwnerFails propagates DB errors from SetOwner.
func TestAcceptTagRequest_SetOwnerFails(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &owner,
		NextOwners:     []string{},
		PreviousOwners: []string{},
		Version:        1,
	}

	coord := &models.Coordination{
		ID:           6,
		FromPosition: "EKCH_A_TWR",
		ToPosition:   "EKCH_D_TWR",
		IsTagRequest: true,
	}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return coord, nil
		},
		DeleteFn: func(_ context.Context, _ int32) error { return nil },
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})

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
	svc.SetFrontendHub(hub)

	err := svc.AcceptTagRequest(context.Background(), 1, "SAS123", "EKCH_A_TWR")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

// TestAcceptTagRequest_RequesterInNextList_OwnerAppendedToPrevious verifies that when the
// requester was already in NextOwners (planned handoff), the displaced owner is appended to
// PreviousOwners and existing previous owners are preserved.
func TestAcceptTagRequest_RequesterInNextList_OwnerAppendedToPrevious(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &owner,
		NextOwners:     []string{"EKCH_D_TWR", "EKCH_APP"},
		PreviousOwners: []string{"EKCH_DEL"},
		Version:        1,
	}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return &models.Coordination{ID: 1, IsTagRequest: true, ToPosition: "EKCH_D_TWR"}, nil
		},
		DeleteFn: func(_ context.Context, _ int32) error { return nil },
	}

	var savedPreviousOwners []string
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})

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

	err := svc.AcceptTagRequest(context.Background(), 1, "SAS123", "EKCH_A_TWR")
	require.NoError(t, err)
	// Existing previous owners preserved + displaced owner appended.
	assert.Equal(t, []string{"EKCH_DEL", "EKCH_A_TWR"}, savedPreviousOwners,
		"displaced owner must be appended when requester was in next list")
}

// TestAcceptTagRequest_RequesterNotInNextList_OwnerNotAppendedToPrevious verifies that when the
// requester was NOT in NextOwners (out-of-band tag grab), the displaced owner is removed from
// previous controllers and not re-appended.
func TestAcceptTagRequest_RequesterNotInNextList_OwnerNotAppendedToPrevious(t *testing.T) {
	owner := "EKCH_A_TWR"
	strip := &models.Strip{
		ID:             1,
		Callsign:       "SAS123",
		Owner:          &owner,
		NextOwners:     []string{"EKCH_APP"}, // EKCH_D_TWR not in list
		PreviousOwners: []string{"EKCH_DEL", "EKCH_A_TWR"},
		Version:        1,
	}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return &models.Coordination{ID: 1, IsTagRequest: true, ToPosition: "EKCH_D_TWR"}, nil
		},
		DeleteFn: func(_ context.Context, _ int32) error { return nil },
	}

	var savedPreviousOwners []string
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})

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

	err := svc.AcceptTagRequest(context.Background(), 1, "SAS123", "EKCH_A_TWR")
	require.NoError(t, err)
	// Displaced owner removed; not re-appended for an out-of-band tag grab.
	assert.Equal(t, []string{"EKCH_DEL"}, savedPreviousOwners,
		"displaced owner must not be appended when requester was not in next list")
}
