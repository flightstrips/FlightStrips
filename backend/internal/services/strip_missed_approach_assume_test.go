package services

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

// missedAssumeResult holds the observable side-effects of an AssumeStripCoordination
// call on a missed-approach strip.
type missedAssumeResult struct {
	movedToBay       string
	setPrevOwners    []string // last value passed to SetPreviousOwners
	routeRecalcCount int      // number of UpdateRouteForStrip calls
}

// buildAssumeAfterMissedSvc wires a StripService for AssumeStripCoordination tests
// that cover the missed-approach path. The strip state is kept mutable so that
// applyMissedApproachOwnerFix re-fetches the correct owners after AcceptCoordination.
func buildAssumeAfterMissedSvc(
	t *testing.T,
	strip *models.Strip,
	coord *models.Coordination,
) (*StripService, *testutil.MockFrontendHub, *missedAssumeResult) {
	t.Helper()

	res := &missedAssumeResult{}

	// Keep a mutable copy so SetNextAndPreviousOwners/SetPreviousOwners are visible on re-fetch.
	cur := *strip

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			return coord, nil
		},
		DeleteFn: func(_ context.Context, _ int32) error { return nil },
	}

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
		SetPreviousOwnersFn: func(_ context.Context, _ int32, _ string, prev []string) error {
			res.setPrevOwners = prev
			cur.PreviousOwners = prev
			return nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			res.movedToBay = bay
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{
		CoordRepoVal: coordRepo,
		UpdateRouteForStripFn: func(_ string, _ int32, _ bool) error {
			res.routeRecalcCount++
			return nil
		},
	})

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)

	return svc, hub, res
}

// TestAssumeAfterMissedApproach_MovesToFinal verifies that when an APP controller
// assumes an AIRBORNE strip transferred from TWR, the strip moves back to FINAL.
func TestAssumeAfterMissedApproach_MovesToFinal(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105", Section: "TWR"},
		{Name: "EKCH_W_APP", Frequency: "119.805", Section: "APP"},
	}))

	strip := &models.Strip{
		ID:             10,
		Callsign:       "SAS001",
		Bay:            shared.BAY_AIRBORNE,
		Version:        1,
		NextOwners:     []string{"119.805"},
		PreviousOwners: []string{},
	}
	coord := &models.Coordination{
		ID: 1, Session: 1, StripID: strip.ID,
		FromPosition: "118.105", ToPosition: "119.805",
	}

	svc, _, res := buildAssumeAfterMissedSvc(t, strip, coord)
	require.NoError(t, svc.AssumeStripCoordination(context.Background(), 1, "SAS001", "119.805"))

	assert.Equal(t, shared.BAY_FINAL, res.movedToBay)
}

// TestAssumeAfterMissedApproach_TWRRemovedFromPreviousOwners verifies that after the
// missed-approach assume, TWR is not left in PreviousOwners.
func TestAssumeAfterMissedApproach_TWRRemovedFromPreviousOwners(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105", Section: "TWR"},
		{Name: "EKCH_W_APP", Frequency: "119.805", Section: "APP"},
	}))

	strip := &models.Strip{
		ID:             10,
		Callsign:       "SAS001",
		Bay:            shared.BAY_AIRBORNE,
		Version:        1,
		NextOwners:     []string{"119.805"},
		PreviousOwners: []string{},
	}
	coord := &models.Coordination{
		ID: 1, Session: 1, StripID: strip.ID,
		FromPosition: "118.105", ToPosition: "119.805",
	}

	svc, _, res := buildAssumeAfterMissedSvc(t, strip, coord)
	require.NoError(t, svc.AssumeStripCoordination(context.Background(), 1, "SAS001", "119.805"))

	// AcceptCoordination adds TWR; applyMissedApproachOwnerFix must remove it again.
	assert.NotContains(t, res.setPrevOwners, "118.105", "TWR must not be in previous owners after missed approach assume")
}

// TestAssumeAfterMissedApproach_RouteRecalculated verifies that UpdateRouteForStrip
// is called after the missed-approach assume so NextOwners are refreshed.
func TestAssumeAfterMissedApproach_RouteRecalculated(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105", Section: "TWR"},
		{Name: "EKCH_W_APP", Frequency: "119.805", Section: "APP"},
	}))

	strip := &models.Strip{
		ID:             10,
		Callsign:       "SAS001",
		Bay:            shared.BAY_AIRBORNE,
		Version:        1,
		NextOwners:     []string{"119.805"},
		PreviousOwners: []string{},
	}
	coord := &models.Coordination{
		ID: 1, Session: 1, StripID: strip.ID,
		FromPosition: "118.105", ToPosition: "119.805",
	}

	svc, _, res := buildAssumeAfterMissedSvc(t, strip, coord)
	require.NoError(t, svc.AssumeStripCoordination(context.Background(), 1, "SAS001", "119.805"))

	assert.Equal(t, 1, res.routeRecalcCount, "UpdateRouteForStrip must be called once after missed approach assume")
}

// TestAssumeAfterMissedApproach_AppToAppDoesNotAutoMove verifies that an APP-to-APP
// handover on an AIRBORNE strip does NOT trigger the missed-approach path.
func TestAssumeAfterMissedApproach_AppToAppDoesNotAutoMove(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_O_APP", Frequency: "118.455", Section: "APP"},
		{Name: "EKCH_W_APP", Frequency: "119.805", Section: "APP"},
	}))

	strip := &models.Strip{
		ID: 11, Callsign: "SAS002", Bay: shared.BAY_AIRBORNE, Version: 1,
		NextOwners: []string{"119.805"}, PreviousOwners: []string{},
	}
	coord := &models.Coordination{
		ID: 2, Session: 1, StripID: strip.ID,
		FromPosition: "118.455", ToPosition: "119.805",
	}

	svc, _, res := buildAssumeAfterMissedSvc(t, strip, coord)
	require.NoError(t, svc.AssumeStripCoordination(context.Background(), 1, "SAS002", "119.805"))

	assert.Empty(t, res.movedToBay, "APP-to-APP handover must not trigger auto-move")
	assert.Zero(t, res.routeRecalcCount)
}

// TestAssumeAfterMissedApproach_NonAirborneBayDoesNotAutoMove verifies that a TWR→APP
// coordination on a non-AIRBORNE strip does not trigger the auto-move.
func TestAssumeAfterMissedApproach_NonAirborneBayDoesNotAutoMove(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105", Section: "TWR"},
		{Name: "EKCH_W_APP", Frequency: "119.805", Section: "APP"},
	}))

	strip := &models.Strip{
		ID: 12, Callsign: "SAS003", Bay: shared.BAY_FINAL, Version: 1,
		NextOwners: []string{"119.805"}, PreviousOwners: []string{},
	}
	coord := &models.Coordination{
		ID: 3, Session: 1, StripID: strip.ID,
		FromPosition: "118.105", ToPosition: "119.805",
	}

	svc, _, res := buildAssumeAfterMissedSvc(t, strip, coord)
	require.NoError(t, svc.AssumeStripCoordination(context.Background(), 1, "SAS003", "119.805"))

	assert.Empty(t, res.movedToBay, "non-AIRBORNE strip must not be auto-moved")
	assert.Zero(t, res.routeRecalcCount)
}

// TestAssumeAfterMissedApproach_UnknownPositionDoesNotAutoMove verifies that when
// frequencies are not in config the auto-move is skipped gracefully.
func TestAssumeAfterMissedApproach_UnknownPositionDoesNotAutoMove(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{}))

	strip := &models.Strip{
		ID: 13, Callsign: "SAS004", Bay: shared.BAY_AIRBORNE, Version: 1,
		NextOwners: []string{"119.805"}, PreviousOwners: []string{},
	}
	coord := &models.Coordination{
		ID: 4, Session: 1, StripID: strip.ID,
		FromPosition: "118.105", ToPosition: "119.805",
	}

	svc, _, res := buildAssumeAfterMissedSvc(t, strip, coord)
	require.NoError(t, svc.AssumeStripCoordination(context.Background(), 1, "SAS004", "119.805"))

	assert.Empty(t, res.movedToBay, "unknown positions must not trigger auto-move")
	assert.Zero(t, res.routeRecalcCount)
}

// ---- HandleTrackingControllerChanged auto-hide bay tests ----

// trackingChangedResult holds observable side-effects of HandleTrackingControllerChanged.
type trackingChangedResult struct {
	updateBayArg     string
	moveToBayArg     string
	setPrevOwners    []string
	routeRecalcCount int
}

// buildTrackingChangedSvc wires a StripService for HandleTrackingControllerChanged tests.
func buildTrackingChangedSvc(
	t *testing.T,
	strip *models.Strip,
	airport string,
) (*StripService, *trackingChangedResult) {
	t.Helper()

	res := &trackingChangedResult{}
	cur := *strip

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			return nil, pgx.ErrNoRows
		},
	}

	stripRepo := &testutil.MockStripRepository{
		UpdateTrackingControllerFn: func(_ context.Context, _ int32, _ string, _ string) (int64, error) {
			return 1, nil
		},
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			copy := cur
			return &copy, nil
		},
		UpdateBayFn: func(_ context.Context, _ int32, _ string, bay string, _ *int32) (int64, error) {
			res.updateBayArg = bay
			return 1, nil
		},
		SetPreviousOwnersFn: func(_ context.Context, _ int32, _ string, prev []string) error {
			res.setPrevOwners = prev
			cur.PreviousOwners = prev
			return nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			res.moveToBayArg = bay
			return 1, nil
		},
	}

	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return nil, pgx.ErrNoRows
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{
		CoordRepoVal: coordRepo,
		SessionRepoVal: &testutil.MockSessionRepository{
			GetByIDFn: func(_ context.Context, _ int32) (*models.Session, error) {
				return &models.Session{ID: 1, Airport: airport}, nil
			},
		},
		UpdateRouteForStripFn: func(_ string, _ int32, _ bool) error {
			res.routeRecalcCount++
			return nil
		},
	})

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)
	svc.SetControllerRepo(controllerRepo)

	return svc, res
}

// buildTrackingChangedSvcWithCoord is like buildTrackingChangedSvc but with a pending
// coordination — used to test the missed-approach TWR→APP path.
func buildTrackingChangedSvcWithCoord(
	t *testing.T,
	strip *models.Strip,
	coord *models.Coordination,
	airport string,
) (*StripService, *trackingChangedResult) {
	t.Helper()

	res := &trackingChangedResult{}
	cur := *strip

	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, _ int32, _ int32) (*models.Coordination, error) {
			return coord, nil
		},
		DeleteFn: func(_ context.Context, _ int32) error { return nil },
	}

	stripRepo := &testutil.MockStripRepository{
		UpdateTrackingControllerFn: func(_ context.Context, _ int32, _ string, _ string) (int64, error) {
			return 1, nil
		},
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
		UpdateBayFn: func(_ context.Context, _ int32, _ string, bay string, _ *int32) (int64, error) {
			res.updateBayArg = bay
			return 1, nil
		},
		SetPreviousOwnersFn: func(_ context.Context, _ int32, _ string, prev []string) error {
			res.setPrevOwners = prev
			cur.PreviousOwners = prev
			return nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			res.moveToBayArg = bay
			return 1, nil
		},
	}

	appController := &models.Controller{Callsign: coord.ToPosition, Position: coord.ToPosition}

	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Controller, error) {
			if callsign == coord.ToPosition {
				return appController, nil
			}
			return nil, pgx.ErrNoRows
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{
		CoordRepoVal: coordRepo,
		SessionRepoVal: &testutil.MockSessionRepository{
			GetByIDFn: func(_ context.Context, _ int32) (*models.Session, error) {
				return &models.Session{ID: 1, Airport: airport}, nil
			},
		},
		UpdateRouteForStripFn: func(_ string, _ int32, _ bool) error {
			res.routeRecalcCount++
			return nil
		},
	})

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)
	svc.SetControllerRepo(controllerRepo)

	return svc, res
}

type coordinationReceivedResult struct {
	updatedBay string
	movedToBay string
	created    *models.Coordination
}

func buildCoordinationReceivedSvc(
	t *testing.T,
	strip *models.Strip,
	controller *models.Controller,
) (*StripService, *testutil.MockFrontendHub, *coordinationReceivedResult) {
	t.Helper()

	res := &coordinationReceivedResult{}
	cur := *strip

	coordRepo := &testutil.MockCoordinationRepository{
		CreateFn: func(_ context.Context, coordination *models.Coordination) error {
			copy := *coordination
			res.created = &copy
			return nil
		},
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			copy := cur
			return &copy, nil
		},
		UpdateBayFn: func(_ context.Context, _ int32, _ string, bay string, _ *int32) (int64, error) {
			res.updatedBay = bay
			cur.Bay = bay
			return 1, nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			res.movedToBay = bay
			cur.Bay = bay
			return 1, nil
		},
	}

	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Controller, error) {
			if callsign != controller.Callsign {
				return nil, pgx.ErrNoRows
			}
			return controller, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{
		CoordRepoVal: coordRepo,
	})

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	svc.SetControllerRepo(controllerRepo)
	svc.SetCoordinationRepo(coordRepo)

	return svc, hub, res
}

// TestHandleTrackingControllerChanged_ArrivalGoesToArrHidden verifies that an AIRBORNE
// arrival strip is moved to ARR_HIDDEN when the ES tracking controller changes.
func TestHandleTrackingControllerChanged_ArrivalGoesToArrHidden(t *testing.T) {
	strip := &models.Strip{ID: 20, Callsign: "THA951", Bay: shared.BAY_AIRBORNE, Destination: "EKCH"}
	svc, res := buildTrackingChangedSvc(t, strip, "EKCH")

	require.NoError(t, svc.HandleTrackingControllerChanged(context.Background(), 1, "THA951", "EKCH_W_APP"))
	assert.Equal(t, shared.BAY_ARR_HIDDEN, res.updateBayArg)
	assert.Equal(t, shared.BAY_ARR_HIDDEN, res.moveToBayArg)
}

// TestHandleTrackingControllerChanged_MissedApproachGoesToFinal verifies that an
// AIRBORNE missed-approach strip returns to FINAL when APP assumes it in ES.
func TestHandleTrackingControllerChanged_MissedApproachGoesToFinal(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105", Section: "TWR"},
		{Name: "EKCH_W_APP", Frequency: "119.805", Section: "APP"},
	}))

	strip := &models.Strip{
		ID: 24, Callsign: "THA952", Bay: shared.BAY_AIRBORNE,
		Destination: "EKCH", PreviousOwners: []string{},
	}
	coord := &models.Coordination{
		ID: 7, Session: 1, StripID: strip.ID,
		FromPosition: "118.105", ToPosition: "119.805",
	}

	svc, res := buildTrackingChangedSvcWithCoord(t, strip, coord, "EKCH")
	require.NoError(t, svc.HandleTrackingControllerChanged(context.Background(), 1, "THA952", "119.805"))

	assert.Equal(t, shared.BAY_FINAL, res.updateBayArg)
	assert.Equal(t, shared.BAY_FINAL, res.moveToBayArg)
}

// TestHandleTrackingControllerChanged_DepartureGoesToHidden verifies that an AIRBORNE
// departure strip is still moved to the regular HIDDEN bay.
func TestHandleTrackingControllerChanged_DepartureGoesToHidden(t *testing.T) {
	strip := &models.Strip{ID: 21, Callsign: "SAS001", Bay: shared.BAY_AIRBORNE, Destination: "ENGM"}
	svc, res := buildTrackingChangedSvc(t, strip, "EKCH")

	require.NoError(t, svc.HandleTrackingControllerChanged(context.Background(), 1, "SAS001", "EKDK_B_CTR"))
	assert.Equal(t, shared.BAY_HIDDEN, res.updateBayArg)
	assert.Equal(t, shared.BAY_HIDDEN, res.moveToBayArg)
}

// TestHandleTrackingControllerChanged_ArrivalTWRRemovedFromPreviousOwners verifies that
// when a missed-approach arrival returns to FINAL, TWR is removed from PreviousOwners.
func TestHandleTrackingControllerChanged_ArrivalTWRRemovedFromPreviousOwners(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105", Section: "TWR"},
		{Name: "EKCH_W_APP", Frequency: "119.805", Section: "APP"},
	}))

	const twr = "118.105"
	strip := &models.Strip{
		ID: 22, Callsign: "THA951", Bay: shared.BAY_AIRBORNE,
		Destination: "EKCH", PreviousOwners: []string{},
	}
	coord := &models.Coordination{
		ID: 5, Session: 1, StripID: strip.ID,
		FromPosition: twr, ToPosition: "119.805",
	}

	svc, res := buildTrackingChangedSvcWithCoord(t, strip, coord, "EKCH")
	require.NoError(t, svc.HandleTrackingControllerChanged(context.Background(), 1, "THA951", "119.805"))

	assert.NotContains(t, res.setPrevOwners, twr, "TWR must not be in previous owners after missed approach ES assume")
}

// TestHandleTrackingControllerChanged_ArrivalRouteRecalculated verifies that
// UpdateRouteForStrip is called after the missed-approach ES assume.
func TestHandleTrackingControllerChanged_ArrivalRouteRecalculated(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105", Section: "TWR"},
		{Name: "EKCH_W_APP", Frequency: "119.805", Section: "APP"},
	}))

	strip := &models.Strip{
		ID: 23, Callsign: "THA951", Bay: shared.BAY_AIRBORNE,
		Destination: "EKCH", PreviousOwners: []string{},
	}
	coord := &models.Coordination{
		ID: 6, Session: 1, StripID: strip.ID,
		FromPosition: "118.105", ToPosition: "119.805",
	}

	svc, res := buildTrackingChangedSvcWithCoord(t, strip, coord, "EKCH")
	require.NoError(t, svc.HandleTrackingControllerChanged(context.Background(), 1, "THA951", "119.805"))

	assert.Equal(t, 1, res.routeRecalcCount, "UpdateRouteForStrip must be called once after missed approach ES assume")
}

func TestHandleCoordinationReceived_ArrHiddenMovesToFinalAndStartsTransfer(t *testing.T) {
	strip := &models.Strip{
		ID: 30, Callsign: "SAS005", Bay: shared.BAY_ARR_HIDDEN,
		Owner: strPtr("119.805"),
	}
	controller := &models.Controller{Callsign: "EKCH_M_TWR", Position: "118.105"}

	svc, hub, res := buildCoordinationReceivedSvc(t, strip, controller)
	require.NoError(t, svc.HandleCoordinationReceived(context.Background(), 1, "SAS005", "EKCH_M_TWR"))

	assert.Equal(t, shared.BAY_FINAL, res.updatedBay)
	assert.Equal(t, shared.BAY_FINAL, res.movedToBay)
	require.NotNil(t, res.created)
	assert.Equal(t, "119.805", res.created.FromPosition)
	assert.Equal(t, "118.105", res.created.ToPosition)
	require.Len(t, hub.CoordinationTransfers, 1)
	assert.Equal(t, "119.805", hub.CoordinationTransfers[0].From)
	assert.Equal(t, "118.105", hub.CoordinationTransfers[0].To)
}

func TestHandleCoordinationReceived_FinalStartsTransferWithoutMovingBay(t *testing.T) {
	strip := &models.Strip{
		ID: 31, Callsign: "SAS006", Bay: shared.BAY_FINAL,
		Owner: strPtr("119.805"),
	}
	controller := &models.Controller{Callsign: "EKCH_M_TWR", Position: "118.105"}

	svc, hub, res := buildCoordinationReceivedSvc(t, strip, controller)
	require.NoError(t, svc.HandleCoordinationReceived(context.Background(), 1, "SAS006", "EKCH_M_TWR"))

	assert.Empty(t, res.updatedBay)
	assert.Empty(t, res.movedToBay)
	require.NotNil(t, res.created)
	assert.Equal(t, "119.805", res.created.FromPosition)
	assert.Equal(t, "118.105", res.created.ToPosition)
	require.Len(t, hub.CoordinationTransfers, 1)
	assert.Equal(t, "119.805", hub.CoordinationTransfers[0].From)
	assert.Equal(t, "118.105", hub.CoordinationTransfers[0].To)
}
