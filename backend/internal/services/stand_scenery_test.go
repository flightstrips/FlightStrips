package services

import (
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"context"
	"errors"
	"testing"

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
