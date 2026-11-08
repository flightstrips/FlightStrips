package navigation

import (
	"context"
	"fmt"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/materializer"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/airacnet"
	"FlightStrips/internal/aman/navdata/viff"
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
	// Arrivals is the shared bulk response used by CDM's existing inbound poll.
	Arrivals interface {
		Arrivals(context.Context, string) ([]byte, error)
	}
	routeSource string
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
	cache := postgres.NewNavigationCache(pool)
	importer, arrivals, err := assembleMaterializer(config, terminalConfig, cache, airacnet.NewPostgresCheckpoints(pool))
	if err != nil {
		return nil, fmt.Errorf("initialize navigation materializer: %w", err)
	}
	result := &Source{Materializer: importer, Geometry: cache, Terminal: terminalConfig, routeSource: config.Source}
	if arrivals != nil {
		result.Arrivals = arrivals
	}
	return result, nil
}

type materializerCache interface {
	navdata.NavigationCandidateWriter
	navdata.NavigationManifestActivator
	navdata.ActiveManifestReader
	navdata.GeometryReader
	navdata.GeometrySnapshotReader
}

func assembleMaterializer(config Config, terminalConfig terminal.Configuration, cache materializerCache, checkpoints airacnet.CheckpointStore) (*materializer.Materializer, *viff.Adapter, error) {
	deps := materializer.Dependencies{Cache: cache, Terminal: terminalConfig, Now: config.Now}
	var arrivals *viff.Adapter
	if config.Source == SourceVIFF {
		resolver, err := viff.New(viff.Config{BaseURL: config.VIFFBaseURL, APIKey: config.VIFFAPIKey, HTTPClient: config.HTTPClient, References: cache, Now: config.Now})
		if err != nil {
			return nil, nil, err
		}
		deps.Routes, deps.CachedReferences = resolver, cache
		arrivals = resolver
	} else {
		source, err := airacnet.New(airacnet.Config{Now: config.Now, HTTPClient: config.HTTPClient, Checkpoints: checkpoints})
		if err != nil {
			return nil, nil, err
		}
		deps.Cycles, deps.Airports, deps.Procedures, deps.Fixes, deps.Routes, deps.Runways = source, source, source, source, source, terminalConfig
	}
	importer, err := materializer.New(deps)
	return importer, arrivals, err
}

func (*Source) Name() string { return "navigation source" }

// MaterializeRoute delegates on-demand route imports to the configured source.
// AMAN consumes this provider boundary rather than owning a second importer.
func (s *Source) MaterializeRoute(ctx context.Context, query navdata.RouteQuery, resolverVersion string) (navdata.RouteKey, error) {
	if s == nil || s.Materializer == nil {
		return "", fmt.Errorf("navigation source is unavailable")
	}
	if s.routeSource == SourceVIFF {
		resolverVersion = viff.ResolverVersion
	}
	return s.Materializer.MaterializeRoute(ctx, query, resolverVersion)
}

// RouteRefreshInterval lets flight-based routes pick up later vIFF processing
// without issuing a network request per flight or on every prediction tick.
func (s *Source) RouteRefreshInterval() time.Duration {
	if s.routeSource == SourceVIFF {
		return viff.RefreshInterval
	}
	return 0
}

// Start refreshes the configured airport immediately and then periodically so
// non-AMAN clients can read an active geometry snapshot from the cache.
func (s *Source) Start(ctx context.Context) {
	if s == nil || s.Materializer == nil || s.Terminal.Airport == "" {
		return
	}
	s.Materializer.Run(ctx, refreshInterval, []navdata.AirportID{s.Terminal.Airport})
}
