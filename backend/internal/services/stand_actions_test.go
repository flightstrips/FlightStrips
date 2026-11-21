package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandActionDepartureStartsReserved(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	allocation, session, assignments := standAllocationFixture(t, pool, queries, "", "")
	testdata.SeedTestStrip(t, queries, session, "SAS801")
	actions := NewStandActionService(allocation, assignments, postgres.NewStripRepository(pool), nil, nil, nil)

	before := time.Now().UTC()
	result, err := actions.AssignManually(context.Background(), session, "EKCH", "GND", "SAS801", "A1", 0)
	require.NoError(t, err)
	assert.Equal(t, StageReserved, result.Assignment.Stage)
	require.NotNil(t, result.Assignment.ExpiresAt)
	assert.WithinDuration(t, before.Add(defaultDepartureHoldDuration), *result.Assignment.ExpiresAt, time.Second)
}

func TestPilotStandRequestSyncsEuroscopeWhenTheStripAlreadyHasTheSelectedStand(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	allocation, session, assignments := standAllocationFixture(t, pool, queries, "", "")
	testdata.SeedTestStrip(t, queries, session, "SAS803")
	stand := "A1"
	updated, err := queries.UpdateStripStandByID(context.Background(), database.UpdateStripStandByIDParams{
		Callsign: "SAS803", Session: session, Stand: &stand,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
	actions := NewStandActionService(allocation, assignments, postgres.NewStripRepository(pool), nil, nil, nil)

	result, err := actions.AssignForPilot(context.Background(), session, "EKCH", "1234567", "SAS803", stand, 0, false)

	require.NoError(t, err)
	assert.False(t, result.StandChanged)
	assert.True(t, result.NotifyEuroscope, "an EFB selection must be forwarded even when the stand was already persisted")
}

func TestPilotStandAvailabilityUsesTheAssignmentRules(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	allocation, session, assignments := standAllocationFixture(t, pool, queries, "", "")
	testdata.SeedTestStrip(t, queries, session, "SAS804")
	testdata.SeedTestStrip(t, queries, session, "SAS805")
	actions := NewStandActionService(allocation, assignments, postgres.NewStripRepository(pool), nil, nil, nil)

	_, err := actions.AssignManually(context.Background(), session, "EKCH", "GND", "SAS804", "A1", 0)
	require.NoError(t, err)
	availability, err := actions.AvailableForPilot(context.Background(), session, "EKCH", "SAS805")

	require.NoError(t, err)
	byStand := make(map[string]StandAvailability, len(availability))
	for _, entry := range availability {
		byStand[entry.Stand] = entry
	}
	assert.False(t, byStand["A1"].Available)
	assert.Contains(t, byStand["A1"].Reason, "reserved by SAS804")
	assert.True(t, byStand["A2"].Available)
}

func TestStandActionPropagatesAssignmentLookupErrors(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	allocation, session, assignments := standAllocationFixture(t, pool, queries, "", "")
	testdata.SeedTestStrip(t, queries, session, "SAS802")
	wanted := errors.New("temporary assignment lookup failure")
	actions := NewStandActionService(allocation, failingGetAssignmentRepository{StandAssignmentRepository: assignments, err: wanted}, postgres.NewStripRepository(pool), nil, nil, nil)

	_, err := actions.AssignManually(context.Background(), session, "EKCH", "GND", "SAS802", "A1", 0)
	require.ErrorIs(t, err, wanted)
	_, queryErr := queries.GetStandAssignment(context.Background(), database.GetStandAssignmentParams{SessionID: session, Callsign: "SAS802"})
	require.Error(t, queryErr)
}

func TestControllerCanForceAutomaticReassignmentOfConfirmedArrival(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	allocation, session, assignments := standAllocationFixture(t, pool, queries, "", "")
	strips := postgres.NewStripRepository(pool)
	testdata.SeedTestStrip(t, queries, session, "SAS806")
	testdata.SeedTestStrip(t, queries, session, "SAS807")
	ctx := context.Background()

	confirmed := withStand(standAllocationRequest(session, "SAS806"), "A1")
	confirmed.Stage = StageConfirmed
	initial, err := allocation.AssignManually(ctx, confirmed)
	require.NoError(t, err)

	departure := withStand(standAllocationRequest(session, "SAS807"), "A1")
	departure.Direction = "DEPARTURE"
	departure.FlightFacts.Direction = "DEPARTURE"
	departure.Stage = StageDepartureBlock
	_, err = allocation.assignObservedStand(ctx, departure)
	require.NoError(t, err)

	actions := NewStandActionService(allocation, assignments, strips, nil, nil, nil)
	forced, err := actions.Allocate(ctx, session, "EKCH", "EKCH_GND", "SAS806", initial.Assignment.Version)
	require.NoError(t, err)
	assert.Equal(t, StageConfirmed, forced.Assignment.Stage)
	assert.NotEqual(t, "A1", forced.Assignment.Stand)
	assert.False(t, forced.Assignment.Manual)
}

func TestStandReassignmentRejectsChangesAfterVersionCheck(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	for _, tc := range []struct {
		name    string
		manual  bool
		initial bool
		replace bool
	}{
		{name: "AUTO after pinned arrival promotion", initial: true},
		{name: "manual after pinned arrival promotion", initial: true, manual: true},
		{name: "manual after different assignment with the same version", initial: true, manual: true, replace: true},
		{name: "AUTO after assignment created", replace: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			allocation, session, assignments := standAllocationFixture(t, pool, queries, "", "")
			const callsign = "SAS808"
			seedTestArrivalStrip(t, queries, session, callsign)
			strips := postgres.NewStripRepository(pool)
			initial := &models.StandAssignment{
				SessionID: session, Callsign: callsign, Stand: "A1", Direction: "ARRIVAL",
				Stage: StageEstimated, Source: "MANUAL", Manual: true,
				ConflictReason: pinnedArrivalPromotionConflict("A1", nil, []string{"reserved"}),
			}
			var version int32
			if tc.initial {
				require.NoError(t, assignments.CreateAssignment(ctx, initial))
				version = initial.Version
			}
			hook := &standActionPreflightRepository{StandAssignmentRepository: assignments}
			hook.afterRead = func() {
				if tc.replace {
					if tc.initial {
						rows, err := assignments.DeleteAssignment(ctx, session, initial.ID, initial.Version)
						require.NoError(t, err)
						require.EqualValues(t, 1, rows)
					}
					replacement := *initial
					replacement.Stage = StageConfirmed
					require.NoError(t, assignments.CreateAssignment(ctx, &replacement))
					if tc.initial {
						require.NotEqual(t, initial.ID, replacement.ID)
						require.Equal(t, version, replacement.Version)
					}
					return
				}
				promoted := *initial
				promoted.Stage = StageConfirmed
				rows, err := assignments.UpdateAssignment(ctx, &promoted)
				require.NoError(t, err)
				require.EqualValues(t, 1, rows)
			}
			actions := NewStandActionService(allocation, hook, strips, nil, nil, nil)
			var err error
			if tc.manual {
				_, err = actions.AssignManually(ctx, session, "EKCH", "GND", callsign, "A2", version)
			} else {
				_, err = actions.Allocate(ctx, session, "EKCH", "GND", callsign, version)
			}
			require.ErrorIs(t, err, ErrStandActionStaleVersion)
			retained, err := assignments.GetAssignment(ctx, session, callsign)
			require.NoError(t, err)
			assert.Equal(t, StageConfirmed, retained.Stage)
			assert.Equal(t, "A1", retained.Stand)
		})
	}
}

type standActionPreflightRepository struct {
	repository.StandAssignmentRepository
	afterRead func()
}

func (r *standActionPreflightRepository) GetAssignment(ctx context.Context, session int32, callsign string) (*models.StandAssignment, error) {
	assignment, err := r.StandAssignmentRepository.GetAssignment(ctx, session, callsign)
	if r.afterRead != nil {
		r.afterRead()
		r.afterRead = nil
	}
	return assignment, err
}

type failingGetAssignmentRepository struct {
	repository.StandAssignmentRepository
	err error
}

func (r failingGetAssignmentRepository) GetAssignment(context.Context, int32, string) (*models.StandAssignment, error) {
	return nil, r.err
}
