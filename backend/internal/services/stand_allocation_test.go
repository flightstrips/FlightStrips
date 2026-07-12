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
		strip, err := queries.GetStrip(ctx, database.GetStripParams{Session: session, Callsign: "SAS101"})
		require.NoError(t, err)
		require.NotNil(t, strip.Stand)
		assert.Equal(t, "A1", *strip.Stand)
		select {
		case event := <-published:
			assert.Equal(t, result.Assignment.ID, event.Assignment.ID)
		default:
			t.Fatal("allocation was not published after commit")
		}
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
