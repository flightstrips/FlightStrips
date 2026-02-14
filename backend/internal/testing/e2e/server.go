package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"time"

	"FlightStrips/internal/cdm"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/euroscope"
	"FlightStrips/internal/frontend"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/server"
	"FlightStrips/internal/services"
	"FlightStrips/internal/websocket"
	pkgEuroscope "FlightStrips/pkg/events/euroscope"
	pkgFrontend "FlightStrips/pkg/events/frontend"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestServer wraps the FlightStrips server for testing
type TestServer struct {
	Server            *http.Server
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

	// Initialize services
	authService := services.NewTestAuthenticationService()

	// Initialize repositories
	stripRepo := postgres.NewStripRepository(dbpool)
	controllerRepo := postgres.NewControllerRepository(dbpool)
	sessionRepo := postgres.NewSessionRepository(dbpool)
	sectorRepo := postgres.NewSectorOwnerRepository(dbpool)
	coordRepo := postgres.NewCoordinationRepository(dbpool)

	// Initialize services
	stripService := services.NewStripService(stripRepo)
	cdmClient := cdm.NewClient(cdm.WithAPIKey(""))
	cdmService := cdm.NewCdmService(cdmClient, stripRepo, sessionRepo)

	// Initialize hubs
	frontendHub := frontend.NewHub(stripService)
	euroscopeHub := euroscope.NewHub(stripService)

	stripService.SetFrontendHub(frontendHub)
	stripService.SetEuroscopeHub(euroscopeHub)
	cdmService.SetFrontendHub(frontendHub)

	// Initialize server
	fsServer := server.NewServer(dbpool, euroscopeHub, frontendHub, cdmService, nil, stripRepo, controllerRepo, sessionRepo, sectorRepo, coordRepo)

	frontendHub.SetServer(fsServer)
	euroscopeHub.SetServer(fsServer)

	// Create WebSocket upgraders
	frontendUpgrader := websocket.NewConnectionUpgrader[pkgFrontend.EventType, *frontend.Client](frontendHub, authService)
	euroscopeUpgrader := websocket.NewConnectionUpgrader[pkgEuroscope.EventType, *euroscope.Client](euroscopeHub, authService)

	// Start services
	go cdmService.Start(ctx)

	// Setup HTTP handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/euroscopeEvents", euroscopeUpgrader.Upgrade)
	mux.HandleFunc("/frontEndEvents", frontendUpgrader.Upgrade)

	// Use fixed port 2994 for testing (same as dev server)
	addr := "127.0.0.1:2994"
	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start server in background
	serverErr := make(chan error, 1)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// Check if server started successfully
	select {
	case err := <-serverErr:
		dbpool.Close()
		cancel()
		return nil, fmt.Errorf("server failed to start: %w", err)
	default:
		// Server started successfully
	}

	queries := database.New(dbpool)

	testServer := &TestServer{
		Server:            httpServer,
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

	// Close database pool
	ts.DBPool.Close()

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
