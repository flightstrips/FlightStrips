package services

import (
	"context"
	"errors"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/frontend"
	pkgModels "FlightStrips/pkg/models"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- MoveStripBetween ----

func TestMoveStripBetween_AppendAtEnd(t *testing.T) {
	// insertAfter=nil and no strip comes after → append to end of bay
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EZY001"
	const bay = shared.BAY_NOT_CLEARED

	var savedSeq int32
	stripRepo := &testutil.MockStripRepository{
		GetNextSequenceFn: func(_ context.Context, _ int32, _ string, prev int32) (int32, error) {
			assert.Equal(t, int32(0), prev) // no insertAfter → prev=0
			return 0, pgx.ErrNoRows         // nothing after
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, _ string, seq int32) (int64, error) {
			savedSeq = seq
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.MoveStripBetween(ctx, session, callsign, nil, bay)
	require.NoError(t, err)
	// calculateOrderBetween(0, nil) = 0+InitialOrderSpacing
	assert.Equal(t, int32(InitialOrderSpacing), savedSeq)
	require.Len(t, hub.BayEvents, 1)
}

func TestMoveStripBetween_MidpointInsertion(t *testing.T) {
	// insertAfter=AAA001 (seq=1000), next=2000 → midpoint=1500
	ctx := context.Background()
	const session = int32(1)
	const callsign = "NEW001"
	const bay = shared.BAY_NOT_CLEARED
	const predecessor = "AAA001"

	var savedSeq int32
	stripRepo := &testutil.MockStripRepository{
		GetSequenceFn: func(_ context.Context, _ int32, cs string, _ string) (int32, error) {
			assert.Equal(t, predecessor, cs)
			return int32(1000), nil
		},
		GetNextSequenceFn: func(_ context.Context, _ int32, _ string, prev int32) (int32, error) {
			assert.Equal(t, int32(1000), prev)
			return int32(2000), nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, cs string, _ string, seq int32) (int64, error) {
			assert.Equal(t, callsign, cs)
			savedSeq = seq
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	predecessorCopy := predecessor
	ref := &frontend.StripRef{Kind: "flight", Callsign: &predecessorCopy}
	err := svc.MoveStripBetween(ctx, session, callsign, ref, bay)
	require.NoError(t, err)
	assert.Equal(t, int32(1500), savedSeq, "midpoint of 1000..2000 should be 1500")
	require.Len(t, hub.BayEvents, 1)
}

func TestMoveStripBetween_GapExhaustion_TriggersRecalculation(t *testing.T) {
	// predecessor=1000, next=1001 → gap=1 ≤ MinOrderGap → recalculate
	ctx := context.Background()
	const session = int32(1)
	const callsign = "NEW002"
	const bay = shared.BAY_NOT_CLEARED
	const predecessor = "AAA001"

	seq1 := int32(1000)
	seq2 := int32(1001)

	var recalcCalled bool
	stripRepo := &testutil.MockStripRepository{
		GetSequenceFn: func(_ context.Context, _ int32, _ string, _ string) (int32, error) {
			return int32(1000), nil
		},
		GetNextSequenceFn: func(_ context.Context, _ int32, _ string, _ int32) (int32, error) {
			return int32(1001), nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			return 1, nil
		},
		RecalculateSequencesFn: func(_ context.Context, _ int32, _ string, spacing int32) error {
			recalcCalled = true
			assert.Equal(t, int32(InitialOrderSpacing), spacing)
			return nil
		},
		ListSequencesFn: func(_ context.Context, _ int32, _ string) ([]*models.StripSequence, error) {
			return []*models.StripSequence{
				{Callsign: "AAA001", Sequence: &seq1},
				{Callsign: "AAA002", Sequence: &seq2},
			}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	predecessorCopy := predecessor
	ref := &frontend.StripRef{Kind: "flight", Callsign: &predecessorCopy}
	err := svc.MoveStripBetween(ctx, session, callsign, ref, bay)
	require.NoError(t, err)
	assert.True(t, recalcCalled, "recalculation should have been triggered")
}

// ---- ClearStrip / UnclearStrip ----

func TestClearStrip_MovesToClearedBay(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS999"
	const cid = "1234567"

	var movedToBay string
	stripRepo := &testutil.MockStripRepository{
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, b string) (int32, error) {
			return int32(0), nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, b string, _ int32) (int64, error) {
			movedToBay = b
			return 1, nil
		},
		UpdateClearedFlagFn: func(_ context.Context, _ int32, _ string, _ bool, _ string, _ *int32) (int64, error) {
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	esHub := &testutil.MockEuroscopeHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetEuroscopeHub(esHub)

	err := svc.ClearStrip(ctx, session, callsign, cid)
	require.NoError(t, err)
	assert.Equal(t, shared.BAY_CLEARED, movedToBay)
	require.Len(t, esHub.ClearedFlags, 1)
	assert.Equal(t, true, esHub.ClearedFlags[0].Flag)
	assert.Equal(t, cid, esHub.ClearedFlags[0].Cid)
	assert.Equal(t, callsign, esHub.ClearedFlags[0].Callsign)
}

func TestUnclearStrip_MovesToNotClearedBay(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "DLH777"
	const cid = "9876543"

	var movedToBay string
	stripRepo := &testutil.MockStripRepository{
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return int32(0), nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, b string, _ int32) (int64, error) {
			movedToBay = b
			return 1, nil
		},
		UpdateClearedFlagFn: func(_ context.Context, _ int32, _ string, _ bool, _ string, _ *int32) (int64, error) {
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	esHub := &testutil.MockEuroscopeHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetEuroscopeHub(esHub)

	err := svc.UnclearStrip(ctx, session, callsign, cid)
	require.NoError(t, err)
	assert.Equal(t, shared.BAY_NOT_CLEARED, movedToBay)
	require.Len(t, esHub.ClearedFlags, 1)
	assert.Equal(t, false, esHub.ClearedFlags[0].Flag)
}

func TestClearStrip_NoEuroscopeHub(t *testing.T) {
	// Without an EuroScope hub attached, ClearStrip should still succeed
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return int32(0), nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			return 1, nil
		},
		UpdateClearedFlagFn: func(_ context.Context, _ int32, _ string, _ bool, _ string, _ *int32) (int64, error) {
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	// no SetEuroscopeHub

	err := svc.ClearStrip(ctx, 1, "EZY100", "CID123")
	require.NoError(t, err)
}

// ---- DeleteStrip ----

func TestDeleteStrip_CallsDeleteAndDisconnect(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "KLM500"

	var deleteCalled bool
	stripRepo := &testutil.MockStripRepository{
		DeleteFn: func(_ context.Context, s int32, cs string) error {
			assert.Equal(t, session, s)
			assert.Equal(t, callsign, cs)
			deleteCalled = true
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.DeleteStrip(ctx, session, callsign)
	require.NoError(t, err)
	assert.True(t, deleteCalled)
	require.Len(t, hub.AircraftDisconnects, 1)
	assert.Equal(t, callsign, hub.AircraftDisconnects[0].Callsign)
}

func TestDeleteStrip_PropagatesRepositoryError(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("connection reset")

	stripRepo := &testutil.MockStripRepository{
		DeleteFn: func(_ context.Context, _ int32, _ string) error {
			return dbErr
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.DeleteStrip(ctx, 1, "ANY")
	require.ErrorIs(t, err, dbErr)
	// Disconnect is still sent even when Delete fails (code sends it unconditionally)
	require.Len(t, hub.AircraftDisconnects, 1)
}

// ---- FreeStrip ----

func TestFreeStrip_Success(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "TAP400"
	const ownerPos = "GND_N"

	owner := ownerPos
	strip := &models.Strip{
		ID:             30,
		Callsign:       callsign,
		Version:        int32(4),
		Owner:          &owner,
		NextOwners:     []string{"APP"},
		PreviousOwners: []string{"DEL_N"},
	}

	var capturedPreviousOwners []string
	var setOwnerNil bool
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetPreviousOwnersFn: func(_ context.Context, _ int32, _ string, prev []string) error {
			capturedPreviousOwners = prev
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, o *string, _ int32) (int64, error) {
			setOwnerNil = o == nil
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.FreeStrip(ctx, session, callsign, ownerPos)
	require.NoError(t, err)
	assert.True(t, setOwnerNil, "owner should be set to nil on free")
	assert.Contains(t, capturedPreviousOwners, ownerPos, "current owner should be added to previous owners")
	require.Len(t, hub.CoordinationFrees, 1)
	assert.Equal(t, callsign, hub.CoordinationFrees[0].Callsign)
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, "", hub.OwnersUpdates[0].Owner, "owner should be cleared in the update")
}

func TestFreeStrip_NotOwner_ReturnsError(t *testing.T) {
	ctx := context.Background()
	const callsign = "BAW100"
	const actualOwner = "GND_N"

	owner := actualOwner
	strip := &models.Strip{Callsign: callsign, Owner: &owner}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.FreeStrip(ctx, 1, callsign, "DEL_N") // wrong position
	require.Error(t, err)
	assert.Empty(t, hub.CoordinationFrees)
}

func TestFreeStrip_NilOwner_ReturnsError(t *testing.T) {
	ctx := context.Background()
	strip := &models.Strip{Callsign: "EZY200", Owner: nil}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.FreeStrip(ctx, 1, "EZY200", "GND_N")
	require.Error(t, err)
}

// ---- CancelCoordinationTransfer ----

func TestCancelCoordinationTransfer_Success(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS001"
	const fromPos = "DEL_N"

	coord := &models.Coordination{
		ID:           77,
		FromPosition: fromPos,
		ToPosition:   "GND_N",
	}

	var deleted bool
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return coord, nil
		},
		DeleteFn: func(_ context.Context, id int32) error {
			assert.Equal(t, coord.ID, id)
			deleted = true
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(&testutil.MockStripRepository{})
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)

	err := svc.CancelCoordinationTransfer(ctx, session, callsign, fromPos)
	require.NoError(t, err)
	assert.True(t, deleted)
	require.Len(t, hub.CoordinationRejects, 1, "cancel notifies via SendCoordinationReject")
}

func TestCancelCoordinationTransfer_WrongInitiator_ReturnsError(t *testing.T) {
	ctx := context.Background()

	coord := &models.Coordination{
		ID:           78,
		FromPosition: "DEL_N",
		ToPosition:   "GND_N",
	}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return coord, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(&testutil.MockStripRepository{})
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)

	// GND_N did not initiate the transfer, only DEL_N can cancel
	err := svc.CancelCoordinationTransfer(ctx, 1, "SAS001", "GND_N")
	require.Error(t, err)
	assert.Empty(t, hub.CoordinationRejects)
}

// ---- AssumeStripCoordination ----

func TestAssumeStripCoordination_DirectAssume_UnownedStrip(t *testing.T) {
	// No pending coordination, strip is unowned → direct assume
	ctx := context.Background()
	const session = int32(1)
	const callsign = "IBE500"
	const position = "GND_N"

	strip := &models.Strip{
		ID:       50,
		Callsign: callsign,
		Owner:    nil,
		Version:  int32(2),
	}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			return nil, pgx.ErrNoRows
		},
	}

	var ownerSet string
	var nextOwnersSet []string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, next []string, _ []string) error {
			nextOwnersSet = next
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, o *string, v int32) (int64, error) {
			assert.NotNil(t, o)
			ownerSet = *o
			assert.Equal(t, strip.Version, v)
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)

	err := svc.AssumeStripCoordination(ctx, session, callsign, position)
	require.NoError(t, err)
	assert.Equal(t, position, ownerSet)
	assert.Empty(t, nextOwnersSet)
	require.Len(t, hub.CoordinationAssumes, 1)
	assert.Equal(t, position, hub.CoordinationAssumes[0].Position)
}

func TestAssumeStripCoordination_WithCoordination_AcceptsIt(t *testing.T) {
	// Pending coordination targeting this position → AcceptCoordination
	ctx := context.Background()
	const session = int32(1)
	const callsign = "AFR600"
	const position = "GND_N"
	const fromPos = "DEL_N"

	strip := &models.Strip{
		ID:             60,
		Callsign:       callsign,
		Version:        int32(3),
		NextOwners:     []string{position},
		PreviousOwners: []string{},
	}

	coord := &models.Coordination{
		ID:           88,
		Session:      session,
		StripID:      strip.ID,
		FromPosition: fromPos,
		ToPosition:   position,
	}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			return coord, nil
		},
		DeleteFn: func(_ context.Context, _ int32) error { return nil },
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _ []string, _ []string) error {
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, _ *string, _ int32) (int64, error) {
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)

	err := svc.AssumeStripCoordination(ctx, session, callsign, position)
	require.NoError(t, err)
	require.Len(t, hub.CoordinationAssumes, 1)
	assert.Equal(t, position, hub.CoordinationAssumes[0].Position)
}

func TestAssumeStripCoordination_AlreadyOwned_ReturnsError(t *testing.T) {
	ctx := context.Background()
	other := "TWR_N"
	strip := &models.Strip{
		ID:       70,
		Callsign: "EZY700",
		Owner:    &other,
		Version:  1,
	}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			return nil, pgx.ErrNoRows
		},
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)

	err := svc.AssumeStripCoordination(ctx, 1, "EZY700", "GND_N")
	require.Error(t, err)
	assert.Empty(t, hub.CoordinationAssumes)
}

// ---- UpdateGroundState ----

func TestUpdateGroundState_StripNotFound_ReturnsNil(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
	}

	svc := NewStripService(stripRepo)
	err := svc.UpdateGroundState(ctx, 1, "GHOST", "PUSH", "EKCH")
	require.NoError(t, err)
}

func TestUpdateGroundState_SameState_IsNoop(t *testing.T) {
	ctx := context.Background()
	state := "PUSH"
	strip := &models.Strip{Callsign: "SAS100", State: &state, Bay: shared.BAY_PUSH}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	}

	svc := NewStripService(stripRepo)
	// Same ground state — should return immediately without calling UpdateGroundState
	err := svc.UpdateGroundState(ctx, 1, "SAS100", "PUSH", "EKCH")
	require.NoError(t, err)
}

func TestUpdateGroundState_NewState_UpdatesAndMovesBay(t *testing.T) {
	ctx := context.Background()
	oldState := ""
	strip := &models.Strip{
		Callsign: "DLH200",
		State:    &oldState,
		Bay:      shared.BAY_CLEARED,
		Origin:   "EKCH",
	}

	var updatedState string
	var updatedBay string
	var movedToBay string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, state *string, bay string, _ *int32) (int64, error) {
			if state != nil {
				updatedState = *state
			}
			updatedBay = bay
			return 1, nil
		},
		// MoveToBay calls GetMaxSequenceInBay + UpdateBayAndSequence
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return int32(0), nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, b string, _ int32) (int64, error) {
			movedToBay = b
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.UpdateGroundState(ctx, 1, "DLH200", "PUSH", "EKCH")
	require.NoError(t, err)
	assert.Equal(t, "PUSH", updatedState)
	assert.Equal(t, shared.BAY_PUSH, updatedBay)
	assert.Equal(t, shared.BAY_PUSH, movedToBay, "strip should be moved to PUSH bay")
}

// ---- AutoAssumeForClearedStrip edge cases ----

func TestAutoAssumeForClearedStrip_NilSectorOwnerRepo_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	svc := NewStripService(&testutil.MockStripRepository{})
	// sectorOwnerRepo is not set — should be a no-op
	err := svc.AutoAssumeForClearedStrip(ctx, 1, "ANY")
	require.NoError(t, err)
}

func TestAutoAssumeForClearedStrip_SetOwnerVersionConflict_NoUpdate(t *testing.T) {
	// SetOwner returns 0 (version mismatch) → no OwnersUpdate sent
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EZY555"
	const sqPosition = "GND_N"

	strip := &models.Strip{Callsign: callsign, Version: 5, NextOwners: []string{}, PreviousOwners: []string{}}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, nextOwners []string, previousOwners []string) error {
			assert.Empty(t, nextOwners)
			assert.Empty(t, previousOwners)
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, _ *string, _ int32) (int64, error) {
			return 0, nil // version conflict — no rows updated
		},
	}

	sectorRepo := &testutil.MockSectorOwnerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.SectorOwner, error) {
			return []*models.SectorOwner{
				{Position: sqPosition, Sector: []string{"SQ"}},
			}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetSectorOwnerRepo(sectorRepo)

	err := svc.AutoAssumeForClearedStrip(ctx, session, callsign)
	require.NoError(t, err)
	assert.Empty(t, hub.OwnersUpdates, "no update should be sent when SetOwner returns 0 rows")
}

// ---- AutoAssumeForControllerOnline edge cases ----

func TestAutoAssumeForControllerOnline_MultipleMatchingStrips(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const position = "DEL_N"

	strips := []*models.Strip{
		{Callsign: "A001", Cleared: true, Owner: nil, NextOwners: []string{position}, Version: 1},
		{Callsign: "A002", Cleared: true, Owner: nil, NextOwners: []string{position}, Version: 2},
		{Callsign: "A003", Cleared: true, Owner: nil, NextOwners: []string{"GND_N"}, Version: 3}, // different next owner
	}

	assumedCallsigns := map[string]bool{}
	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return strips, nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, cs string, _ *string, _ int32) (int64, error) {
			assumedCallsigns[cs] = true
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.AutoAssumeForControllerOnline(ctx, session, position)
	require.NoError(t, err)
	assert.True(t, assumedCallsigns["A001"])
	assert.True(t, assumedCallsigns["A002"])
	assert.False(t, assumedCallsigns["A003"], "A003 has a different next owner and should not be assumed")
	assert.Len(t, hub.OwnersUpdates, 2)
}

func TestAutoAssumeForControllerOnline_SetOwnerConflict_SkipsUpdate(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const position = "GND_N"

	strips := []*models.Strip{
		{Callsign: "B001", Cleared: true, Owner: nil, NextOwners: []string{position}, Version: 1},
	}

	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return strips, nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, _ *string, _ int32) (int64, error) {
			return 0, nil // another goroutine claimed it first
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.AutoAssumeForControllerOnline(ctx, session, position)
	require.NoError(t, err)
	assert.Empty(t, hub.OwnersUpdates, "no update when SetOwner returns 0 rows")
}

// ---- PropagateRunwayChange ----

func TestPropagateRunwayChange_UpdatesMatchingDepartureStrips(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const airport = "EKCH"
	oldRunway := "22L"
	newRunway := "04R"

	strips := []*models.Strip{
		{Callsign: "SAS1", Origin: airport, Destination: "EGLL", Runway: &oldRunway}, // departure, matches old runway
		{Callsign: "SAS2", Origin: airport, Destination: "EGLL", Runway: &newRunway}, // departure, already on new runway
		{Callsign: "SAS3", Origin: "EGLL", Destination: airport, Runway: &oldRunway}, // arrival — old runway list doesn't cover this
	}

	updatedCallsigns := map[string]string{}
	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return strips, nil
		},
		UpdateRunwayFn: func(_ context.Context, _ int32, cs string, rwy *string, _ *int32) (int64, error) {
			updatedCallsigns[cs] = *rwy
			return 1, nil
		},
	}

	svc := NewStripService(stripRepo)

	oldRunways := pkgModels.ActiveRunways{DepartureRunways: []string{oldRunway}, ArrivalRunways: []string{}}
	newRunways := pkgModels.ActiveRunways{DepartureRunways: []string{newRunway}, ArrivalRunways: []string{}}

	err := svc.PropagateRunwayChange(ctx, session, airport, oldRunways, newRunways)
	require.NoError(t, err)
	assert.Equal(t, newRunway, updatedCallsigns["SAS1"], "SAS1 should get updated to new departure runway")
	assert.NotContains(t, updatedCallsigns, "SAS2", "SAS2 is already on the new runway")
	assert.NotContains(t, updatedCallsigns, "SAS3", "SAS3 is an arrival, departure runway list doesn't apply")
}

func TestPropagateRunwayChange_NoNewRunways_SkipsUpdate(t *testing.T) {
	ctx := context.Background()
	oldRunway := "22L"

	strips := []*models.Strip{
		{Callsign: "EZY1", Origin: "EKCH", Destination: "EGLL", Runway: &oldRunway},
	}

	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return strips, nil
		},
	}

	svc := NewStripService(stripRepo)

	oldRunways := pkgModels.ActiveRunways{DepartureRunways: []string{oldRunway}}
	newRunways := pkgModels.ActiveRunways{DepartureRunways: []string{}} // empty new list

	err := svc.PropagateRunwayChange(ctx, 1, "EKCH", oldRunways, newRunways)
	require.NoError(t, err)
	// UpdateRunway should NOT have been called (no new runway to assign)
}

// ---- UpdateClearedFlagForMove ----

func TestUpdateClearedFlagForMove_FlagChangesToCleared_TriggersAutoAssume(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS010"
	const cid = "111222"
	const sqPosition = "GND_N"

	strip := &models.Strip{
		Callsign:       callsign,
		Cleared:        false, // currently NOT cleared
		Version:        int32(7),
		NextOwners:     []string{},
		PreviousOwners: []string{},
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		UpdateClearedFlagFn: func(_ context.Context, _ int32, _ string, cleared bool, _ string, _ *int32) (int64, error) {
			assert.True(t, cleared)
			return 1, nil
		},
		// AutoAssumeForClearedStrip updates owners before setting the owner.
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, nextOwners []string, previousOwners []string) error {
			assert.Empty(t, nextOwners)
			assert.Empty(t, previousOwners)
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, owner *string, _ int32) (int64, error) {
			assert.Equal(t, sqPosition, *owner)
			return 1, nil
		},
	}

	sectorRepo := &testutil.MockSectorOwnerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.SectorOwner, error) {
			return []*models.SectorOwner{
				{Position: sqPosition, Sector: []string{"SQ"}},
			}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	esHub := &testutil.MockEuroscopeHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetEuroscopeHub(esHub)
	svc.SetSectorOwnerRepo(sectorRepo)

	err := svc.UpdateClearedFlagForMove(ctx, session, callsign, true, shared.BAY_CLEARED, cid)
	require.NoError(t, err)
	// AutoAssume should have been triggered → owner update sent
	require.Len(t, hub.OwnersUpdates, 1)
	// EuroScope should be notified
	require.Len(t, esHub.ClearedFlags, 1)
	assert.True(t, esHub.ClearedFlags[0].Flag)
}

func TestUpdateClearedFlagForMove_FlagUnchanged_NoSideEffects(t *testing.T) {
	ctx := context.Background()

	strip := &models.Strip{
		Callsign: "EZY020",
		Cleared:  true, // already cleared
		Version:  int32(2),
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		UpdateClearedFlagFn: func(_ context.Context, _ int32, _ string, _ bool, _ string, _ *int32) (int64, error) {
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	esHub := &testutil.MockEuroscopeHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetEuroscopeHub(esHub)

	// Passing cleared=true when strip is already cleared → no side-effects
	err := svc.UpdateClearedFlagForMove(ctx, 1, "EZY020", true, shared.BAY_CLEARED, "CID")
	require.NoError(t, err)
	assert.Empty(t, hub.OwnersUpdates, "no auto-assume when flag didn't change")
	assert.Empty(t, esHub.ClearedFlags, "no EuroScope notification when flag didn't change")
}
