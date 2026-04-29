package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type syncRouteComputerTestServer struct {
	*testutil.MockServer
	computeNextOwnersFn func(ctx context.Context, strip *models.Strip, sessionId int32) ([]string, bool, error)
}

func (s *syncRouteComputerTestServer) ComputeNextOwnersForStripContext(ctx context.Context, strip *models.Strip, sessionId int32) ([]string, bool, error) {
	if s.computeNextOwnersFn == nil {
		return nil, false, nil
	}
	return s.computeNextOwnersFn(ctx, strip, sessionId)
}

func TestSyncEuroscopeStrip_NewLocalDepartureWithoutPositionStartsInNotCleared(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS123"

	var createdStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, strip *models.Strip) error {
			createdStrip = strip
			return nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: callsign,
		Origin:   "EKCH",
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, createdStrip)
	assert.Equal(t, callsign, createdStrip.Callsign)
	assert.Equal(t, shared.BAY_NOT_CLEARED, createdStrip.Bay)
}

func TestSyncEuroscopeStrip_NewStripWithFlightPlanDoesNotCallSeparateHasFPUpdate(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS124"

	var (
		createdStrip  *models.Strip
		setHasFPCalls int
	)
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, strip *models.Strip) error {
			createdStrip = strip
			return nil
		},
		SetHasFPFn: func(_ context.Context, _ int32, _ string, _ bool) error {
			setHasFPCalls++
			return nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: callsign,
		Origin:   "EKCH",
		HasFP:    true,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, createdStrip)
	assert.True(t, createdStrip.HasFP)
	assert.Zero(t, setHasFPCalls)
}

func TestSyncEuroscopeStrip_ExistingStripWritesRouteAndHasFPInPrimaryUpdate(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS125"
	sequence := int32(300)

	existingStrip := &models.Strip{
		Callsign:    callsign,
		Origin:      "EKCH",
		Destination: "EGLL",
		Bay:         shared.BAY_PUSH,
		Sequence:    &sequence,
		HasFP:       false,
	}

	var (
		updatedStrip     *models.Strip
		setHasFPCalls    int
		routeUpdateCalls int
	)
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
		SetHasFPFn: func(_ context.Context, _ int32, _ string, _ bool) error {
			setHasFPCalls++
			return nil
		},
	}

	svc, hub, esHub := newSyncTestFixture(t, existingStrip, stripRepo)
	mockServer, ok := hub.GetServer().(*testutil.MockServer)
	require.True(t, ok)
	mockServer.UpdateRouteForStripFn = func(_ string, _ int32, _ bool) error {
		routeUpdateCalls++
		return nil
	}

	routeServer := &syncRouteComputerTestServer{
		MockServer: mockServer,
		computeNextOwnersFn: func(_ context.Context, strip *models.Strip, sessionId int32) ([]string, bool, error) {
			require.Equal(t, session, sessionId)
			require.Equal(t, callsign, strip.Callsign)
			return []string{"EKCH_TWR"}, true, nil
		},
	}
	hub.SetServer(routeServer)
	esHub.SetServer(routeServer)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:    callsign,
		Origin:      "EKCH",
		Destination: "EGLL",
		Runway:      "22L",
		HasFP:       true,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	assert.Equal(t, []string{"EKCH_TWR"}, updatedStrip.NextOwners)
	assert.True(t, updatedStrip.HasFP)
	assert.Zero(t, setHasFPCalls)
	assert.Zero(t, routeUpdateCalls)
}

func TestSyncEuroscopeStrip_BlankFailoverSyncPreservesAdvancedDepartureBay(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS515"
	pushState := euroscope.GroundStatePush
	stand := "D4"

	existingStrip := &models.Strip{
		Callsign:    callsign,
		Origin:      "EKCH",
		Destination: "EGLL",
		Bay:         shared.BAY_PUSH,
		State:       &pushState,
		Stand:       &stand,
	}

	var updatedStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			existingStrip.Bay = bay
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: callsign,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	assert.Equal(t, "EKCH", updatedStrip.Origin)
	assert.Equal(t, "EGLL", updatedStrip.Destination)
	assert.Equal(t, pushState, *updatedStrip.State)
	assert.Equal(t, shared.BAY_PUSH, updatedStrip.Bay)
}

func TestSyncEuroscopeStrip_DepartIgnoresStaleTaxiGroundState(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS516"
	departState := euroscope.GroundStateDepart

	existingStrip := &models.Strip{
		Callsign:    callsign,
		Origin:      "EKCH",
		Destination: "EGLL",
		Bay:         shared.BAY_DEPART,
		State:       &departState,
	}

	var updatedStrip *models.Strip
	bayMoveCalls := 0
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			bayMoveCalls++
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:    callsign,
		Origin:      "EKCH",
		Destination: "EGLL",
		GroundState: euroscope.GroundStateTaxi,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	require.NotNil(t, updatedStrip.State)
	assert.Equal(t, departState, *updatedStrip.State)
	assert.Equal(t, shared.BAY_DEPART, updatedStrip.Bay)
	assert.Zero(t, bayMoveCalls, "stale TAXI sync must not resequence a DEPART strip")
}

func TestSyncEuroscopeStrip_DepartWithoutStateTreatsStaleTaxiAsLineup(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS517"

	existingStrip := &models.Strip{
		Callsign:    callsign,
		Origin:      "EKCH",
		Destination: "EGLL",
		Bay:         shared.BAY_DEPART,
	}

	var updatedStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:    callsign,
		Origin:      "EKCH",
		Destination: "EGLL",
		GroundState: euroscope.GroundStateTaxi,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	require.NotNil(t, updatedStrip.State)
	assert.Equal(t, euroscope.GroundStateLineup, *updatedStrip.State)
	assert.Equal(t, shared.BAY_DEPART, updatedStrip.Bay)
}

func TestSyncEuroscopeStrip_AirborneIgnoresStaleTaxiGroundState(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS518"

	existingStrip := &models.Strip{
		Callsign:    callsign,
		Origin:      "EKCH",
		Destination: "EGLL",
		Bay:         shared.BAY_AIRBORNE,
	}

	var updatedStrip *models.Strip
	bayMoveCalls := 0
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			bayMoveCalls++
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:    callsign,
		Origin:      "EKCH",
		Destination: "EGLL",
		GroundState: euroscope.GroundStateTaxi,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	require.NotNil(t, updatedStrip.State)
	assert.Equal(t, euroscope.GroundStateUnknown, *updatedStrip.State)
	assert.Equal(t, shared.BAY_AIRBORNE, updatedStrip.Bay)
	assert.Zero(t, bayMoveCalls, "stale TAXI sync must not resequence an AIRBORNE strip")
}

func TestSyncEuroscopeStrip_AirborneWithStoredTaxiTreatsStaleTaxiAsUnknown(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS519"
	taxiState := euroscope.GroundStateTaxi

	existingStrip := &models.Strip{
		Callsign:    callsign,
		Origin:      "EKCH",
		Destination: "EGLL",
		Bay:         shared.BAY_AIRBORNE,
		State:       &taxiState,
	}

	var updatedStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	strip := euroscope.Strip{
		Callsign:    callsign,
		Origin:      "EKCH",
		Destination: "EGLL",
		GroundState: euroscope.GroundStateTaxi,
	}
	strip.Position.Lat = shared.AirportLatitude
	strip.Position.Lon = shared.AirportLongitude
	strip.Position.Altitude = shared.AirportElevation + 700

	err := svc.syncEuroscopeStrip(ctx, session, "", strip, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	require.NotNil(t, updatedStrip.State)
	assert.Equal(t, euroscope.GroundStateUnknown, *updatedStrip.State)
	assert.Equal(t, shared.BAY_AIRBORNE, updatedStrip.Bay)
}

func TestSyncEuroscopeStrip_ExistingHiddenNonArrivalBecomesArrivalStartsInArrHidden(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "DLH8LL"

	existingStrip := &models.Strip{
		Callsign:    callsign,
		Origin:      "EDDF",
		Destination: "ESSA",
		Bay:         shared.BAY_HIDDEN,
	}

	var updatedStrip *models.Strip
	var movedToBay string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			movedToBay = bay
			return 1, nil
		},
	}

	svc, hub, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:    callsign,
		Origin:      "EDDF",
		Destination: "EKCH",
	}, "EKCH")
	require.NoError(t, err)

	require.NotNil(t, updatedStrip)
	assert.Equal(t, "EKCH", updatedStrip.Destination)
	assert.Equal(t, shared.BAY_ARR_HIDDEN, updatedStrip.Bay)
	assert.Empty(t, movedToBay)
	require.Len(t, hub.StripUpdates, 1)
	assert.Equal(t, callsign, hub.StripUpdates[0].Callsign)
}

func TestSyncEuroscopeStrip_ExistingArrivalAutoHiddenRemainsHidden(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS432"
	sequence := int32(500)

	existingStrip := &models.Strip{
		Callsign:    callsign,
		Origin:      "ESSA",
		Destination: "EKCH",
		Bay:         shared.BAY_HIDDEN,
		Sequence:    &sequence,
	}

	var updatedStrip *models.Strip
	var movedToBay string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			movedToBay = bay
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:    callsign,
		Origin:      "ESSA",
		Destination: "EKCH",
	}, "EKCH")
	require.NoError(t, err)

	require.NotNil(t, updatedStrip)
	assert.Equal(t, shared.BAY_HIDDEN, updatedStrip.Bay)
	require.NotNil(t, updatedStrip.Sequence)
	assert.Equal(t, sequence, *updatedStrip.Sequence)
	assert.Empty(t, movedToBay)
}

func TestSyncEuroscopeStrip_LocalDepartureTriggersCdmRecalculation(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, strip *models.Strip) error {
			return nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, nil, stripRepo)
	cdmService := &spyStripCdmService{}
	svc.SetCdmService(cdmService)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: "SAS123",
		Origin:   "EKCH",
	}, "EKCH")
	require.NoError(t, err)
	assert.True(t, cdmService.recalcTriggered)
	assert.Equal(t, session, cdmService.recalcSession)
	assert.Equal(t, "EKCH", cdmService.recalcAirport)
}

func TestSyncEuroscopeStrip_ArrivalDoesNotTriggerCdmRecalculation(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, strip *models.Strip) error {
			return nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, nil, stripRepo)
	cdmService := &spyStripCdmService{}
	svc.SetCdmService(cdmService)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:    "DLH432",
		Origin:      "EDDF",
		Destination: "EKCH",
	}, "EKCH")
	require.NoError(t, err)
	assert.False(t, cdmService.recalcTriggered)
}

func TestSyncEuroscopeStrip_NewStrip_TaxiNoGndOnline_InsertsTaxiLwr(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	var createdBay string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, strip *models.Strip) error {
			createdBay = strip.Bay
			return nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, nil, stripRepo)
	svc.SetControllerRepo(&testutil.MockControllerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
			return []*models.Controller{
				{Position: "118.105"}, // EKCH_A_TWR — no GND
			}, nil
		},
	})

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:    "SAS500",
		Origin:      "EKCH",
		GroundState: "TAXI",
	}, "EKCH")
	require.NoError(t, err)
	assert.Equal(t, shared.BAY_TAXI_LWR, createdBay, "no GND online → new TAXI strip should be inserted at TAXI_LWR")
}

func TestSyncEuroscopeStrip_ExistingStrip_TaxiNoGndOnline_UpdatesTaxiLwr(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	pushState := "PUSH"
	existingStrip := &models.Strip{
		Callsign: "SAS501",
		Origin:   "EKCH",
		Bay:      shared.BAY_PUSH,
		State:    &pushState,
	}

	var updatedBay string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedBay = strip.Bay
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)
	svc.SetControllerRepo(&testutil.MockControllerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
			return []*models.Controller{
				{Position: "118.105"}, // EKCH_A_TWR — no GND
			}, nil
		},
	})

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:    "SAS501",
		Origin:      "EKCH",
		GroundState: "TAXI",
	}, "EKCH")
	require.NoError(t, err)
	assert.Equal(t, shared.BAY_TAXI_LWR, updatedBay, "no GND online → PUSH→TAXI transition should land in TAXI_LWR")
}

func TestSyncEuroscopeStrip_WithSyncState_DefersBayAndValidationFollowUp(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	pushState := euroscope.GroundStatePush
	existingStrip := &models.Strip{
		Callsign: "SAS599",
		Origin:   "EKCH",
		Bay:      shared.BAY_PUSH,
		State:    &pushState,
	}

	var updateCalls int
	var bayMoveCalls int
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updateCalls++
			assert.Equal(t, shared.BAY_TAXI_LWR, strip.Bay)
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			bayMoveCalls++
			return 1, nil
		},
	}

	svc, hub, _ := newSyncTestFixture(t, existingStrip, stripRepo)
	syncState := &shared.SyncState{
		Session:             &models.Session{ID: session},
		ExistingControllers: map[string]*models.Controller{},
		ExistingStrips: map[string]*models.Strip{
			existingStrip.Callsign: existingStrip,
		},
	}
	ctx = shared.WithSyncState(ctx, syncState)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:    "SAS599",
		Origin:      "EKCH",
		GroundState: euroscope.GroundStateTaxi,
	}, "EKCH")
	require.NoError(t, err)

	assert.Equal(t, 1, updateCalls)
	assert.Zero(t, bayMoveCalls, "sync-mode should not issue a separate bay resequencing write")
	assert.Empty(t, syncState.BayUpdates)
	assert.True(t, syncState.SquawkValidation, "sync-mode should batch session validation work")
	assert.Contains(t, syncState.SortedStripUpdates(), "SAS599")
	assert.Empty(t, hub.StripUpdates, "sync-mode should defer frontend strip updates until finalization")
}

func TestSyncEuroscopeStrip_WithSyncState_SameBayPreservesSequenceAndSkipsBayUpdate(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS598"
	sequence := int32(600)
	existingStrip := &models.Strip{
		Callsign: callsign,
		Origin:   "EKCH",
		Bay:      shared.BAY_CLEARED,
		PdcState: "CONFIRMED",
		Cleared:  true,
		Sequence: &sequence,
	}

	var updatedStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
	}

	svc, hub, _ := newSyncTestFixture(t, existingStrip, stripRepo)
	syncState := &shared.SyncState{
		Session:             &models.Session{ID: session},
		ExistingControllers: map[string]*models.Controller{},
		ExistingStrips: map[string]*models.Strip{
			existingStrip.Callsign: existingStrip,
		},
	}
	ctx = shared.WithSyncState(ctx, syncState)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: callsign,
		Origin:   "EKCH",
		Cleared:  false,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	require.NotNil(t, updatedStrip.Sequence)
	assert.Equal(t, sequence, *updatedStrip.Sequence)
	assert.Empty(t, syncState.BayUpdates, "same-bay sync updates must not schedule MoveToBay")
	assert.Contains(t, syncState.SortedStripUpdates(), callsign)
	assert.Empty(t, hub.StripUpdates, "sync-mode should defer frontend strip updates until finalization")
}

func TestSyncEuroscopeStrip_ExistingPendingPdcStrip_DoesNotFallBackToNotCleared(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS502"
	sequence := int32(700)

	existingStrip := &models.Strip{
		Callsign: callsign,
		Origin:   "EKCH",
		Bay:      shared.BAY_CLEARED,
		PdcState: "CLEARED",
		Cleared:  false,
		Sequence: &sequence,
	}

	var updatedStrip *models.Strip
	var movedToBay string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			movedToBay = bay
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: callsign,
		Origin:   "EKCH",
		Cleared:  false,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	assert.Equal(t, shared.BAY_CLEARED, updatedStrip.Bay)
	assert.False(t, updatedStrip.Cleared)
	require.NotNil(t, updatedStrip.Sequence)
	assert.Equal(t, sequence, *updatedStrip.Sequence)
	assert.Empty(t, movedToBay)
}

func TestSyncEuroscopeStrip_ExistingConfirmedPdcStrip_PreservesClearedFlagUntilSyncCatchesUp(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS503"
	sequence := int32(800)

	existingStrip := &models.Strip{
		Callsign: callsign,
		Origin:   "EKCH",
		Bay:      shared.BAY_CLEARED,
		PdcState: "CONFIRMED",
		Cleared:  true,
		Sequence: &sequence,
	}

	var updatedStrip *models.Strip
	var movedToBay string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			movedToBay = bay
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: callsign,
		Origin:   "EKCH",
		Cleared:  false,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	assert.True(t, updatedStrip.Cleared)
	assert.Equal(t, shared.BAY_CLEARED, updatedStrip.Bay)
	require.NotNil(t, updatedStrip.Sequence)
	assert.Equal(t, sequence, *updatedStrip.Sequence)
	assert.Empty(t, movedToBay)
}

func TestSyncEuroscopeStrip_MoveBackToNotCleared_ClearsOwner(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS504"
	owner := "EKCH_GND"
	getByCallsignCalls := 0
	var routeUpdateCallsign string
	var routeUpdateSession int32
	var routeUpdateSendUpdate bool

	existingStrip := &models.Strip{
		Callsign:       callsign,
		Origin:         "EKCH",
		Bay:            shared.BAY_PUSH,
		Cleared:        false,
		Owner:          &owner,
		Version:        int32(7),
		NextOwners:     []string{"EKCH_TWR"},
		PreviousOwners: []string{"EKCH_DEL"},
	}

	var updatedStrip *models.Strip
	var movedToBay string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			getByCallsignCalls++
			if getByCallsignCalls == 1 {
				return existingStrip, nil
			}

			return &models.Strip{
				Callsign:       callsign,
				Origin:         "EKCH",
				Bay:            shared.BAY_NOT_CLEARED,
				Cleared:        false,
				Owner:          nil,
				Version:        int32(8),
				NextOwners:     []string{"EKCH_TWR"},
				PreviousOwners: []string{},
			}, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			movedToBay = bay
			return 1, nil
		},
		SetPreviousOwnersFn: func(_ context.Context, _ int32, _ string, previousOwners []string) error {
			assert.Empty(t, previousOwners)
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, newOwner *string, version int32) (int64, error) {
			assert.Nil(t, newOwner)
			assert.Equal(t, int32(8), version)
			return 1, nil
		},
	}

	svc, hub, _ := newSyncTestFixture(t, existingStrip, stripRepo)
	mockServer, ok := hub.GetServer().(*testutil.MockServer)
	require.True(t, ok)
	mockServer.UpdateRouteForStripFn = func(cs string, sess int32, sendUpdate bool) error {
		routeUpdateCallsign = cs
		routeUpdateSession = sess
		routeUpdateSendUpdate = sendUpdate
		return nil
	}

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: callsign,
		Origin:   "EKCH",
		Cleared:  false,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	assert.Equal(t, shared.BAY_NOT_CLEARED, updatedStrip.Bay)
	assert.Nil(t, updatedStrip.Owner)
	assert.Empty(t, updatedStrip.PreviousOwners)
	assert.Empty(t, movedToBay)
	require.Len(t, hub.StripUpdates, 1)
	assert.Equal(t, callsign, hub.StripUpdates[0].Callsign)
	assert.Equal(t, callsign, routeUpdateCallsign)
	assert.Equal(t, session, routeUpdateSession)
	assert.False(t, routeUpdateSendUpdate)
}

func TestSyncEuroscopeStrip_NewLocalDepartureWithReservedAssignedSquawk_GeneratesSquawk(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, _ *models.Strip) error {
			return nil
		},
	}

	svc, _, esHub := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:       "SAS777",
		Origin:         "EKCH",
		AssignedSquawk: "2000",
	}, "EKCH")
	require.NoError(t, err)
	require.Len(t, esHub.GenerateSquawks, 1)
	assert.Equal(t, session, esHub.GenerateSquawks[0].Session)
	assert.Equal(t, "SAS777", esHub.GenerateSquawks[0].Callsign)
	assert.Equal(t, "", esHub.GenerateSquawks[0].Cid)
}

func TestSyncEuroscopeStrip_NewLocalDepartureWithReservedAssignedSquawkAndCleared_DoesNotGenerateSquawk(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, _ *models.Strip) error {
			return nil
		},
	}

	svc, _, esHub := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:       "SAS778",
		Origin:         "EKCH",
		AssignedSquawk: "2000",
		Cleared:        true,
	}, "EKCH")
	require.NoError(t, err)
	assert.Empty(t, esHub.GenerateSquawks)
}

func TestSyncEuroscopeStrip_NewLocalDepartureWithReservedAssignedSquawkAndTaxiState_DoesNotGenerateSquawk(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, _ *models.Strip) error {
			return nil
		},
	}

	svc, _, esHub := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:       "SAS779",
		Origin:         "EKCH",
		AssignedSquawk: "2000",
		GroundState:    euroscope.GroundStateTaxi,
	}, "EKCH")
	require.NoError(t, err)
	assert.Empty(t, esHub.GenerateSquawks)
}

func TestSyncEuroscopeStrip_NewArrivalWithReservedAssignedSquawk_DoesNotGenerateSquawk(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, _ *models.Strip) error {
			return nil
		},
	}

	svc, _, esHub := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:       "DLH777",
		Origin:         "EDDF",
		Destination:    "EKCH",
		AssignedSquawk: "2000",
	}, "EKCH")
	require.NoError(t, err)
	assert.Empty(t, esHub.GenerateSquawks)
}
