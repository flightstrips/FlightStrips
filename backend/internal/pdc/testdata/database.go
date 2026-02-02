package testdata

import (
	"FlightStrips/internal/database"
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// getMigrationsPath returns the absolute path to the migrations directory
func getMigrationsPath() string {
	// Get the path to this file
	_, filename, _, _ := runtime.Caller(0)
	// Navigate up to backend/internal/pdc/testdata -> backend -> migrations
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "migrations")
}

// SetupTestDB creates a test database connection with automatic PostgreSQL container
func SetupTestDB(t *testing.T) (*pgxpool.Pool, *database.Queries) {
	ctx := context.Background()

	// Start PostgreSQL container
	postgresContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err, "Failed to start PostgreSQL container")

	// Cleanup container when test finishes
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(postgresContainer); err != nil {
			t.Logf("Failed to terminate PostgreSQL container: %v", err)
		}
	})

	// Get connection string
	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Run migrations
	migrationsPath := getMigrationsPath()
	err = database.Migrate(connStr, migrationsPath)
	require.NoError(t, err, "Failed to run migrations")

	// Connect to database
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "Failed to connect to test database")

	t.Cleanup(func() {
		pool.Close()
	})

	queries := database.New(pool)
	return pool, queries
}

// SeedTestSession inserts a test session with required sector owners
func SeedTestSession(t *testing.T, queries *database.Queries) int32 {
	ctx := context.Background()

	// First insert the airport (required by foreign key)
	err := queries.InsertAirport(ctx, "EKCH")
	if err != nil {
		// Ignore error if airport already exists
		t.Logf("Airport insert warning (may already exist): %v", err)
	}

	sessionID, err := queries.InsertSession(ctx, database.InsertSessionParams{
		Name:    "LIVE",
		Airport: "EKCH",
	})
	require.NoError(t, err)

	// Insert sector owners for PDC frequency lookup
	// Based on EKCH configuration: SQ and DEL sectors assigned to tower
	_, err = queries.InsertSectorOwners(ctx, []database.InsertSectorOwnersParams{
		{
			Session:    sessionID,
			Sector:     []string{"AA", "AD", "DEL", "GW", "SQ", "TE", "TW"},
			Position:   "118.105", // Position name (max 7 chars)
			Identifier: "TE",      // Frequency identifier
		},
	})
	require.NoError(t, err)

	return sessionID
}

// SeedTestStrip inserts a test strip
func SeedTestStrip(t *testing.T, queries *database.Queries, sessionID int32, callsign string) {
	ctx := context.Background()

	err := queries.InsertStrip(ctx, database.InsertStripParams{
		Callsign:       callsign,
		Session:        sessionID,
		Origin:         "EKCH",
		Destination:    "ESSA",
		AircraftType:   ptr("A320"),
		Runway:         ptr("22L"),
		Sid:            ptr("VEMBO2E"),
		Squawk:         ptr("2401"),
		AssignedSquawk: ptr("2401"),
		Bay:            "NOT_CLEARED",
	})
	require.NoError(t, err)
}

func ptr[T any](v T) *T { return &v }

// CleanupTestSession removes test session and all related data
func CleanupTestSession(t *testing.T, queries *database.Queries, sessionID int32) {
	ctx := context.Background()

	// Delete session (should cascade to strips)
	_, err := queries.DeleteSession(ctx, sessionID)
	require.NoError(t, err)
}
