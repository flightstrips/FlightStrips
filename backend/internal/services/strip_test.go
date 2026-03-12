package services

import (
	"context"
	"errors"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- MoveToBay ----

func TestMoveToBay_Success(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS123"
	const bay = shared.BAY_NOT_CLEARED

	var updatedBay string
	var updatedSeq int32

	stripRepo := &testutil.MockStripRepository{
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return int32(2000), nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, cs string, b string, seq int32) (int64, error) {
			assert.Equal(t, callsign, cs)
			updatedBay = b
			updatedSeq = seq
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.MoveToBay(ctx, session, callsign, bay, true)
	require.NoError(t, err)
	assert.Equal(t, bay, updatedBay)
	assert.Equal(t, int32(2000+InitialOrderSpacing), updatedSeq)
	require.Len(t, hub.BayEvents, 1)
	assert.Equal(t, callsign, hub.BayEvents[0].Callsign)
	assert.Equal(t, bay, hub.BayEvents[0].Bay)
}

func TestMoveToBay_NoNotification(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return int32(1000), nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.MoveToBay(ctx, 1, "EZY999", shared.BAY_CLEARED, false)
	require.NoError(t, err)
	assert.Empty(t, hub.BayEvents, "no bay event should be sent when sendNotification=false")
}

func TestMoveToBay_RepositoryError(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("database error")

	stripRepo := &testutil.MockStripRepository{
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, dbErr
		},
	}

	svc := NewStripService(stripRepo)
	err := svc.MoveToBay(ctx, 1, "EZY123", shared.BAY_NOT_CLEARED, true)
	require.Error(t, err)
}

// ---- AutoAssumeForClearedStrip ----

func TestAutoAssume_ClearedStrip(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "DLH456"
	const stripVersion = int32(3)
	const sqPosition = "GND_N"

	strip := &models.Strip{
		ID:             10,
		Callsign:       callsign,
		Version:        stripVersion,
		NextOwners:     []string{"APP"},
		PreviousOwners: []string{},
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, cs string, owner *string, version int32) (int64, error) {
			assert.Equal(t, callsign, cs)
			assert.NotNil(t, owner)
			assert.Equal(t, sqPosition, *owner)
			assert.Equal(t, stripVersion, version)
			return 1, nil
		},
	}

	sectorRepo := &testutil.MockSectorOwnerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.SectorOwner, error) {
			return []*models.SectorOwner{
				{Position: sqPosition, Sector: []string{"SQ", "DEL"}},
			}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetSectorOwnerRepo(sectorRepo)

	err := svc.AutoAssumeForClearedStrip(ctx, session, callsign, stripVersion)
	require.NoError(t, err)
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, sqPosition, hub.OwnersUpdates[0].Owner)
}

func TestAutoAssume_NoMatchingController(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "BAW789"

	sectorRepo := &testutil.MockSectorOwnerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.SectorOwner, error) {
			// No SQ or DEL owners
			return []*models.SectorOwner{
				{Position: "TWR_N", Sector: []string{"TWR"}},
			}, nil
		},
	}

	stripRepo := &testutil.MockStripRepository{}
	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetSectorOwnerRepo(sectorRepo)

	err := svc.AutoAssumeForClearedStrip(ctx, session, callsign, 1)
	require.NoError(t, err)
	assert.Empty(t, hub.OwnersUpdates, "no owner update should be sent when no SQ/DEL controller is found")
}

func TestAutoAssume_FallbackToDel(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "KLM321"
	const delPosition = "DEL_W"
	const stripVersion = int32(1)

	strip := &models.Strip{Callsign: callsign, Version: stripVersion, NextOwners: []string{}, PreviousOwners: []string{}}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, owner *string, _ int32) (int64, error) {
			assert.Equal(t, delPosition, *owner)
			return 1, nil
		},
	}

	sectorRepo := &testutil.MockSectorOwnerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.SectorOwner, error) {
			// Only DEL, no SQ
			return []*models.SectorOwner{
				{Position: delPosition, Sector: []string{"DEL"}},
			}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetSectorOwnerRepo(sectorRepo)

	err := svc.AutoAssumeForClearedStrip(ctx, session, callsign, stripVersion)
	require.NoError(t, err)
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, delPosition, hub.OwnersUpdates[0].Owner)
}

// ---- AutoAssumeForControllerOnline ----

func TestAutoAssumeForControllerOnline_AssumesMatchingStrip(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const position = "GND_N"

	owner := ""
	strips := []*models.Strip{
		{
			ID:         1,
			Callsign:   "SAS100",
			Cleared:    true,
			Owner:      nil,
			NextOwners: []string{position},
			Version:    int32(2),
		},
		{
			ID:         2,
			Callsign:   "SAS200",
			Cleared:    true,
			Owner:      &owner,
			NextOwners: []string{position},
			Version:    int32(3),
		},
	}

	var assumedCallsign string
	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return strips, nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, cs string, o *string, _ int32) (int64, error) {
			assumedCallsign = cs
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.AutoAssumeForControllerOnline(ctx, session, position)
	require.NoError(t, err)
	// Only SAS100 should be auto-assumed (SAS200 has a non-nil but empty owner — owner != nil)
	assert.Equal(t, "SAS100", assumedCallsign)
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, "SAS100", hub.OwnersUpdates[0].Callsign)
}

func TestAutoAssumeForControllerOnline_NoMatchingStrips(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const position = "APP_W"

	strips := []*models.Strip{
		{Callsign: "EZY001", Cleared: true, Owner: nil, NextOwners: []string{"GND_N"}},
		{Callsign: "EZY002", Cleared: false, Owner: nil, NextOwners: []string{position}},
	}

	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
			return strips, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.AutoAssumeForControllerOnline(ctx, session, position)
	require.NoError(t, err)
	assert.Empty(t, hub.OwnersUpdates)
}

// ---- CreateCoordinationTransfer ----

func TestCreateCoordinationTransfer_Success(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS456"
	const fromPos = "DEL_N"
	const toPos = "GND_N"

	strip := &models.Strip{ID: 42, Callsign: callsign}

	coordRepo := &testutil.MockCoordinationRepository{
		CreateFn: func(_ context.Context, c *models.Coordination) error {
			assert.Equal(t, session, c.Session)
			assert.Equal(t, strip.ID, c.StripID)
			assert.Equal(t, fromPos, c.FromPosition)
			assert.Equal(t, toPos, c.ToPosition)
			return nil
		},
	}

	mockServer := &testutil.MockServer{CoordRepoVal: coordRepo}
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.CreateCoordinationTransfer(ctx, session, callsign, fromPos, toPos)
	require.NoError(t, err)
	require.Len(t, hub.CoordinationTransfers, 1)
	ct := hub.CoordinationTransfers[0]
	assert.Equal(t, callsign, ct.Callsign)
	assert.Equal(t, fromPos, ct.From)
	assert.Equal(t, toPos, ct.To)
}

func TestCreateCoordinationTransfer_StripNotFound(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
	}

	coordRepo := &testutil.MockCoordinationRepository{}
	mockServer := &testutil.MockServer{CoordRepoVal: coordRepo}
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.CreateCoordinationTransfer(ctx, 1, "UNKNOWN", "DEL_N", "GND_N")
	require.Error(t, err)
	assert.Empty(t, hub.CoordinationTransfers)
}

// ---- AcceptCoordination ----

func TestAcceptCoordination_UpdatesOwner(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "DLH100"
	const assumingPos = "GND_N"
	const fromPos = "DEL_N"

	strip := &models.Strip{
		ID:             20,
		Callsign:       callsign,
		Version:        int32(5),
		NextOwners:     []string{assumingPos, "APP"},
		PreviousOwners: []string{},
	}

	coord := &models.Coordination{
		ID:           99,
		Session:      session,
		StripID:      strip.ID,
		FromPosition: fromPos,
		ToPosition:   assumingPos,
	}

	var setOwnerCalled bool
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, nextOwners []string, previousOwners []string) error {
			// assumingPos was at index 0 in NextOwners, so nextOwners becomes [NextOwners[1:]] = ["APP"]
			assert.Equal(t, []string{"APP"}, nextOwners)
			assert.Contains(t, previousOwners, fromPos)
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, cs string, owner *string, version int32) (int64, error) {
			assert.Equal(t, callsign, cs)
			assert.Equal(t, assumingPos, *owner)
			assert.Equal(t, strip.Version, version)
			setOwnerCalled = true
			return 1, nil
		},
	}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, stripID int32) (*models.Coordination, error) {
			assert.Equal(t, strip.ID, stripID)
			return coord, nil
		},
		DeleteFn: func(_ context.Context, id int32) error {
			assert.Equal(t, coord.ID, id)
			return nil
		},
	}

	mockServer := &testutil.MockServer{CoordRepoVal: coordRepo}
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.AcceptCoordination(ctx, session, callsign, assumingPos)
	require.NoError(t, err)
	assert.True(t, setOwnerCalled)
	require.Len(t, hub.CoordinationAssumes, 1)
	assert.Equal(t, assumingPos, hub.CoordinationAssumes[0].Position)
	require.Len(t, hub.OwnersUpdates, 1)
}

func TestAcceptCoordination_NoCoordination(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "TAP200"

	strip := &models.Strip{ID: 21, Callsign: callsign, Version: 1}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			return nil, pgx.ErrNoRows
		},
	}

	mockServer := &testutil.MockServer{CoordRepoVal: coordRepo}
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.AcceptCoordination(ctx, session, callsign, "GND_N")
	require.NoError(t, err) // no coordination is not an error
	assert.Empty(t, hub.CoordinationAssumes)
}

// ---- RejectCoordination ----

func TestRejectCoordination_RestoresState(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "IBE300"
	const rejectingPos = "GND_N"

	coord := &models.Coordination{
		ID:           55,
		Session:      session,
		FromPosition: "DEL_N",
		ToPosition:   rejectingPos,
	}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return coord, nil
		},
		DeleteFn: func(_ context.Context, id int32) error {
			assert.Equal(t, coord.ID, id)
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(&testutil.MockStripRepository{})
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)

	err := svc.RejectCoordination(ctx, session, callsign, rejectingPos)
	require.NoError(t, err)
	require.Len(t, hub.CoordinationRejects, 1)
	assert.Equal(t, callsign, hub.CoordinationRejects[0].Callsign)
	assert.Equal(t, rejectingPos, hub.CoordinationRejects[0].Position)
}

func TestRejectCoordination_WrongPosition(t *testing.T) {
	ctx := context.Background()

	coord := &models.Coordination{
		ID:           56,
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

	// position "APP_N" does not match ToPosition "GND_N"
	err := svc.RejectCoordination(ctx, 1, "SAS001", "APP_N")
	require.Error(t, err)
	assert.Empty(t, hub.CoordinationRejects)
}
