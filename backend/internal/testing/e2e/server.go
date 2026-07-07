package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"time"

	"FlightStrips/internal/app"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/services"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestServer wraps the FlightStrips server for testing
type TestServer struct {
	Server            *http.Server
	App               *app.App
	DBPool            *pgxpool.Pool
	Queries           *database.Queries
	ServerAddr        string
	postgresContainer testcontainers.Container
	ctx               context.Context
	cancel            context.CancelFunc
}

// getMigrationsPath returns the absolute path to the migrations directory
func getMigrationsPath() string {
	// Get the path to this file
	_, filename, _, _ := runtime.Caller(0)
	// Navigate up to backend/internal/testing/e2e -> backend -> migrations
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "migrations")
}

// StartTestServer starts a test instance of the FlightStrips server
func StartTestServer() (*TestServer, error) {
	// Ensure TEST_MODE is enabled
	if !config.IsTestMode() {
		return nil, fmt.Errorf("TEST_MODE must be enabled for E2E tests")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start PostgreSQL container
	slog.Info("Starting PostgreSQL test container...")
	postgresContainer, err := postgrescontainer.Run(ctx,
		"postgres:16-alpine",
		postgrescontainer.WithDatabase("testdb"),
		postgrescontainer.WithUsername("postgres"),
		postgrescontainer.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start PostgreSQL container: %w", err)
	}

	// Get connection string
	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		testcontainers.TerminateContainer(postgresContainer)
		cancel()
		return nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	slog.Info("Running database migrations...")
	// Run migrations
	migrationsPath := getMigrationsPath()
	if err := database.Migrate(connStr, migrationsPath); err != nil {
		testcontainers.TerminateContainer(postgresContainer)
		cancel()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Connect to database
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		testcontainers.TerminateContainer(postgresContainer)
		cancel()
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	dbpool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		testcontainers.TerminateContainer(postgresContainer)
		cancel()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	application, err := app.Build(ctx, app.Config{
		Environment:    "test",
		CloseDBOnClose: true,
		EnablePDC:      false,
		EnableECFMP:    false,
		EnableECFMPAPI: false,
		EnablePilotAPI: false,
		EnableALB:      false,
		EnableMetar:    false,
		EnableVATSIM:   false,
		EnableTraffic:  false,
		EnableDBSeed:   false,
	}, app.Dependencies{
		DBPool:                dbpool,
		AuthenticationService: services.NewTestAuthenticationService(),
		TransceiversInterval:  30 * time.Second,
	})
	if err != nil {
		dbpool.Close()
		testcontainers.TerminateContainer(postgresContainer)
		cancel()
		return nil, fmt.Errorf("failed to build app: %w", err)
	}
	application.StartWorkers(ctx)

	// Bind on :0 so the OS assigns a free port, avoiding conflicts when tests run in parallel.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = application.Close(context.Background())
		cancel()
		return nil, fmt.Errorf("failed to bind listener: %w", err)
	}
	addr := listener.Addr().String()

	httpServer := &http.Server{
		Addr:    addr,
		Handler: application.Handler(),
	}

	// Start server in background
	serverErr := make(chan error, 1)

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Check if server started successfully (non-blocking — Serve returns immediately on error)
	select {
	case err := <-serverErr:
		_ = application.Close(context.Background())
		cancel()
		return nil, fmt.Errorf("server failed to start: %w", err)
	default:
		// Server started successfully
	}

	queries := database.New(dbpool)

	testServer := &TestServer{
		Server:            httpServer,
		App:               application,
		DBPool:            dbpool,
		Queries:           queries,
		ServerAddr:        addr,
		postgresContainer: postgresContainer,
		ctx:               ctx,
		cancel:            cancel,
	}

	slog.Info("Test server started", slog.String("addr", addr))

	return testServer, nil
}

// Stop shuts down the test server and cleans up resources
func (ts *TestServer) Stop() error {
	slog.Info("Stopping test server")

	// Cancel context to stop services
	ts.cancel()

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := ts.Server.Shutdown(ctx); err != nil {
		slog.Error("Failed to shutdown server gracefully", slog.Any("error", err))
	}

	if err := ts.App.Close(context.Background()); err != nil {
		slog.Error("Failed to close app", slog.Any("error", err))
	}

	// Terminate PostgreSQL container
	if err := testcontainers.TerminateContainer(ts.postgresContainer); err != nil {
		slog.Error("Failed to terminate PostgreSQL container", slog.Any("error", err))
		return err
	}

	slog.Info("Test server stopped and container terminated")
	return nil
}

// CleanupDatabase removes all test data from the database
func (ts *TestServer) CleanupDatabase() error {
	ctx := context.Background()

	// Delete in order to respect foreign key constraints
	// Note: Only delete from tables that exist

	if _, err := ts.DBPool.Exec(ctx, "DELETE FROM strips"); err != nil {
		return fmt.Errorf("failed to cleanup strips: %w", err)
	}

	if _, err := ts.DBPool.Exec(ctx, "DELETE FROM controllers"); err != nil {
		return fmt.Errorf("failed to cleanup controllers: %w", err)
	}

	if _, err := ts.DBPool.Exec(ctx, "DELETE FROM sessions"); err != nil {
		return fmt.Errorf("failed to cleanup sessions: %w", err)
	}

	if _, err := ts.DBPool.Exec(ctx, "DELETE FROM sector_owners"); err != nil {
		return fmt.Errorf("failed to cleanup sector_owners: %w", err)
	}

	if _, err := ts.DBPool.Exec(ctx, "DELETE FROM pdc_clearances"); err != nil {
		// Ignore error if table doesn't exist
		slog.Debug("Note: pdc_clearances cleanup skipped", slog.Any("error", err))
	}

	slog.Debug("Database cleaned up")
	return nil
}

// GetWebSocketURL returns the WebSocket URL for EuroScope connections
func (ts *TestServer) GetWebSocketURL() string {
	return fmt.Sprintf("ws://%s/euroscopeEvents", ts.ServerAddr)
}

// GetFrontendWebSocketURL returns the WebSocket URL for frontend connections
func (ts *TestServer) GetFrontendWebSocketURL() string {
	return fmt.Sprintf("ws://%s/frontEndEvents", ts.ServerAddr)
}
