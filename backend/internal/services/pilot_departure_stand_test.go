package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pilotStandFixture struct {
	service     *PilotDepartureStandService
	lifecycle   *DepartureLifecycleService
	allocations *StandAllocationService
	pool        *pgxpool.Pool
	queries     *database.Queries
	session     int32
	assignments repository.StandAssignmentRepository
	strip       *models.Strip
	clock       *fakeClock
}

func newPilotStandFixture(t *testing.T) pilotStandFixture {
	t.Helper()
	pool, queries := testdata.SetupTestDB(t)
	lifecycle, allocations, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
	_, err := pool.Exec(context.Background(), "UPDATE sessions SET name='LIVE' WHERE id=$1", session)
	require.NoError(t, err)
	testdata.SeedTestStrip(t, queries, session, "SAS123")
	return pilotStandFixture{
		service: NewPilotDepartureStandService(lifecycle), lifecycle: lifecycle, allocations: allocations,
		pool: pool, queries: queries, session: session, assignments: assignments,
		strip: loadStrip(t, strips, session, "SAS123"), clock: clock,
	}
}

func TestPilotDepartureStandPrefileHold(t *testing.T) {
	f := newPilotStandFixture(t)
	ctx := context.Background()
	flight := offlineFlight("SAS123", 1)
	state, err := f.service.Read(ctx, f.session, flight)
	require.NoError(t, err)
	require.Nil(t, state, "reading must not start an assignment")
	state, err = f.service.Open(ctx, f.session, f.strip, flight)
	require.NoError(t, err)
	require.Equal(t, PilotStandAssigned, state.State)
	require.Equal(t, "A1", state.Stand)
	require.Equal(t, f.clock.now.Add(15*time.Minute), *state.ExpiresAt)
	firstExpiry, firstVersion := *state.ExpiresAt, state.Version

	f.clock.advance(7 * time.Minute)
	state, err = f.service.Open(ctx, f.session, f.strip, flight)
	require.NoError(t, err)
	assert.Equal(t, firstExpiry, *state.ExpiresAt)
	assert.Equal(t, firstVersion, state.Version)
	flight.Revision++
	require.NoError(t, f.lifecycle.ProcessDeparture(ctx, f.session, f.strip, flight))
	state, err = f.service.Read(ctx, f.session, flight)
	require.NoError(t, err)
	assert.Equal(t, firstExpiry, *state.ExpiresAt, "feed revisions must not renew EFB reservations")

	state, err = f.service.Select(ctx, f.session, f.strip, flight, "A2", state.Version, false)
	require.NoError(t, err)
	assert.Equal(t, PilotStandSelected, state.State)
	assert.Equal(t, "A2", state.Stand)
	assert.Equal(t, f.clock.now.Add(15*time.Minute), *state.ExpiresAt)
	reserved, err := f.assignments.ListAssignments(ctx, f.session)
	require.NoError(t, err)
	require.Len(t, reserved, 1)
	assert.Equal(t, "A2", reserved[0].Stand, "manual selection releases the previous reservation")
	selectedExpiry := *state.ExpiresAt
	f.clock.advance(time.Minute)
	state, err = f.service.Select(ctx, f.session, f.strip, flight, "A2", state.Version, false)
	require.NoError(t, err)
	assert.Equal(t, selectedExpiry, *state.ExpiresAt, "selecting the same stand is not a new selection")

	f.clock.set(selectedExpiry)
	state, err = f.service.Open(ctx, f.session, f.strip, flight)
	require.NoError(t, err)
	assert.Equal(t, PilotStandExpired, state.State)
	assert.Equal(t, "A2", state.Stand)
	assert.Zero(t, state.Version)
	_, err = f.assignments.GetAssignment(ctx, f.session, flight.Callsign)
	require.Error(t, err)
	restarted := NewPilotDepartureStandService(f.lifecycle)
	state, err = restarted.Open(ctx, f.session, f.strip, flight)
	require.NoError(t, err)
	assert.Equal(t, PilotStandExpired, state.State, "reopening/restarting must not renew an expired episode")
	state, err = restarted.Select(ctx, f.session, f.strip, flight, "A1", 0, false)
	require.NoError(t, err)
	assert.Equal(t, PilotStandSelected, state.State)
}

func TestPilotDepartureStandConfirmedUnsuitableSelection(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "ATYP:B737", nil)
	_, err := pool.Exec(context.Background(), "UPDATE sessions SET name='LIVE' WHERE id=$1", session)
	require.NoError(t, err)
	testdata.SeedTestStrip(t, queries, session, "SAS123")
	strip := loadStrip(t, strips, session, "SAS123")
	service := NewPilotDepartureStandService(lifecycle)
	ctx := context.Background()
	flight := offlineFlight("SAS123", 1)
	state, err := service.Open(ctx, session, strip, flight)
	require.NoError(t, err)
	_, err = service.Select(ctx, session, strip, flight, "A2", state.Version, false)
	require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
	state, err = service.Select(ctx, session, strip, flight, "A2", state.Version, true)
	require.NoError(t, err)
	assert.Equal(t, PilotStandSelected, state.State)
	assert.Equal(t, "A2", state.Stand)
	assert.Equal(t, clock.now.Add(15*time.Minute), *state.ExpiresAt)
	persisted, err := assignments.GetAssignment(ctx, session, "SAS123")
	require.NoError(t, err)
	assert.Contains(t, *persisted.ConflictReason, "pilot confirmed unsuitable stand")
	reopened, err := NewPilotDepartureStandService(lifecycle).Read(ctx, session, flight)
	require.NoError(t, err)
	assert.Equal(t, "A2", reopened.Stand)
	assert.Equal(t, state.Version, reopened.Version)
}

func TestPilotDepartureStandDisconnect(t *testing.T) {
	for _, kind := range []string{"prefile", "confirmed", "mismatch"} {
		t.Run(kind, func(t *testing.T) {
			f := newPilotStandFixture(t)
			ctx := context.Background()
			if kind == "mismatch" {
				require.NoError(t, f.assignments.CreateAssignment(ctx, &models.StandAssignment{
					SessionID: f.session, Callsign: "ARRIVAL", Stand: "A1", Direction: string(sat.AssignmentDirectionArrival), Stage: StageAssigned, Source: "AUTOMATIC",
				}))
			}
			flight := onlineFlight("SAS123", 1)
			if kind == "prefile" {
				flight = offlineFlight("SAS123", 1)
			}
			first, err := f.service.Open(ctx, f.session, f.strip, flight)
			require.NoError(t, err)
			f.clock.advance(4 * time.Minute)
			require.NoError(t, f.lifecycle.CancelPilotDeparture(ctx, f.session, flight.Callsign))
			state, err := f.service.Read(ctx, f.session, offlineFlight("SAS123", 1))
			require.NoError(t, err)
			assert.Nil(t, state.MismatchSince)
			reservation, err := f.assignments.GetAssignment(ctx, f.session, flight.Callsign)
			if kind == "prefile" {
				require.NoError(t, err)
				assert.Equal(t, PilotStandAssigned, state.State)
				assert.Equal(t, *first.ExpiresAt, *state.ExpiresAt)
				assert.Equal(t, first.Stand, reservation.Stand)
			} else {
				require.Error(t, err, "departed pilots must not leave permanent occupancy")
				assert.Equal(t, PilotStandExpired, state.State)
			}
			if kind == "mismatch" {
				f.clock.advance(2 * time.Minute)
				require.NoError(t, f.lifecycle.ProcessDeparture(ctx, f.session, f.strip, flight))
				state, err = f.service.Read(ctx, f.session, flight)
				require.NoError(t, err)
				assert.Equal(t, PilotStandMismatch, state.State)
				require.NotNil(t, state.MismatchSince)
				assert.Equal(t, f.clock.now, *state.MismatchSince)
			}
		})
	}
}

func TestPilotDepartureStandDoesNotManageDifferentPilotOrRoute(t *testing.T) {
	f := newPilotStandFixture(t)
	ctx := context.Background()
	flight := offlineFlight("SAS123", 1)
	_, err := f.service.Open(ctx, f.session, f.strip, flight)
	require.NoError(t, err)
	otherPilot, otherRoute := flight, flight
	otherPilot.CID, otherRoute.Destination = "another-pilot", "EGLL"
	for _, changed := range []vatsim.DepartureFlightInfo{otherPilot, otherRoute} {
		managed, err := f.service.reconcile(ctx, f.session, f.strip, changed)
		require.NoError(t, err)
		assert.False(t, managed)
	}
}

func TestPilotDepartureStandOnlineTransition(t *testing.T) {
	f := newPilotStandFixture(t)
	ctx := context.Background()
	_, err := f.service.Open(ctx, f.session, f.strip, offlineFlight("SAS123", 1))
	require.NoError(t, err)
	flight := onlineFlightAtA2("SAS123", 1)
	require.NoError(t, f.lifecycle.ProcessDeparture(ctx, f.session, f.strip, flight))
	state, err := f.service.Read(ctx, f.session, flight)
	require.NoError(t, err)
	assert.Equal(t, PilotStandConfirmed, state.State)
	assert.Equal(t, "A2", state.Stand)
	firstVersion := state.Version
	f.clock.advance(20 * time.Minute)
	require.NoError(t, f.lifecycle.ProcessDeparture(ctx, f.session, f.strip, flight))
	state, err = f.service.Read(ctx, f.session, flight)
	require.NoError(t, err)
	assert.Equal(t, firstVersion, state.Version, "unchanged online position must not churn versions")
	reservation, err := f.assignments.GetAssignment(ctx, f.session, flight.Callsign)
	require.NoError(t, err)
	assert.Nil(t, reservation.ExpiresAt)
	assert.Nil(t, reservation.ProjectedReleaseAt)
	_, err = f.service.Select(ctx, f.session, f.strip, flight, "A1", state.Version, false)
	require.ErrorContains(t, err, "prefile-only")
	flight.Latitude, flight.Longitude = 55, 12
	require.NoError(t, f.lifecycle.ProcessDeparture(ctx, f.session, f.strip, flight))
	state, err = f.service.Read(ctx, f.session, flight)
	require.NoError(t, err)
	assert.Equal(t, PilotStandNotDetected, state.State)
	assert.Empty(t, state.Stand)
	_, err = f.assignments.GetAssignment(ctx, f.session, flight.Callsign)
	require.Error(t, err, "leaving the stand must release physical occupancy")
}

func TestPilotDepartureStandMismatchOverwrite(t *testing.T) {
	for _, stage := range []string{StageEstimated, StageAssigned, StageConfirmed} {
		t.Run(stage, func(t *testing.T) {
			f := newPilotStandFixture(t)
			ctx := context.Background()
			testdata.SeedTestStrip(t, f.queries, f.session, "SAS456")
			arrival := standAllocationRequest(f.session, "SAS456")
			arrival.Stand, arrival.Stage = "A1", stage
			_, err := f.allocations.AssignManually(ctx, arrival)
			require.NoError(t, err)
			relocations := 0
			f.allocations.relocate = func(ctx context.Context, displaced models.StandAssignment) error {
				relocations++
				_, err := f.allocations.Allocate(ctx, standAllocationRequest(f.session, displaced.Callsign))
				return err
			}
			flight := onlineFlight("SAS123", 1)
			state, err := f.service.Open(ctx, f.session, f.strip, flight)
			require.NoError(t, err)
			assert.Equal(t, PilotStandMismatch, state.State)
			assert.Equal(t, "A2", state.Stand)
			require.NotNil(t, state.MismatchSince)
			f.clock.advance(5*time.Minute - time.Millisecond)
			state, err = f.service.Open(ctx, f.session, f.strip, flight)
			require.NoError(t, err)
			assert.Equal(t, PilotStandMismatch, state.State)
			assert.Zero(t, relocations)
			f.clock.advance(time.Millisecond)
			require.NoError(t, f.lifecycle.ProcessDeparture(ctx, f.session, f.strip, flight))
			state, err = f.service.Read(ctx, f.session, flight)
			require.NoError(t, err)
			assert.Equal(t, PilotStandOverwritten, state.State)
			assert.Equal(t, "A1", state.Stand)
			assert.Equal(t, 1, relocations)
			moved, err := f.assignments.GetAssignment(ctx, f.session, "SAS456")
			require.NoError(t, err)
			assert.Equal(t, "A2", moved.Stand)
		})
	}
}

func TestPilotDepartureStandNeverOverwritesDepartureOrManualBlock(t *testing.T) {
	for _, kind := range []string{"departure", "manual block"} {
		t.Run(kind, func(t *testing.T) {
			f := newPilotStandFixture(t)
			ctx := context.Background()
			if kind == "departure" {
				observed := "A1"
				require.NoError(t, f.assignments.CreateAssignment(ctx, &models.StandAssignment{
					SessionID: f.session, Callsign: "OTHER", Stand: "A1", Direction: string(sat.AssignmentDirectionDeparture),
					Stage: StageDepartureBlock, Source: "AUTOMATIC", ObservedStand: &observed,
				}))
			} else {
				require.NoError(t, f.assignments.CreateBlock(ctx, &models.StandBlock{
					SessionID: f.session, Stand: "A1", Manual: true, BlockType: "MANUAL", Source: "CONTROLLER",
				}))
			}
			flight := onlineFlight("SAS123", 1)
			state, err := f.service.Open(ctx, f.session, f.strip, flight)
			require.NoError(t, err)
			assert.Equal(t, PilotStandMismatch, state.State)
			f.clock.advance(10 * time.Minute)
			state, err = f.service.Open(ctx, f.session, f.strip, flight)
			require.NoError(t, err)
			assert.Equal(t, PilotStandMismatch, state.State)
			assert.Equal(t, "A2", state.Stand)
			if kind == "departure" {
				other, err := f.assignments.GetAssignment(ctx, f.session, "OTHER")
				require.NoError(t, err)
				assert.Equal(t, "A1", other.Stand)
			} else {
				blocks, err := f.assignments.ListBlocks(ctx, f.session)
				require.NoError(t, err)
				require.Len(t, blocks, 1)
			}
		})
	}
}

func TestPilotDepartureStandMismatchResetsWhenLeaving(t *testing.T) {
	f := newPilotStandFixture(t)
	ctx := context.Background()
	require.NoError(t, f.assignments.CreateAssignment(ctx, &models.StandAssignment{
		SessionID: f.session, Callsign: "ARRIVAL", Stand: "A1", Direction: string(sat.AssignmentDirectionArrival), Stage: StageAssigned, Source: "AUTOMATIC",
	}))
	flight := onlineFlight("SAS123", 1)
	_, err := f.service.Open(ctx, f.session, f.strip, flight)
	require.NoError(t, err)
	f.clock.advance(4 * time.Minute)
	flight.Latitude, flight.Longitude = 55, 12
	_, err = f.service.Open(ctx, f.session, f.strip, flight)
	require.NoError(t, err)
	f.clock.advance(2 * time.Minute)
	state, err := f.service.Open(ctx, f.session, f.strip, onlineFlight("SAS123", 1))
	require.NoError(t, err)
	assert.Equal(t, PilotStandMismatch, state.State)
	assert.Equal(t, f.clock.now, *state.MismatchSince)
}

func TestPilotDepartureStandConcurrentFirstOpen(t *testing.T) {
	f := newPilotStandFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.service.Open(ctx, f.session, f.strip, offlineFlight("SAS123", 1))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	assignments, err := f.assignments.ListAssignments(ctx, f.session)
	require.NoError(t, err)
	require.Len(t, assignments, 1)
	assert.EqualValues(t, 1, assignments[0].Version)
}

func TestPilotDepartureStandRejectsTrainingSession(t *testing.T) {
	f := newPilotStandFixture(t)
	_, err := f.pool.Exec(context.Background(), "UPDATE sessions SET name='SWEATBOX' WHERE id=$1", f.session)
	require.NoError(t, err)
	_, err = f.service.Open(context.Background(), f.session, f.strip, offlineFlight("SAS123", 1))
	require.ErrorContains(t, err, "LIVE")
}
