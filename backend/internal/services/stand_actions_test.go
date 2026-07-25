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

	result, err := actions.AssignForPilot(context.Background(), session, "EKCH", "1234567", "SAS803", stand, 0)

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

type failingGetAssignmentRepository struct {
	repository.StandAssignmentRepository
	err error
}

func (r failingGetAssignmentRepository) GetAssignment(context.Context, int32, string) (*models.StandAssignment, error) {
	return nil, r.err
}
