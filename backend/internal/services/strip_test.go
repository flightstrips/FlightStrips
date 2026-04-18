package services

import (
	"context"
	"errors"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type spyStripCdmService struct {
	callsign        string
	groundState     string
	recalcAirport   string
	recalcSession   int32
	called          bool
	recalcTriggered bool
}

func (s *spyStripCdmService) TriggerRecalculate(_ context.Context, session int32, airport string) {
	s.recalcTriggered = true
	s.recalcSession = session
	s.recalcAirport = airport
}

func (s *spyStripCdmService) HandleReadyRequest(_ context.Context, _ int32, _ string) error {
	panic("unexpected")
}
func (s *spyStripCdmService) HandleTobtUpdate(_ context.Context, _ int32, _ string, _ string, _ string, _ string) error {
	panic("unexpected")
}
func (s *spyStripCdmService) HandleDeiceUpdate(_ context.Context, _ int32, _ string, _ string) error {
	panic("unexpected")
}
func (s *spyStripCdmService) HandleAsrtToggle(_ context.Context, _ int32, _ string, _ string) error {
	panic("unexpected")
}
func (s *spyStripCdmService) HandleTsacUpdate(_ context.Context, _ int32, _ string, _ string) error {
	panic("unexpected")
}
func (s *spyStripCdmService) HandleManualCtot(_ context.Context, _ int32, _ string, _ string) error {
	panic("unexpected")
}
func (s *spyStripCdmService) HandleCtotRemove(_ context.Context, _ int32, _ string) error {
	panic("unexpected")
}
func (s *spyStripCdmService) HandleApproveReqTobt(_ context.Context, _ int32, _ string, _ string, _ string) error {
	panic("unexpected")
}
func (s *spyStripCdmService) SyncAsatForGroundState(_ context.Context, _ int32, callsign string, groundState string) error {
	s.called = true
	s.callsign = callsign
	s.groundState = groundState
	return nil
}
func (s *spyStripCdmService) RequestBetterTobt(_ context.Context, _ int32, _ string) error {
	panic("unexpected")
}

func (s *spyStripCdmService) SetSessionCdmMaster(_ context.Context, _ int32, _ bool) error {
	panic("SetSessionCdmMaster should not be called in this test")
}

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
	var getByCallsignCalls int

	strip := &models.Strip{
		ID:             10,
		Callsign:       callsign,
		Version:        stripVersion,
		NextOwners:     []string{"APP"},
		PreviousOwners: []string{},
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			getByCallsignCalls++
			if getByCallsignCalls == 1 {
				return strip, nil
			}
			return &models.Strip{Callsign: callsign, NextOwners: []string{"TWR_N"}}, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, cs string, nextOwners []string, previousOwners []string) error {
			assert.Equal(t, callsign, cs)
			assert.Equal(t, []string{"APP"}, nextOwners)
			assert.Empty(t, previousOwners)
			return nil
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

	var routeUpdateCallsign string
	var routeUpdateSession int32
	var routeUpdateSendUpdate bool
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{
		UpdateRouteForStripFn: func(cs string, sess int32, sendUpdate bool) error {
			routeUpdateCallsign = cs
			routeUpdateSession = sess
			routeUpdateSendUpdate = sendUpdate
			return nil
		},
	})
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetSectorOwnerRepo(sectorRepo)

	err := svc.AutoAssumeForClearedStrip(ctx, session, callsign)
	require.NoError(t, err)
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, sqPosition, hub.OwnersUpdates[0].Owner)
	assert.Equal(t, []string{"TWR_N"}, hub.OwnersUpdates[0].NextOwners)
	assert.Equal(t, callsign, routeUpdateCallsign)
	assert.Equal(t, session, routeUpdateSession)
	assert.False(t, routeUpdateSendUpdate)
}

func TestAutoAssume_ClearedStrip_AppendsDisplacedOwnerToPrevious(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "DLH789"
	const stripVersion = int32(4)
	const newOwner = "GND_N"
	deliveryOwner := "DEL_N"
	var getByCallsignCalls int

	strip := &models.Strip{
		ID:             11,
		Callsign:       callsign,
		Version:        stripVersion,
		Owner:          &deliveryOwner,
		NextOwners:     []string{newOwner, "TWR_N"},
		PreviousOwners: []string{"CLX_N"},
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			getByCallsignCalls++
			if getByCallsignCalls == 1 {
				return strip, nil
			}
			return &models.Strip{Callsign: callsign, NextOwners: []string{"TWR_N"}}, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, cs string, nextOwners []string, previousOwners []string) error {
			assert.Equal(t, callsign, cs)
			assert.Equal(t, []string{"TWR_N"}, nextOwners)
			assert.Equal(t, []string{"CLX_N", deliveryOwner}, previousOwners)
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, cs string, owner *string, version int32) (int64, error) {
			assert.Equal(t, callsign, cs)
			assert.NotNil(t, owner)
			assert.Equal(t, newOwner, *owner)
			assert.Equal(t, stripVersion, version)
			return 1, nil
		},
	}

	sectorRepo := &testutil.MockSectorOwnerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.SectorOwner, error) {
			return []*models.SectorOwner{
				{Position: newOwner, Sector: []string{"SQ"}},
			}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetSectorOwnerRepo(sectorRepo)

	err := svc.AutoAssumeForClearedStrip(ctx, session, callsign)
	require.NoError(t, err)
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, newOwner, hub.OwnersUpdates[0].Owner)
	assert.Equal(t, []string{"TWR_N"}, hub.OwnersUpdates[0].NextOwners)
	assert.Equal(t, []string{"CLX_N", deliveryOwner}, hub.OwnersUpdates[0].PreviousOwners)
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

	err := svc.AutoAssumeForClearedStrip(ctx, session, callsign)
	require.NoError(t, err)
	assert.Empty(t, hub.OwnersUpdates, "no owner update should be sent when no SQ/DEL controller is found")
}

func TestAutoAssume_FallbackToDel(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "KLM321"
	const delPosition = "DEL_W"
	const stripVersion = int32(1)
	var getByCallsignCalls int

	strip := &models.Strip{Callsign: callsign, Version: stripVersion, NextOwners: []string{}, PreviousOwners: []string{}}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			getByCallsignCalls++
			if getByCallsignCalls == 1 {
				return strip, nil
			}
			return &models.Strip{Callsign: callsign, NextOwners: []string{}}, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, nextOwners []string, previousOwners []string) error {
			assert.Empty(t, nextOwners)
			assert.Empty(t, previousOwners)
			return nil
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
	hub.SetServer(&testutil.MockServer{
		UpdateRouteForStripFn: func(cs string, sess int32, sendUpdate bool) error {
			assert.Equal(t, callsign, cs)
			assert.Equal(t, session, sess)
			assert.False(t, sendUpdate)
			return nil
		},
	})
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetSectorOwnerRepo(sectorRepo)

	err := svc.AutoAssumeForClearedStrip(ctx, session, callsign)
	require.NoError(t, err)
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, delPosition, hub.OwnersUpdates[0].Owner)
}

// ---- AutoAssumeForControllerOnline ----

func TestAutoAssumeForControllerOnline_AssumesMatchingStrip(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const position = "GND_N"
	refreshedNextOwners := []string{"TWR_N"}
	getByCallsignCount := 0

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
		GetByCallsignFn: func(_ context.Context, _ int32, cs string) (*models.Strip, error) {
			assert.Equal(t, "SAS100", cs)
			getByCallsignCount++
			if getByCallsignCount == 1 {
				return strips[0], nil
			}
			nextOwners := refreshedNextOwners
			refreshedNextOwners = nil
			return &models.Strip{Callsign: cs, NextOwners: nextOwners}, nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, cs string, o *string, _ int32) (int64, error) {
			assumedCallsign = cs
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{
		UpdateRouteForStripFn: func(cs string, sess int32, sendUpdate bool) error {
			assert.Equal(t, "SAS100", cs)
			assert.Equal(t, session, sess)
			assert.False(t, sendUpdate)
			return nil
		},
	})
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.AutoAssumeForControllerOnline(ctx, session, position)
	require.NoError(t, err)
	// Only SAS100 should be auto-assumed (SAS200 has a non-nil but empty owner — owner != nil)
	assert.Equal(t, "SAS100", assumedCallsign)
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, "SAS100", hub.OwnersUpdates[0].Callsign)
	assert.Equal(t, []string{"TWR_N"}, hub.OwnersUpdates[0].NextOwners)
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

func TestCreateCoordinationTransfer_TaxiBayToTower_MovesToLowerTaxiBay(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS789"
	const fromPos = "121.630"
	const toPos = "118.105"

	strip := &models.Strip{ID: 77, Callsign: callsign, Bay: shared.BAY_TAXI}

	var movedBay string
	var movedSequence int32

	coordRepo := &testutil.MockCoordinationRepository{
		CreateFn: func(_ context.Context, c *models.Coordination) error {
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
		GetMaxSequenceInBayFn: func(_ context.Context, gotSession int32, bay string) (int32, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, shared.BAY_TAXI_LWR, bay)
			return 1000, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, gotSession int32, gotCallsign string, bay string, sequence int32) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			movedBay = bay
			movedSequence = sequence
			return 1, nil
		},
	}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.CreateCoordinationTransfer(ctx, session, callsign, fromPos, toPos)
	require.NoError(t, err)
	assert.Equal(t, shared.BAY_TAXI_LWR, movedBay)
	assert.Equal(t, int32(1000+InitialOrderSpacing), movedSequence)
	require.Len(t, hub.BayEvents, 1)
	assert.Equal(t, shared.BAY_TAXI_LWR, hub.BayEvents[0].Bay)
	require.Len(t, hub.CoordinationTransfers, 1)
}

func TestCreateCoordinationTransfer_TaxiBayToNonTower_DoesNotMoveToLowerTaxiBay(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "DLH321"
	const fromPos = "121.630"
	const toPos = "119.805"

	strip := &models.Strip{ID: 78, Callsign: callsign, Bay: shared.BAY_TAXI}

	coordRepo := &testutil.MockCoordinationRepository{
		CreateFn: func(_ context.Context, _ *models.Coordination) error { return nil },
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
	assert.Empty(t, hub.BayEvents)
	require.Len(t, hub.CoordinationTransfers, 1)
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

func TestCreateCoordinationTransfer_AirborneBay_SendsEsHandover(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS123"
	const fromPos = "TWR_N"
	const toPos = "APP_N"
	const ownerCid = "1234567"
	const targetCallsign = "ENGM_APP"

	strip := &models.Strip{ID: 10, Callsign: callsign, Bay: shared.BAY_AIRBORNE}

	coordRepo := &testutil.MockCoordinationRepository{
		CreateFn: func(_ context.Context, _ *models.Coordination) error { return nil },
	}

	controllerRepo := &testutil.MockControllerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
			cid := ownerCid
			return []*models.Controller{
				{Position: fromPos, Cid: &cid},
				{Position: toPos, Callsign: targetCallsign},
			}, nil
		},
	}

	mockServer := &testutil.MockServer{CoordRepoVal: coordRepo, ControllerRepoVal: controllerRepo}
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)

	esHub := &testutil.MockEuroscopeHub{}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetEuroscopeHub(esHub)

	err := svc.CreateCoordinationTransfer(ctx, session, callsign, fromPos, toPos)
	require.NoError(t, err)

	require.Len(t, esHub.CoordinationHandovers, 1)
	h := esHub.CoordinationHandovers[0]
	assert.Equal(t, session, h.Session)
	assert.Equal(t, ownerCid, h.Cid)
	assert.Equal(t, callsign, h.Callsign)
	assert.Equal(t, targetCallsign, h.TargetCallsign)
}

func TestCreateCoordinationTransfer_NonAirborneBay_NoEsHandover(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS456"
	const fromPos = "DEL_N"
	const toPos = "GND_N"

	strip := &models.Strip{ID: 42, Callsign: callsign, Bay: shared.BAY_CLEARED}

	coordRepo := &testutil.MockCoordinationRepository{
		CreateFn: func(_ context.Context, _ *models.Coordination) error { return nil },
	}

	mockServer := &testutil.MockServer{CoordRepoVal: coordRepo}
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)

	esHub := &testutil.MockEuroscopeHub{}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
	}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetEuroscopeHub(esHub)

	err := svc.CreateCoordinationTransfer(ctx, session, callsign, fromPos, toPos)
	require.NoError(t, err)
	assert.Empty(t, esHub.CoordinationHandovers)
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

// ---- RunwayClearance ----

func TestRunwayClearance_TaxiLwrBaySetsGroundStateAndNotifiesES(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS456"

	strip := &models.Strip{Callsign: callsign, Bay: shared.BAY_TAXI_LWR, Origin: "EKCH"}

	var repoSession int32
	var repoCallsign string
	groundStateUpdated := false

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		UpdateRunwayClearanceFn: func(_ context.Context, s int32, cs string) (int64, error) {
			repoSession = s
			repoCallsign = cs
			return 1, nil
		},
		UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, state *string, _ string, _ *int32) (int64, error) {
			groundStateUpdated = true
			require.NotNil(t, state)
			assert.Equal(t, euroscope.GroundStateDepart, *state)
			return 1, nil
		},
	}

	esHub := &testutil.MockEuroscopeHub{}
	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetEuroscopeHub(esHub)

	err := svc.RunwayClearance(ctx, session, callsign, "CID123", "EKCH")
	require.NoError(t, err)
	assert.Equal(t, session, repoSession)
	assert.Equal(t, callsign, repoCallsign)
	assert.True(t, groundStateUpdated, "ground state must be updated in DB")
	require.Len(t, esHub.GroundStates, 1)
	assert.Equal(t, euroscope.GroundStateDepart, esHub.GroundStates[0].GroundState)
	require.Len(t, hub.StripUpdates, 1)
}

func TestRunwayClearance_DepartBaySetsGroundStateAndNotifiesES(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS456"

	strip := &models.Strip{Callsign: callsign, Bay: shared.BAY_DEPART, Origin: "EKCH"}

	var groundStateSent string
	groundStateUpdated := false

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		UpdateRunwayClearanceFn: func(_ context.Context, _ int32, _ string) (int64, error) {
			return 1, nil
		},
		UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, state *string, _ string, _ *int32) (int64, error) {
			groundStateUpdated = true
			require.NotNil(t, state)
			groundStateSent = *state
			return 1, nil
		},
	}

	esHub := &testutil.MockEuroscopeHub{}
	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetEuroscopeHub(esHub)

	err := svc.RunwayClearance(ctx, session, callsign, "CID123", "EKCH")
	require.NoError(t, err)
	assert.True(t, groundStateUpdated, "ground state must be updated in DB")
	assert.Equal(t, euroscope.GroundStateDepart, groundStateSent)
	require.Len(t, esHub.GroundStates, 1)
	assert.Equal(t, euroscope.GroundStateDepart, esHub.GroundStates[0].GroundState)
}

func TestRunwayClearance_StripNotFound(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.RunwayClearance(ctx, 1, "XXX000", "", "")
	require.Error(t, err)
	assert.Empty(t, hub.StripUpdates, "no strip update should be broadcast when strip not found")
}

func TestRunwayClearance_RepositoryError(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("database error")

	strip := &models.Strip{Callsign: "SAS789", Bay: shared.BAY_TAXI_LWR, Origin: "EKCH"}
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		UpdateRunwayClearanceFn: func(_ context.Context, _ int32, _ string) (int64, error) {
			return 0, dbErr
		},
	}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})
	err := svc.RunwayClearance(ctx, 1, "SAS789", "", "")
	require.Error(t, err)
	assert.Equal(t, dbErr, err)
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

// ---- UpdateStand ----

func TestUpdateStand_TriggersRouteRecalculation(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS123"
	const stand = "A15"

	stripRepo := &testutil.MockStripRepository{
		UpdateStandFn: func(_ context.Context, _ int32, cs string, s *string, _ *int32) (int64, error) {
			assert.Equal(t, callsign, cs)
			assert.Equal(t, stand, *s)
			return 1, nil
		},
	}

	var routeUpdateCallsign string
	var routeUpdateSession int32
	var routeUpdateSendUpdate bool
	mockServer := &testutil.MockServer{
		UpdateRouteForStripFn: func(cs string, sess int32, sendUpdate bool) error {
			routeUpdateCallsign = cs
			routeUpdateSession = sess
			routeUpdateSendUpdate = sendUpdate
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.UpdateStand(ctx, session, callsign, stand)
	require.NoError(t, err)
	assert.Equal(t, callsign, routeUpdateCallsign)
	assert.Equal(t, session, routeUpdateSession)
	assert.True(t, routeUpdateSendUpdate, "UpdateStand must send owner update to frontend")
}

func TestUpdateStand_NoRouteUpdateWhenStripNotFound(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		UpdateStandFn: func(_ context.Context, _ int32, _ string, _ *string, _ *int32) (int64, error) {
			return 0, nil // strip not found
		},
	}

	routeUpdateCalled := false
	mockServer := &testutil.MockServer{
		UpdateRouteForStripFn: func(_ string, _ int32, _ bool) error {
			routeUpdateCalled = true
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.UpdateStand(ctx, 1, "SAS999", "B5")
	require.NoError(t, err)
	assert.False(t, routeUpdateCalled, "route should not be recalculated when strip is not found")
}

// ---- UpdateGroundState ----

// TestUpdateGroundState_DepartToEmptyKeepsBayDepart verifies that when the ground state
// clears (aircraft lifts off), the strip stays in BAY_DEPART so that the next position
// update can still detect the altitude threshold and transition to BAY_AIRBORNE.
// Regression test: previously dbStrip was built without Bay, causing GetDepartureBayFromGroundState
// to return BAY_HIDDEN and the strip to disappear before airborne detection could fire.
func TestUpdateGroundState_DepartToEmptyKeepsBayDepart(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS001"

	departState := euroscope.GroundStateDepart
	strip := &models.Strip{
		Callsign: callsign,
		Bay:      shared.BAY_DEPART,
		State:    &departState,
		Origin:   "EKCH",
	}

	var savedBay string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, _ *string, bay string, _ *int32) (int64, error) {
			savedBay = bay
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	// Ground state clears when the aircraft lifts off.
	err := svc.UpdateGroundState(ctx, session, callsign, "", "EKCH")
	require.NoError(t, err)

	// Bay must remain DEPART, not be moved to HIDDEN.
	assert.Equal(t, shared.BAY_DEPART, savedBay,
		"bay must stay DEPART when ground state clears so position updates can detect airborne")
	assert.Empty(t, hub.BayEvents, "no bay change event should be sent when bay did not change")
}

func TestUpdateGroundState_SyncsAsatAfterPersist(t *testing.T) {
	ctx := context.Background()
	state := ""
	strip := &models.Strip{
		Callsign: "SAS555",
		State:    &state,
		Bay:      shared.BAY_CLEARED,
		Origin:   "EKCH",
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, _ *string, _ string, _ *int32) (int64, error) {
			return 1, nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			return 1, nil
		},
	}

	cdmService := &spyStripCdmService{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})
	svc.SetCdmService(cdmService)

	err := svc.UpdateGroundState(ctx, 1, "SAS555", "PUSH", "EKCH")
	require.NoError(t, err)
	assert.True(t, cdmService.called)
	assert.Equal(t, "SAS555", cdmService.callsign)
	assert.Equal(t, "PUSH", cdmService.groundState)
}
