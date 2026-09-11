package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"FlightStrips/internal/app"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/services"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestServer wraps the FlightStrips server for testing
type TestServer struct {
	Server          *http.Server
	App             *app.App
	DBPool          *pgxpool.Pool
	Queries         *database.Queries
	ServerAddr      string
	databaseCleanup func() error
	ctx             context.Context
	cancel          context.CancelFunc
}

// StartTestServer starts a test instance of the FlightStrips server
func StartTestServer() (*TestServer, error) {
	// Ensure TEST_MODE is enabled
	if !config.IsTestMode() {
		return nil, fmt.Errorf("TEST_MODE must be enabled for E2E tests")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Allocate an isolated database on the suite's shared PostgreSQL server.
	slog.Info("Creating PostgreSQL test database...")
	dbpool, _, databaseCleanup, err := testdata.OpenTestDB(ctx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create PostgreSQL test database: %w", err)
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
		_ = databaseCleanup()
		cancel()
		return nil, fmt.Errorf("failed to build app: %w", err)
	}
	application.StartWorkers(ctx)

	// Bind on :0 so the OS assigns a free port, avoiding conflicts when tests run in parallel.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = application.Close(context.Background())
		_ = databaseCleanup()
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
		_ = databaseCleanup()
		cancel()
		return nil, fmt.Errorf("server failed to start: %w", err)
	default:
		// Server started successfully
	}

	queries := database.New(dbpool)

	testServer := &TestServer{
		Server:          httpServer,
		App:             application,
		DBPool:          dbpool,
		Queries:         queries,
		ServerAddr:      addr,
		databaseCleanup: databaseCleanup,
		ctx:             ctx,
		cancel:          cancel,
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

	// Drop the isolated database. The suite-level server is stopped by testdb.
	if err := ts.databaseCleanup(); err != nil {
		slog.Error("Failed to drop PostgreSQL test database", slog.Any("error", err))
		return err
	}

	slog.Info("Test server stopped and database dropped")
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
