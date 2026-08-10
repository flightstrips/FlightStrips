package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var standAllocationSessionSequence atomic.Int64

func TestStandAllocationServiceTransactions(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()

	t.Run("updates strip and assignment before publishing", func(t *testing.T) {
		service, session, _ := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS101")
		published := make(chan StandAllocationResult, 1)
		service.publish = func(_ context.Context, result StandAllocationResult) error { published <- result; return nil }

		result, err := service.Allocate(ctx, standAllocationRequest(session, "SAS101"))
		require.NoError(t, err)
		assert.Equal(t, "A1", result.Assignment.Stand)
		assert.True(t, result.StandChanged)
		assert.True(t, result.NotifyEuroscope)
		strip, err := queries.GetStrip(ctx, database.GetStripParams{Session: session, Callsign: "SAS101"})
		require.NoError(t, err)
		require.NotNil(t, strip.Stand)
		assert.Equal(t, "A1", *strip.Stand)
		select {
		case event := <-published:
			assert.Equal(t, result.Assignment.ID, event.Assignment.ID)
			assert.True(t, event.StandChanged)
		default:
			t.Fatal("allocation was not published after commit")
		}
	})

	t.Run("does not report a committed allocation as failed when publication fails", func(t *testing.T) {
		service, session, assignments := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS103")
		service.SetPublisher(func(context.Context, StandAllocationResult) error {
			return errors.New("publisher unavailable")
		})

		result, err := service.Allocate(ctx, standAllocationRequest(session, "SAS103"))
		require.NoError(t, err)
		persisted, err := assignments.GetAssignment(ctx, session, "SAS103")
		require.NoError(t, err)
		assert.Equal(t, result.Assignment.ID, persisted.ID)
	})

	t.Run("releases strip and assignment atomically before publishing removal", func(t *testing.T) {
		service, session, assignments := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS102")
		allocated, err := service.Allocate(ctx, standAllocationRequest(session, "SAS102"))
		require.NoError(t, err)

		stale := allocated.Assignment
		stale.Version--
		require.ErrorIs(t, service.ReleaseAssignment(ctx, &stale), errAllocationVersionConflict)
		retained, err := assignments.GetAssignment(ctx, session, "SAS102")
		require.NoError(t, err, "a stale release rolls back the assignment deletion")
		strip, err := queries.GetStrip(ctx, database.GetStripParams{Session: session, Callsign: "SAS102"})
		require.NoError(t, err)
		require.NotNil(t, strip.Stand, "a stale release rolls back the strip update")
		assert.Equal(t, retained.Stand, *strip.Stand)

		published := make(chan StandAllocationResult, 1)
		service.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
			published <- result
			return errors.New("publisher unavailable")
		})
		require.NoError(t, service.ReleaseAssignment(ctx, retained))
		_, err = assignments.GetAssignment(ctx, session, "SAS102")
		require.Error(t, err)
		strip, err = queries.GetStrip(ctx, database.GetStripParams{Session: session, Callsign: "SAS102"})
		require.NoError(t, err)
		assert.Nil(t, strip.Stand)
		select {
		case event := <-published:
			assert.True(t, event.Removed)
			assert.Equal(t, retained.ID, event.Assignment.ID)
			assert.True(t, event.StandChanged)
		default:
			t.Fatal("removal was not published after commit")
		}
	})

	t.Run("reports displaced assignments after commit", func(t *testing.T) {
		service, session, assignments := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS104")
		testdata.SeedTestStrip(t, queries, session, "SAS105")
		estimatedRequest := withStand(standAllocationRequest(session, "SAS104"), "A1")
		estimatedRequest.Stage = StageEstimated
		_, err := service.AssignManually(ctx, estimatedRequest)
		require.NoError(t, err)
		observedStand := "B2"
		updated, err := queries.UpdateStripStandByID(ctx, database.UpdateStripStandByIDParams{
			Stand: &observedStand, Callsign: "SAS104", Session: session,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, updated)

		var published StandAllocationResult
		service.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
			published = result
			return nil
		})
		assignedRequest := standAllocationRequest(session, "SAS105")
		assignedRequest.Stage = StageAssigned
		assignedRequest.DisplaceStage = StageEstimated
		result, err := service.Allocate(ctx, assignedRequest)
		require.NoError(t, err)
		require.Len(t, result.RemovedAssignments, 1)
		assert.Equal(t, "SAS104", result.RemovedAssignments[0].Callsign)
		require.Len(t, result.RemovedStandChanges, 1)
		assert.Equal(t, "SAS104", result.RemovedStandChanges[0].Callsign)
		require.Len(t, published.RemovedAssignments, 1)
		assert.Equal(t, "SAS104", published.RemovedAssignments[0].Callsign)
		_, err = assignments.GetAssignment(ctx, session, "SAS104")
		require.Error(t, err)
		displacedStrip, err := queries.GetStrip(ctx, database.GetStripParams{Session: session, Callsign: "SAS104"})
		require.NoError(t, err)
		assert.Nil(t, displacedStrip.Stand)
	})

	t.Run("future arrival booking does not occupy its stand before ETA", func(t *testing.T) {
		service, session, _ := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS106")
		testdata.SeedTestStrip(t, queries, session, "SAS107")
		eta := time.Now().UTC().Add(time.Hour)
		arrival := withStand(standAllocationRequest(session, "SAS106"), "A1")
		arrival.Stage = StageConfirmed
		arrival.ETA = &eta
		_, err := service.AssignManually(ctx, arrival)
		require.NoError(t, err)

		departure := withStand(standAllocationRequest(session, "SAS107"), "A1")
		departure.Direction = sat.AssignmentDirectionDeparture
		departure.FlightFacts.Direction = sat.Departure
		departure.Stage = StageDepartureBlock
		result, err := service.assignObservedStand(ctx, departure)
		require.NoError(t, err)
		assert.Equal(t, "A1", result.Assignment.Stand)
	})

	t.Run("arrival booking occupies its stand from ETA", func(t *testing.T) {
		service, session, _ := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS108")
		testdata.SeedTestStrip(t, queries, session, "SAS109")
		now := time.Now().UTC()
		eta := now.Add(time.Hour)
		service.now = func() time.Time { return now }
		arrival := withStand(standAllocationRequest(session, "SAS108"), "A1")
		arrival.Stage = StageConfirmed
		arrival.ETA = &eta
		_, err := service.AssignManually(ctx, arrival)
		require.NoError(t, err)

		service.now = func() time.Time { return eta }
		departure := withStand(standAllocationRequest(session, "SAS109"), "A1")
		departure.Direction = sat.AssignmentDirectionDeparture
		departure.FlightFacts.Direction = sat.Departure
		departure.Stage = StageDepartureBlock
		_, err = service.assignObservedStand(ctx, departure)
		require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
	})

	t.Run("parked arrival adopts an observed stand with a confirmed booking conflict", func(t *testing.T) {
		service, session, assignments := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SASOBS1")
		testdata.SeedTestStrip(t, queries, session, "SASOBS2")

		booked := withStand(standAllocationRequest(session, "SASOBS1"), "A1")
		booked.Stage = StageConfirmed
		_, err := service.AssignManually(ctx, booked)
		require.NoError(t, err)

		parked := withStand(standAllocationRequest(session, "SASOBS2"), "A1")
		parked.Stage = StageConfirmed
		result, err := service.assignObservedStand(ctx, parked)
		require.NoError(t, err)
		assert.Equal(t, "A1", result.Assignment.Stand)
		require.NotNil(t, result.Assignment.ConflictReason)
		assert.Contains(t, *result.Assignment.ConflictReason, "observed parked arrival")

		retained, err := assignments.GetAssignment(ctx, session, "SASOBS1")
		require.NoError(t, err)
		assert.Equal(t, "A1", retained.Stand, "the existing confirmed booking remains visible for conflict resolution")
	})

	t.Run("future arrival booking blocks a departure whose TOBT release is after ETA", func(t *testing.T) {
		service, session, _ := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS110")
		testdata.SeedTestStrip(t, queries, session, "SAS111")
		now := time.Now().UTC()
		eta := now.Add(time.Hour)
		arrival := withStand(standAllocationRequest(session, "SAS110"), "A1")
		arrival.Stage = StageAssigned
		arrival.ETA = &eta
		_, err := service.AssignManually(ctx, arrival)
		require.NoError(t, err)

		laterTobt := eta.Add(-5 * time.Minute)
		departure := withStand(standAllocationRequest(session, "SAS111"), "A1")
		departure.Direction = sat.AssignmentDirectionDeparture
		departure.FlightFacts.Direction = sat.Departure
		departure.Stage = StageDepartureBlock
		departure.DepartureTOBT = &laterTobt
		_, err = service.assignObservedStand(ctx, departure)
		require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
	})

	t.Run("future arrival booking blocks an overlapping later inbound", func(t *testing.T) {
		service, session, _ := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS112")
		testdata.SeedTestStrip(t, queries, session, "SAS113")
		now := time.Now().UTC()
		firstETA := now.Add(time.Hour)
		firstArrival := withStand(standAllocationRequest(session, "SAS112"), "A1")
		firstArrival.Stage = StageConfirmed
		firstArrival.ETA = &firstETA
		_, err := service.AssignManually(ctx, firstArrival)
		require.NoError(t, err)

		laterETA := firstETA.Add(15 * time.Minute)
		laterArrival := withStand(standAllocationRequest(session, "SAS113"), "A1")
		laterArrival.Stage = StageConfirmed
		laterArrival.ETA = &laterETA
		_, err = service.AssignManually(ctx, laterArrival)
		require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
	})

	t.Run("rejects direct occupancy and one-way or two-way blocks", func(t *testing.T) {
		for _, directives := range []struct{ a1, a2 string }{
			{a1: "BLOCKS:A2"},
			{a1: "BLOCKS:A2", a2: "BLOCKS:A1"},
		} {
			service, session, _ := standAllocationFixture(t, pool, queries, directives.a1, directives.a2)
			testdata.SeedTestStrip(t, queries, session, "SAS201")
			testdata.SeedTestStrip(t, queries, session, "SAS202")
			_, err := service.Allocate(ctx, standAllocationRequest(session, "SAS201"))
			require.NoError(t, err)
			_, err = service.AssignManually(ctx, withStand(standAllocationRequest(session, "SAS202"), "A1"))
			require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
			_, err = service.AssignManually(ctx, withStand(standAllocationRequest(session, "SAS202"), "A2"))
			require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
		}
	})

	t.Run("locks active manual blocks", func(t *testing.T) {
		service, session, assignments := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS301")
		require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{
			SessionID: session, Stand: "A1", BlockType: "CLOSURE", Source: "CONTROLLER", Manual: true,
		}))
		_, err := service.AssignManually(ctx, withStand(standAllocationRequest(session, "SAS301"), "A1"))
		require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
	})

	t.Run("manual blocks make configured neighbors unavailable bidirectionally", func(t *testing.T) {
		for _, testCase := range []struct {
			blocked  string
			neighbor string
		}{
			{blocked: "A1", neighbor: "A2"},
			{blocked: "A2", neighbor: "A1"},
		} {
			t.Run(testCase.blocked, func(t *testing.T) {
				service, session, assignments := standAllocationFixture(t, pool, queries, "BLOCKS:A2", "")
				testdata.SeedTestStrip(t, queries, session, "SAS311")
				reason := "marshaller closed"
				require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{
					SessionID: session, Stand: testCase.blocked, BlockType: "CLOSURE", Source: "CONTROLLER", Reason: &reason, Manual: true,
				}))

				available, err := service.StandAvailable(ctx, standAllocationRequest(session, "SAS311"), testCase.neighbor)
				require.NoError(t, err)
				assert.False(t, available)
				_, err = service.AssignManually(ctx, withStand(standAllocationRequest(session, "SAS311"), testCase.neighbor))
				require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
				_, err = service.Allocate(ctx, standAllocationRequest(session, "SAS311"))
				require.ErrorIs(t, err, ErrNoAvailableStand)
			})
		}
	})

	t.Run("never bypasses WTC compatibility when no compatible stand exists", func(t *testing.T) {
		service, session, assignments := standAllocationFixture(t, pool, queries, "WTC:M", "WTC:M")
		testdata.SeedTestStrip(t, queries, session, "SAS312")
		request := standAllocationRequest(session, "SAS312")
		request.FlightFacts = sat.FlightCompatibilityFacts{Direction: sat.Arrival, WTC: "H"}

		_, err := service.Allocate(ctx, request)
		require.ErrorIs(t, err, ErrNoCompatibleStand)
		_, err = assignments.GetAssignment(ctx, session, "SAS312")
		require.Error(t, err, "a Heavy aircraft must not be assigned to a Medium-only stand")
	})

	t.Run("retries automatic allocation after a transient stand shortage", func(t *testing.T) {
		service, session, assignments := standAllocationFixture(t, pool, queries, "BLOCKS:A2", "")
		testdata.SeedTestStrip(t, queries, session, "SAS313")
		closure := &models.StandBlock{
			SessionID: session, Stand: "A1", BlockType: "CLOSURE", Source: "CONTROLLER", Manual: true,
		}
		require.NoError(t, assignments.CreateBlock(ctx, closure))

		_, err := service.Allocate(ctx, standAllocationRequest(session, "SAS313"))
		require.ErrorIs(t, err, ErrNoAvailableStand)
		_, err = service.Allocate(ctx, standAllocationRequest(session, "SAS313"))
		require.ErrorIs(t, err, ErrAutomaticAllocationSuppressed)

		deleted, err := service.DeleteManualBlock(ctx, session, closure.ID, closure.Version)
		require.NoError(t, err)
		require.Equal(t, int64(1), deleted)
		result, err := service.Allocate(ctx, standAllocationRequest(session, "SAS313"))
		require.NoError(t, err, "a silent probe claims capacity freed during diagnostic backoff")
		assert.Equal(t, "A1", result.Assignment.Stand)
	})

	t.Run("manual blocks use allocation occupancy and adjacency locks", func(t *testing.T) {
		service, session, assignments := standAllocationFixture(t, pool, queries, "BLOCKS:A2", "")
		testdata.SeedTestStrip(t, queries, session, "SAS302")
		_, err := service.AssignManually(ctx, withStand(standAllocationRequest(session, "SAS302"), "A1"))
		require.NoError(t, err)

		block := &models.StandBlock{SessionID: session, Stand: "A2", BlockType: "MANUAL", Source: "CONTROLLER", Manual: true}
		err = service.CreateManualBlock(ctx, "EKCH", block)
		require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
		listed, listErr := assignments.ListBlocks(ctx, session)
		require.NoError(t, listErr)
		assert.Empty(t, listed)

		free := &models.StandBlock{SessionID: session, Stand: "A1", BlockType: "MANUAL", Source: "CONTROLLER", Manual: true}
		otherService, otherSession, otherAssignments := standAllocationFixture(t, pool, queries, "", "")
		free.SessionID = otherSession
		require.NoError(t, otherService.CreateManualBlock(ctx, "EKCH", free))
		count, deleteErr := otherService.DeleteManualBlock(ctx, otherSession, free.ID, free.Version)
		require.NoError(t, deleteErr)
		assert.Equal(t, int64(1), count)
		remaining, listErr := otherAssignments.ListBlocks(ctx, otherSession)
		require.NoError(t, listErr)
		assert.Empty(t, remaining)
	})

	t.Run("records an explicit incompatible override and leaves failures unpublished", func(t *testing.T) {
		service, session, _ := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS401")
		testdata.SeedTestStrip(t, queries, session, "SAS402")
		_, err := service.Allocate(ctx, standAllocationRequest(session, "SAS401"))
		require.NoError(t, err)
		override := withStand(standAllocationRequest(session, "SAS402"), "A1")
		override.ConflictReason = "controller approved overlap"
		result, err := service.OverrideManually(ctx, override)
		require.NoError(t, err)
		assert.Equal(t, "MANUAL_OVERRIDE", result.Assignment.Source)
		require.NotNil(t, result.Assignment.ConflictReason)
		assert.Contains(t, *result.Assignment.ConflictReason, "reserved by SAS401")

		failed, failedSession, _ := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, failedSession, "SAS403")
		published := false
		failed.publish = func(context.Context, StandAllocationResult) error { published = true; return nil }
		_, err = failed.AssignManually(ctx, withStand(standAllocationRequest(failedSession, "SAS403"), "A9"))
		require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
		assert.False(t, published)
		_, err = queries.GetStandAssignment(ctx, database.GetStandAssignmentParams{SessionID: failedSession, Callsign: "SAS403"})
		require.Error(t, err)
		strip, err := queries.GetStrip(ctx, database.GetStripParams{Session: failedSession, Callsign: "SAS403"})
		require.NoError(t, err)
		assert.Nil(t, strip.Stand)
	})

	t.Run("retries with remaining candidates and stops at its configured bound", func(t *testing.T) {
		service, session, assignments := standAllocationFixture(t, pool, queries, "", "")
		testdata.SeedTestStrip(t, queries, session, "SAS501")
		_, err := service.Allocate(ctx, standAllocationRequest(session, "SAS501"))
		require.NoError(t, err)
		recorder := &retryRecorder{}
		service.assignments = retryConflictRepository{StandAssignmentRepository: assignments, recorder: recorder}
		service.random = func() float64 { return .99 }
		service.attempts = 2
		_, err = service.Reallocate(ctx, standAllocationRequest(session, "SAS501"))
		require.ErrorIs(t, err, ErrAllocationRetriesExhausted)
		assert.Equal(t, []string{"A2", "A1"}, recorder.stands, "the retry excludes the conflicted selection")
		assignment, err := assignments.GetAssignment(ctx, session, "SAS501")
		require.NoError(t, err)
		assert.Equal(t, "A1", assignment.Stand, "failed attempts roll back the assignment")
	})

	t.Run("concurrent calls cannot allocate blocked neighbors", func(t *testing.T) {
		service, session, _ := standAllocationFixture(t, pool, queries, "BLOCKS:A2", "")
		testdata.SeedTestStrip(t, queries, session, "SAS601")
		testdata.SeedTestStrip(t, queries, session, "SAS602")
		start, results := make(chan struct{}), make(chan error, 2)
		var wait sync.WaitGroup
		for _, callsign := range []string{"SAS601", "SAS602"} {
			wait.Add(1)
			go func(callsign string) {
				defer wait.Done()
				<-start
				_, err := service.Allocate(ctx, standAllocationRequest(session, callsign))
				results <- err
			}(callsign)
		}
		close(start)
		wait.Wait()
		close(results)
		var successes, unavailable int
		for err := range results {
			if err == nil {
				successes++
			} else if errors.Is(err, ErrNoAvailableStand) {
				unavailable++
			} else {
				t.Fatalf("unexpected allocation error: %v", err)
			}
		}
		assert.Equal(t, 1, successes)
		assert.Equal(t, 1, unavailable)
	})
}

func TestPublishAssignmentOnlyNotifiesEuroscopeForConfirmedArrival(t *testing.T) {
	service := &StandAllocationService{}
	var published []StandAllocationResult
	service.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
		published = append(published, result)
		return nil
	})
	assignment := models.StandAssignment{Callsign: "SAS777", Direction: string(sat.AssignmentDirectionArrival), Stage: StageConfirmed}

	require.NoError(t, service.PublishAssignment(context.Background(), assignment))
	require.NoError(t, service.PublishConfirmedArrival(context.Background(), assignment))

	require.Len(t, published, 2)
	assert.False(t, published[0].NotifyEuroscope, "ordinary lifecycle refreshes must not emit stand updates")
	assert.True(t, published[1].NotifyEuroscope, "confirmation must deliver the previously assigned arrival stand")
}

func TestStandAllocationRejectsHeavyAircraftOnMediumOnlyFallback(t *testing.T) {
	registry, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
WTC:M
`))
	require.NoError(t, err)
	policy, err := sat.LoadAirlineAssignment(strings.NewReader(`{
  "rules": [{"id":"test","callsigns":["TST"],"stands":{"tier1":{"A1":100}}}],
  "stand_groups": {}, "fallback_rules": {`+testFallbackJSON("A1")+`}
}`), registry)
	require.NoError(t, err)
	service := &StandAllocationService{
		stands: registry,
		policy: policy,
		random: func() float64 { return 0 },
		now:    time.Now,
	}
	request := StandAllocationRequest{
		Callsign: "TST1", Airport: "EKCH", Direction: sat.AssignmentDirectionArrival,
		FlightFacts: sat.FlightCompatibilityFacts{Direction: sat.Arrival, WTC: "H"},
	}
	evaluation := registry.EvaluateCompatibility(request.Airport, request.FlightFacts)

	_, _, _, _, _, err = service.selectStand(AutomaticStandAllocation, request, evaluation, nil, nil, nil)

	require.ErrorIs(t, err, ErrNoCompatibleStand)
}

func TestEstimatedArrivalIsSoftReservationUntilPromoted(t *testing.T) {
	registry, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
WTC:M
`))
	require.NoError(t, err)
	evaluation := registry.EvaluateCompatibility("EKCH", sat.FlightCompatibilityFacts{
		Direction: sat.Departure,
		WTC:       "M",
	})
	require.Len(t, evaluation.Matches, 1)
	service := &StandAllocationService{
		stands: registry,
		now:    func() time.Time { return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC) },
	}
	request := StandAllocationRequest{
		Callsign:  "DEP1",
		Airport:   "EKCH",
		Direction: sat.AssignmentDirectionDeparture,
	}
	matches := map[string]sat.StandCompatibilityMatch{"A1": evaluation.Matches[0]}
	arrival := &models.StandAssignment{
		Callsign:  "ARR1",
		Stand:     "A1",
		Direction: string(sat.AssignmentDirectionArrival),
		Stage:     StageEstimated,
	}

	unavailable := service.availability(request, []*models.StandAssignment{arrival}, nil, matches)
	assert.Contains(t, unavailable["A1"], "soft-reserved by ARR1", "an advisory arrival reserves its exact stand")

	eta := service.now().Add(30 * time.Minute)
	arrival.ETA = &eta
	unavailable = service.availability(request, []*models.StandAssignment{arrival}, nil, matches)
	assert.Contains(t, unavailable["A1"], "soft-reserved by ARR1", "the soft reservation remains after an ETA becomes available")

	request.DisplaceStage = StageEstimated
	unavailable = service.availability(request, []*models.StandAssignment{arrival}, nil, matches)
	assert.Empty(t, unavailable["A1"], "a request explicitly allowed to displace ESTIMATED may use the stand")
	request.DisplaceStage = ""

	arrival.Stage = StageAssigned
	departureTOBT := eta.Add(-5 * time.Minute)
	request.DepartureTOBT = &departureTOBT
	unavailable = service.availability(request, []*models.StandAssignment{arrival}, nil, matches)
	assert.Contains(t, unavailable["A1"], "reserved by ARR1", "ASSIGNED begins the normal ETA reservation window")

	arrival.ETA = nil
	unavailable = service.availability(request, []*models.StandAssignment{arrival}, nil, matches)
	assert.Contains(t, unavailable["A1"], "reserved by ARR1", "a close unknown-ETA arrival blocks immediately")
}

func TestArrivalReservationWindowsAreSymmetric(t *testing.T) {
	service := &StandAllocationService{now: func() time.Time {
		return time.Date(2026, 8, 10, 17, 0, 0, 0, time.UTC)
	}}
	requestETA := time.Date(2026, 8, 10, 18, 45, 0, 0, time.UTC)
	existingETA := time.Date(2026, 8, 10, 19, 0, 0, 0, time.UTC)
	request := StandAllocationRequest{
		Callsign: "EARLY", Direction: sat.AssignmentDirectionArrival, ETA: &requestETA,
	}
	existing := &models.StandAssignment{
		Callsign: "LATE", Stand: "A1", Direction: string(sat.AssignmentDirectionArrival),
		Stage: StageConfirmed, ETA: &existingETA,
	}
	matches := map[string]sat.StandCompatibilityMatch{"A1": {}}

	unavailable := service.availability(request, []*models.StandAssignment{existing}, nil, matches)
	assert.Contains(t, unavailable["A1"], "reserved by LATE", "an earlier booking must not overlap a later booking inserted first")

	requestETA = existingETA.Add(-arrivalStandRetention)
	unavailable = service.availability(request, []*models.StandAssignment{existing}, nil, matches)
	assert.Empty(t, unavailable["A1"], "touching reservation boundaries do not overlap")

	requestETA = existingETA.Add(arrivalStandRetention)
	unavailable = service.availability(request, []*models.StandAssignment{existing}, nil, matches)
	assert.Empty(t, unavailable["A1"], "the stand can be reused after the existing retention window")
}

func TestAutomaticAllocationOnlyReleasesEstimatedReservationForBetterPolicyTier(t *testing.T) {
	registry, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
WTC:M
BLOCKS:A2
STAND:EKCH:A2:N055.37.42.711:E012.38.33.451:30
WTC:M
STAND:EKCH:A3:N055.37.42.712:E012.38.33.452:30
WTC:M
`))
	require.NoError(t, err)
	policy, err := sat.LoadAirlineAssignment(strings.NewReader(`{
  "rules": [{
    "id":"test",
    "callsigns":["TST"],
    "stands":{
      "tier1":{"A1":100,"A2":100},
      "tier2":{"A3":100}
    }
  }],
  "stand_groups": {},
  "fallback_rules": {`+testFallbackJSON("A3")+`}
}`), registry)
	require.NoError(t, err)
	service := &StandAllocationService{
		stands: registry,
		policy: policy,
		random: func() float64 { return 0 },
		now:    time.Now,
	}
	request := StandAllocationRequest{
		Callsign: "TST9", Airport: "EKCH", Direction: sat.AssignmentDirectionArrival,
		Stage:       StageAssigned,
		FlightFacts: sat.FlightCompatibilityFacts{Direction: sat.Arrival, WTC: "M"},
		AssignmentFacts: sat.AssignmentFlightFacts{
			Callsign: "TST9", Direction: sat.AssignmentDirectionArrival,
		},
	}
	evaluation := registry.EvaluateCompatibility(request.Airport, request.FlightFacts)
	a1 := &models.StandAssignment{
		Callsign: "ARR1", Stand: "A1",
		Direction: string(sat.AssignmentDirectionArrival), Stage: StageEstimated,
	}

	selected, selection, _, _, _, err := service.selectStand(AutomaticStandAllocation, request, evaluation, []*models.StandAssignment{a1}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "A2", selected, "an estimated reservation neither yields nor adjacency-blocks an open stand in the same tier")
	require.NotNil(t, selection)
	assert.Equal(t, 1, selection.Tier)

	a2 := &models.StandAssignment{
		Callsign: "ARR2", Stand: "A2",
		Direction: string(sat.AssignmentDirectionArrival), Stage: StageEstimated,
	}
	selected, selection, _, _, _, err = service.selectStand(AutomaticStandAllocation, request, evaluation, []*models.StandAssignment{a1, a2}, nil, nil)
	require.NoError(t, err)
	assert.Contains(t, []string{"A1", "A2"}, selected, "a soft reservation yields before forcing the request to tier 2")
	require.NotNil(t, selection)
	assert.Equal(t, 1, selection.Tier)
	assert.True(t, selectedHasEstimatedReservation([]*models.StandAssignment{a1, a2}, selected, request.Callsign))

	request.Stage = StageEstimated
	request.ETA = nil
	selected, selection, _, _, _, err = service.selectStand(AutomaticStandAllocation, request, evaluation, []*models.StandAssignment{a1, a2}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "A3", selected, "an equal-priority estimate must not churn existing reservations")
	require.NotNil(t, selection)
	assert.Equal(t, 2, selection.Tier)

	earlierETA := time.Now().Add(15 * time.Minute)
	laterETA := earlierETA.Add(30 * time.Minute)
	request.ETA = &earlierETA
	a1.ETA = &laterETA
	selected, selection, _, _, _, err = service.selectStand(AutomaticStandAllocation, request, evaluation, []*models.StandAssignment{a1, a2}, nil, nil)
	require.NoError(t, err)
	assert.Contains(t, []string{"A1", "A2"}, selected, "an earlier estimated arrival may take priority over a later or untimed estimate")
	require.NotNil(t, selection)
	assert.Equal(t, 1, selection.Tier)

	request.Stage = StageAssigned
	request.ETA = nil
	fallbackPolicy, err := sat.LoadAirlineAssignment(strings.NewReader(`{
  "rules": [{"id":"test","callsigns":["TST"],"stands":{"tier1":{"A1":100}}}],
  "stand_groups": {},
  "fallback_rules": {`+testFallbackJSON("A3")+`}
}`), registry)
	require.NoError(t, err)
	service.policy = fallbackPolicy

	selected, selection, _, _, _, err = service.selectStand(AutomaticStandAllocation, request, evaluation, []*models.StandAssignment{a1}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "A1", selected, "a soft reservation yields before forcing the request onto a fallback rule")
	require.NotNil(t, selection)
	assert.False(t, selection.FallbackUsed)
}

func standAllocationFixture(t *testing.T, pool *pgxpool.Pool, queries *database.Queries, a1Directive, a2Directive string) (*StandAllocationService, int32, repository.StandAssignmentRepository) {
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
	service, err := NewStandAllocationService(pool, postgres.NewStripRepository(pool), assignments, registry, policy, WithStandAllocationRandom(func() float64 { return 0 }))
	require.NoError(t, err)
	name := fmt.Sprintf("%s-%d", t.Name(), standAllocationSessionSequence.Add(1))
	return service, testdata.SeedTestSessionNamedWithSectors(t, queries, name, nil), assignments
}

func standAllocationRequest(session int32, callsign string) StandAllocationRequest {
	return StandAllocationRequest{
		SessionID: session, Callsign: callsign, Airport: "EKCH", Direction: sat.AssignmentDirectionArrival,
		FlightFacts: sat.FlightCompatibilityFacts{Direction: sat.Arrival},
	}
}

func withStand(request StandAllocationRequest, stand string) StandAllocationRequest {
	request.Stand = stand
	return request
}

func testFallbackJSON(stand string) string {
	names := []string{"airliner_default", "business_vip", "cargo", "military", "military_helicopter", "helicopter", "ga_private", "unknown"}
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, `"`+name+`":{"stands":{"tier1":{"`+stand+`":100}}}`)
	}
	return strings.Join(parts, ",")
}

type retryRecorder struct {
	stands []string
}

type retryConflictRepository struct {
	repository.StandAssignmentRepository
	recorder *retryRecorder
}

func (r retryConflictRepository) WithTx(tx pgx.Tx) repository.StandAssignmentRepository {
	return retryConflictRepository{StandAssignmentRepository: r.StandAssignmentRepository.WithTx(tx), recorder: r.recorder}
}

func (r retryConflictRepository) UpdateAssignment(_ context.Context, assignment *models.StandAssignment) (int64, error) {
	r.recorder.stands = append(r.recorder.stands, assignment.Stand)
	return 0, nil
}
