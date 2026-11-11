package navigation

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/materializer"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/airacnet"
	"FlightStrips/internal/aman/navdata/fixture"
	"FlightStrips/internal/aman/navdata/sector"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/aman/trajectory"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"github.com/stretchr/testify/require"
)

const localTestSector = `; AIRAC 2608
[FIXES]
ENROUTE N056.00.00.000 E012.00.00.000
SOK N055.42.00.000 E012.12.00.000
KEMAX N055.48.00.000 E012.24.00.000
ROSBI N055.36.00.000 E012.42.00.000
[AIRPORT]
EKCH 000.000 N055.37.04.800 E012.39.21.600 D
`

func sectorTestConfig() terminal.Configuration {
	data := fixture.EKCH()
	course := 221.2
	p := data.Provenance
	provenance := terminal.ProvenanceDefinition{SourceID: p.SourceID, SourceRevision: p.SourceRevision, ImportedAt: p.ImportedAt, EffectiveFrom: p.EffectiveFrom, EffectiveUntil: p.EffectiveUntil}
	holdSeconds := int64(60)
	return terminal.Configuration{
		SchemaVersion: terminal.SchemaVersion, ConfigVersion: "sector-test", Airport: "EKCH",
		ApplicabilityFrom: data.Version.EffectiveFrom, ApplicabilityUntil: data.Version.EffectiveUntil,
		Dataset:      terminal.DatasetCompatibility{Cycle: data.Version.Cycle, EffectiveFrom: data.Version.EffectiveFrom, EffectiveUntil: data.Version.EffectiveUntil},
		Sources:      []terminal.Source{{ID: "fixture-aip", Document: "synthetic terminal", EffectiveFrom: data.Version.EffectiveFrom, EffectiveUntil: data.Version.EffectiveUntil}},
		RunwayGroups: []terminal.RunwayGroup{{ID: "SOUTH", Runways: []navdata.RunwayID{"22L"}, FinalApproaches: []terminal.FinalApproachDefinition{{Runway: "22L", FinalApproachFix: "KEMAX", Threshold: terminal.ThresholdDefinition{Position: terminal.CoordinateDefinition{LatitudeDeg: 55.6254111111, LongitudeDeg: 12.6675805556}, CourseTrueDeg: &course}, CourseTrueDeg: course, PhysicalLengthM: 3302, Provenance: provenance}}}},
		Feeders:      []terminal.Feeder{{ID: "SOK"}}, Paths: []terminal.Path{{Feeder: "SOK", RunwayGroup: "SOUTH", Fixes: []navdata.FixID{"SOK", "KEMAX"}, MergeFix: "KEMAX", SelectedHolding: "SOK-HF"}},
		OverlayHoldings: []terminal.HoldingDefinition{{ID: "SOK-HF", Fix: "SOK", InboundCourseTrueDeg: 220, TurnDirection: navdata.TurnRight, LegTimeSeconds: &holdSeconds, Termination: navdata.HoldingToFix, Provenance: provenance}},
	}
}

func TestSectorStartupRoutesAndRestartWithoutProviders(t *testing.T) {
	config := sectorTestConfig()
	now := fixture.EKCH().Provenance.ImportedAt.Add(time.Hour)
	path := filepath.Join(t.TempDir(), "synthetic.sct")
	require.NoError(t, os.WriteFile(path, []byte(localTestSector), 0600))
	for _, mode := range []string{SourceSector, SourceAIRACNet, SourceVIFF} {
		t.Run(mode, func(t *testing.T) {
			pool, _ := testdata.SetupTestDB(t)
			cache := postgres.NewNavigationCache(pool)
			var requests atomic.Int32
			cfg := Config{Source: mode, SectorFilePath: path, Now: func() time.Time { return now }, HTTPClient: &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
				requests.Add(1)
				return nil, fmt.Errorf("all online navigation providers are down")
			})}}
			importer, _, err := assembleMaterializer(cfg, config, cache, airacnet.NewMemoryCheckpoints())
			require.NoError(t, err)
			require.NoError(t, importer.Refresh(t.Context(), materializer.Request{Airport: "EKCH"}))
			require.True(t, importer.Health("EKCH").CacheReady)
			version, err := cache.ActiveVersion(t.Context(), "EKCH")
			require.NoError(t, err)
			require.Contains(t, version.SourceRevision, "sector-")
			entry, group := navdata.FixID("SOK"), aman.RunwayGroupID("SOUTH")
			q := navdata.RouteQuery{Version: version, Callsign: "SWEAT123", Origin: "ENGM", Destination: "EKCH", FiledRoute: "ENROUTE DCT SOK", TerminalEntry: &entry, RunwayGroup: &group}
			source := &Source{Materializer: importer, Geometry: cache, Terminal: config, routeSource: mode}
			key, err := source.MaterializeRoute(t.Context(), q, "test-route")
			require.NoError(t, err)
			g, err := cache.Route(t.Context(), key)
			require.NoError(t, err)
			require.Equal(t, "sector", g.Provenance.SourceID)
			full, err := trajectory.ReadFiledRoute(t.Context(), trajectory.Readers{Geometry: cache, Snapshot: cache}, "EKCH", key, "", "SOK", "SOUTH")
			require.NoError(t, err)
			require.Greater(t, len(full.Legs), len(g.Legs), "terminal continuation is appended exactly once")
			projected, err := trajectory.Project(t.Context(), trajectory.Readers{Geometry: cache, Snapshot: cache}, trajectory.Input{Airport: "EKCH", RouteKey: key, Feeder: "SOK", RunwayGroup: group, Observation: aman.SurveillanceFact{LatitudeDegrees: 55.9, LongitudeDegrees: 12.0667, ObservedAt: &now}}, trajectory.Config{ReferenceTime: now})
			require.NoError(t, err)
			require.NotNil(t, projected.DistanceToGoNM)
			require.Greater(t, *projected.DistanceToGoNM, 0.0)
			if mode == SourceSector {
				require.Zero(t, requests.Load())
			}
			// Restart against the durable manifest, without needing online data.
			fresh, _, err := assembleMaterializer(cfg, config, cache, airacnet.NewMemoryCheckpoints())
			require.NoError(t, err)
			require.NoError(t, fresh.Refresh(t.Context(), materializer.Request{Airport: "EKCH"}))
			recovered, err := cache.Route(t.Context(), key)
			require.NoError(t, err)
			require.Equal(t, g.Digest, recovered.Digest)
			_, err = fresh.MaterializeRoute(t.Context(), q, "restart")
			require.NoError(t, err)
			// Replacing a local file must activate a new local revision rather
			// than silently retaining geometry from its previous fingerprint.
			require.NoError(t, os.WriteFile(path, []byte(localTestSector+"; local revision updated\n"), 0600))
			updated, _, err := assembleMaterializer(cfg, config, cache, airacnet.NewMemoryCheckpoints())
			require.NoError(t, err)
			require.NoError(t, updated.Refresh(t.Context(), materializer.Request{Airport: "EKCH"}))
			if mode != SourceVIFF {
				newVersion, err := cache.ActiveVersion(t.Context(), "EKCH")
				require.NoError(t, err)
				require.NotEqual(t, version.SourceRevision, newVersion.SourceRevision)
			}
			require.NoError(t, os.WriteFile(path, []byte(localTestSector), 0600))
		})
	}
}

type resolverFunc func(context.Context, navdata.RouteQuery) (navdata.RouteGeometry, error)

func (f resolverFunc) Resolve(ctx context.Context, q navdata.RouteQuery) (navdata.RouteGeometry, error) {
	return f(ctx, q)
}

func TestSectorFallbackPreservesOnlineManifestAndPrimaryPreference(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	cache := postgres.NewNavigationCache(pool)
	config, data := sectorTestConfig(), fixture.EKCH()
	now := func() time.Time { return data.Provenance.ImportedAt.Add(time.Hour) }
	path := filepath.Join(t.TempDir(), "synthetic.sct")
	require.NoError(t, os.WriteFile(path, []byte(localTestSector), 0600))
	local, err := sector.Load(path, config, now)
	require.NoError(t, err)
	primarySource := fixture.New(data)
	primary, err := materializer.New(materializer.Dependencies{Cycles: primarySource, Airports: primarySource, Runways: config, Procedures: primarySource, Fixes: primarySource, Routes: primarySource, Terminal: config, Cache: cache, Now: now})
	require.NoError(t, err)
	require.NoError(t, primary.Refresh(t.Context(), materializer.Request{Airport: "EKCH"}))
	entry := navdata.FixID("SOK")
	q := navdata.RouteQuery{Version: data.Version, Origin: "ENGM", Destination: "EKCH", FiledRoute: "ENROUTE DCT SOK", TerminalEntry: &entry}
	var down bool
	primaryRoute, err := local.Resolve(t.Context(), q)
	require.NoError(t, err)
	primaryRoute.Provenance.SourceID = "online-test"
	routes := &fallbackRoutes{local: local, references: cache, primary: resolverFunc(func(_ context.Context, _ navdata.RouteQuery) (navdata.RouteGeometry, error) {
		if down {
			return navdata.RouteGeometry{}, fmt.Errorf("provider down")
		}
		return primaryRoute, nil
	})}
	g, err := routes.Resolve(t.Context(), q)
	require.NoError(t, err)
	require.Equal(t, "online-test", g.Provenance.SourceID)
	down = true
	g, err = routes.Resolve(t.Context(), q)
	require.NoError(t, err)
	require.Equal(t, "sector", g.Provenance.SourceID)
	version, err := cache.ActiveVersion(t.Context(), "EKCH")
	require.NoError(t, err)
	require.Equal(t, data.Version, version, "a per-flight fallback must not replace the online terminal manifest")
	down = false
	g, err = routes.Resolve(t.Context(), q)
	require.NoError(t, err)
	require.Equal(t, "online-test", g.Provenance.SourceID)
}

func TestSectorFallbackRetainsOnlineCatalogAndRejectsJoinMismatch(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	cache := postgres.NewNavigationCache(pool)
	config, data := sectorTestConfig(), fixture.EKCH()
	now := func() time.Time { return data.Provenance.ImportedAt.Add(time.Hour) }
	path := filepath.Join(t.TempDir(), "synthetic.sct")
	require.NoError(t, os.WriteFile(path, []byte(localTestSector), 0600))
	primary := fixture.New(data)
	seed, err := materializer.New(materializer.Dependencies{Cycles: primary, Airports: primary, Runways: config, Procedures: primary, Fixes: primary, Routes: primary, Cache: cache, Terminal: config, Now: now})
	require.NoError(t, err)
	require.NoError(t, seed.Refresh(t.Context(), materializer.Request{Airport: "EKCH"}))
	cfg := Config{Source: SourceAIRACNet, SectorFilePath: path, Now: now, HTTPClient: &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("provider down") })}}
	importer, _, err := assembleMaterializer(cfg, config, cache, airacnet.NewMemoryCheckpoints())
	require.NoError(t, err)
	require.NoError(t, importer.Refresh(t.Context(), materializer.Request{Airport: "EKCH"}))
	version, err := cache.ActiveVersion(t.Context(), "EKCH")
	require.NoError(t, err)
	require.Equal(t, data.Version, version)
	entry := navdata.FixID("SOK")
	q := navdata.RouteQuery{Version: version, Origin: "ENGM", Destination: "EKCH", FiledRoute: "ENROUTE DCT SOK", TerminalEntry: &entry}
	_, err = importer.MaterializeRoute(t.Context(), q, "fallback-test")
	require.NoError(t, err)
	// A matching name cannot join a terminal at different cached coordinates.
	moved := strings.Replace(localTestSector, "SOK N055.42.00.000 E012.12.00.000", "SOK N054.42.00.000 E012.12.00.000", 1)
	require.NoError(t, os.WriteFile(path, []byte(moved), 0600))
	local, err := sector.Load(path, config, now)
	require.NoError(t, err)
	routes := &fallbackRoutes{local: local, references: cache, primary: resolverFunc(func(context.Context, navdata.RouteQuery) (navdata.RouteGeometry, error) {
		return navdata.RouteGeometry{}, fmt.Errorf("provider down")
	})}
	_, err = routes.Resolve(t.Context(), q)
	require.ErrorContains(t, err, "join does not match")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = routes.Resolve(ctx, q)
	require.ErrorIs(t, err, context.Canceled)
}

func TestLocalSectorColdDatabase(t *testing.T) {
	path := os.Getenv("FLIGHTSTRIPS_TEST_SECTOR_FILE")
	if path == "" {
		t.Skip("set FLIGHTSTRIPS_TEST_SECTOR_FILE to check a local SCT")
	}
	config, err := terminal.LoadFile("../../config/aman/ekch-terminal-2611.json")
	require.NoError(t, err)
	pool, _ := testdata.SetupTestDB(t)
	cache := postgres.NewNavigationCache(pool)
	cfg := Config{Source: SourceSector, SectorFilePath: path, Now: func() time.Time { return config.Dataset.EffectiveFrom.Add(time.Hour) }}
	importer, _, err := assembleMaterializer(cfg, config, cache, nil)
	require.NoError(t, err)
	require.NoError(t, importer.Refresh(t.Context(), materializer.Request{Airport: "EKCH"}))
	version, err := cache.ActiveVersion(t.Context(), "EKCH")
	require.NoError(t, err)
	entry, group := navdata.FixID("ERNOV"), aman.RunwayGroupID("ARRIVAL-22L")
	q := navdata.RouteQuery{Version: version, Callsign: "SWEAT123", Origin: "ESSA", Destination: "EKCH", FiledRoute: "TESPI DCT ERNOV", TerminalEntry: &entry, RunwayGroup: &group}
	key, err := importer.MaterializeRoute(t.Context(), q, sector.ResolverVersion)
	require.NoError(t, err)
	full, err := trajectory.ReadFiledRoute(t.Context(), trajectory.Readers{Geometry: cache, Snapshot: cache}, "EKCH", key, "ERNOV", "ERNOV", group)
	require.NoError(t, err)
	require.Greater(t, len(full.Legs), 1)
	require.Empty(t, full.Reasons)
}
