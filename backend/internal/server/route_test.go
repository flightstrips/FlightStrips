package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"FlightStrips/internal/vatsim"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		panic("failed to chdir to backend root: " + err.Error())
	}
	if err := config.InitConfig(); err != nil {
		panic("failed to initialize config: " + err.Error())
	}
	os.Exit(m.Run())
}

type routeTransceiverStub map[string][]string

func (s routeTransceiverStub) GetFrequencies(callsign string) []string {
	return s[callsign]
}

func TestComputeRouteStateForStrip_ClearsRouteForTrafficUnrelatedToSessionAirport(t *testing.T) {
	strip := &models.Strip{
		Callsign:    "DLH2958",
		Origin:      "EDDH",
		Destination: "EFHK",
		Runway:      stringPtr("33"),
		Stand:       stringPtr("A12"),
		NextOwners:  []string{"EKCH_TWR"},
	}
	session := &models.Session{ID: 42, Airport: "ekch"}

	result, shouldUpdate, err := computeRouteStateForStrip(strip, session, nil, routeRadioState{})

	require.NoError(t, err)
	assert.True(t, shouldUpdate)
	assert.Empty(t, result.NextOwners)
}

func TestComputeRouteStateForStrip_ClearsDepartureRouteUntilStandIsKnown(t *testing.T) {
	strip := &models.Strip{
		Callsign:    "SAS123",
		Origin:      "EKCH",
		Destination: "ESSA",
		Runway:      stringPtr("22R"),
		NextOwners:  []string{"EKCH_DEL"},
	}
	session := &models.Session{ID: 42, Airport: "EKCH"}

	result, shouldUpdate, err := computeRouteStateForStrip(strip, session, nil, routeRadioState{})

	require.NoError(t, err)
	assert.True(t, shouldUpdate)
	assert.Empty(t, result.NextOwners)
}

func TestArrivalRouteCanContinueFromGroundEastToNorthStand(t *testing.T) {
	for _, test := range []struct {
		runway string
		stand  string
	}{
		{runway: "22L", stand: "E36"},
		{runway: "22L", stand: "A12"},
		{runway: "04L", stand: "A23"},
	} {
		t.Run(test.runway+"_"+test.stand, func(t *testing.T) {
			route, ok := config.ComputeToStand([]string{test.runway}, "GE", test.stand)
			require.True(t, ok)
			assert.Equal(t, []string{"GE", "GWA", "AA"}, route.Path)
		})
	}
}

func TestUpdateRouteForStrip_ArrivalOutsideSupportedRegionFallsBackToTowerOwner(t *testing.T) {

	arrivalRunway, towerSector := mustArrivalRunwayAndTowerSector(t)

	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	strip := &models.Strip{
		Callsign:          "SAS123",
		Session:           42,
		Destination:       "EKCH",
		Stand:             stringPtr("A12"),
		PositionLatitude:  float64Ptr(0),
		PositionLongitude: float64Ptr(0),
	}

	var updatedNextOwners []string

	stripRepo.GetByCallsignFn = func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		require.Equal(t, int32(42), session)
		require.Equal(t, "SAS123", callsign)
		return strip, nil
	}
	stripRepo.SetRouteStateFn = func(_ context.Context, session int32, callsign string, nextOwners []string, _ *models.NextDisplay) error {
		require.Equal(t, int32(42), session)
		require.Equal(t, "SAS123", callsign)
		updatedNextOwners = append([]string(nil), nextOwners...)
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(42), id)
		return &models.Session{
			ID:      42,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{arrivalRunway},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(42), session)
		return []*models.SectorOwner{
			{
				Session:  42,
				Sector:   []string{towerSector},
				Position: "EKCH_TWR",
			},
		}, nil
	}

	srv := &Server{
		frontendHub: frontendHub,
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err := srv.UpdateRouteForStrip("SAS123", 42, true)
	require.NoError(t, err)

	assert.Equal(t, []string{"EKCH_TWR"}, updatedNextOwners)
	require.Len(t, frontendHub.OwnersUpdates, 1)
	assert.Equal(t, []string{"EKCH_TWR"}, frontendHub.OwnersUpdates[0].NextOwners)
	assert.Equal(t, "SAS123", frontendHub.OwnersUpdates[0].Callsign)
}

func TestAirborneSectorDisplayNames_UseShortLabels(t *testing.T) {
	assert.Equal(t, "K", config.GetSectorDisplayName("K_DEP"))
	assert.Equal(t, "R", config.GetSectorDisplayName("R_DEP"))
}

func TestComputeDepartureFrequencyForStrip_UsesSIDSpecificAirborneController(t *testing.T) {
	controllerRepo := &testutil.MockControllerRepository{
		ListFn: func(_ context.Context, session int32) ([]*models.Controller, error) {
			require.Equal(t, int32(42), session)
			return []*models.Controller{
				{Session: session, Callsign: "EKCH_K_DEP", Position: "124.980"},
				{Session: session, Callsign: "EKCH_R_DEP", Position: "120.255"},
			}, nil
		},
	}
	sid := "NEXEN2A"
	srv := &Server{controllerRepo: controllerRepo}

	frequency, err := srv.ComputeDepartureFrequencyForStripContext(context.Background(), &models.Strip{Sid: &sid}, 42)

	require.NoError(t, err)
	require.NotNil(t, frequency)
	assert.Equal(t, "124.980", *frequency)
}

func TestComputeDepartureFrequencyForStrip_UsesCrossCoupledDepartureFrequency(t *testing.T) {
	controllerRepo := &testutil.MockControllerRepository{
		ListFn: func(_ context.Context, session int32) ([]*models.Controller, error) {
			return []*models.Controller{{Session: session, Callsign: "EKCH_O_APP", Position: "118.455"}}, nil
		},
	}
	sid := "NEXEN2A"
	srv := &Server{
		controllerRepo:     controllerRepo,
		frequencyProviders: []TransceiverLookup{routeTransceiverStub{"EKCH_O_APP": {"124.980"}}},
	}

	frequency, err := srv.ComputeDepartureFrequencyForStripContext(context.Background(), &models.Strip{Sid: &sid}, 42)

	require.NoError(t, err)
	require.NotNil(t, frequency)
	assert.Equal(t, "124.980", *frequency)
}

func TestUpdateRouteForStrip_UsesCrossCoupledFrequencyFromOfficialTransceiverPayload(t *testing.T) {
	aTowerPosition := frequencyForPosition(t, "EKCH_A_TWR")
	aGroundPosition := frequencyForPosition(t, "EKCH_A_GND")
	bGroundPosition := frequencyForPosition(t, "EKCH_B_GND")

	var transceiverPayload atomic.Value
	transceiverPayload.Store(`[{
		"callsign":"EKCH_B_GND",
		"transceivers":[
			{"id":0,"frequency":121905000,"latDeg":55.6,"lonDeg":12.6},
			{"id":1,"frequency":121630000,"latDeg":55.6,"lonDeg":12.6}
		],
		"shoutLineIds":[]
	}]`)
	transceiverServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(transceiverPayload.Load().(string)))
	}))
	defer transceiverServer.Close()

	refreshed := make(chan struct{}, 1)
	cache := vatsim.NewTransceiverCache(transceiverServer.URL, 10*time.Millisecond, transceiverServer.Client(), func(context.Context) error {
		select {
		case refreshed <- struct{}{}:
		default:
		}
		return nil
	})
	cacheContext, cancelCache := context.WithCancel(context.Background())
	t.Cleanup(cancelCache)
	go cache.Start(cacheContext)

	select {
	case <-refreshed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for transceiver cache refresh")
	}

	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}
	controllerRepo := &testutil.MockControllerRepository{}
	strip := &models.Strip{
		Callsign:          "SAS123",
		Session:           42,
		Destination:       "EKCH",
		Runway:            stringPtr("22L"),
		Stand:             stringPtr("A17"),
		Owner:             stringPtr(aTowerPosition),
		PositionLatitude:  float64Ptr(0),
		PositionLongitude: float64Ptr(0),
	}

	stripRepo.GetByCallsignFn = func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
		return strip, nil
	}
	var persistedDisplay *models.NextDisplay
	stripRepo.SetRouteStateFn = func(_ context.Context, _ int32, _ string, _ []string, display *models.NextDisplay) error {
		persistedDisplay = cloneNextDisplay(display)
		return nil
	}
	sessionRepo.GetByIDFn = func(_ context.Context, _ int32) (*models.Session, error) {
		return &models.Session{
			ID:      42,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{"22L"},
			},
		}, nil
	}
	sectorRepo.ListBySessionFn = func(_ context.Context, _ int32) ([]*models.SectorOwner, error) {
		return []*models.SectorOwner{
			{Session: 42, Sector: []string{"TE", "GWA"}, Position: aTowerPosition},
			{Session: 42, Sector: []string{"AA"}, Position: bGroundPosition, Identifier: "SQ"},
		}, nil
	}
	controllerRepo.ListFn = func(_ context.Context, _ int32) ([]*models.Controller, error) {
		return []*models.Controller{{Session: 42, Callsign: "EKCH_B_GND", Position: bGroundPosition}}, nil
	}

	srv := &Server{
		frontendHub:        frontendHub,
		stripRepo:          stripRepo,
		sessionRepo:        sessionRepo,
		sectorRepo:         sectorRepo,
		controllerRepo:     controllerRepo,
		frequencyProviders: []TransceiverLookup{cache},
	}

	require.NoError(t, srv.UpdateRouteForStrip("SAS123", 42, true))
	require.Len(t, frontendHub.OwnersUpdates, 1)
	assert.Equal(t, []string{bGroundPosition}, frontendHub.OwnersUpdates[0].NextOwners)
	require.NotNil(t, frontendHub.OwnersUpdates[0].NextDisplay)
	assert.Equal(t, "AA", frontendHub.OwnersUpdates[0].NextDisplay.Label)
	assert.Equal(t, aGroundPosition, frontendHub.OwnersUpdates[0].NextDisplay.Frequency)
	require.NotNil(t, persistedDisplay)
	assert.Equal(t, "AA", persistedDisplay.Label)
	assert.Equal(t, aGroundPosition, persistedDisplay.Frequency)

	transceiverPayload.Store(`[{
		"callsign":"EKCH_B_GND",
		"transceivers":[{"id":0,"frequency":121905000,"latDeg":55.6,"lonDeg":12.6}],
		"shoutLineIds":[]
	}]`)
	select {
	case <-refreshed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for transceiver cache to remove cross-coupled frequency")
	}

	frontendHub.OwnersUpdates = nil
	require.NoError(t, srv.UpdateRouteForStrip("SAS123", 42, true))
	require.Len(t, frontendHub.OwnersUpdates, 1)
	assert.Equal(t, []string{bGroundPosition}, frontendHub.OwnersUpdates[0].NextOwners)
	require.NotNil(t, frontendHub.OwnersUpdates[0].NextDisplay)
	assert.Equal(t, "SEQ PLN", frontendHub.OwnersUpdates[0].NextDisplay.Label)
	assert.Equal(t, bGroundPosition, frontendHub.OwnersUpdates[0].NextDisplay.Frequency)
}

func TestComputeDepartureFrequencyForStrip_DoesNotReturnApproachFallback(t *testing.T) {
	controllerRepo := &testutil.MockControllerRepository{
		ListFn: func(_ context.Context, session int32) ([]*models.Controller, error) {
			return []*models.Controller{{Session: session, Callsign: "EKCH_O_APP", Position: "118.455"}}, nil
		},
	}
	sid := "NEXEN2A"
	srv := &Server{controllerRepo: controllerRepo}

	frequency, err := srv.ComputeDepartureFrequencyForStripContext(context.Background(), &models.Strip{Sid: &sid}, 42)

	require.NoError(t, err)
	assert.Nil(t, frequency)
}

func TestUpdateRouteForStrip_ArrivalOutsideSupportedRegionUsesTowerAsRouteStart(t *testing.T) {

	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	towerPosition := "118.105"
	apronPosition := "121.630"

	strip := &models.Strip{
		Callsign:          "NSZ3097",
		Session:           76,
		Destination:       "EKCH",
		Runway:            stringPtr("22L"),
		Stand:             stringPtr("B3"),
		Owner:             stringPtr(towerPosition),
		PositionLatitude:  float64Ptr(0),
		PositionLongitude: float64Ptr(0),
	}

	var updatedNextOwners []string

	stripRepo.GetByCallsignFn = func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		require.Equal(t, int32(76), session)
		require.Equal(t, "NSZ3097", callsign)
		return strip, nil
	}
	stripRepo.SetRouteStateFn = func(_ context.Context, session int32, callsign string, nextOwners []string, _ *models.NextDisplay) error {
		require.Equal(t, int32(76), session)
		require.Equal(t, "NSZ3097", callsign)
		updatedNextOwners = append([]string(nil), nextOwners...)
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(76), id)
		return &models.Session{
			ID:      76,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{"22L"},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(76), session)
		return []*models.SectorOwner{
			{
				Session:  76,
				Sector:   []string{"TE"},
				Position: towerPosition,
			},
			{
				Session:  76,
				Sector:   []string{"AA"},
				Position: apronPosition,
			},
		}, nil
	}

	srv := &Server{
		frontendHub: frontendHub,
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err := srv.UpdateRouteForStrip("NSZ3097", 76, true)
	require.NoError(t, err)

	assert.Equal(t, []string{apronPosition}, updatedNextOwners)
	require.Len(t, frontendHub.OwnersUpdates, 1)
	assert.Equal(t, []string{apronPosition}, frontendHub.OwnersUpdates[0].NextOwners)
	assert.Equal(t, "NSZ3097", frontendHub.OwnersUpdates[0].Callsign)
}

func TestUpdateRouteForStrip_ArrivalUsesConfigDrivenCrossingSectorSplit(t *testing.T) {

	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	aTowerPosition := frequencyForPosition(t, "EKCH_A_TWR")
	apronPosition := frequencyForPosition(t, "EKCH_A_GND")

	strip := &models.Strip{
		Callsign:          "SAS789",
		Session:           91,
		Destination:       "EKCH",
		Runway:            stringPtr("22L"),
		Stand:             stringPtr("A17"),
		Owner:             stringPtr(aTowerPosition),
		PositionLatitude:  float64Ptr(0),
		PositionLongitude: float64Ptr(0),
	}

	var updatedNextOwners []string

	stripRepo.GetByCallsignFn = func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		require.Equal(t, int32(91), session)
		require.Equal(t, "SAS789", callsign)
		return strip, nil
	}
	stripRepo.SetRouteStateFn = func(_ context.Context, session int32, callsign string, nextOwners []string, _ *models.NextDisplay) error {
		require.Equal(t, int32(91), session)
		require.Equal(t, "SAS789", callsign)
		updatedNextOwners = append([]string(nil), nextOwners...)
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(91), id)
		return &models.Session{
			ID:      91,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{"22L"},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(91), session)
		return []*models.SectorOwner{
			{
				Session:  91,
				Sector:   []string{"TE"},
				Position: aTowerPosition,
			},
			{
				Session:  91,
				Sector:   []string{"GWA"},
				Position: aTowerPosition,
			},
			{
				Session:  91,
				Sector:   []string{"AA"},
				Position: apronPosition,
			},
		}, nil
	}

	srv := &Server{
		frontendHub: frontendHub,
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err := srv.UpdateRouteForStrip("SAS789", 91, true)
	require.NoError(t, err)

	assert.Equal(t, []string{apronPosition}, updatedNextOwners)
	require.Len(t, frontendHub.OwnersUpdates, 1)
	assert.Equal(t, []string{apronPosition}, frontendHub.OwnersUpdates[0].NextOwners)
	require.NotNil(t, frontendHub.OwnersUpdates[0].NextDisplay)
	assert.Equal(t, "AA", frontendHub.OwnersUpdates[0].NextDisplay.Label)
	assert.Equal(t, apronPosition, frontendHub.OwnersUpdates[0].NextDisplay.Frequency)
	assert.Equal(t, "SAS789", frontendHub.OwnersUpdates[0].Callsign)
}

func TestUpdateRouteForStrip_ArrivalKeepsGWAOwnerWhenControllersAreSplit(t *testing.T) {

	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	const (
		lat = 55.6235
		lon = 12.6380
	)

	region, err := config.GetRegionForPosition(lat, lon)
	require.NoError(t, err)
	require.Equal(t, "GROUND_WEST", region.Name)

	aTowerPosition := frequencyForPosition(t, "EKCH_A_TWR")
	gwTowerPosition := frequencyForPosition(t, "EKCH_GW_TWR")
	apronPosition := frequencyForPosition(t, "EKCH_A_GND")

	strip := &models.Strip{
		Callsign:          "SAS790",
		Session:           92,
		Destination:       "EKCH",
		Runway:            stringPtr("22L"),
		Stand:             stringPtr("A17"),
		Owner:             stringPtr(aTowerPosition),
		PositionLatitude:  float64Ptr(lat),
		PositionLongitude: float64Ptr(lon),
	}

	var updatedNextOwners []string

	stripRepo.GetByCallsignFn = func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		require.Equal(t, int32(92), session)
		require.Equal(t, "SAS790", callsign)
		return strip, nil
	}
	stripRepo.SetRouteStateFn = func(_ context.Context, session int32, callsign string, nextOwners []string, _ *models.NextDisplay) error {
		require.Equal(t, int32(92), session)
		require.Equal(t, "SAS790", callsign)
		updatedNextOwners = append([]string(nil), nextOwners...)
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(92), id)
		return &models.Session{
			ID:      92,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{"22L"},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(92), session)
		return []*models.SectorOwner{
			{
				Session:  92,
				Sector:   []string{"TE"},
				Position: aTowerPosition,
			},
			{
				Session:  92,
				Sector:   []string{"GWA"},
				Position: gwTowerPosition,
			},
			{
				Session:  92,
				Sector:   []string{"AA"},
				Position: apronPosition,
			},
		}, nil
	}

	srv := &Server{
		frontendHub: frontendHub,
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err = srv.UpdateRouteForStrip("SAS790", 92, true)
	require.NoError(t, err)

	assert.Equal(t, []string{gwTowerPosition, apronPosition}, updatedNextOwners)
	require.Len(t, frontendHub.OwnersUpdates, 1)
	assert.Equal(t, []string{gwTowerPosition, apronPosition}, frontendHub.OwnersUpdates[0].NextOwners)
	require.NotNil(t, frontendHub.OwnersUpdates[0].NextDisplay)
	assert.Equal(t, "GW", frontendHub.OwnersUpdates[0].NextDisplay.Label)
	assert.Equal(t, gwTowerPosition, frontendHub.OwnersUpdates[0].NextDisplay.Frequency)
	assert.Equal(t, "SAS790", frontendHub.OwnersUpdates[0].Callsign)
}

func TestResolveRouteSectorOwner_UsesOverrideTargetFirst(t *testing.T) {

	owner, ok := resolveRouteSectorOwner(
		"GWA",
		map[string]string{
			"TE":  "EKCH_A_TWR",
			"GWA": "EKCH_GW_TWR",
		},
		map[string]string{"GWA": "TE"},
	)

	require.True(t, ok)
	assert.Equal(t, "EKCH_A_TWR", owner)
}

func TestResolveRouteSectorOwner_FallsBackToOriginalSector(t *testing.T) {

	owner, ok := resolveRouteSectorOwner(
		"GWA",
		map[string]string{
			"GWA": "EKCH_GW_TWR",
		},
		map[string]string{"GWA": "TE"},
	)

	require.True(t, ok)
	assert.Equal(t, "EKCH_GW_TWR", owner)
}

func TestResolveRouteDisplayFrequency_UsesSectorFrequencyForCrossCoupledAirborneSector(t *testing.T) {

	session := &models.Session{
		ActiveRunways: pkgModels.ActiveRunways{
			DepartureRunways: []string{"22L"},
		},
	}

	nextDisplay := buildRouteNextDisplay(
		session,
		"K_DEP",
		frequencyForPosition(t, "EKCH_W_APP"),
		map[string]struct{}{frequencyForPosition(t, "EKCH_K_DEP"): {}},
		false,
	)

	require.NotNil(t, nextDisplay)
	assert.Equal(t, "K", nextDisplay.Label)
	assert.Equal(t, frequencyForPosition(t, "EKCH_K_DEP"), nextDisplay.Frequency)
}

func TestResolveRouteDisplayFrequency_UsesSectorFrequencyForGroundSectorWhenCoveredByAnotherGroundController(t *testing.T) {

	session := &models.Session{
		ActiveRunways: pkgModels.ActiveRunways{
			DepartureRunways: []string{"22L"},
		},
	}

	nextDisplay := buildRouteNextDisplay(
		session,
		"AD",
		frequencyForPosition(t, "EKCH_A_GND"),
		map[string]struct{}{frequencyForPosition(t, "EKCH_C_GND"): {}},
		false,
	)

	require.NotNil(t, nextDisplay)
	assert.Equal(t, "AD", nextDisplay.Label)
	assert.Equal(t, frequencyForPosition(t, "EKCH_C_GND"), nextDisplay.Frequency)
}

func TestResolveRouteDisplayFrequency_UsesPrimaryDisplayWhenOwnerIsConfiguredPrimary(t *testing.T) {

	session := &models.Session{
		ActiveRunways: pkgModels.ActiveRunways{
			ArrivalRunways: []string{"04L"},
		},
	}

	nextDisplay := buildRouteNextDisplay(session, "TE", frequencyForPosition(t, "EKCH_A_TWR"), nil, true)

	assert.Nil(t, nextDisplay)
}

func TestEKCHArrivalRoute_22LHighAFromTWOnlyOverridesEntryTower(t *testing.T) {

	route, ok := config.ComputeToStand([]string{"22L"}, "TW", "A34")
	require.True(t, ok)
	assert.Equal(t, []string{"TW", "GWA", "AA"}, route.Path)
	assert.Equal(t, "TE", route.OwnerOverrides["TW"])
	_, hasGWAOverride := route.OwnerOverrides["GWA"]
	assert.False(t, hasGWAOverride)
}

func TestEKCHArrivalRoute_22LCargoFromTWUsesTEOwnedTransitSectors(t *testing.T) {

	route, ok := config.ComputeToStand([]string{"22L"}, "TW", "G120")
	require.True(t, ok)
	assert.Equal(t, []string{"TW", "GWA", "GE"}, route.Path)
	assert.Equal(t, "TE", route.OwnerOverrides["TW"])
	assert.Equal(t, "TE", route.OwnerOverrides["GWA"])
	_, hasGEOverride := route.OwnerOverrides["GE"]
	assert.False(t, hasGEOverride)
}

func TestEKCHArrivalRoute_04LHighAFromTEOnlyOverridesEntryTower(t *testing.T) {

	route, ok := config.ComputeToStand([]string{"04L"}, "TE", "A34")
	require.True(t, ok)
	assert.Equal(t, []string{"TE", "GWA", "AA"}, route.Path)
	assert.Equal(t, "TW", route.OwnerOverrides["TE"])
	_, hasGWAOverride := route.OwnerOverrides["GWA"]
	assert.False(t, hasGWAOverride)
}

func TestEKCHArrivalRoute_30RestFromGWAUsesTEOverride(t *testing.T) {

	route, ok := config.ComputeToStand([]string{"30"}, "GWA", "A12")
	require.True(t, ok)
	assert.Equal(t, []string{"GWA", "AA"}, route.Path)
	assert.Equal(t, "TE", route.OwnerOverrides["GWA"])
}

func TestEKCHArrivalRoute_CargoFromAAUsesDirectEndpointRoute(t *testing.T) {

	testCases := []struct {
		name   string
		active []string
		stand  string
	}{
		{name: "22L group", active: []string{"22L"}, stand: "G120"},
		{name: "04L group", active: []string{"04L"}, stand: "G127"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			route, ok := config.ComputeToStand(tc.active, "AA", tc.stand)
			require.True(t, ok)
			assert.Equal(t, []string{"AA", "GE"}, route.Path)
			assert.Empty(t, route.OwnerOverrides)
		})
	}
}

func TestEKCHArrivalRoute_04LCargoUsesTWThenGWAThenGE(t *testing.T) {
	route, ok := config.ComputeToStand([]string{"04L"}, "TW", "G120")
	require.True(t, ok)
	assert.Equal(t, []string{"TW", "GWA", "GE"}, route.Path)
}

func TestEKCHArrivalRoute_CargoAtGroundEastHasNoFollowingHandover(t *testing.T) {
	testCases := []struct {
		name   string
		active []string
	}{
		{name: "22L group", active: []string{"22L"}},
		{name: "04L group", active: []string{"04L"}},
		{name: "runway 30 group", active: []string{"30"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			route, ok := config.ComputeToStand(tc.active, "GE", "G120")
			require.True(t, ok)
			assert.Equal(t, []string{"GE"}, route.Path)
			_, hasGEOverride := route.OwnerOverrides["GE"]
			assert.False(t, hasGEOverride)
		})
	}
}

func TestEKCHArrivalRoute_W1UsesTEThenGWAChainAcrossRunwayGroups(t *testing.T) {

	testCases := []struct {
		name          string
		active        []string
		currentSector string
		expectedPath  []string
	}{
		{name: "22L from TE", active: []string{"22L"}, currentSector: "TE", expectedPath: []string{"TE", "TW", "GWA"}},
		{name: "22L from TW", active: []string{"22L"}, currentSector: "TW", expectedPath: []string{"TW", "GWA"}},
		{name: "04L from TE", active: []string{"04L"}, currentSector: "TE", expectedPath: []string{"TE", "TW", "GWA"}},
		{name: "30 from TE", active: []string{"30"}, currentSector: "TE", expectedPath: []string{"TE", "TW", "GWA"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			route, ok := config.ComputeToStand(tc.active, tc.currentSector, "W1")
			require.True(t, ok)
			assert.Equal(t, tc.expectedPath, route.Path)
			assert.Equal(t, "TE", route.OwnerOverrides["TW"])
			_, hasGWAOverride := route.OwnerOverrides["GWA"]
			assert.False(t, hasGWAOverride)
		})
	}
}

func TestUpdateRoutesForSession_RecalculatesEachStrip(t *testing.T) {

	arrivalRunway, towerSector := mustArrivalRunwayAndTowerSector(t)

	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	strips := []*models.Strip{
		{Callsign: "SAS123", Session: 42, Destination: "EKCH"},
		{Callsign: "KLM456", Session: 42, Destination: "EKCH"},
	}

	var updatedCallsigns []string

	stripRepo.ListFn = func(_ context.Context, session int32) ([]*models.Strip, error) {
		require.Equal(t, int32(42), session)
		return strips, nil
	}
	stripRepo.SetRouteStateFn = func(_ context.Context, session int32, callsign string, nextOwners []string, _ *models.NextDisplay) error {
		require.Equal(t, int32(42), session)
		assert.Equal(t, []string{"EKCH_TWR"}, nextOwners)
		updatedCallsigns = append(updatedCallsigns, callsign)
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(42), id)
		return &models.Session{
			ID:      42,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{arrivalRunway},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(42), session)
		return []*models.SectorOwner{
			{
				Session:  42,
				Sector:   []string{towerSector},
				Position: "EKCH_TWR",
			},
		}, nil
	}

	srv := &Server{
		frontendHub: frontendHub,
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err := srv.UpdateRoutesForSession(42, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"SAS123", "KLM456"}, updatedCallsigns)
	assert.Empty(t, frontendHub.OwnersUpdates)
}

func TestUpdateRoutesForSession_DoesNotRetargetPendingCoordination(t *testing.T) {
	arrivalRunway, towerSector := mustArrivalRunwayAndTowerSector(t)
	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}
	coordRepo := &testutil.MockCoordinationRepository{}

	strips := []*models.Strip{
		{ID: 1, Callsign: "SAS123", Session: 42, Destination: "EKCH"},
		{ID: 2, Callsign: "KLM456", Session: 42, Destination: "EKCH"},
	}
	var updatedCallsigns []string

	stripRepo.ListFn = func(_ context.Context, _ int32) ([]*models.Strip, error) {
		return strips, nil
	}
	stripRepo.SetRouteStateFn = func(_ context.Context, _ int32, callsign string, _ []string, _ *models.NextDisplay) error {
		updatedCallsigns = append(updatedCallsigns, callsign)
		return nil
	}
	sessionRepo.GetByIDFn = func(_ context.Context, _ int32) (*models.Session, error) {
		return &models.Session{
			ID:      42,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{arrivalRunway},
			},
		}, nil
	}
	sectorRepo.ListBySessionFn = func(_ context.Context, _ int32) ([]*models.SectorOwner, error) {
		return []*models.SectorOwner{{
			Session:  42,
			Sector:   []string{towerSector},
			Position: "EKCH_TWR",
		}}, nil
	}
	coordRepo.ListBySessionFn = func(_ context.Context, _ int32) ([]*models.Coordination, error) {
		return []*models.Coordination{{Session: 42, StripID: 2}}, nil
	}

	srv := &Server{
		frontendHub: frontendHub,
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
		coordRepo:   coordRepo,
	}

	err := srv.UpdateRoutesForSession(42, true)

	require.NoError(t, err)
	assert.Equal(t, []string{"SAS123"}, updatedCallsigns)
	require.Len(t, frontendHub.OwnersUpdates, 1)
	assert.Equal(t, "SAS123", frontendHub.OwnersUpdates[0].Callsign)
}

func TestUpdateRouteForStrip_DoesNotRetargetPendingCoordination(t *testing.T) {
	stripRepo := &testutil.MockStripRepository{}
	coordRepo := &testutil.MockCoordinationRepository{}

	stripRepo.GetByCallsignFn = func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		require.Equal(t, int32(42), session)
		require.Equal(t, "SAS123", callsign)
		return &models.Strip{
			ID:       7,
			Session:  42,
			Callsign: "SAS123",
		}, nil
	}
	coordRepo.GetByStripIDFn = func(_ context.Context, session int32, stripID int32) (*models.Coordination, error) {
		require.Equal(t, int32(42), session)
		require.Equal(t, int32(7), stripID)
		return &models.Coordination{
			Session: 42,
			StripID: 7,
		}, nil
	}

	srv := &Server{
		stripRepo: stripRepo,
		coordRepo: coordRepo,
	}

	err := srv.UpdateRouteForStripContext(context.Background(), "SAS123", 42, true)

	require.NoError(t, err)
}

func TestComputeNextOwnersForStrip_PreservesRouteDuringPendingCoordination(t *testing.T) {
	strip := &models.Strip{ID: 7, Session: 42, NextOwners: []string{"121.630", "118.580"}}
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, session int32, stripID int32) (*models.Coordination, error) {
			require.Equal(t, int32(42), session)
			require.Equal(t, strip.ID, stripID)
			return &models.Coordination{Session: session, StripID: stripID}, nil
		},
	}

	nextOwners, handled, err := (&Server{coordRepo: coordRepo}).ComputeNextOwnersForStripContext(context.Background(), strip, 42)

	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, strip.NextOwners, nextOwners)
	assert.NotSame(t, &strip.NextOwners[0], &nextOwners[0])
}

func TestComputeNextDisplayForStrip_PreservesDisplayDuringPendingCoordination(t *testing.T) {
	coordRepo := &testutil.MockCoordinationRepository{}
	strip := &models.Strip{
		ID:      7,
		Session: 42,
		NextDisplay: &models.NextDisplay{
			Label:     "GW",
			Frequency: "118.580",
		},
	}
	coordRepo.GetByStripIDFn = func(_ context.Context, session int32, stripID int32) (*models.Coordination, error) {
		require.Equal(t, int32(42), session)
		require.Equal(t, int32(7), stripID)
		return &models.Coordination{Session: 42, StripID: 7}, nil
	}

	srv := &Server{coordRepo: coordRepo}
	display, err := srv.ComputeNextDisplayForStripContext(context.Background(), strip, 42)

	require.NoError(t, err)
	require.NotNil(t, display)
	assert.Equal(t, "GW", display.Label)
	assert.Equal(t, "118.580", display.Frequency)
	assert.NotSame(t, strip.NextDisplay, display)
}

func TestComputeNextDisplaysForStrips_PreservesDisplaysDuringPendingCoordination(t *testing.T) {
	coordRepo := &testutil.MockCoordinationRepository{}
	strip := &models.Strip{
		ID:      7,
		Session: 42,
		NextDisplay: &models.NextDisplay{
			Label:     "GW",
			Frequency: "118.580",
		},
	}
	coordRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.Coordination, error) {
		require.Equal(t, int32(42), session)
		return []*models.Coordination{{Session: 42, StripID: 7}}, nil
	}

	srv := &Server{coordRepo: coordRepo}
	err := srv.ComputeNextDisplaysForStripsContext(context.Background(), []*models.Strip{strip}, 42)

	require.NoError(t, err)
	require.NotNil(t, strip.NextDisplay)
	assert.Equal(t, "GW", strip.NextDisplay.Label)
	assert.Equal(t, "118.580", strip.NextDisplay.Frequency)
}

func TestComputeNextDisplayForStrip_ReconstructsMissingDisplayDuringPendingCoordination(t *testing.T) {
	strip, sessionRepo, sectorRepo, controllerRepo, deliveryPosition, apronOwnerPosition := pendingDepartureDisplayFixture(t)
	apronDisplayFrequency := frequencyForPosition(t, "EKCH_C_GND")
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, session int32, stripID int32) (*models.Coordination, error) {
			require.Equal(t, int32(42), session)
			require.Equal(t, strip.ID, stripID)
			return &models.Coordination{Session: session, StripID: stripID}, nil
		},
	}
	var persistedDisplay *models.NextDisplay
	stripRepo := &testutil.MockStripRepository{
		SetRouteStateFn: func(_ context.Context, session int32, callsign string, nextOwners []string, display *models.NextDisplay) error {
			require.Equal(t, int32(42), session)
			require.Equal(t, strip.Callsign, callsign)
			assert.Equal(t, strip.NextOwners, nextOwners)
			persistedDisplay = cloneNextDisplay(display)
			return nil
		},
	}

	srv := &Server{
		coordRepo:      coordRepo,
		stripRepo:      stripRepo,
		sessionRepo:    sessionRepo,
		sectorRepo:     sectorRepo,
		controllerRepo: controllerRepo,
		frequencyProviders: []TransceiverLookup{routeTransceiverStub{
			"EKCH_A_GND": {apronDisplayFrequency},
		}},
	}
	display, err := srv.ComputeNextDisplayForStripContext(context.Background(), strip, 42)

	require.NoError(t, err)
	require.NotNil(t, display)
	assert.Equal(t, "AD", display.Label)
	assert.Equal(t, apronDisplayFrequency, display.Frequency)
	assert.Equal(t, apronOwnerPosition, strip.NextOwners[0])
	assert.Equal(t, deliveryPosition, *strip.Owner)
	assert.Equal(t, display, persistedDisplay)
}

func TestComputeNextDisplaysForStrips_ReconstructsMissingDisplayDuringPendingCoordination(t *testing.T) {
	strip, sessionRepo, sectorRepo, controllerRepo, _, apronOwnerPosition := pendingDepartureDisplayFixture(t)
	apronDisplayFrequency := frequencyForPosition(t, "EKCH_C_GND")
	coordRepo := &testutil.MockCoordinationRepository{
		ListBySessionFn: func(_ context.Context, session int32) ([]*models.Coordination, error) {
			require.Equal(t, int32(42), session)
			return []*models.Coordination{{Session: session, StripID: strip.ID}}, nil
		},
	}
	var persistedDisplay *models.NextDisplay
	stripRepo := &testutil.MockStripRepository{
		SetRouteStateFn: func(_ context.Context, _ int32, _ string, nextOwners []string, display *models.NextDisplay) error {
			assert.Equal(t, strip.NextOwners, nextOwners)
			persistedDisplay = cloneNextDisplay(display)
			return nil
		},
	}

	srv := &Server{
		coordRepo:      coordRepo,
		stripRepo:      stripRepo,
		sessionRepo:    sessionRepo,
		sectorRepo:     sectorRepo,
		controllerRepo: controllerRepo,
		frequencyProviders: []TransceiverLookup{routeTransceiverStub{
			"EKCH_A_GND": {apronDisplayFrequency},
		}},
	}
	err := srv.ComputeNextDisplaysForStripsContext(context.Background(), []*models.Strip{strip}, 42)

	require.NoError(t, err)
	require.NotNil(t, strip.NextDisplay)
	assert.Equal(t, "AD", strip.NextDisplay.Label)
	assert.Equal(t, apronDisplayFrequency, strip.NextDisplay.Frequency)
	assert.Equal(t, apronOwnerPosition, strip.NextOwners[0])
	assert.Equal(t, strip.NextDisplay, persistedDisplay)
}

func TestComputeNextDisplayForStrip_PreservesPersistedCrossCoupledDepartureDisplayWhenStandTemporarilyMissing(t *testing.T) {
	strip, sessionRepo, _, controllerRepo, _, apronOwnerPosition := pendingDepartureDisplayFixture(t)
	apronDisplayFrequency := frequencyForPosition(t, "EKCH_C_GND")
	strip.NextDisplay = &models.NextDisplay{Label: "AD", Frequency: apronDisplayFrequency}
	strip.Stand = nil
	groundWestPosition := frequencyForPosition(t, "EKCH_GW_TWR")
	towerWestPosition := frequencyForPosition(t, "EKCH_A_TWR")
	sectorRepo := &testutil.MockSectorOwnerRepository{
		ListBySessionFn: func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
			require.Equal(t, int32(42), session)
			return []*models.SectorOwner{
				{
					Session:    session,
					Sector:     []string{"SQ", "AD"},
					Position:   apronOwnerPosition,
					Identifier: "SQ",
				},
				{Session: session, Sector: []string{"GWD"}, Position: groundWestPosition},
				{Session: session, Sector: []string{"TW"}, Position: towerWestPosition},
			}, nil
		},
	}
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(_ context.Context, session int32, stripID int32) (*models.Coordination, error) {
			return &models.Coordination{Session: session, StripID: stripID}, nil
		},
	}
	srv := &Server{
		coordRepo:      coordRepo,
		sessionRepo:    sessionRepo,
		sectorRepo:     sectorRepo,
		controllerRepo: controllerRepo,
		frequencyProviders: []TransceiverLookup{routeTransceiverStub{
			"EKCH_A_GND": {apronDisplayFrequency},
		}},
	}

	display, err := srv.ComputeNextDisplayForStripContext(context.Background(), strip, 42)

	require.NoError(t, err)
	require.NotNil(t, display)
	assert.Equal(t, "AD", display.Label)
	assert.Equal(t, apronDisplayFrequency, display.Frequency)
	assert.Equal(t, apronOwnerPosition, strip.NextOwners[0])
}

func TestUpdateRoutesForSession_ReturnsFirstStripError(t *testing.T) {

	arrivalRunway, towerSector := mustArrivalRunwayAndTowerSector(t)
	expectedErr := errors.New("set next owners failed")

	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	strips := []*models.Strip{
		{Callsign: "SAS123", Session: 42, Destination: "EKCH"},
		{Callsign: "KLM456", Session: 42, Destination: "EKCH"},
	}

	var updatedCallsigns []string

	stripRepo.ListFn = func(_ context.Context, session int32) ([]*models.Strip, error) {
		require.Equal(t, int32(42), session)
		return strips, nil
	}
	stripRepo.SetRouteStateFn = func(_ context.Context, session int32, callsign string, nextOwners []string, _ *models.NextDisplay) error {
		require.Equal(t, int32(42), session)
		assert.Equal(t, []string{"EKCH_TWR"}, nextOwners)
		updatedCallsigns = append(updatedCallsigns, callsign)
		if callsign == "KLM456" {
			return expectedErr
		}
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(42), id)
		return &models.Session{
			ID:      42,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{arrivalRunway},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(42), session)
		return []*models.SectorOwner{
			{
				Session:  42,
				Sector:   []string{towerSector},
				Position: "EKCH_TWR",
			},
		}, nil
	}

	srv := &Server{
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err := srv.UpdateRoutesForSession(42, false)
	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, []string{"SAS123", "KLM456"}, updatedCallsigns)
}

func mustArrivalRunwayAndTowerSector(t *testing.T) (string, string) {
	t.Helper()

	for _, runway := range config.GetRunways() {
		if towerSector, ok := config.GetArrivalTowerSector([]string{runway}); ok {
			return runway, towerSector
		}
	}

	t.Fatal("expected at least one configured arrival runway with a tower sector")
	return "", ""
}

func pendingDepartureDisplayFixture(t *testing.T) (*models.Strip, *testutil.MockSessionRepository, *testutil.MockSectorOwnerRepository, *testutil.MockControllerRepository, string, string) {
	t.Helper()

	deliveryPosition := frequencyForPosition(t, "EKCH_DEL")
	apronOwnerPosition := frequencyForPosition(t, "EKCH_A_GND")
	groundWestPosition := frequencyForPosition(t, "EKCH_GW_TWR")
	towerWestPosition := frequencyForPosition(t, "EKCH_A_TWR")
	strip := &models.Strip{
		ID:          7,
		Session:     42,
		Callsign:    "SAS909",
		Origin:      "EKCH",
		Destination: "EKYT",
		Runway:      stringPtr("22R"),
		Stand:       stringPtr("C35"),
		Owner:       stringPtr(deliveryPosition),
		NextOwners:  []string{apronOwnerPosition, groundWestPosition, towerWestPosition},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, session int32) (*models.Session, error) {
			require.Equal(t, int32(42), session)
			return &models.Session{
				ID:      session,
				Airport: "EKCH",
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{"22R"},
				},
			}, nil
		},
	}
	sectorRepo := &testutil.MockSectorOwnerRepository{
		ListBySessionFn: func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
			require.Equal(t, int32(42), session)
			return []*models.SectorOwner{
				{Session: session, Sector: []string{"SQ"}, Position: deliveryPosition},
				{Session: session, Sector: []string{"AD"}, Position: apronOwnerPosition},
				{Session: session, Sector: []string{"GWD"}, Position: groundWestPosition},
				{Session: session, Sector: []string{"TW"}, Position: towerWestPosition},
			}, nil
		},
	}
	controllerRepo := &testutil.MockControllerRepository{
		ListFn: func(_ context.Context, session int32) ([]*models.Controller, error) {
			require.Equal(t, int32(42), session)
			return []*models.Controller{{Session: session, Callsign: "EKCH_A_GND", Position: apronOwnerPosition}}, nil
		},
	}

	return strip, sessionRepo, sectorRepo, controllerRepo, deliveryPosition, apronOwnerPosition
}

func frequencyForPosition(t *testing.T, name string) string {
	t.Helper()

	position, err := config.GetPositionByName(name)
	require.NoError(t, err)
	return position.Frequency
}

func stringPtr(value string) *string {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}
