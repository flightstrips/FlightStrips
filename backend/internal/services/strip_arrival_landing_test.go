package services

// Tests for arrival landing detection features:
//   - HandleCoordinationReceived now accepts coordination for RWY_ARR strips.
//   - handleArrivalPositionUpdate records ALDT on runway entry and moves strip
//     to TWY_ARR on runway exit.

import (
	"context"
	"testing"

	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- HandleCoordinationReceived: RWY_ARR now accepted ----

func TestHandleCoordinationReceived_RwyArrCreatesCoordination(t *testing.T) {
	strip := &models.Strip{
		ID: 40, Callsign: "SAS007", Bay: shared.BAY_RWY_ARR,
		Owner: strPtr("119.805"),
	}
	controller := &models.Controller{Callsign: "EKCH_M_TWR", Position: "118.105"}

	svc, hub, res := buildCoordinationReceivedSvc(t, strip, controller)
	require.NoError(t, svc.HandleCoordinationReceived(context.Background(), 1, "SAS007", "EKCH_M_TWR"))

	// Bay must NOT be changed — strip stays in RWY_ARR.
	assert.Empty(t, res.updatedBay, "RWY_ARR strip bay must not be updated")
	assert.Empty(t, res.movedToBay, "RWY_ARR strip must not be moved")

	// Coordination must be created.
	require.NotNil(t, res.created, "coordination must be created for RWY_ARR strip")
	assert.Equal(t, "119.805", res.created.FromPosition)
	assert.Equal(t, "118.105", res.created.ToPosition)
	require.Len(t, hub.CoordinationTransfers, 1)
}

func TestHandleCoordinationReceived_TwyArrIsIgnored(t *testing.T) {
	// A strip already in TWY_ARR (has landed) must not receive a new coordination.
	strip := &models.Strip{
		ID: 41, Callsign: "SAS008", Bay: shared.BAY_TWY_ARR,
		Owner: strPtr("119.805"),
	}
	controller := &models.Controller{Callsign: "EKCH_M_TWR", Position: "118.105"}

	svc, _, res := buildCoordinationReceivedSvc(t, strip, controller)
	require.NoError(t, svc.HandleCoordinationReceived(context.Background(), 1, "SAS008", "EKCH_M_TWR"))

	assert.Nil(t, res.created, "TWY_ARR strip must not receive a new coordination")
}

// ---- helpers for landing detection tests ----

// testRunwayRegion is a small polygon in the EKCH area that we inject as a runway.
// Points form a thin parallelogram. The centroid (12.650, 55.614) is inside.
const (
	rwyTestName = "RWY_TEST"
	insideLon   = 12.650
	insideLat   = 55.614
	outsideLon  = 12.640 // clearly west of the polygon
	outsideLat  = 55.614
)

var (
	lowAltitude  = int32(30)  // well below AirportElevation(17)+LandingAGL(50) = 67 ft
	highAltitude = int32(100) // above threshold (67 ft)
)

const landingTestSession = int32(1)

// testRunwayCoords is a small rectangle used as the injected test runway polygon.
// The centroid (55.614, 12.650) is inside; (55.614, 12.640) is outside.
var testRunwayCoords = [][2]float64{
	{12.648, 55.613},
	{12.652, 55.613},
	{12.652, 55.615},
	{12.648, 55.615},
}

func injectTestRunway(t *testing.T) {
	t.Helper()
	region := config.MakeTestRunwayRegion(rwyTestName, []string{"12", "30"}, testRunwayCoords)
	t.Cleanup(config.SetRunwayRegionsForTest([]config.Region{region}))
}

// buildLandingDetectionSvc creates a StripService wired up for landing-detection tests.
// cdmCapture receives the CdmData passed to SetCdmData (nil if not called).
// bayCapture receives the bay name passed to UpdateBayAndSequence (empty if not called).
func buildLandingDetectionSvc(
	t *testing.T,
	strip *models.Strip,
	coordRepo *testutil.MockCoordinationRepository,
) (*StripService, *testutil.MockFrontendHub, *struct {
	cdm *models.CdmData
	bay string
}) {
	t.Helper()

	out := &struct {
		cdm *models.CdmData
		bay string
	}{}

	cur := *strip

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			c := cur
			return &c, nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			cp := *data
			out.cdm = &cp
			cur.CdmData = &cp
			return 1, nil
		},
		// MoveToBay path (GetMaxSequenceInBay + UpdateBayAndSequence)
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			out.bay = bay
			cur.Bay = bay
			return 1, nil
		},
		// AcceptCoordination path
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _, _ []string) error {
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, _ *string, _ int32) (int64, error) {
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	if coordRepo != nil {
		hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})
	}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	if coordRepo != nil {
		svc.SetCoordinationRepo(coordRepo)
	}

	return svc, hub, out
}

// noCoordRepo returns a coord repo that always reports no pending coordination.
func noCoordRepo() *testutil.MockCoordinationRepository {
	return &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			return nil, pgx.ErrNoRows
		},
	}
}

// ---- ALDT recording ----

func TestHandleArrivalPositionUpdate_RecordsAldtOnTouchdown(t *testing.T) {
	injectTestRunway(t)

	strip := &models.Strip{
		ID: 50, Callsign: "NAX100", Bay: shared.BAY_FINAL,
		Destination: "EKCH",
	}
	svc, hub, out := buildLandingDetectionSvc(t, strip, noCoordRepo())

	svc.handleArrivalPositionUpdate(context.Background(), landingTestSession, strip.Callsign, insideLat, insideLon, int64(lowAltitude), strip)

	require.NotNil(t, out.cdm, "CdmData must be updated with ALDT")
	require.NotNil(t, out.cdm.Aldt, "ALDT must be set")
	assert.Len(t, *out.cdm.Aldt, 4, "ALDT must be a 4-char HHmm string")

	// Frontend must be notified.
	require.Len(t, hub.StripUpdates, 1)
	assert.Equal(t, strip.Callsign, hub.StripUpdates[0].Callsign)
}

func TestHandleArrivalPositionUpdate_DoesNotOverwriteExistingAldt(t *testing.T) {
	injectTestRunway(t)

	aldt := "1234"
	strip := &models.Strip{
		ID: 51, Callsign: "NAX101", Bay: shared.BAY_RWY_ARR,
		Destination: "EKCH",
		CdmData:     &models.CdmData{Aldt: &aldt},
	}
	svc, _, out := buildLandingDetectionSvc(t, strip, noCoordRepo())

	svc.handleArrivalPositionUpdate(context.Background(), landingTestSession, strip.Callsign, insideLat, insideLon, int64(lowAltitude), strip)

	assert.Nil(t, out.cdm, "SetCdmData must not be called when ALDT already recorded")
}

func TestHandleArrivalPositionUpdate_HighAltitudeInsideRunwayNoAldt(t *testing.T) {
	injectTestRunway(t)

	strip := &models.Strip{
		ID: 52, Callsign: "NAX102", Bay: shared.BAY_FINAL,
		Destination: "EKCH",
	}
	svc, _, out := buildLandingDetectionSvc(t, strip, noCoordRepo())

	// Inside runway polygon but too high — not a landing.
	svc.handleArrivalPositionUpdate(context.Background(), landingTestSession, strip.Callsign, insideLon, insideLat, int64(highAltitude), strip)

	assert.Nil(t, out.cdm, "ALDT must not be recorded when altitude is above threshold")
}

func TestHandleArrivalPositionUpdate_WrongRunwayNoAldt(t *testing.T) {
	// Region covers runways "12" and "30" — not "22L".
	injectTestRunway(t)

	rwy := "22L"
	strip := &models.Strip{
		ID: 53, Callsign: "NAX103", Bay: shared.BAY_FINAL,
		Destination: "EKCH", Runway: &rwy,
	}
	svc, _, out := buildLandingDetectionSvc(t, strip, noCoordRepo())

	// Position is inside the polygon, but region.Runways=["12","30"] doesn't contain "22L".
	svc.handleArrivalPositionUpdate(context.Background(), landingTestSession, strip.Callsign, insideLat, insideLon, int64(lowAltitude), strip)

	assert.Nil(t, out.cdm, "ALDT must not be recorded when assigned runway doesn't match the polygon's runway list")
}

func TestHandleArrivalPositionUpdate_MatchingRunwayRecordsAldt(t *testing.T) {
	// Region covers "22L" and "04R" — matches the strip's assigned runway.
	region := config.MakeTestRunwayRegion("RWY_22L04R", []string{"22L", "04R"}, testRunwayCoords)
	t.Cleanup(config.SetRunwayRegionsForTest([]config.Region{region}))

	rwy := "22L"
	strip := &models.Strip{
		ID: 54, Callsign: "NAX104", Bay: shared.BAY_FINAL,
		Destination: "EKCH", Runway: &rwy,
	}
	svc, _, out := buildLandingDetectionSvc(t, strip, noCoordRepo())

	svc.handleArrivalPositionUpdate(context.Background(), landingTestSession, strip.Callsign, insideLat, insideLon, int64(lowAltitude), strip)

	require.NotNil(t, out.cdm, "CdmData must be updated")
	assert.NotNil(t, out.cdm.Aldt, "ALDT must be set when assigned runway matches polygon")
}

// ---- TWY_ARR transition ----

func TestHandleArrivalPositionUpdate_MovesToTwyArrWhenExitingRunway(t *testing.T) {
	injectTestRunway(t)

	aldt := "1000"
	strip := &models.Strip{
		ID: 60, Callsign: "NAX200", Bay: shared.BAY_RWY_ARR,
		Destination: "EKCH",
		CdmData:     &models.CdmData{Aldt: &aldt},
	}
	svc, _, out := buildLandingDetectionSvc(t, strip, noCoordRepo())

	// Outside runway polygon, low altitude, ALDT already set → should move to TWY_ARR.
	svc.handleArrivalPositionUpdate(context.Background(), landingTestSession, strip.Callsign, outsideLat, outsideLon, int64(lowAltitude), strip)

	assert.Equal(t, shared.BAY_TWY_ARR, out.bay, "strip must move to TWY_ARR after vacating runway")
}

func TestHandleArrivalPositionUpdate_MovesToTwyArrFromFinal(t *testing.T) {
	injectTestRunway(t)

	aldt := "1000"
	strip := &models.Strip{
		ID: 61, Callsign: "NAX201", Bay: shared.BAY_FINAL,
		Destination: "EKCH",
		CdmData:     &models.CdmData{Aldt: &aldt},
	}
	svc, _, out := buildLandingDetectionSvc(t, strip, noCoordRepo())

	svc.handleArrivalPositionUpdate(context.Background(), landingTestSession, strip.Callsign, outsideLat, outsideLon, int64(lowAltitude), strip)

	assert.Equal(t, shared.BAY_TWY_ARR, out.bay)
}

func TestHandleArrivalPositionUpdate_DoesNotMoveTwyArrIfNotArrivalBay(t *testing.T) {
	injectTestRunway(t)

	aldt := "1000"
	strip := &models.Strip{
		ID: 62, Callsign: "NAX202", Bay: shared.BAY_TWY_ARR, // already in TWY_ARR
		Destination: "EKCH",
		CdmData:     &models.CdmData{Aldt: &aldt},
	}
	svc, _, out := buildLandingDetectionSvc(t, strip, noCoordRepo())

	svc.handleArrivalPositionUpdate(context.Background(), landingTestSession, strip.Callsign, outsideLat, outsideLon, int64(lowAltitude), strip)

	assert.Empty(t, out.bay, "strip already in TWY_ARR must not be moved again")
}

func TestHandleArrivalPositionUpdate_NoTwyArrIfNotYetLanded(t *testing.T) {
	injectTestRunway(t)

	// No ALDT set yet → aircraft hasn't landed → must not move to TWY_ARR.
	strip := &models.Strip{
		ID: 63, Callsign: "NAX203", Bay: shared.BAY_FINAL,
		Destination: "EKCH",
	}
	svc, _, out := buildLandingDetectionSvc(t, strip, noCoordRepo())

	svc.handleArrivalPositionUpdate(context.Background(), landingTestSession, strip.Callsign, outsideLat, outsideLon, int64(lowAltitude), strip)

	assert.Empty(t, out.bay, "strip must not move to TWY_ARR without ALDT (has not landed)")
}

// ---- auto-accept pending coordination on touchdown ----

func TestHandleArrivalPositionUpdate_AutoAcceptsCoordinationOnTouchdown(t *testing.T) {
	injectTestRunway(t)

	const fromPos = "119.805"
	const toPos = "118.105"
	strip := &models.Strip{
		ID: 70, Callsign: "NAX300", Bay: shared.BAY_FINAL,
		Destination: "EKCH",
		NextOwners:  []string{toPos},
	}
	coord := &models.Coordination{
		ID:           99,
		FromPosition: fromPos,
		ToPosition:   toPos,
	}

	var coordDeleted bool
	var ownerSet *string
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			return coord, nil
		},
		DeleteFn: func(_ context.Context, _ int32) error {
			coordDeleted = true
			return nil
		},
	}

	svc, hub, _ := buildLandingDetectionSvc(t, strip, coordRepo)
	// AcceptCoordination also calls GetByStripID via the server's coord repo.
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})

	// Capture SetOwner to verify the strip is assumed by toPos.
	svc.stripRepo.(*testutil.MockStripRepository).SetOwnerFn = func(_ context.Context, _ int32, _ string, o *string, _ int32) (int64, error) {
		ownerSet = o
		return 1, nil
	}

	svc.handleArrivalPositionUpdate(context.Background(), landingTestSession, strip.Callsign, insideLat, insideLon, int64(lowAltitude), strip)

	assert.True(t, coordDeleted, "coordination must be deleted (accepted) on touchdown")
	require.NotNil(t, ownerSet, "strip owner must be set")
	assert.Equal(t, toPos, *ownerSet, "strip must be assumed by the coordination's ToPosition")

	require.Len(t, hub.CoordinationAssumes, 1)
	assert.Equal(t, toPos, hub.CoordinationAssumes[0].Position)
}

func TestHandleArrivalPositionUpdate_AutoAcceptsEsArrivalCoordinationWithAssumeOnly(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_W_APP", Frequency: "119.805", Section: "APP"},
		{Name: "EKCH_M_TWR", Frequency: "118.105", Section: "TWR"},
	}))
	injectTestRunway(t)

	const fromPos = "119.805"
	const toPos = "118.105"
	const handoverCid = "CID-TWR"

	strip := &models.Strip{
		ID:          71,
		Callsign:    "NAX301",
		Bay:         shared.BAY_FINAL,
		Destination: "EKCH",
		NextOwners:  []string{toPos},
		Version:     1,
	}
	coord := &models.Coordination{
		ID:           101,
		FromPosition: fromPos,
		ToPosition:   toPos,
		FromEs:       true,
		EsHandoverCid: func() *string {
			cid := handoverCid
			return &cid
		}(),
	}

	cur := *strip
	coordExists := true

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			if coordExists {
				return coord, nil
			}
			return nil, pgx.ErrNoRows
		},
		DeleteFn: func(_ context.Context, _ int32) error {
			coordExists = false
			return nil
		},
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			copy := cur
			return &copy, nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			cp := *data
			cur.CdmData = &cp
			return 1, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, next []string, prev []string) error {
			cur.NextOwners = next
			cur.PreviousOwners = prev
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, owner *string, _ int32) (int64, error) {
			cur.Owner = owner
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})
	esHub := &testutil.MockEuroscopeHub{}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)
	svc.SetEuroscopeHub(esHub)

	svc.handleArrivalPositionUpdate(context.Background(), landingTestSession, strip.Callsign, insideLat, insideLon, int64(lowAltitude), strip)

	require.Len(t, esHub.AssumeOnlys, 1)
	assert.Equal(t, handoverCid, esHub.AssumeOnlys[0].Cid)
	assert.Equal(t, strip.Callsign, esHub.AssumeOnlys[0].Callsign)
	assert.Empty(t, esHub.AssumeAndDrops, "arrival APP->TWR touchdown must not drop tracking in ES")
}

func TestAssumeStripCoordination_EsArrivalUsesAssumeOnly(t *testing.T) {
	const session = int32(1)
	const callsign = "NAX302"
	const toPos = "118.105"
	const handoverCid = "CID-TWR"

	strip := &models.Strip{
		ID:         72,
		Callsign:   callsign,
		Bay:        shared.BAY_FINAL,
		Version:    1,
		NextOwners: []string{toPos},
	}
	coord := &models.Coordination{
		ID:           102,
		Session:      session,
		StripID:      strip.ID,
		FromPosition: "",
		ToPosition:   toPos,
		FromEs:       true,
		EsHandoverCid: func() *string {
			cid := handoverCid
			return &cid
		}(),
	}

	cur := *strip

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			copy := cur
			return &copy, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, next []string, prev []string) error {
			cur.NextOwners = next
			cur.PreviousOwners = prev
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, owner *string, _ int32) (int64, error) {
			cur.Owner = owner
			return 1, nil
		},
	}

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			return coord, nil
		},
		DeleteFn: func(_ context.Context, _ int32) error {
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{CoordRepoVal: coordRepo})
	esHub := &testutil.MockEuroscopeHub{}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)
	svc.SetEuroscopeHub(esHub)

	require.NoError(t, svc.AssumeStripCoordination(context.Background(), session, callsign, toPos))

	require.Len(t, esHub.AssumeOnlys, 1)
	assert.Equal(t, handoverCid, esHub.AssumeOnlys[0].Cid)
	assert.Equal(t, callsign, esHub.AssumeOnlys[0].Callsign)
	assert.Empty(t, esHub.AssumeAndDrops, "manual FS assume for ES arrival must not drop tracking in ES")
}

func TestHandleArrivalPositionUpdate_DropsTrackingInEsOnTouchdownForAirborneOwner(t *testing.T) {
	injectTestRunway(t)
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_W_APP", Frequency: "119.805", Section: "APP"},
	}))

	const ownerPos = "119.805"
	const ownerCid = "CID-APP"

	strip := &models.Strip{
		ID:          80,
		Callsign:    "NAX400",
		Bay:         shared.BAY_FINAL,
		Destination: "EKCH",
		Owner:       strPtr(ownerPos),
	}

	stripRepo := &testutil.MockStripRepository{
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, _ *models.CdmData) (int64, error) { return 1, nil },
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			copy := *strip
			return &copy, nil
		},
	}

	controllerRepo := &testutil.MockControllerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
			return []*models.Controller{{Position: ownerPos, Cid: strPtr(ownerCid)}}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	esHub := &testutil.MockEuroscopeHub{}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetControllerRepo(controllerRepo)
	svc.SetEuroscopeHub(esHub)

	svc.handleArrivalPositionUpdate(context.Background(), 1, strip.Callsign, insideLat, insideLon, int64(lowAltitude), strip)

	require.Len(t, esHub.DropTrackings, 1)
	assert.Equal(t, ownerCid, esHub.DropTrackings[0].Cid)
	assert.Equal(t, strip.Callsign, esHub.DropTrackings[0].Callsign)
}

func TestMoveToBay_TwyArrDoesNotDropTrackingInEs(t *testing.T) {
	const ownerPos = "118.105"
	const ownerCid = "CID-TWR"

	cur := models.Strip{
		ID:          81,
		Callsign:    "NAX401",
		Bay:         shared.BAY_RWY_ARR,
		Destination: "EKCH",
		Owner:       strPtr(ownerPos),
	}

	stripRepo := &testutil.MockStripRepository{
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			cur.Bay = bay
			return 1, nil
		},
	}

	controllerRepo := &testutil.MockControllerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
			return []*models.Controller{{Position: ownerPos, Cid: strPtr(ownerCid)}}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	esHub := &testutil.MockEuroscopeHub{}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetControllerRepo(controllerRepo)
	svc.SetEuroscopeHub(esHub)

	require.NoError(t, svc.MoveToBay(context.Background(), 1, cur.Callsign, shared.BAY_TWY_ARR, true))
	assert.Empty(t, esHub.DropTrackings, "TWY_ARR transition must no longer drop tracking; touchdown/ALDT handles it")
}
