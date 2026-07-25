package navigation

import (
	"context"
	"fmt"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/materializer"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/airacnet"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/repository/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

const refreshInterval = 6 * time.Hour

// GeometryCache is the cache-only view shared by AMAN and flight-facing
// consumers. It never exposes provider acquisition methods.
type GeometryCache interface {
	aman.Component
	navdata.GeometryReader
	navdata.GeometrySnapshotReader
}

// Source is the assembled provider, canonical cache, and terminal data for a
// single airport. AMAN may consume it, but does not own its lifecycle.
type Source struct {
	Materializer *materializer.Materializer
	Geometry     GeometryCache
	Terminal     terminal.Configuration
}

func Assemble(config Config, pool *pgxpool.Pool) (*Source, error) {
	config = config.Normalize()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.Enabled() {
		return nil, nil
	}
	terminalConfig, err := terminal.LoadFile(config.TerminalGeometryPath)
	if err != nil {
		return nil, fmt.Errorf("load navigation terminal configuration: %w", err)
	}
	source, err := airacnet.New(airacnet.Config{Checkpoints: airacnet.NewPostgresCheckpoints(pool)})
	if err != nil {
		return nil, fmt.Errorf("initialize AIRAC.NET navigation source: %w", err)
	}
	cache := postgres.NewNavigationCache(pool)
	importer, err := materializer.New(materializer.Dependencies{
		Cycles: source, Airports: source, Runways: terminalConfig, Procedures: source, Fixes: source, Routes: source,
		Cache: cache, Terminal: terminalConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize navigation materializer: %w", err)
	}
	return &Source{Materializer: importer, Geometry: cache, Terminal: terminalConfig}, nil
}

func (*Source) Name() string { return "navigation source" }

// MaterializeRoute delegates on-demand route imports to the configured source.
// AMAN consumes this provider boundary rather than owning a second importer.
func (s *Source) MaterializeRoute(ctx context.Context, query navdata.RouteQuery, resolverVersion string) (navdata.RouteKey, error) {
	if s == nil || s.Materializer == nil {
		return "", fmt.Errorf("navigation source is unavailable")
	}
	return s.Materializer.MaterializeRoute(ctx, query, resolverVersion)
}

// Start refreshes the configured airport immediately and then periodically so
// non-AMAN clients can read an active geometry snapshot from the cache.
func (s *Source) Start(ctx context.Context) {
	if s == nil || s.Materializer == nil || s.Terminal.Airport == "" {
		return
	}
	s.Materializer.Run(ctx, refreshInterval, []navdata.AirportID{s.Terminal.Airport})
}
