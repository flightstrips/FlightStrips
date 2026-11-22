package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestObservedDepartureRetriesDisplacementWhileETAWriterWaitsForRevision(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	service, session, assignments := standAllocationFixture(t, pool, queries, "", "")
	for _, callsign := range []string{"SAS910", "SAS911"} {
		testdata.SeedTestStrip(t, queries, session, callsign)
	}
	arrival := withStand(standAllocationRequest(session, "SAS910"), "A1")
	arrival.Stage = StageAssigned
	_, err := service.AssignManually(ctx, arrival)
	require.NoError(t, err)

	writer, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer writer.Rollback(context.Background())
	var writerPID int32
	require.NoError(t, writer.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&writerPID))
	writerResult := make(chan error, 1)
	freshETA := models.ArrivalETA{
		Time:         time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond),
		CalculatedAt: time.Now().UTC().Truncate(time.Microsecond), Source: "aman",
	}
	strips := postgres.NewStripRepository(pool)
	service.strips = &displacementRevisionGuardStore{
		StripRepository: strips,
		guard: &displacementRevisionGuard{
			afterLock: func(ctx context.Context, allocation pgx.Tx) error {
				// Reproduce the real pass guard with the revision held before
				// starting the competing ETA write. The writer then owns the
				// displaced strip and waits in its revision trigger.
				var allocatorPID, stripID int32
				if err := allocation.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&allocatorPID); err != nil {
					return err
				}
				if err := writer.QueryRow(ctx, "SELECT id FROM strips WHERE session=$1 AND callsign='SAS910' FOR UPDATE", session).Scan(&stripID); err != nil {
					return err
				}
				go func() {
					_, err := strips.WithTx(writer).UpdateArrivalETA(ctx, session, "SAS910", freshETA)
					if err == nil {
						err = writer.Commit(ctx)
					}
					writerResult <- err
				}()
				ticker := time.NewTicker(time.Millisecond)
				defer ticker.Stop()
				for {
					var blocked bool
					if err := pool.QueryRow(ctx, "SELECT $1 = ANY(pg_blocking_pids($2))", allocatorPID, writerPID).Scan(&blocked); err != nil {
						return err
					}
					if blocked {
						return nil
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-ticker.C:
					}
				}
			},
		},
	}
	guard := service.strips.(*displacementRevisionGuardStore).guard
	departure := withStand(standAllocationRequest(session, "SAS911"), "A1")
	departure.Direction = sat.AssignmentDirectionDeparture
	departure.FlightFacts.Direction = sat.Departure
	departure.Stage = StageDepartureBlock
	departure.ObservedStand = stringPointer("A1")
	var publications []StandAllocationResult
	service.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
		publications = append(publications, result)
		return nil
	})

	result, err := service.assignObservedStand(ctx, departure)
	// A deadlock can abort the allocator, whose existing retry would hide the
	// regression. Require the first contested lock to fail without waiting,
	// independently of which transaction PostgreSQL would choose to abort.
	require.True(t, guard.victimLockAttempted, "displacement must attempt to lock the arrival")
	var lockErr *pgconn.PgError
	require.ErrorAs(t, guard.victimLockErr, &lockErr)
	require.Equal(t, "55P03", lockErr.Code, "the contested lock must report lock_not_available, never a recovered deadlock")
	require.NoError(t, err)
	require.Equal(t, "A1", result.Assignment.Stand, "retry must retain the aircraft's physical stand")
	require.Greater(t, result.Attempts, 1)
	require.Len(t, result.RemovedAssignments, 1)
	require.Equal(t, "SAS910", result.RemovedAssignments[0].Callsign)
	require.Len(t, publications, 1, "the rolled-back attempt must not publish")
	select {
	case err := <-writerResult:
		require.NoError(t, err, "the ETA writer must commit without being a deadlock victim")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	strip, err := strips.GetByCallsign(ctx, session, "SAS910")
	require.NoError(t, err)
	require.Equal(t, &freshETA, strip.ArrivalETA, "displacement must preserve the committed ETA")
	require.Nil(t, strip.Stand)
	_, err = assignments.GetAssignment(ctx, session, "SAS910")
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

// The first transaction holds the same revision lock as a VATSIM arrival pass.
// The hook coordinates the competing transaction after that guard and before
// allocation reads assignments or attempts displacement.
type displacementRevisionGuard struct {
	once      sync.Once
	afterLock func(context.Context, pgx.Tx) error

	victimLockOnce      sync.Once
	victimLockAttempted bool
	victimLockErr       error
}

func (g *displacementRevisionGuard) recordVictimLock(callsign string, err error) {
	if callsign == "SAS910" {
		g.victimLockOnce.Do(func() {
			g.victimLockAttempted = true
			g.victimLockErr = err
		})
	}
}

type displacementRevisionGuardStore struct {
	repository.StripRepository
	guard *displacementRevisionGuard
	tx    pgx.Tx
}

func (s *displacementRevisionGuardStore) WithTx(tx pgx.Tx) repository.StripRepository {
	return &displacementRevisionGuardStore{StripRepository: s.StripRepository.WithTx(tx), guard: s.guard, tx: tx}
}

func (s *displacementRevisionGuardStore) LockByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	strip, err := s.StripRepository.LockByCallsign(ctx, session, callsign)
	s.guard.recordVictimLock(callsign, err)
	if err != nil {
		return nil, err
	}
	s.guard.once.Do(func() {
		_, err = s.StripRepository.LockArrivalETARevision(ctx, session)
		if err == nil {
			err = s.guard.afterLock(ctx, s.tx)
		}
	})
	return strip, err
}

func (s *displacementRevisionGuardStore) TryLockByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	strip, err := s.StripRepository.TryLockByCallsign(ctx, session, callsign)
	s.guard.recordVictimLock(callsign, err)
	return strip, err
}
