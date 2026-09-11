package testdata

import (
	"FlightStrips/internal/database"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var sharedPostgres struct {
	once      sync.Once
	container *postgres.PostgresContainer
	adminPool *pgxpool.Pool
	baseURL   string
	err       error
}

var databaseSequence atomic.Uint64

const testDatabaseServerURLEnv = "FLIGHTSTRIPS_TEST_DATABASE_SERVER_URL"

// getMigrationsPath returns the absolute path to the migrations directory
func getMigrationsPath() string {
	// Get the path to this file
	_, filename, _, _ := runtime.Caller(0)
	// Navigate up to backend/internal/pdc/testdata -> backend -> migrations
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "migrations")
}

// SetupTestDB creates an isolated database cloned from a migrated template. When
// the test suite is run through internal/testing/testdb, all package test processes
// share that command's PostgreSQL server. Direct package test runs fall back to one
// server for that package process.
func SetupTestDB(t *testing.T) (*pgxpool.Pool, *database.Queries) {
	t.Helper()
	pool, queries, cleanup, err := OpenTestDB(context.Background())
	require.NoError(t, err, "Failed to prepare isolated PostgreSQL test database")
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Logf("Failed to clean up PostgreSQL test database: %v", err)
		}
	})
	return pool, queries
}

// OpenTestDB creates an isolated database without requiring a testing.T. Callers
// must invoke the returned cleanup function after all database users have stopped.
func OpenTestDB(ctx context.Context) (*pgxpool.Pool, *database.Queries, func() error, error) {

	sharedPostgres.once.Do(func() {
		if serverURL := os.Getenv(testDatabaseServerURLEnv); serverURL != "" {
			sharedPostgres.baseURL = serverURL
			sharedPostgres.err = openAdminPool(ctx)
			return
		}

		sharedPostgres.container, sharedPostgres.err = postgres.Run(ctx,
			"postgres:16-alpine",
			postgres.WithDatabase("testdb"),
			postgres.WithUsername("postgres"),
			postgres.WithPassword("postgres"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(30*time.Second)),
		)
		if sharedPostgres.err != nil {
			return
		}

		sharedPostgres.baseURL, sharedPostgres.err = sharedPostgres.container.ConnectionString(ctx, "sslmode=disable")
		if sharedPostgres.err != nil {
			return
		}
		sharedPostgres.err = database.Migrate(sharedPostgres.baseURL, getMigrationsPath())
		if sharedPostgres.err != nil {
			return
		}

		sharedPostgres.err = openAdminPool(ctx)
	})
	if sharedPostgres.err != nil {
		return nil, nil, nil, sharedPostgres.err
	}

	databaseName := fmt.Sprintf("test_%d_%d", os.Getpid(), databaseSequence.Add(1))
	_, err := sharedPostgres.adminPool.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{databaseName}.Sanitize()+" TEMPLATE testdb")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("clone PostgreSQL test database: %w", err)
	}

	testConfig, err := pgxpool.ParseConfig(sharedPostgres.baseURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse PostgreSQL test database URL: %w", err)
	}
	testConfig.ConnConfig.Database = databaseName
	pool, err := pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect to PostgreSQL test database: %w", err)
	}

	cleanup := func() error {
		pool.Close()
		if _, err := sharedPostgres.adminPool.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{databaseName}.Sanitize()+" WITH (FORCE)"); err != nil {
			return fmt.Errorf("drop PostgreSQL test database %s: %w", databaseName, err)
		}
		return nil
	}

	queries := database.New(pool)
	return pool, queries, cleanup, nil
}

func openAdminPool(ctx context.Context) error {
	adminConfig, err := pgxpool.ParseConfig(sharedPostgres.baseURL)
	if err != nil {
		return err
	}
	adminConfig.ConnConfig.Database = "postgres"
	sharedPostgres.adminPool, err = pgxpool.NewWithConfig(ctx, adminConfig)
	return err
}

// ShutdownTestDB closes this package process's server connection and terminates
// its fallback container. A suite-level server is owned by the testdb command.
func ShutdownTestDB() error {
	if sharedPostgres.adminPool != nil {
		sharedPostgres.adminPool.Close()
	}
	if sharedPostgres.container != nil {
		return testcontainers.TerminateContainer(sharedPostgres.container)
	}
	return nil
}

// SeedTestSession inserts a test session with realistic sector owners including an
// airborne controller (EKCH_K_DEP, 124.980) so PDC frequency lookup works correctly.
func SeedTestSession(t *testing.T, queries *database.Queries) int32 {
	return SeedTestSessionNamedWithSectors(t, queries, "LIVE", []database.InsertSectorOwnersParams{
		{
			Sector:     []string{"AA", "AD", "DEL", "GW", "SQ", "TE", "TW"},
			Position:   "118.105", // EKCH_A_TWR
			Identifier: "TE",
		},
		{
			Sector:     []string{"K_DEP"},
			Position:   "124.980", // EKCH_K_DEP (airborne)
			Identifier: "K_DEP",
		},
	})
}

// SeedTestSessionWithSectors inserts a test session with the provided sector owners.
func SeedTestSessionWithSectors(t *testing.T, queries *database.Queries, sectors []database.InsertSectorOwnersParams) int32 {
	return SeedTestSessionNamedWithSectors(t, queries, "LIVE", sectors)
}

// SeedTestSessionNamedWithSectors inserts a test session with a custom session name and provided sector owners.
func SeedTestSessionNamedWithSectors(t *testing.T, queries *database.Queries, name string, sectors []database.InsertSectorOwnersParams) int32 {
	ctx := context.Background()

	// First insert the airport (required by foreign key)
	err := queries.InsertAirport(ctx, "EKCH")
	if err != nil {
		t.Logf("Airport insert warning (may already exist): %v", err)
	}

	sessionID, err := queries.InsertSession(ctx, database.InsertSessionParams{
		Name:    name,
		Airport: "EKCH",
	})
	require.NoError(t, err)

	for i := range sectors {
		sectors[i].Session = sessionID
	}
	_, err = queries.InsertSectorOwners(ctx, sectors)
	require.NoError(t, err)

	return sessionID
}

// SeedTestStrip inserts a test strip
func SeedTestStrip(t *testing.T, queries *database.Queries, sessionID int32, callsign string) {
	SeedTestStripWithSquawks(t, queries, sessionID, callsign, ptr("2401"), ptr("2401"))
}

func SeedTestStripWithSquawks(t *testing.T, queries *database.Queries, sessionID int32, callsign string, squawk, assignedSquawk *string) {
	ctx := context.Background()

	err := queries.InsertStrip(ctx, database.InsertStripParams{
		Callsign:       callsign,
		Session:        sessionID,
		Origin:         "EKCH",
		Destination:    "ESSA",
		AircraftType:   ptr("A320"),
		Runway:         ptr("22L"),
		Sid:            ptr("VEMBO2E"),
		Squawk:         squawk,
		AssignedSquawk: assignedSquawk,
		Bay:            "NOT_CLEARED",
		CdmData:        []byte(`{"canonical":{}}`),
		NextOwners:     []byte(`[]`),
		PreviousOwners: []byte(`[]`),
	})
	require.NoError(t, err)
}

func ptr[T any](v T) *T { return &v }

// SeedTestStripWithoutRouting inserts a test strip that has a runway but neither a
// SID nor vectored departure info (heading + cleared altitude), mirroring a flight
// plan filed without a SID.
func SeedTestStripWithoutRouting(t *testing.T, queries *database.Queries, sessionID int32, callsign string) {
	ctx := context.Background()

	err := queries.InsertStrip(ctx, database.InsertStripParams{
		Callsign:       callsign,
		Session:        sessionID,
		Origin:         "EKCH",
		Destination:    "ESSA",
		AircraftType:   ptr("A320"),
		Runway:         ptr("22L"),
		Sid:            ptr(""),
		Squawk:         ptr("2401"),
		AssignedSquawk: ptr("2401"),
		Bay:            "NOT_CLEARED",
		CdmData:        []byte(`{"canonical":{}}`),
		NextOwners:     []byte(`[]`),
		PreviousOwners: []byte(`[]`),
	})
	require.NoError(t, err)
}

// SeedTestStripWithAircraftType inserts a test strip with a custom aircraft type
func SeedTestStripWithAircraftType(t *testing.T, queries *database.Queries, sessionID int32, callsign, aircraftType string) {
	ctx := context.Background()

	err := queries.InsertStrip(ctx, database.InsertStripParams{
		Callsign:       callsign,
		Session:        sessionID,
		Origin:         "EKCH",
		Destination:    "ESSA",
		AircraftType:   ptr(aircraftType),
		Runway:         ptr("22L"),
		Sid:            ptr("VEMBO2E"),
		Squawk:         ptr("2401"),
		AssignedSquawk: ptr("2401"),
		Bay:            "NOT_CLEARED",
		CdmData:        []byte(`{"canonical":{}}`),
		NextOwners:     []byte(`[]`),
		PreviousOwners: []byte(`[]`),
	})
	require.NoError(t, err)
}

// SeedClearedTestStrip inserts a test strip that has already been cleared (bay = CLEARED, cleared = true)
func SeedClearedTestStrip(t *testing.T, queries *database.Queries, sessionID int32, callsign string) {
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
		Bay:            "CLEARED",
		CdmData:        []byte(`{"canonical":{}}`),
		NextOwners:     []byte(`[]`),
		PreviousOwners: []byte(`[]`),
	})
	require.NoError(t, err)

	_, err = queries.UpdateStripClearedFlagByID(ctx, database.UpdateStripClearedFlagByIDParams{
		Cleared:  true,
		Bay:      "CLEARED",
		Callsign: callsign,
		Session:  sessionID,
		Version:  nil,
	})
	require.NoError(t, err)
}

// CleanupTestSession removes test session and all related data
func CleanupTestSession(t *testing.T, queries *database.Queries, sessionID int32) {
	ctx := context.Background()

	// Delete session (should cascade to strips)
	_, err := queries.DeleteSession(ctx, sessionID)
	require.NoError(t, err)
}
