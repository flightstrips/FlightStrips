package services

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type departureActivationRaceRepository struct {
	repository.StandAssignmentRepository
	afterRead    func(context.Context) error
	beforeUpdate func(context.Context) error
}

type departureActivationConflictRepository struct {
	repository.StandAssignmentRepository
	attempts int
}

func (r *departureActivationConflictRepository) UpdateAssignment(context.Context, *models.StandAssignment) (int64, error) {
	r.attempts++
	return 0, nil
}

func (r *departureActivationRaceRepository) GetAssignment(ctx context.Context, session int32, callsign string) (*models.StandAssignment, error) {
	assignment, err := r.StandAssignmentRepository.GetAssignment(ctx, session, callsign)
	if err == nil && r.afterRead != nil {
		hook := r.afterRead
		r.afterRead = nil
		err = hook(ctx)
	}
	return assignment, err
}

func (r *departureActivationRaceRepository) UpdateAssignment(ctx context.Context, assignment *models.StandAssignment) (int64, error) {
	if r.beforeUpdate != nil {
		hook := r.beforeUpdate
		r.beforeUpdate = nil
		if err := hook(ctx); err != nil {
			return 0, err
		}
	}
	return r.StandAssignmentRepository.UpdateAssignment(ctx, assignment)
}

func TestDepartureActivationReassignment(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	for _, race := range []string{"between reads", "during update"} {
		for _, tsat := range []string{"", "0950"} {
			name := race + "/without projection"
			if tsat != "" {
				name = race + "/expired projection"
			}
			t.Run(name, func(t *testing.T) {
				lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
				const callsign = "SASRACE1"
				testdata.SeedTestStrip(t, queries, session, callsign)
				require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), offlineFlight(callsign, 1)))
				if tsat != "" {
					_, err := strips.SetCdmData(ctx, session, callsign, &models.CdmData{Tsat: &tsat})
					require.NoError(t, err)
				}
				var replacement models.StandAssignment
				reassigned := false
				reassign := func(ctx context.Context) error {
					current, err := assignments.GetAssignment(ctx, session, callsign)
					require.NoError(t, err)
					current.Stand = "A2"
					current.ExpiresAt = nil
					affected, err := assignments.UpdateAssignment(ctx, current)
					require.NoError(t, err)
					require.EqualValues(t, 1, affected)
					current.Version++
					replacement = *current
					// A protected occupant prevents the normal observation path
					// from moving this aircraft's new reservation back to A1.
					createProtectedStandOccupant(t, assignments, session, "SASRACE1O", "A1")
					reassigned = true
					return nil
				}
				raced := &departureActivationRaceRepository{StandAssignmentRepository: assignments}
				if race == "between reads" {
					raced.afterRead = reassign
				} else {
					raced.beforeUpdate = reassign
				}
				lifecycle.assignments = raced
				require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
				require.True(t, reassigned)
				assignment, err := assignments.GetAssignment(ctx, session, callsign)
				require.NoError(t, err)
				assert.Equal(t, replacement.ID, assignment.ID)
				assert.Equal(t, "A2", assignment.Stand)
				assert.Equal(t, StageReserved, assignment.Stage, "a stale activation must re-evaluate the replacement reservation")
				assert.Nil(t, assignment.ExpiresAt, "live mismatches must not inherit a projected release deadline")
				require.NotNil(t, assignment.ConflictReason)
				assert.Equal(t, wrongStandPendingPrefix+"A1", *assignment.ConflictReason)
				assert.Nil(t, assignment.ObservedStand)
				if tsat != "" {
					require.NotNil(t, assignment.ProjectedReleaseAt)
					assert.True(t, assignment.ProjectedReleaseAt.Before(clock.current()))
				}
				clock.advance(time.Hour)
				require.NoError(t, lifecycle.ReleaseExpired(ctx))
				retained, err := assignments.GetAssignment(ctx, session, callsign)
				require.NoError(t, err, "the sweep must retain the replacement while the aircraft is live on another stand")
				assert.Equal(t, replacement.ID, retained.ID)
			})
		}
	}

	t.Run("repeated conflicts stop after bounded retries", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, _ := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASRACE2"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), offlineFlight(callsign, 1)))
		conflicts := &departureActivationConflictRepository{StandAssignmentRepository: assignments}
		lifecycle.assignments = conflicts
		err := lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1))
		require.ErrorIs(t, err, errAllocationVersionConflict)
		assert.Equal(t, maxSerializableOperationAttempts, conflicts.attempts)
		assignment, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
		assert.Equal(t, StageReserved, assignment.Stage)
		assert.Nil(t, assignment.ObservedStand)
	})
}
