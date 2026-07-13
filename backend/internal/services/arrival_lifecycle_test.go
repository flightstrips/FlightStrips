package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArrivalLifecycle(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()

	t.Run("at ETA−45 min allocates ESTIMATED and does not transition earlier", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(50 * time.Minute)
		clock.set(arrivalETA.Add(-50 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS101")
		setArrivalETA(t, strips, session, "SAS101", arrivalETA)

		err := lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS101"), arrivalFlight("SAS101", 1))
		require.NoError(t, err)

		_, err = assignments.GetAssignment(ctx, session, "SAS101")
		require.Error(t, err, "too early for any stage")

		clock.advance(6 * time.Minute)

		err = lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS101"), arrivalFlight("SAS101", 1))
		require.NoError(t, err)

		assignment, err := assignments.GetAssignment(ctx, session, "SAS101")
		require.NoError(t, err)
		assert.Equal(t, StageEstimated, assignment.Stage)
		assert.NotEmpty(t, assignment.Stand)
	})

	t.Run("time alone promotes to ASSIGNED without altitude", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS201")
		setArrivalETA(t, strips, session, "SAS201", arrivalETA)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS201"), arrivalFlight("SAS201", 1)))

		clock.set(arrivalETA.Add(-5 * time.Minute))

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS201"), arrivalFlight("SAS201", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS201")
		require.NoError(t, err)
		assert.Equal(t, StageAssigned, assignment.Stage, "promoted to ASSIGNED by time alone within 10 min of ETA")
	})

	t.Run("transitions to ASSIGNED at ETA−10 min and below 10000 ft", func(t *testing.T) {
		lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		var published []StandAllocationResult
		allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
			published = append(published, result)
			return nil
		})
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS301")
		setArrivalETA(t, strips, session, "SAS301", arrivalETA)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS301"), arrivalFlight("SAS301", 1)))
		published = nil

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS301", posPtr(0), posPtr(0), altPtr(8000), "FINAL", nil)
		require.NoError(t, err)

		clock.set(arrivalETA.Add(-5 * time.Minute))

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS301"), arrivalFlight("SAS301", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS301")
		require.NoError(t, err)
		assert.Equal(t, StageAssigned, assignment.Stage, "promoted to ASSIGNED at ETA−5 min below 8000 ft")
		require.Len(t, published, 1, "an in-place lifecycle transition is published")
		assert.Equal(t, StageAssigned, published[0].Assignment.Stage)
		assert.Equal(t, assignment.Version, published[0].Assignment.Version)
	})

	t.Run("transitions to CONFIRMED at ETA−2 min and below 3000 ft", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS401")
		setArrivalETA(t, strips, session, "SAS401", arrivalETA)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS401"), arrivalFlight("SAS401", 1)))

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS401", posPtr(0), posPtr(0), altPtr(8000), "FINAL", nil)
		require.NoError(t, err)
		clock.set(arrivalETA.Add(-5 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS401"), arrivalFlight("SAS401", 1)))

		_, err = strips.UpdateAircraftPosition(ctx, session, "SAS401", posPtr(0), posPtr(0), altPtr(2000), "FINAL", nil)
		require.NoError(t, err)
		clock.set(arrivalETA.Add(-1 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS401"), arrivalFlight("SAS401", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS401")
		require.NoError(t, err)
		assert.Equal(t, StageConfirmed, assignment.Stage, "promoted to CONFIRMED at ETA−1 min below 2000 ft")
	})

	t.Run("CONFIRMED triggered by time alone even at higher altitude", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS402")
		setArrivalETA(t, strips, session, "SAS402", arrivalETA)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS402"), arrivalFlight("SAS402", 1)))

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS402", posPtr(0), posPtr(0), altPtr(8000), "FINAL", nil)
		require.NoError(t, err)
		clock.set(arrivalETA.Add(-5 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS402"), arrivalFlight("SAS402", 1)))

		clock.set(arrivalETA.Add(-1 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS402"), arrivalFlight("SAS402", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS402")
		require.NoError(t, err)
		assert.Equal(t, StageConfirmed, assignment.Stage, "promoted to CONFIRMED by time alone at ETA−1 min")
	})

	t.Run("ASSIGNED by altitude alone before ETA−10 min", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(60 * time.Minute)
		clock.set(arrivalETA.Add(-60 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS403")
		setArrivalETA(t, strips, session, "SAS403", arrivalETA)

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS403", posPtr(0), posPtr(0), altPtr(5000), "FINAL", nil)
		require.NoError(t, err)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS403"), arrivalFlight("SAS403", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS403")
		require.NoError(t, err)
		assert.Equal(t, StageAssigned, assignment.Stage, "promoted to ASSIGNED by altitude alone below 10000 ft")
	})

	t.Run("CONFIRMED by altitude alone well before ETA−2 min", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(30 * time.Minute)
		clock.set(arrivalETA.Add(-30 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS404")
		setArrivalETA(t, strips, session, "SAS404", arrivalETA)

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS404", posPtr(0), posPtr(0), altPtr(2000), "FINAL", nil)
		require.NoError(t, err)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS404"), arrivalFlight("SAS404", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS404")
		require.NoError(t, err)
		assert.Equal(t, StageConfirmed, assignment.Stage, "promoted to CONFIRMED by altitude alone below 3000 ft")
	})

	t.Run("transitions are idempotent", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS501")
		setArrivalETA(t, strips, session, "SAS501", arrivalETA)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS501"), arrivalFlight("SAS501", 1)))
		original, err := assignments.GetAssignment(ctx, session, "SAS501")
		require.NoError(t, err)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS501"), arrivalFlight("SAS501", 1)))
		repeated, err := assignments.GetAssignment(ctx, session, "SAS501")
		require.NoError(t, err)
		assert.Equal(t, original.Stage, repeated.Stage)
		assert.Equal(t, original.Stand, repeated.Stand)
		assert.Equal(t, original.Version, repeated.Version)
	})

	t.Run("stage promotion never downgrades", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS601")
		setArrivalETA(t, strips, session, "SAS601", arrivalETA)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS601"), arrivalFlight("SAS601", 1)))

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS601", posPtr(0), posPtr(0), altPtr(8000), "FINAL", nil)
		require.NoError(t, err)
		clock.set(arrivalETA.Add(-5 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS601"), arrivalFlight("SAS601", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS601")
		require.NoError(t, err)
		assert.Equal(t, StageAssigned, assignment.Stage)

		clock.set(arrivalETA.Add(-50 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS601"), arrivalFlight("SAS601", 1)))

		assignment, err = assignments.GetAssignment(ctx, session, "SAS601")
		require.NoError(t, err)
		assert.Equal(t, StageAssigned, assignment.Stage, "never downgrades from ASSIGNED back to ESTIMATED")
	})

	t.Run("ESTIMATED is displaced by later-stage arrival", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))

		seedTestArrivalStrip(t, queries, session, "SAS701")
		seedTestArrivalStrip(t, queries, session, "SAS702")
		setArrivalETA(t, strips, session, "SAS701", arrivalETA)
		setArrivalETA(t, strips, session, "SAS702", arrivalETA)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS701"), arrivalFlight("SAS701", 1)))
		require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
			SessionID: session, Callsign: "SAS702", Stand: "A1", Direction: "ARRIVAL",
			Stage: StageEstimated, Source: "AUTOMATIC", ETA: &arrivalETA,
		}))
		_, err := strips.UpdateStand(ctx, session, "SAS702", strp("A1"), nil)
		require.NoError(t, err)

		_, err = strips.UpdateAircraftPosition(ctx, session, "SAS702", posPtr(0), posPtr(0), altPtr(8000), "FINAL", nil)
		require.NoError(t, err)
		clock.set(arrivalETA.Add(-5 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS702"), arrivalFlight("SAS702", 2)))

		updated702, err := assignments.GetAssignment(ctx, session, "SAS702")
		require.NoError(t, err)
		assert.Equal(t, StageAssigned, updated702.Stage)
		assert.Equal(t, "A1", updated702.Stand, "promoted arrival keeps A1")

		displaced701, err := assignments.GetAssignment(ctx, session, "SAS701")
		if err == nil && displaced701 != nil {
			assert.NotEqual(t, "A1", displaced701.Stand, "displaced ESTIMATED arrival must not keep A1")
		}
		strip701 := loadStrip(t, strips, session, "SAS701")
		if strip701.Stand != nil {
			assert.NotEqual(t, "A1", *strip701.Stand, "displaced ESTIMATED arrival's strip stand must not be A1")
		}
	})

	t.Run("departure block expiring before arrival ETA is compatible", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		now := time.Now().UTC()
		clock.set(now)
		arrivalETA := now.Add(45 * time.Minute)
		seedTestArrivalStrip(t, queries, session, "SAS1001")
		setArrivalETA(t, strips, session, "SAS1001", arrivalETA)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS1001"), arrivalFlight("SAS1001", 1)))

		expiresAt := arrivalETA.Add(-5 * time.Minute)
		require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
			SessionID: session, Callsign: "SAS1002", Stand: "A1", Direction: "DEPARTURE",
			Stage: StageDepartureBlock, Source: "AUTOMATIC", ExpiresAt: &expiresAt,
		}))

		clock.advance(5 * time.Minute)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS1001"), arrivalFlight("SAS1001", 1)))

		updated, err := assignments.GetAssignment(ctx, session, "SAS1001")
		require.NoError(t, err)
		assert.Equal(t, "A1", updated.Stand, "arrival keeps A1 when departure block expires before ETA")
	})

	t.Run("departure block extending past arrival ETA forces reallocation", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		now := time.Now().UTC()
		clock.set(now)
		arrivalETA := now.Add(45 * time.Minute)
		seedTestArrivalStrip(t, queries, session, "SAS1003")
		setArrivalETA(t, strips, session, "SAS1003", arrivalETA)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS1003"), arrivalFlight("SAS1003", 1)))

		expiresAt := arrivalETA.Add(5 * time.Minute)
		require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
			SessionID: session, Callsign: "SAS1004", Stand: "A1", Direction: "DEPARTURE",
			Stage: StageDepartureBlock, Source: "AUTOMATIC", ExpiresAt: &expiresAt,
		}))

		clock.advance(5 * time.Minute)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS1003"), arrivalFlight("SAS1003", 1)))

		updated, err := assignments.GetAssignment(ctx, session, "SAS1003")
		require.NoError(t, err)
		assert.NotEqual(t, "A1", updated.Stand, "arrival is reallocated when departure block extends past ETA")
	})

	t.Run("keeps the same optimal stand when retaining is correct", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS801")
		setArrivalETA(t, strips, session, "SAS801", arrivalETA)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS801"), arrivalFlight("SAS801", 1)))

		original, err := assignments.GetAssignment(ctx, session, "SAS801")
		require.NoError(t, err)

		_, err = strips.UpdateAircraftPosition(ctx, session, "SAS801", posPtr(0), posPtr(0), altPtr(2000), "FINAL", nil)
		require.NoError(t, err)
		clock.set(arrivalETA.Add(-1 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS801"), arrivalFlight("SAS801", 1)))

		retained, err := assignments.GetAssignment(ctx, session, "SAS801")
		require.NoError(t, err)
		assert.Equal(t, StageConfirmed, retained.Stage)
		assert.Equal(t, original.Stand, retained.Stand, "optimal Tier-1 stand is retained through promotion")
	})

	t.Run("reallocation and released expired on restart", func(t *testing.T) {
		_, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS901")
		setArrivalETA(t, strips, session, "SAS901", arrivalETA)

		require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
			SessionID: session, Callsign: "SAS901", Stand: "A1", Direction: "ARRIVAL",
			Stage: StageEstimated, Source: "AUTOMATIC", ETA: &arrivalETA,
		}))

		restarted, err := NewArrivalLifecycleService(
			allocations, assignments, strips, postgres.NewSessionRepository(pool),
			allocations.stands, nil, nil, sat.NewAirportCountryRegistry(),
			WithArrivalLifecycleClock(func() time.Time { return arrivalETA.Add(-30 * time.Minute) }),
		)
		require.NoError(t, err)

		require.NoError(t, restarted.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS901"), arrivalFlight("SAS901", 1)))
		assignment, err := assignments.GetAssignment(ctx, session, "SAS901")
		require.NoError(t, err)
		assert.Equal(t, StageEstimated, assignment.Stage)
		assert.NotEmpty(t, assignment.Stand)
	})

	t.Run("sweep cleans up arrival assignments when strip is gone", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS910")
		setArrivalETA(t, strips, session, "SAS910", arrivalETA)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS910"), arrivalFlight("SAS910", 1)))
		require.NoError(t, strips.Delete(ctx, session, "SAS910"))

		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err := assignments.GetAssignment(ctx, session, "SAS910")
		require.Error(t, err, "assignment cleaned up when strip no longer exists")
	})

	t.Run("blocked stand triggers reallocation at same stage", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS920")
		setArrivalETA(t, strips, session, "SAS920", arrivalETA)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS920"), arrivalFlight("SAS920", 1)))
		original, err := assignments.GetAssignment(ctx, session, "SAS920")
		require.NoError(t, err)

		require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{
			SessionID: session, Stand: original.Stand, BlockType: "CLOSURE", Source: "CONTROLLER", Manual: true,
		}))

		clock.advance(5 * time.Minute)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS920"), arrivalFlight("SAS920", 1)))

		reallocated, err := assignments.GetAssignment(ctx, session, "SAS920")
		require.NoError(t, err)
		assert.Equal(t, StageEstimated, reallocated.Stage)
		assert.NotEqual(t, original.Stand, reallocated.Stand, "reallocated to a different stand when original is blocked")
	})
}

func seedTestArrivalStrip(t *testing.T, queries *database.Queries, sessionID int32, callsign string) {
	ctx := context.Background()
	err := queries.InsertStrip(ctx, database.InsertStripParams{
		Callsign:       callsign,
		Session:        sessionID,
		Origin:         "ESSA",
		Destination:    "EKCH",
		AircraftType:   strp("A320"),
		Runway:         strp("22L"),
		Squawk:         strp("2401"),
		AssignedSquawk: strp("2401"),
		Bay:            "ARR_HIDDEN",
		CdmData:        []byte(`{"canonical":{}}`),
	})
	require.NoError(t, err)
}

func strp(v string) *string { return &v }

func arrivalLifecycleFixture(t *testing.T, pool *pgxpool.Pool, queries *database.Queries, a1Directive, a2Directive string, aircraft *sat.AircraftRegistry) (*ArrivalLifecycleService, *StandAllocationService, int32, repository.StandAssignmentRepository, repository.StripRepository, *fakeClock) {
	t.Helper()
	return arrivalLifecycleFixtureWithEngines(t, pool, queries, a1Directive, a2Directive, aircraft, nil)
}

func arrivalLifecycleFixtureWithEngines(t *testing.T, pool *pgxpool.Pool, queries *database.Queries, a1Directive, a2Directive string, aircraft *sat.AircraftRegistry, engines *sat.AircraftEngineRegistry) (*ArrivalLifecycleService, *StandAllocationService, int32, repository.StandAssignmentRepository, repository.StripRepository, *fakeClock) {
	t.Helper()
	registry, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
` + a1Directive + `
STAND:EKCH:A2:N055.37.42.710:E012.38.33.451:30
` + a2Directive + `
`))
	require.NoError(t, err)
	policy, err := sat.LoadAirlineAssignment(strings.NewReader(`{
  "rules": [{"id":"sas","callsigns":["SAS"],"stands":{"tier1":{"A1":100,"A2":100}}}],
  "stand_groups": {}, "fallback_rules": {`+testFallbackJSON("A1")+`}
}`), registry)
	require.NoError(t, err)
	assignments := postgres.NewStandAssignmentRepository(pool)
	strips := postgres.NewStripRepository(pool)
	sessions := postgres.NewSessionRepository(pool)
	clock := &fakeClock{now: time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)}
	allocations, err := NewStandAllocationService(pool, strips, assignments, registry, policy,
		WithStandAllocationRandom(func() float64 { return 0 }),
		WithStandAllocationClock(clock.current),
	)
	require.NoError(t, err)
	lifecycle, err := NewArrivalLifecycleService(allocations, assignments, strips, sessions, registry, aircraft, engines, sat.NewAirportCountryRegistry(),
		WithArrivalLifecycleClock(clock.current),
	)
	require.NoError(t, err)
	name := fmt.Sprintf("%s-%d", t.Name(), standAllocationSessionSequence.Add(1))
	session := testdata.SeedTestSessionNamedWithSectors(t, queries, name, nil)
	return lifecycle, allocations, session, assignments, strips, clock
}

func arrivalFlight(callsign string, revision int64) vatsim.ArrivalFlightInfo {
	return vatsim.ArrivalFlightInfo{
		Callsign: callsign, CID: "1001", Online: false, Revision: revision,
		Origin: "ESSA", Destination: "EKCH", AircraftType: "A320",
	}
}

func setArrivalETA(t *testing.T, strips repository.StripRepository, session int32, callsign string, eta time.Time) {
	t.Helper()
	_, err := strips.UpdateArrivalETA(context.Background(), session, callsign, models.ArrivalETA{
		Time: eta, Source: "FILED", CalculatedAt: eta.Add(-1 * time.Hour),
	})
	require.NoError(t, err)
}

func posPtr(value float64) *float64 { return &value }
func altPtr(value int32) *int32     { return &value }
