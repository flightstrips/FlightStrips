package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
	"FlightStrips/pkg/events/euroscope"
	"context"
	"fmt"
	"path/filepath"
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

	t.Run("allocates ESTIMATED before ETA−45 and retains it", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(50 * time.Minute)
		clock.set(arrivalETA.Add(-50 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS101")
		setArrivalETA(t, strips, session, "SAS101", arrivalETA)

		err := lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS101"), arrivalFlight("SAS101", 1))
		require.NoError(t, err)

		early, err := assignments.GetAssignment(ctx, session, "SAS101")
		require.NoError(t, err)
		assert.Equal(t, StageEstimated, early.Stage)
		assert.NotEmpty(t, early.Stand)

		clock.advance(6 * time.Minute)

		err = lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS101"), arrivalFlight("SAS101", 1))
		require.NoError(t, err)

		assignment, err := assignments.GetAssignment(ctx, session, "SAS101")
		require.NoError(t, err)
		assert.Equal(t, StageEstimated, assignment.Stage)
		assert.Equal(t, early.Stand, assignment.Stand)
	})

	t.Run("far arrival without ETA receives an advisory ESTIMATED stand", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		seedTestArrivalStrip(t, queries, session, "SAS102")

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS102"), arrivalFlight("SAS102", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS102")
		require.NoError(t, err)
		assert.Equal(t, StageEstimated, assignment.Stage)
		assert.NotEmpty(t, assignment.Stand)
		assert.Nil(t, assignment.ETA)
		assert.Nil(t, assignment.ETASource)

		eta := clock.current().Add(2 * time.Hour)
		setArrivalETA(t, strips, session, "SAS102", eta)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS102"), arrivalFlight("SAS102", 2)))

		timed, err := assignments.GetAssignment(ctx, session, "SAS102")
		require.NoError(t, err)
		require.NotNil(t, timed.ETA)
		assert.True(t, eta.Equal(*timed.ETA), "ETA instant is preserved across database timezone conversion")
		require.NotNil(t, timed.ETASource)
		assert.Equal(t, "ARRIVAL_ETA", *timed.ETASource)
		assert.Equal(t, assignment.Stand, timed.Stand)
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

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS301", posPtr(55), posPtr(10), altPtr(8000), "FINAL", nil)
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

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS401", posPtr(55), posPtr(10), altPtr(8000), "FINAL", nil)
		require.NoError(t, err)
		clock.set(arrivalETA.Add(-5 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS401"), arrivalFlight("SAS401", 1)))

		_, err = strips.UpdateAircraftPosition(ctx, session, "SAS401", posPtr(55), posPtr(10), altPtr(2000), "FINAL", nil)
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

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS402", posPtr(55), posPtr(10), altPtr(8000), "FINAL", nil)
		require.NoError(t, err)
		clock.set(arrivalETA.Add(-5 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS402"), arrivalFlight("SAS402", 1)))

		clock.set(arrivalETA.Add(-1 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS402"), arrivalFlight("SAS402", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS402")
		require.NoError(t, err)
		assert.Equal(t, StageConfirmed, assignment.Stage, "promoted to CONFIRMED by time alone at ETA−1 min")
	})

	t.Run("ASSIGNED by altitude near destination before ETA−10 min", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(60 * time.Minute)
		clock.set(arrivalETA.Add(-60 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS403")
		setArrivalETA(t, strips, session, "SAS403", arrivalETA)

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS403", posPtr(55.6285306), posPtr(12.642625), altPtr(5000), "FINAL", nil)
		require.NoError(t, err)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS403"), arrivalFlight("SAS403", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS403")
		require.NoError(t, err)
		assert.Equal(t, StageAssigned, assignment.Stage, "promoted to ASSIGNED by altitude alone below 10000 ft")
	})

	t.Run("CONFIRMED by altitude near destination well before ETA−2 min", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(30 * time.Minute)
		clock.set(arrivalETA.Add(-30 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS404")
		setArrivalETA(t, strips, session, "SAS404", arrivalETA)

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS404", posPtr(55.6285306), posPtr(12.642625), altPtr(2000), "FINAL", nil)
		require.NoError(t, err)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS404"), arrivalFlight("SAS404", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS404")
		require.NoError(t, err)
		assert.Equal(t, StageConfirmed, assignment.Stage, "promoted to CONFIRMED by altitude alone below 3000 ft")
	})

	t.Run("low altitude at origin remains ESTIMATED", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(60 * time.Minute)
		seedTestArrivalStrip(t, queries, session, "SAS405")
		setArrivalETA(t, strips, session, "SAS405", arrivalETA)

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS405", posPtr(60.1939), posPtr(11.1004), altPtr(700), "PARK", nil)
		require.NoError(t, err)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS405"), arrivalFlight("SAS405", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS405")
		require.NoError(t, err)
		assert.Equal(t, StageEstimated, assignment.Stage)
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

		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS601", posPtr(55), posPtr(10), altPtr(8000), "FINAL", nil)
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

	t.Run("later-stage arrival uses an open same-tier stand before displacing ESTIMATED", func(t *testing.T) {
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

		_, err = strips.UpdateAircraftPosition(ctx, session, "SAS702", posPtr(55), posPtr(10), altPtr(8000), "FINAL", nil)
		require.NoError(t, err)
		clock.set(arrivalETA.Add(-5 * time.Minute))
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS702"), arrivalFlight("SAS702", 2)))

		updated702, err := assignments.GetAssignment(ctx, session, "SAS702")
		require.NoError(t, err)
		assert.Equal(t, StageAssigned, updated702.Stage)
		assert.Equal(t, "A2", updated702.Stand, "an equal-tier open stand avoids unnecessary displacement")

		retained701, err := assignments.GetAssignment(ctx, session, "SAS701")
		require.NoError(t, err)
		assert.Equal(t, "A1", retained701.Stand, "the ESTIMATED reservation remains stable")
		strip701 := loadStrip(t, strips, session, "SAS701")
		require.NotNil(t, strip701.Stand)
		assert.Equal(t, "A1", *strip701.Stand)
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

		_, err = strips.UpdateAircraftPosition(ctx, session, "SAS801", posPtr(55), posPtr(10), altPtr(2000), "FINAL", nil)
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

	t.Run("stale ETA does not expire an arrival before airport detection", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		seedTestArrivalStrip(t, queries, session, "SAS911")
		setArrivalETA(t, strips, session, "SAS911", clock.current().Add(-2*time.Hour))

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS911"), arrivalFlight("SAS911", 1)))
		require.NoError(t, lifecycle.ReleaseExpired(ctx))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS911")
		require.NoError(t, err)
		assert.Nil(t, assignment.ExpiresAt, "ETA alone must not start stand-retention expiry")
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

	t.Run("arrival at the airport keeps its stand when the allocation becomes blocked", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		arrivalETA := clock.current().Add(45 * time.Minute)
		clock.set(arrivalETA.Add(-45 * time.Minute))
		seedTestArrivalStrip(t, queries, session, "SAS921")
		setArrivalETA(t, strips, session, "SAS921", arrivalETA)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS921"), arrivalFlight("SAS921", 1)))
		original, err := assignments.GetAssignment(ctx, session, "SAS921")
		require.NoError(t, err)
		require.Equal(t, "A1", original.Stand)

		require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{
			SessionID: session, Stand: original.Stand, BlockType: "CLOSURE", Source: "CONTROLLER", Manual: true,
		}))
		// This is near A1 but outside every configured stand radius, exercising
		// the airport-area guard rather than exact stand detection.
		_, err = strips.UpdateAircraftPosition(ctx, session, "SAS921", posPtr(55.6285306), posPtr(12.644625), altPtr(0), "TAXI", nil)
		require.NoError(t, err)

		clock.advance(5 * time.Minute)
		parked := arrivalFlight("SAS921", 2)
		parked.Online = true
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS921"), parked))

		retained, err := assignments.GetAssignment(ctx, session, "SAS921")
		require.NoError(t, err)
		assert.Equal(t, original.Stand, retained.Stand, "an arrival at the airport is never automatically moved to another stand")
		assert.Equal(t, StageConfirmed, retained.Stage, "the lifecycle still advances without changing the stand")
	})

	t.Run("arrival at the airport refreshes fallback retention without moving stand", func(t *testing.T) {
		lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		var published []StandAllocationResult
		allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
			published = append(published, result)
			return nil
		})
		seedTestArrivalStrip(t, queries, session, "SAS923")
		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS923", posPtr(55.6285306), posPtr(12.644625), altPtr(0), "TAXI", nil)
		require.NoError(t, err)

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS923"), arrivalFlight("SAS923", 1)))
		first, err := assignments.GetAssignment(ctx, session, "SAS923")
		require.NoError(t, err)
		require.NotNil(t, first.ExpiresAt)
		require.NotNil(t, first.AssignedAt)
		assert.Equal(t, clock.current().Add(30*time.Minute), first.ExpiresAt.UTC())
		published = nil

		clock.advance(time.Minute)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS923"), arrivalFlight("SAS923", 2)))
		repeated, err := assignments.GetAssignment(ctx, session, "SAS923")
		require.NoError(t, err)
		assert.Equal(t, first.Stand, repeated.Stand)
		assert.Equal(t, first.AssignedAt.UTC(), repeated.AssignedAt.UTC())
		assert.Equal(t, first.ExpiresAt.UTC(), repeated.ExpiresAt.UTC(), "ordinary feed polls must not slide the fallback window")
		assert.Equal(t, first.Version, repeated.Version)
		assert.Empty(t, published, "ordinary feed polls must not publish unchanged assignment snapshots")

		clock.advance(20 * time.Minute)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS923"), arrivalFlight("SAS923", 3)))
		refreshed, err := assignments.GetAssignment(ctx, session, "SAS923")
		require.NoError(t, err)
		assert.Equal(t, first.Stand, refreshed.Stand)
		assert.Equal(t, first.AssignedAt.UTC(), refreshed.AssignedAt.UTC(), "retention refresh must not look like a new allocation")
		assert.Equal(t, clock.current().Add(30*time.Minute), refreshed.ExpiresAt.UTC(), "a live update refreshes the fallback window near expiry")
		assert.Greater(t, refreshed.Version, first.Version)
		require.Len(t, published, 1, "retention bookkeeping must publish its current controller-facing version")
		assert.Equal(t, refreshed.Version, published[0].Assignment.Version)
		assert.False(t, published[0].StandChanged)
		assert.False(t, published[0].NotifyEuroscope, "retention bookkeeping must not publish a fresh EuroScope stand request")
	})

	t.Run("controller can change a locked airport-area stand", func(t *testing.T) {
		lifecycle, allocations, session, assignments, strips, _ := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		registry, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
STAND:EKCH:A2:N055.37.42.710:E012.39.03.450:30
STAND:EKCH:A3:N055.37.42.710:E012.39.33.450:30
`))
		require.NoError(t, err)
		lifecycle.stands = registry
		allocations.stands = registry
		var published []StandAllocationResult
		allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
			published = append(published, result)
			return nil
		})
		seedTestArrivalStrip(t, queries, session, "SAS925")
		_, err = strips.UpdateAircraftPosition(ctx, session, "SAS925", posPtr(55.6285306), posPtr(12.642625), altPtr(0), "TAXI", nil)
		require.NoError(t, err)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS925"), arrivalFlight("SAS925", 1)))

		current, err := assignments.GetAssignment(ctx, session, "SAS925")
		require.NoError(t, err)
		currentVersion := current.Version

		parked := euroscope.GroundStateParked
		_, err = strips.UpdateGroundState(ctx, session, "SAS925", &parked, "STAND", nil)
		require.NoError(t, err)

		original, err := assignments.GetAssignment(ctx, session, "SAS925")
		require.NoError(t, err)
		assert.Equal(t, currentVersion, original.Version, "published metadata must carry the actionable version")
		actions := NewStandActionService(allocations, assignments, strips, nil, nil, nil)
		manual, err := actions.AssignManually(ctx, session, "EKCH", "GND", "SAS925", "A2", currentVersion)
		require.NoError(t, err)
		assert.Equal(t, "A2", manual.Assignment.Stand)
		assert.True(t, manual.Assignment.Manual)
		require.NotNil(t, manual.Assignment.ObservedStand)
		assert.Equal(t, "A1", *manual.Assignment.ObservedStand)

		// The PARK state and coordinates still identify A1. Automatic observation
		// must not undo the controller's newer A2 decision.
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS925"), arrivalFlight("SAS925", 3)))
		retained, err := assignments.GetAssignment(ctx, session, "SAS925")
		require.NoError(t, err)
		assert.Equal(t, "A2", retained.Stand, "automatic airport-area handling must not undo controller intent")
		assert.True(t, retained.Manual)

		// Reaching the planned A2 stand advances the physical baseline without
		// turning the controller assignment back into an automatic one.
		_, err = strips.UpdateAircraftPosition(ctx, session, "SAS925", posPtr(55.6285306), posPtr(12.6509583), altPtr(0), "STAND", nil)
		require.NoError(t, err)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS925"), arrivalFlight("SAS925", 4)))
		atPlannedStand, err := assignments.GetAssignment(ctx, session, "SAS925")
		require.NoError(t, err)
		assert.Equal(t, "A2", atPlannedStand.Stand)
		assert.True(t, atPlannedStand.Manual)
		require.NotNil(t, atPlannedStand.ObservedStand)
		assert.Equal(t, "A2", *atPlannedStand.ObservedStand)

		// A later position at A3 is new operational evidence that the aircraft
		// parked somewhere other than either the old stand or the manual plan.
		_, err = strips.UpdateAircraftPosition(ctx, session, "SAS925", posPtr(55.6285306), posPtr(12.6592917), altPtr(0), "STAND", nil)
		require.NoError(t, err)
		observedStrip := loadStrip(t, strips, session, "SAS925")
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, observedStrip, arrivalFlight("SAS925", 5)))
		observed, err := assignments.GetAssignment(ctx, session, "SAS925")
		require.NoError(t, err)
		assert.Equal(t, "A3", observed.Stand, "a different physical parking observation replaces the manual plan")
		assert.False(t, observed.Manual)
	})

	t.Run("parked arrival adopts its observed stand", func(t *testing.T) {
		lifecycle, allocations, session, assignments, strips, _ := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		registry, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
STAND:EKCH:A2:N055.37.42.710:E012.39.03.450:30
`))
		require.NoError(t, err)
		lifecycle.stands = registry
		allocations.stands = registry

		seedTestArrivalStrip(t, queries, session, "SAS926")
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS926"), arrivalFlight("SAS926", 1)))
		planned, err := assignments.GetAssignment(ctx, session, "SAS926")
		require.NoError(t, err)
		assert.Equal(t, "A1", planned.Stand)

		parked := euroscope.GroundStateParked
		_, err = strips.UpdateGroundState(ctx, session, "SAS926", &parked, "STAND", nil)
		require.NoError(t, err)
		_, err = strips.UpdateAircraftPosition(ctx, session, "SAS926", posPtr(55.6285306), posPtr(12.6509583), altPtr(0), "STAND", nil)
		require.NoError(t, err)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS926"), arrivalFlight("SAS926", 2)))

		observed, err := assignments.GetAssignment(ctx, session, "SAS926")
		require.NoError(t, err)
		assert.Equal(t, "A2", observed.Stand)
		assert.False(t, observed.Manual, "physical observation is recorded separately from controller intent")
		require.NotNil(t, observed.ExpiresAt)
	})

	t.Run("ALDT replaces fallback retention with landing time plus thirty minutes", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		seedTestArrivalStrip(t, queries, session, "SAS924")
		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS924", posPtr(55.6285306), posPtr(12.644625), altPtr(0), "TAXI", nil)
		require.NoError(t, err)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS924"), arrivalFlight("SAS924", 1)))

		clock.advance(5 * time.Minute)
		aldt := clock.current().Format("1504")
		_, err = strips.SetCdmData(ctx, session, "SAS924", &models.CdmData{Aldt: &aldt})
		require.NoError(t, err)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS924"), arrivalFlight("SAS924", 2)))

		landed, err := assignments.GetAssignment(ctx, session, "SAS924")
		require.NoError(t, err)
		require.NotNil(t, landed.ExpiresAt)
		expectedExpiry := clock.current().Truncate(time.Minute).Add(30 * time.Minute)
		assert.Equal(t, expectedExpiry, landed.ExpiresAt.UTC())

		clock.set(expectedExpiry.Add(-time.Second))
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, "SAS924")
		require.NoError(t, err, "stand remains until the ALDT retention window ends")

		clock.set(expectedExpiry)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, "SAS924")
		require.Error(t, err, "stand is released thirty minutes after ALDT")

		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS924"), arrivalFlight("SAS924", 3)))
		_, err = assignments.GetAssignment(ctx, session, "SAS924")
		require.Error(t, err, "an expired ALDT retention window must not recreate the stand")
	})

	t.Run("arrival at the airport without an assignment receives a stand", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, _ := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		seedTestArrivalStrip(t, queries, session, "SAS922")
		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS922", posPtr(55.6285306), posPtr(12.644625), altPtr(0), "TAXI", nil)
		require.NoError(t, err)

		arrived := arrivalFlight("SAS922", 1)
		arrived.Online = true
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS922"), arrived))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS922")
		require.NoError(t, err)
		assert.NotEmpty(t, assignment.Stand, "an unassigned arrival still receives a compatible stand at the airport")
		assert.Equal(t, StageConfirmed, assignment.Stage)
		assert.Nil(t, assignment.ETA)
	})

	t.Run("automatic pre-arrival reservation is released when the flight disappears", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, _ := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		seedTestArrivalStrip(t, queries, session, "SAS927")
		_, err := strips.UpdateAircraftPosition(ctx, session, "SAS927", posPtr(55.0), posPtr(10.0), altPtr(8000), "ARR_HIDDEN", nil)
		require.NoError(t, err)
		require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS927"), arrivalFlight("SAS927", 1)))

		reserved, err := assignments.GetAssignment(ctx, session, "SAS927")
		require.NoError(t, err)
		assert.Equal(t, StageEstimated, reserved.Stage)
		assert.Nil(t, reserved.ExpiresAt)
		assert.False(t, reserved.Manual)

		require.NoError(t, lifecycle.CancelArrival(ctx, session, "SAS927"))
		_, err = assignments.GetAssignment(ctx, session, "SAS927")
		require.Error(t, err, "a vanished automatic arrival must stop retaining its stand")
	})

	t.Run("sweep removes an unsafe planned overlap behind physical occupancy", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
		seedTestArrivalStrip(t, queries, session, "SAS929")
		seedTestArrivalStrip(t, queries, session, "SAS930")
		// LockAssignments filters active rows with PostgreSQL NOW(), while this
		// lifecycle fixture intentionally uses a frozen application clock.
		physicalExpiry := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
		physicalFiledETA := clock.current().Add(90 * time.Minute)
		plannedETA := clock.current().Add(5 * time.Minute)
		require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
			SessionID: session, Callsign: "SAS929", Stand: "A1", Direction: string(sat.AssignmentDirectionArrival),
			Stage: StageConfirmed, Source: "AUTOMATIC", ETA: &physicalFiledETA, ExpiresAt: &physicalExpiry,
		}))
		require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
			SessionID: session, Callsign: "SAS930", Stand: "A1", Direction: string(sat.AssignmentDirectionArrival),
			Stage: StageAssigned, Source: "AUTOMATIC", ETA: &plannedETA,
		}))
		_, err := strips.UpdateStand(ctx, session, "SAS929", strp("A1"), nil)
		require.NoError(t, err)
		_, err = strips.UpdateStand(ctx, session, "SAS930", strp("A1"), nil)
		require.NoError(t, err)

		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, "SAS929")
		require.NoError(t, err, "physical occupancy remains authoritative")
		_, err = assignments.GetAssignment(ctx, session, "SAS930")
		require.Error(t, err, "the unsafe automatic plan is released for reallocation")
		assert.Nil(t, loadStrip(t, strips, session, "SAS930").Stand)
	})

}

func TestDetermineArrivalTargetStageWithoutETA(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

	assert.Equal(t, StageEstimated, determineArrivalTargetStage(nil, now, nil, false, false))
	assert.Equal(t, StageEstimated, determineArrivalTargetStage(nil, now, altPtr(8000), false, false))
	assert.Equal(t, StageAssigned, determineArrivalTargetStage(nil, now, altPtr(8000), false, true))
	assert.Equal(t, StageConfirmed, determineArrivalTargetStage(nil, now, altPtr(2000), false, true))
	assert.Equal(t, StageConfirmed, determineArrivalTargetStage(nil, now, nil, true, true))
}

func TestArrivalAltitudeRequiresValidPosition(t *testing.T) {
	altitude := int32(0)
	zero := float64(0)
	strip := &models.Strip{
		PositionLatitude:  &zero,
		PositionLongitude: &zero,
		PositionAltitude:  &altitude,
	}

	assert.Nil(t, arrivalAltitude(strip), "an omitted EuroScope position must not confirm a distant arrival")

	latitude, longitude := 55.0, 10.0
	strip.PositionLatitude = &latitude
	strip.PositionLongitude = &longitude
	assert.Equal(t, &altitude, arrivalAltitude(strip))

	invalidLatitude := 91.0
	strip.PositionLatitude = &invalidLatitude
	assert.Nil(t, arrivalAltitude(strip), "out-of-range coordinates must not make altitude operational")
}

func TestArrivalIsAtAirportUsesReconciledDestination(t *testing.T) {
	stands, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
`))
	require.NoError(t, err)
	lifecycle := &ArrivalLifecycleService{stands: stands}
	strip := &models.Strip{
		Destination:       "ZZZZ",
		PositionLatitude:  posPtr(55.6285306),
		PositionLongitude: posPtr(12.644625),
		PositionAltitude:  altPtr(0),
	}

	assert.True(t, lifecycle.arrivalIsAtAirport(strip, "EKCH"))

	invalidLatitude := 91.0
	strip.PositionLatitude = &invalidLatitude
	assert.False(t, lifecycle.arrivalIsAtAirport(strip, "EKCH"))

	strip.PositionLatitude = posPtr(55.6285306)
	strip.PositionAltitude = altPtr(12000)
	assert.False(t, lifecycle.arrivalIsAtAirport(strip, "EKCH"), "an aircraft over the airport at cruise altitude has not arrived")
}

func TestParseArrivalLandingClockUTCUsesPreviousDayAcrossMidnight(t *testing.T) {
	now := time.Date(2026, 7, 13, 0, 10, 0, 0, time.UTC)
	landedAt, ok := parseArrivalLandingClockUTC("2355", now)
	require.True(t, ok)
	assert.Equal(t, time.Date(2026, 7, 12, 23, 55, 0, 0, time.UTC), landedAt)
}

func TestArrivalStandExpiresAt(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 5, 30, 0, time.UTC)

	fallback := arrivalStandExpiresAt(&models.Strip{}, nil, now)
	require.NotNil(t, fallback)
	assert.Equal(t, now.Add(30*time.Minute), *fallback)

	current := now.Add(20 * time.Minute)
	unchanged := arrivalStandExpiresAt(&models.Strip{}, &current, now)
	require.NotNil(t, unchanged)
	assert.Equal(t, current, *unchanged)

	nearExpiry := now.Add(9 * time.Minute)
	refreshed := arrivalStandExpiresAt(&models.Strip{}, &nearExpiry, now)
	require.NotNil(t, refreshed)
	assert.Equal(t, now.Add(30*time.Minute), *refreshed)

	aldt := "1000"
	fromLanding := arrivalStandExpiresAt(&models.Strip{CdmData: &models.CdmData{Aldt: &aldt}}, &current, now)
	require.NotNil(t, fromLanding)
	assert.Equal(t, time.Date(2026, 7, 12, 10, 30, 0, 0, time.UTC), *fromLanding)
}

func TestArrivalAirportRetentionPublishesActionableVersionWithoutStandSync(t *testing.T) {
	existing := &models.StandAssignment{
		ID: 1, SessionID: 7, Callsign: "SAS930", Stand: "A1",
		Stage: StageConfirmed, Version: 4,
	}
	store := &arrivalLifecycleAssignmentStore{assignment: existing}
	var published StandAllocationResult
	allocations := &StandAllocationService{publish: func(_ context.Context, result StandAllocationResult) error {
		published = result
		return nil
	}}
	lifecycle := &ArrivalLifecycleService{assignments: store, allocations: allocations}
	expiresAt := time.Date(2026, 7, 12, 10, 30, 0, 0, time.UTC)

	require.NoError(t, lifecycle.updateArrivalAtAirport(t.Context(), existing, StageConfirmed, &expiresAt, nil))
	assert.Equal(t, int32(5), published.Assignment.Version)
	assert.Equal(t, expiresAt, published.Assignment.ExpiresAt.UTC())
	assert.False(t, published.StandChanged)
	assert.False(t, published.NotifyEuroscope)
}

func TestArrivalLifecyclePreservesManualStandAgainstParkObservation(t *testing.T) {
	stands, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
STAND:EKCH:A2:N055.37.42.710:E012.39.03.450:30
`))
	require.NoError(t, err)
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	existing := &models.StandAssignment{
		ID: 1, SessionID: 7, Callsign: "SAS931", Stand: "A2",
		Direction: string(sat.AssignmentDirectionArrival), Stage: StageConfirmed,
		Manual: true, Version: 4, ObservedStand: strp(""),
	}
	store := &arrivalLifecycleAssignmentStore{assignment: existing}
	allocations := &StandAllocationService{}
	lifecycle := &ArrivalLifecycleService{
		assignments: store,
		allocations: allocations,
		stands:      stands,
		now:         func() time.Time { return now },
	}
	parked := euroscope.GroundStateParked
	strip := &models.Strip{
		Callsign: "SAS931", Destination: "EKCH", State: &parked,
		PositionLatitude: posPtr(55.6285306), PositionLongitude: posPtr(12.642625), PositionAltitude: altPtr(0),
	}

	require.NoError(t, lifecycle.ProcessArrival(t.Context(), 7, strip, arrivalFlight("SAS931", 2)))
	require.NotNil(t, store.updated)
	assert.Equal(t, "A2", store.updated.Stand)
	assert.True(t, store.updated.Manual)
	require.NotNil(t, store.updated.ObservedStand)
	assert.Equal(t, "A1", *store.updated.ObservedStand, "the first post-migration observation becomes a baseline without replacing controller intent")
}

type arrivalLifecycleAssignmentStore struct {
	repository.StandAssignmentRepository
	assignment *models.StandAssignment
	updated    *models.StandAssignment
}

func (s *arrivalLifecycleAssignmentStore) GetAssignment(_ context.Context, _ int32, _ string) (*models.StandAssignment, error) {
	copy := *s.assignment
	return &copy, nil
}

func (s *arrivalLifecycleAssignmentStore) UpdateAssignment(_ context.Context, assignment *models.StandAssignment) (int64, error) {
	copy := *assignment
	s.updated = &copy
	return 1, nil
}

func TestArrivalResolveFactsUsesVatsimFactsWhenFreshSessionStripIsUnrecognized(t *testing.T) {
	aircraft := mustLoadAircraftRegistry(t, "A359")
	engines := mustLoadEngineRegistry(t, aircraft, []engineRecord{
		{ICAO: "A359", Engine: "J", WTC: "H"},
	})
	lifecycle := &ArrivalLifecycleService{
		aircraft: aircraft,
		engines:  engines,
		borders:  sat.NewAirportCountryRegistry(),
	}
	strip := &models.Strip{
		Callsign:     "SAS926",
		Origin:       "ZZZZ",
		Destination:  "ZZZZ",
		AircraftType: strp("UNRECOGNIZED"),
	}
	flight := vatsim.ArrivalFlightInfo{
		Callsign: "SAS926", Origin: "VABB", Destination: "EKCH", AircraftType: "A359",
	}

	facts, assignmentFacts := lifecycle.resolveFacts(strip, flight)

	assert.True(t, facts.AircraftKnown)
	assert.Equal(t, "A359", facts.Aircraft.Type)
	assert.Equal(t, "H", facts.WTC)
	assert.Equal(t, sat.BorderStatusNonSchengen, facts.BorderStatus)
	assert.Equal(t, "EKCH", facts.Destination)
	assert.Equal(t, "A359", assignmentFacts.AircraftType)
	assert.Equal(t, sat.AircraftUseCodeA, assignmentFacts.AircraftUse)
	assert.Equal(t, sat.BorderStatusNonSchengen, assignmentFacts.BorderStatus)

	request := lifecycle.buildRequest(1225, strip, flight, StageEstimated, nil, nil)
	assert.Equal(t, "EKCH", request.Airport)
}

func TestFreshSessionSASHeavyUsesNormalNonSchengenRule(t *testing.T) {
	configDir := filepath.Join("config", "ekch")
	stands, err := sat.LoadStandCapabilityFile(filepath.Join(configDir, "GRpluginStands.txt"))
	require.NoError(t, err)
	policy, err := sat.LoadAirlineAssignmentFile(filepath.Join(configDir, "airline_assignment.json"), stands)
	require.NoError(t, err)
	aircraft, err := sat.LoadAircraftReferenceFile(filepath.Join(configDir, "GRpluginAircraftInfo.txt"))
	require.NoError(t, err)
	engines := mustLoadEngineRegistry(t, aircraft, []engineRecord{
		{ICAO: "A359", Engine: "J", WTC: "H"},
	})
	allocations := &StandAllocationService{
		stands: stands,
		policy: policy,
		random: func() float64 { return 0 },
		now:    time.Now,
	}
	lifecycle := &ArrivalLifecycleService{
		allocations: allocations,
		stands:      stands,
		aircraft:    aircraft,
		engines:     engines,
		borders:     sat.NewAirportCountryRegistry(),
	}
	strip := &models.Strip{
		Callsign: "SAS926", Origin: "ZZZZ", Destination: "EKCH", AircraftType: strp("UNRECOGNIZED"),
	}
	flight := vatsim.ArrivalFlightInfo{
		Callsign: "SAS926", Origin: "VABB", Destination: "EKCH", AircraftType: "A359",
	}
	request := lifecycle.buildRequest(1225, strip, flight, StageEstimated, nil, nil)
	evaluation := stands.EvaluateCompatibility(request.Airport, request.FlightFacts)

	selected, selection, match, _, _, err := allocations.selectStand(
		AutomaticStandAllocation, request, evaluation, nil, nil, nil,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, match)
	assert.Equal(t, "SAS_NON-SCHENGEN", selection.RuleID)
	assert.False(t, selection.FallbackUsed)
	assert.True(t, strings.HasPrefix(selected, "C"), "a SAS non-Schengen Heavy should use its configured Charlie rule")
	assert.Contains(t, strings.Join(match.Variant.WTC, ""), "H")
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
