package postgres

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestStandAssignmentRepositoryCRUDVersioningAndSessionCleanup(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	sessionID := testdata.SeedTestSessionNamedWithSectors(t, queries, "SAT", nil)
	repo := NewStandAssignmentRepository(pool)
	ctx := context.Background()

	eta := time.Now().UTC().Add(time.Hour)
	assignment := &models.StandAssignment{
		SessionID: sessionID,
		Callsign:  "SAS123",
		Stand:     "ECHO12",
		Direction: "ARRIVAL",
		Stage:     "ESTIMATED",
		Source:    "AUTOMATIC",
		ETA:       &eta,
		ETASource: stringPtrForTest("FILED"),
		Manual:    false,
	}
	require.NoError(t, repo.CreateAssignment(ctx, assignment))
	require.NotZero(t, assignment.ID)
	require.Equal(t, int32(1), assignment.Version)

	got, err := repo.GetAssignment(ctx, sessionID, "SAS123")
	require.NoError(t, err)
	require.Equal(t, assignment.ID, got.ID)
	require.Equal(t, assignment.ETA, got.ETA)

	duplicate := &models.StandAssignment{
		SessionID: sessionID,
		Callsign:  "SAS123",
		Stand:     "ECHO13",
		Direction: "ARRIVAL",
		Stage:     "ESTIMATED",
		Source:    "AUTOMATIC",
	}
	require.Error(t, repo.CreateAssignment(ctx, duplicate), "callsign uniqueness must be session-scoped")

	got.Stage = "ASSIGNED"
	got.Stand = "ECHO13"
	rows, err := repo.UpdateAssignment(ctx, got)
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	stale := *got
	stale.Version = 1
	rows, err = repo.UpdateAssignment(ctx, &stale)
	require.NoError(t, err)
	require.Zero(t, rows, "a stale assignment version must not update")

	deletable := &models.StandAssignment{
		SessionID: sessionID,
		Callsign:  "SAS999",
		Stand:     "ECHO15",
		Direction: "DEPARTURE",
		Stage:     "ASSIGNED",
		Source:    "MANUAL",
	}
	require.NoError(t, repo.CreateAssignment(ctx, deletable))
	rows, err = repo.DeleteAssignment(ctx, sessionID, deletable.ID, deletable.Version+1)
	require.NoError(t, err)
	require.Zero(t, rows, "a stale assignment version must not delete")
	rows, err = repo.DeleteAssignment(ctx, sessionID, deletable.ID, deletable.Version)
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	block := &models.StandBlock{
		SessionID: sessionID,
		Stand:     "FOXTROT1",
		BlockType: "CLOSURE",
		Source:    "CONTROLLER",
		Reason:    stringPtrForTest("maintenance"),
		CreatedBy: stringPtrForTest("EKCH_TWR"),
		Manual:    true,
	}
	require.NoError(t, repo.CreateBlock(ctx, block))
	require.NotZero(t, block.ID)
	require.Equal(t, int32(1), block.Version)

	blocks, err := repo.ListBlocksByStand(ctx, sessionID, "FOXTROT1")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	blocks[0].Reason = stringPtrForTest("closed")
	rows, err = repo.UpdateBlock(ctx, blocks[0])
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	rows, err = repo.DeleteBlock(ctx, sessionID, block.ID, block.Version)
	require.NoError(t, err)
	require.Zero(t, rows, "a stale block version must not delete")
	currentBlock, err := repo.GetBlock(ctx, sessionID, block.ID)
	require.NoError(t, err)
	rows, err = repo.DeleteBlock(ctx, sessionID, block.ID, currentBlock.Version)
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	_, err = queries.DeleteSession(ctx, sessionID)
	require.NoError(t, err)
	_, err = repo.GetAssignment(ctx, sessionID, "SAS123")
	require.True(t, errors.Is(err, pgx.ErrNoRows), "session deletion must cascade SAT assignments: %v", err)
}

func TestStandAssignmentRepositoryWithTx(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	sessionID := testdata.SeedTestSessionNamedWithSectors(t, queries, "SAT-TX", nil)
	repo := NewStandAssignmentRepository(pool)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	txRepo := repo.WithTx(tx)
	assignment := &models.StandAssignment{
		SessionID: sessionID,
		Callsign:  "SAS456",
		Stand:     "ECHO14",
		Direction: "DEPARTURE",
		Stage:     "ASSIGNED",
		Source:    "MANUAL",
		Manual:    true,
	}
	require.NoError(t, txRepo.CreateAssignment(ctx, assignment))
	require.NoError(t, tx.Commit(ctx))

	got, err := repo.GetAssignment(ctx, sessionID, "SAS456")
	require.NoError(t, err)
	require.Equal(t, "ECHO14", got.Stand)
}

func TestCreateStandBlockWaitsForSessionAllocationGuard(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	sessionID := testdata.SeedTestSessionNamedWithSectors(t, queries, "SAT-BLOCK-GUARD", nil)
	repo := NewStandAssignmentRepository(pool)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, "SELECT id FROM sessions WHERE id = $1 FOR UPDATE", sessionID)
	require.NoError(t, err)

	timedOut, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	err = repo.CreateBlock(timedOut, &models.StandBlock{
		SessionID: sessionID,
		Stand:     "ECHO12",
		BlockType: "CLOSURE",
		Source:    "CONTROLLER",
		Manual:    true,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func stringPtrForTest(value string) *string {
	return &value
}
