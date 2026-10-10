package pdc

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/testutil"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emptyFrequencyTestControllerRepository() *testutil.MockControllerRepository {
	return &testutil.MockControllerRepository{
		ListBySessionFn: func(context.Context, int32) ([]*models.Controller, error) {
			return nil, errors.New("no online controllers")
		},
	}
}

func TestMain(m *testing.M) {
	// Change to backend root so config/ekch.yaml is found.
	if err := os.Chdir("../.."); err != nil {
		panic("failed to chdir to backend root: " + err.Error())
	}
	if err := config.InitConfig(); err != nil {
		panic("failed to initialize config: " + err.Error())
	}
	code := m.Run()
	if err := testdata.ShutdownTestDB(); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}

type staticTransceiverLookup struct {
	frequenciesByCallsign map[string][]string
}

func (s staticTransceiverLookup) GetFrequencies(callsign string) []string {
	return append([]string(nil), s.frequenciesByCallsign[callsign]...)
}

type clearanceOwnerResolverFunc func(context.Context, *models.Strip, int32) (string, bool, error)

func (f clearanceOwnerResolverFunc) ResolveClearedStripOwnerContext(ctx context.Context, strip *models.Strip, session int32) (string, bool, error) {
	return f(ctx, strip, session)
}

type clearanceSequenceRepository struct {
	*testutil.MockSessionRepository
}

func (clearanceSequenceRepository) IncrementPdcSequence(context.Context, int32) (int32, error) {
	return 1, nil
}
func (clearanceSequenceRepository) IncrementPdcMessageSequence(context.Context, int32) (int32, error) {
	return 1, nil
}

func TestClearanceTextUsesStripSpecificRecipient(t *testing.T) {
	for _, test := range []struct{ name, stand, frequency string }{
		{"planner", "G120", "121.905"},
		{"delivery as planner", "G120", "119.905"},
		{"north apron", "A12", "121.730"},
		{"south ground", "273-1", "121.830"},
		{"west ground", "RII", "118.580"},
		{"cross coupled primary", "G120", "121.630"},
	} {
		t.Run(test.name, func(t *testing.T) {
			strip := &models.Strip{
				Callsign: "SAS123", Origin: "EKCH", Destination: "ENGM",
				Stand: &test.stand, Runway: stringPtr("22R"), Sid: stringPtr("ODDON1F"),
				AssignedSquawk: stringPtr("2401"),
				PdcData:        (&models.PdcData{Web: &models.PdcWebData{}}).Normalize(),
			}
			calls := 0
			svc := &Service{
				sessionRepo:    clearanceSequenceRepository{&testutil.MockSessionRepository{}},
				controllerRepo: emptyFrequencyTestControllerRepository(),
				sectorRepo:     &testutil.MockSectorOwnerRepository{ListBySessionFn: func(context.Context, int32) ([]*models.SectorOwner, error) { return nil, nil }},
				frontendHub:    &mockPdcFrontendHub{},
				clearanceOwnerResolver: clearanceOwnerResolverFunc(func(_ context.Context, got *models.Strip, session int32) (string, bool, error) {
					calls++
					require.Same(t, strip, got)
					require.Equal(t, int32(42), session)
					return test.frequency, true, nil
				}),
			}
			for _, web := range []bool{false, true} {
				options, err := svc.BuildClearanceOptions(context.Background(), sessionInformation{id: 42}, strip, "", web, nil)
				require.NoError(t, err)
				assert.Equal(t, test.frequency, options.NextFrequency)
				assert.Contains(t, buildPDCClearance(options), "NEXT FRQ: @"+test.frequency+"@")
				assert.Contains(t, buildWebPDCClearance(options), "NEXT FRQ: "+test.frequency)
				assert.False(t, strip.Cleared, "building a clearance must not assume it before acknowledgment")
			}
			assert.Equal(t, 2, calls)
		})
	}
}

func TestGetNextFrequencyRejectsUnresolvedRecipient(t *testing.T) {
	lookupErr := errors.New("controller lookup failed")
	for _, test := range []struct {
		name     string
		resolver ClearanceOwnerResolver
		want     string
	}{
		{"missing resolver", nil, "clearance owner resolver is unavailable"},
		{"no route or recipient", clearanceOwnerResolverFunc(func(context.Context, *models.Strip, int32) (string, bool, error) { return "", false, nil }), "no clearance recipient frequency found"},
		{"empty frequency", clearanceOwnerResolverFunc(func(context.Context, *models.Strip, int32) (string, bool, error) { return "", true, nil }), "no clearance recipient frequency found"},
		{"lookup error", clearanceOwnerResolverFunc(func(context.Context, *models.Strip, int32) (string, bool, error) { return "", false, lookupErr }), "failed to resolve clearance owner: controller lookup failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := &Service{clearanceOwnerResolver: test.resolver}
			freq, err := svc.getNextFrequency(context.Background(), 42, &models.Strip{Callsign: "SAS123"})
			require.EqualError(t, err, test.want)
			assert.Empty(t, freq)
			if test.name == "lookup error" {
				assert.ErrorIs(t, err, lookupErr)
			}
		})
	}
}

// ── getAirborneFrequency (Departure frequency in clearance = SID-specific airborne sector) ─

func TestGetAirborneFrequency_UsesSidSpecificSectorPriority(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
		{Sector: []string{"R_DEP"}, Position: "120.255", Identifier: "R_DEP"},
		{Sector: []string{"SQ", "DEL"}, Position: "119.905", Identifier: "DEL"}, // not airborne
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}
	sid := "BETUD2A"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "124.980", freq)
}

func TestGetAirborneFrequency_UsesDifferentSidSpecificSectorPriority(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
		{Sector: []string{"R_DEP"}, Position: "120.255", Identifier: "R_DEP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}
	sid := "GOLGA2C"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "120.255", freq)
}

func TestGetAirborneFrequency_FallsBackWithinSidSpecificPriority(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"R_DEP"}, Position: "120.255", Identifier: "R_DEP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}
	sid := "BETUD2A"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "120.255", freq)
}

func TestGetAirborneFrequency_NoAirborneOnline_ReturnsUNICOM(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"SQ", "DEL"}, Position: "119.905", Identifier: "DEL"},
		{Sector: []string{"TE", "TW"}, Position: "118.105", Identifier: "TE"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}
	sid := "BETUD2A"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "122.8", freq)
}

func TestGetAirborneFrequency_UsesDefaultSectorWhenSidMissing(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
		{Sector: []string{"R_DEP"}, Position: "120.255", Identifier: "R_DEP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, nil)
	require.NoError(t, err)
	assert.Equal(t, "124.980", freq)
}

func TestGetAirborneFrequency_UsesOnlineControllersWhenSectorOwnersAreStale(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"SQ", "DEL"}, Position: "119.905", Identifier: "DEL"},
	})

	controllerRepo := &testutil.MockControllerRepository{
		ListBySessionFn: func(ctx context.Context, session int32) ([]*models.Controller, error) {
			return []*models.Controller{
				{Session: session, Position: "120.2550"},
			}, nil
		},
	}

	svc := &Service{
		sectorRepo:     postgres.NewSectorOwnerRepository(dbPool),
		controllerRepo: controllerRepo,
	}
	sid := "GOLGA2C"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "120.255", freq)
}

func TestGetAirborneFrequency_UsesCoveredFrequencyForCrossCoupledController(t *testing.T) {
	t.Parallel()

	sessionID := int32(1)

	controllerRepo := &testutil.MockControllerRepository{
		ListBySessionFn: func(ctx context.Context, session int32) ([]*models.Controller, error) {
			return []*models.Controller{
				{Session: session, Callsign: "EKCH_O_APP", Position: "118.455"},
			}, nil
		},
	}

	svc := &Service{
		sectorRepo: &testutil.MockSectorOwnerRepository{
			ListBySessionFn: func(ctx context.Context, session int32) ([]*models.SectorOwner, error) {
				return []*models.SectorOwner{
					{Session: session, Sector: []string{"SQ", "DEL"}, Position: "119.905", Identifier: "DEL"},
				}, nil
			},
		},
		controllerRepo: controllerRepo,
		transceiverLookups: []TransceiverLookup{staticTransceiverLookup{frequenciesByCallsign: map[string][]string{
			"EKCH_O_APP": {"124.980"},
		}}},
	}
	sid := "BETUD2A"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "124.980", freq)
}

func TestGetAirborneFrequency_UsesDefaultSectorForUnknownSid(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
		{Sector: []string{"R_DEP"}, Position: "120.255", Identifier: "R_DEP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}
	sid := "UNKNOWN1A"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "124.980", freq)
}
