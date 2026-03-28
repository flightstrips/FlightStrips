package services

import (
	"context"
	"testing"

	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	pkgModels "FlightStrips/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helpers for pointer values
func strPtr(s string) *string { return &s }

// buildMissedApproachSvc wires up a StripService with all the mocks needed
// for MissedApproach tests. The caller provides a strip and a list of online
// controllers; session 1 is configured with runway "04L" as the arrival runway.
func buildMissedApproachSvc(
	t *testing.T,
	strip *models.Strip,
	controllers []*models.Controller,
	esHub *testutil.MockEuroscopeHub,
) (*StripService, *testutil.MockFrontendHub, *testutil.MockCoordinationRepository) {
	t.Helper()

	coordRepo := &testutil.MockCoordinationRepository{
		CreateFn: func(_ context.Context, _ *models.Coordination) error { return nil },
	}

	mockServer := &testutil.MockServer{
		SessionRepoVal: &testutil.MockSessionRepository{
			GetByIDFn: func(_ context.Context, _ int32) (*models.Session, error) {
				return &models.Session{
					ID: 1,
					ActiveRunways: pkgModels.ActiveRunways{
						ArrivalRunways: []string{"04L"},
					},
				}, nil
			},
		},
		ControllerRepoVal: &testutil.MockControllerRepository{
			ListBySessionFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
				return controllers, nil
			},
		},
		CoordRepoVal: coordRepo,
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return strip, nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			return 1, nil
		},
	})
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)
	if esHub != nil {
		svc.SetEuroscopeHub(esHub)
	}

	return svc, hub, coordRepo
}

// TestMissedApproach_WrongBay rejects strips not in FINAL or RWY_ARR.
func TestMissedApproach_WrongBay(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{ID: 1, Callsign: "SAS001", Bay: shared.BAY_DEPART, Owner: strPtr("118.105")}, nil
		},
	})

	err := svc.MissedApproach(context.Background(), 1, "SAS001", "118.105")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in FINAL or RWY_ARR")
}

// TestMissedApproach_WrongOwner rejects when the requesting position does not own the strip.
func TestMissedApproach_WrongOwner(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{ID: 1, Callsign: "SAS001", Bay: shared.BAY_FINAL, Owner: strPtr("118.105")}, nil
		},
	})

	err := svc.MissedApproach(context.Background(), 1, "SAS001", "119.000")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not owned by you")
}

// TestMissedApproach_NoRunwayMapping errors when no arrival runway has a handover mapping.
func TestMissedApproach_NoRunwayMapping(t *testing.T) {
	// No missed_approach_handover entries → toPositionName stays empty.
	t.Cleanup(config.SetMissedApproachHandoverForTest(map[string]string{}))
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_W_APP", Frequency: "119.805"},
	}))

	mockServer := &testutil.MockServer{
		SessionRepoVal: &testutil.MockSessionRepository{
			GetByIDFn: func(_ context.Context, _ int32) (*models.Session, error) {
				return &models.Session{
					ID:            1,
					ActiveRunways: pkgModels.ActiveRunways{ArrivalRunways: []string{"04L"}},
				}, nil
			},
		},
	}
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{ID: 1, Bay: shared.BAY_FINAL, Owner: strPtr("118.105")}, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.MissedApproach(context.Background(), 1, "SAS001", "118.105")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no approach controller configured")
}

// TestMissedApproach_ConfiguredControllerOnline sends the ES handover directly to the
// configured APP controller when they are online.
func TestMissedApproach_ConfiguredControllerOnline(t *testing.T) {
	t.Cleanup(config.SetMissedApproachHandoverForTest(map[string]string{"04L": "EKCH_O_APP"}))
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105"},
		{Name: "EKCH_O_APP", Frequency: "118.455"},
		{Name: "EKCH_W_APP", Frequency: "119.805"},
	}))
	t.Cleanup(config.SetAirborneOwnersForTest([]string{"EKCH_O_APP", "EKCH_W_APP"}))

	twr := strPtr("1001")
	controllers := []*models.Controller{
		{Callsign: "EKCH_M_TWR", Position: "118.105", Cid: twr},
		{Callsign: "EKCH_O_APP", Position: "118.455", Cid: strPtr("1002")},
	}

	strip := &models.Strip{ID: 10, Callsign: "SAS001", Bay: shared.BAY_FINAL, Owner: strPtr("118.105")}
	esHub := &testutil.MockEuroscopeHub{}
	svc, hub, _ := buildMissedApproachSvc(t, strip, controllers, esHub)

	err := svc.MissedApproach(context.Background(), 1, "SAS001", "118.105")
	require.NoError(t, err)

	// ES handover must go to the configured APP controller.
	require.Len(t, esHub.CoordinationHandovers, 1)
	assert.Equal(t, "1001", esHub.CoordinationHandovers[0].Cid)
	assert.Equal(t, "EKCH_O_APP", esHub.CoordinationHandovers[0].TargetCallsign)

	// Frontend coordination transfer must be emitted.
	require.Len(t, hub.CoordinationTransfers, 1)
	assert.Equal(t, "118.455", hub.CoordinationTransfers[0].To)
}

// TestMissedApproach_FallbackToHigherPriorityApp falls back to the first online controller
// in the airborne_owners list when the runway-configured controller is offline.
func TestMissedApproach_FallbackToHigherPriorityApp(t *testing.T) {
	// 04L → EKCH_O_APP, but EKCH_O_APP is offline; EKCH_W_APP is online.
	t.Cleanup(config.SetMissedApproachHandoverForTest(map[string]string{"04L": "EKCH_O_APP"}))
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105"},
		{Name: "EKCH_O_APP", Frequency: "118.455"},
		{Name: "EKCH_W_APP", Frequency: "119.805"},
		{Name: "EKCH_E_DEP", Frequency: "121.800"},
	}))
	t.Cleanup(config.SetAirborneOwnersForTest([]string{"EKCH_W_APP", "EKCH_O_APP", "EKCH_E_DEP"}))

	twr := strPtr("1001")
	// Only EKCH_W_APP is online (EKCH_O_APP absent).
	controllers := []*models.Controller{
		{Callsign: "EKCH_M_TWR", Position: "118.105", Cid: twr},
		{Callsign: "EKCH_W_APP", Position: "119.805", Cid: strPtr("2001")},
	}

	strip := &models.Strip{ID: 10, Callsign: "SAS001", Bay: shared.BAY_RWY_ARR, Owner: strPtr("118.105")}
	esHub := &testutil.MockEuroscopeHub{}
	svc, hub, _ := buildMissedApproachSvc(t, strip, controllers, esHub)

	err := svc.MissedApproach(context.Background(), 1, "SAS001", "118.105")
	require.NoError(t, err)

	// Must fall back to EKCH_W_APP, the first online entry in airborne_owners.
	require.Len(t, esHub.CoordinationHandovers, 1)
	assert.Equal(t, "1001", esHub.CoordinationHandovers[0].Cid)
	assert.Equal(t, "EKCH_W_APP", esHub.CoordinationHandovers[0].TargetCallsign)

	// Coordination transfer must use the fallback frequency.
	require.Len(t, hub.CoordinationTransfers, 1)
	assert.Equal(t, "119.805", hub.CoordinationTransfers[0].To)
}

// TestMissedApproach_FallbackToDepController falls back all the way to a DEP controller
// when no APP controllers are online.
func TestMissedApproach_FallbackToDepController(t *testing.T) {
	t.Cleanup(config.SetMissedApproachHandoverForTest(map[string]string{"04L": "EKCH_O_APP"}))
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105"},
		{Name: "EKCH_O_APP", Frequency: "118.455"},
		{Name: "EKCH_W_APP", Frequency: "119.805"},
		{Name: "EKCH_E_DEP", Frequency: "121.800"},
	}))
	t.Cleanup(config.SetAirborneOwnersForTest([]string{"EKCH_O_APP", "EKCH_W_APP", "EKCH_E_DEP"}))

	twr := strPtr("1001")
	// Only the DEP controller is online.
	controllers := []*models.Controller{
		{Callsign: "EKCH_M_TWR", Position: "118.105", Cid: twr},
		{Callsign: "EKCH_E_DEP", Position: "121.800", Cid: strPtr("3001")},
	}

	strip := &models.Strip{ID: 10, Callsign: "SAS001", Bay: shared.BAY_FINAL, Owner: strPtr("118.105")}
	esHub := &testutil.MockEuroscopeHub{}
	svc, _, _ := buildMissedApproachSvc(t, strip, controllers, esHub)

	err := svc.MissedApproach(context.Background(), 1, "SAS001", "118.105")
	require.NoError(t, err)

	require.Len(t, esHub.CoordinationHandovers, 1)
	assert.Equal(t, "EKCH_E_DEP", esHub.CoordinationHandovers[0].TargetCallsign)
}

// TestMissedApproach_NilEuroscopeHub creates the coordination record but skips the ES handover.
func TestMissedApproach_NilEuroscopeHub(t *testing.T) {
	t.Cleanup(config.SetMissedApproachHandoverForTest(map[string]string{"04L": "EKCH_W_APP"}))
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105"},
		{Name: "EKCH_W_APP", Frequency: "119.805"},
	}))
	t.Cleanup(config.SetAirborneOwnersForTest([]string{"EKCH_W_APP"}))

	controllers := []*models.Controller{
		{Callsign: "EKCH_M_TWR", Position: "118.105", Cid: strPtr("1001")},
		{Callsign: "EKCH_W_APP", Position: "119.805", Cid: strPtr("2001")},
	}

	strip := &models.Strip{ID: 10, Callsign: "SAS001", Bay: shared.BAY_FINAL, Owner: strPtr("118.105")}
	// Pass nil esHub.
	svc, hub, _ := buildMissedApproachSvc(t, strip, controllers, nil)

	err := svc.MissedApproach(context.Background(), 1, "SAS001", "118.105")
	require.NoError(t, err)

	// Coordination transfer must still be sent to the frontend.
	require.Len(t, hub.CoordinationTransfers, 1)
}

// TestMissedApproach_OwnerHasNoCID creates the coordination but skips the ES handover
// because the owning controller has no CID.
func TestMissedApproach_OwnerHasNoCID(t *testing.T) {
	t.Cleanup(config.SetMissedApproachHandoverForTest(map[string]string{"04L": "EKCH_W_APP"}))
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_M_TWR", Frequency: "118.105"},
		{Name: "EKCH_W_APP", Frequency: "119.805"},
	}))
	t.Cleanup(config.SetAirborneOwnersForTest([]string{"EKCH_W_APP"}))

	controllers := []*models.Controller{
		// TWR has no CID set.
		{Callsign: "EKCH_M_TWR", Position: "118.105", Cid: nil},
		{Callsign: "EKCH_W_APP", Position: "119.805", Cid: strPtr("2001")},
	}

	strip := &models.Strip{ID: 10, Callsign: "SAS001", Bay: shared.BAY_FINAL, Owner: strPtr("118.105")}
	esHub := &testutil.MockEuroscopeHub{}
	svc, hub, _ := buildMissedApproachSvc(t, strip, controllers, esHub)

	err := svc.MissedApproach(context.Background(), 1, "SAS001", "118.105")
	require.NoError(t, err)

	// No ES handover because the owner CID is unknown.
	assert.Empty(t, esHub.CoordinationHandovers)

	// But the frontend coordination transfer must still happen.
	require.Len(t, hub.CoordinationTransfers, 1)
}
