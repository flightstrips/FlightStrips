package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pilotSceneryStub struct {
	scenery string
	err     error
	calls   int
}

func (s *pilotSceneryStub) PilotScenery(context.Context, int64, string) (string, error) {
	s.calls++
	return s.scenery, s.err
}

func TestPilotSceneryResolvesFlyTampaExclusions(t *testing.T) {
	cid := int64(1234567)
	request := StandAllocationRequest{Callsign: "SAS1", Airport: "EKCH", VatsimCID: &cid}

	lookup := &pilotSceneryStub{scenery: "flytampa"}
	resolved := (&StandAllocationService{sceneries: lookup}).withPilotScenery(context.Background(), request)
	for _, stand := range []string{"E20", "E22", "E24", "E25", "E27", "E29", "E31", "E33", "E35", "e36"} {
		assert.True(t, resolved.sceneryExcludes(stand), stand)
	}
	for _, stand := range []string{"E21", "E26", "E37", "A1"} {
		assert.False(t, resolved.sceneryExcludes(stand), stand)
	}
	assert.Equal(t, "E25 is not available in FlyTampa Copenhagen", resolved.sceneryExclusionReason("e25"))

	(&StandAllocationService{sceneries: lookup}).withPilotScenery(context.Background(), resolved)
	assert.Equal(t, 1, lookup.calls, "a resolved request must not be looked up again")

	for name, service := range map[string]*StandAllocationService{
		"simnord":        {sceneries: &pilotSceneryStub{scenery: "simnord"}},
		"default":        {sceneries: &pilotSceneryStub{scenery: "default"}},
		"undeclared":     {sceneries: &pilotSceneryStub{}},
		"lookup failure": {sceneries: &pilotSceneryStub{scenery: "flytampa", err: errors.New("down")}},
		"no lookup":      {},
	} {
		assert.False(t, service.withPilotScenery(context.Background(), request).sceneryExcludes("E25"), name)
	}
	withoutCID := request
	withoutCID.VatsimCID = nil
	assert.False(t, (&StandAllocationService{sceneries: &pilotSceneryStub{scenery: "flytampa"}}).withPilotScenery(context.Background(), withoutCID).sceneryExcludes("E25"))
}

func excludingA1(request StandAllocationRequest) StandAllocationRequest {
	request.SceneryExcludedStands, request.SceneryName = map[string]struct{}{"A1": {}}, "Test Scenery"
	return request
}

func TestSceneryExcludedStandsInAllocation(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()

	t.Run("automatic allocation skips the excluded preferred stand", func(t *testing.T) {
		service, session, _ := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS201")
		result, err := service.Allocate(ctx, excludingA1(standAllocationRequest(session, "SAS201")))
		require.NoError(t, err)
		assert.Equal(t, "A2", result.Assignment.Stand)
	})

	t.Run("a pilot cannot pick an excluded stand even when accepting unsuitable stands", func(t *testing.T) {
		service, session, _ := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS202")
		request := excludingA1(withStand(standAllocationRequest(session, "SAS202"), "A1"))
		request.AllowUnsuitable = true
		_, err := service.AssignManually(ctx, request)
		require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
		assert.Contains(t, err.Error(), "A1 is not available in Test Scenery")
	})

	for _, tc := range []struct {
		name, directive string
		exclude         bool
	}{
		{"scenery excludes the only compatible stand", "", true},
		{"the only compatible stand is closed to arrivals", "ARRIVAL_CLOSED:Not available for arrivals", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, session, _ := standAllocationFixture(t, pool, queries, tc.directive, "ATYP:B737")
			testdata.SeedTestStrip(t, queries, session, "SAS205")
			request := withStand(standAllocationRequest(session, "SAS205"), "A2")
			if tc.exclude {
				request = excludingA1(request)
			}
			_, err := service.AssignManually(ctx, request)
			require.ErrorIs(t, err, ErrIncompatibleManualAssignment)

			request.AllowUnsuitable = true
			result, err := service.AssignManually(ctx, request)
			require.NoError(t, err)
			require.Equal(t, "A2", result.Assignment.Stand)
			require.NotNil(t, result.Assignment.ConflictReason)
			assert.Contains(t, *result.Assignment.ConflictReason, "pilot confirmed unsuitable stand")
		})
	}

	t.Run("availability marks the excluded stand unselectable with the reason", func(t *testing.T) {
		service, session, _ := standAllocationFixture(t, pool, queries, "", "")
		available, err := service.AvailableStands(ctx, excludingA1(standAllocationRequest(session, "SAS203")))
		require.NoError(t, err)
		require.Len(t, available, 2)
		assert.Equal(t, "A1", available[0].Stand)
		assert.False(t, available[0].Available)
		assert.False(t, available[0].Selectable)
		assert.Equal(t, "A1 is not available in Test Scenery", available[0].Reason)
		assert.True(t, available[1].Available)

		free, err := service.StandAvailable(ctx, excludingA1(standAllocationRequest(session, "SAS203")), "A1")
		require.NoError(t, err)
		assert.False(t, free, "an existing reservation on an excluded stand must not be renewed")
	})

	t.Run("a controller override can still assign the excluded stand", func(t *testing.T) {
		service, session, _ := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS204")
		request := excludingA1(withStand(standAllocationRequest(session, "SAS204"), "A1"))
		request.ConflictReason = "pilot asked"
		result, err := service.OverrideManually(ctx, request)
		require.NoError(t, err)
		assert.Equal(t, "A1", result.Assignment.Stand)
	})

	t.Run("the stored FlyTampa choice is looked up by CID", func(t *testing.T) {
		repo := postgres.NewPilotSceneryRepository(pool)
		value, err := repo.PilotScenery(ctx, 7654321, "ekch")
		require.NoError(t, err)
		assert.Empty(t, value)
		require.NoError(t, repo.SetPilotScenery(ctx, 7654321, "ekch", "SimNord"))
		require.NoError(t, repo.SetPilotScenery(ctx, 7654321, "EKCH", "flytampa"))
		value, err = repo.PilotScenery(ctx, 7654321, "EKCH")
		require.NoError(t, err)
		assert.Equal(t, "flytampa", value)

		cid := int64(7654321)
		service := &StandAllocationService{sceneries: repo}
		resolved := service.withPilotScenery(ctx, StandAllocationRequest{Airport: "EKCH", VatsimCID: &cid})
		assert.True(t, resolved.sceneryExcludes("E25"))
	})
}

func TestSceneryExclusionKeepsAControllerOverrideOnAnAssignedArrival(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	registry, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:E25:N055.37.42.710:E012.38.33.450:30
STAND:EKCH:E26:N055.37.42.710:E012.38.33.451:30
`))
	require.NoError(t, err)
	policy, err := sat.LoadAirlineAssignment(strings.NewReader(`{
  "rules": [{"id":"sas","callsigns":["SAS"],"stands":{"tier1":{"E25":100,"E26":100}}}],
  "stand_groups": {}, "fallback_rules": {`+testFallbackJSON("E26")+`}
}`), registry)
	require.NoError(t, err)
	assignments := postgres.NewStandAssignmentRepository(pool)
	strips := postgres.NewStripRepository(pool)
	clock := &fakeClock{now: time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)}
	allocations, err := NewStandAllocationService(pool, strips, assignments, registry, policy,
		WithStandAllocationRandom(func() float64 { return 0 }),
		WithStandAllocationClock(clock.current),
	)
	require.NoError(t, err)
	allocations.SetPilotSceneries(&pilotSceneryStub{scenery: "flytampa"})
	lifecycle, err := NewArrivalLifecycleService(allocations, assignments, strips, postgres.NewSessionRepository(pool), registry, nil, nil, sat.NewAirportCountryRegistry(),
		WithArrivalLifecycleClock(clock.current),
		WithArrivalPrefileAssignments(true),
	)
	require.NoError(t, err)
	session := testdata.SeedTestSessionNamedWithSectors(t, queries, fmt.Sprintf("%s-%d", t.Name(), standAllocationSessionSequence.Add(1)), nil)

	seedTestArrivalStrip(t, queries, session, "SAS951")
	arrivalETA := clock.current().Add(5 * time.Minute)
	setArrivalETA(t, strips, session, "SAS951", arrivalETA)
	tier := int32(1)
	reason := "pilot asked"
	require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
		SessionID: session, Callsign: "SAS951", Stand: "E25", Direction: "ARRIVAL",
		Stage: StageAssigned, Source: string(IncompatibleManualOverride), Manual: true,
		Tier: &tier, ETA: &arrivalETA, ConflictReason: &reason,
	}))
	_, err = strips.UpdateStand(ctx, session, "SAS951", strp("E25"), nil)
	require.NoError(t, err)
	_, err = strips.UpdateAircraftPosition(ctx, session, "SAS951", posPtr(55.8), posPtr(12.642625), altPtr(8000), "FINAL", nil)
	require.NoError(t, err)

	require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS951"), arrivalFlight("SAS951", 1)))

	kept, err := assignments.GetAssignment(ctx, session, "SAS951")
	require.NoError(t, err)
	assert.Equal(t, "E25", kept.Stand, "FlyTampa lacks E25, but the controller override must stay")
	assert.Equal(t, string(IncompatibleManualOverride), kept.Source)
	assert.True(t, kept.Manual)
}
