package materializer

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/fixture"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"github.com/stretchr/testify/require"
)

func TestPostgresStartupRecoveryAndWarmRouteWithoutSource(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	data := completeEKCH()
	source := fixture.New(data)
	clock := data.Provenance.ImportedAt.Add(time.Hour)
	repo := postgres.NewNavigationCache(pool)
	m := newPostgresMaterializer(t, source, repo, data, &clock)
	m.deps.Runways = configFor(data) // official non-HTTP terminal configuration, not fixture geometry

	// Empty persistent cache starts from one complete atomic manifest.
	require.NoError(t, m.Refresh(ctx, Request{Airport: "EKCH"}))
	require.Equal(t, 6, source.Calls(), "activation uses only provider-neutral fixture acquisition calls; terminal runways make no source/HTTP/checkpoint call")
	active, err := repo.ActiveManifest(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, int64(1), active.Revision)
	beforeTerminalRead := source.Calls()
	path, err := m.GeometryReader().TerminalPath(ctx, "EKCH", "SOK", "SOUTH")
	require.NoError(t, err)
	require.NotEmpty(t, path.Legs)
	require.Equal(t, beforeTerminalRead, source.Calls(), "terminal GeometryReader read is cache-only and has no EuroScope/source fallback")

	procedure, runway, group := navdata.ProcedureID("SOK1P"), navdata.RunwayID("22L"), aman.RunwayGroupID("SOUTH")
	query := navdata.RouteQuery{Version: data.Version, Origin: "ENGM", Destination: "EKCH", FiledRoute: "DCT KEMAX", ArrivalProcedure: &procedure, Runway: &runway, RunwayGroup: &group}
	key, err := m.MaterializeRoute(ctx, query, "fixture-v1")
	require.NoError(t, err)

	// A fresh repository/materializer sees the durable active dataset while
	// its acquisition source is unavailable, and GeometryReader stays cache-only.
	freshRepo := postgres.NewNavigationCache(pool)
	fresh := newPostgresMaterializer(t, source, freshRepo, data, &clock)
	fresh.deps.Cycles = unavailableCycle{}
	require.Error(t, fresh.Refresh(ctx, Request{Airport: "EKCH"}))
	health := fresh.Health("EKCH")
	require.True(t, health.CacheReady)
	require.True(t, health.FragmentsValid)
	require.True(t, health.TerminalValid)
	require.Equal(t, ReasonSourceUnavailable, health.Reason)
	calls := source.Calls()
	_, err = fresh.GeometryReader().Route(ctx, key)
	require.NoError(t, err)
	require.Equal(t, calls, source.Calls(), "warm route restart reads without resolver/source calls")

	// An expired available cycle cannot replace the active manifest.
	expired := data.Version
	expired.EffectiveUntil = clock
	fresh.deps.Cycles = fixedCycle{version: expired}
	require.Error(t, fresh.Refresh(ctx, Request{Airport: "EKCH"}))
	stillActive, err := freshRepo.ActiveManifest(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, int64(1), stillActive.Revision)
	require.Equal(t, ReasonDatasetExpired, fresh.Health("EKCH").Reason)

	// A corrupt referenced fragment is never ready. A full new-revision source
	// refresh can recover atomically using the corrupt manifest's CAS revision.
	_, err = pool.Exec(ctx, `UPDATE aman_nav_airport_fragments SET payload='{}'::jsonb WHERE digest=$1`, active.Candidate.AirportDigest)
	require.NoError(t, err)
	_, err = freshRepo.ActiveManifest(ctx, "EKCH")
	require.Error(t, err)
	revised := data
	revised.Version.SourceRevision = "fixture-r2"
	revisedSource := fixture.New(revised)
	recovering := newPostgresMaterializer(t, revisedSource, freshRepo, revised, &clock)
	require.NoError(t, recovering.Refresh(ctx, Request{Airport: "EKCH"}))
	recovered, err := freshRepo.ActiveManifest(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, int64(2), recovered.Revision)
	require.Equal(t, revised.Version, recovered.Candidate.Version)
	require.True(t, recovering.Health("EKCH").CacheReady)
}

func TestConfiguredTerminalManifestWithoutDigestIsNeverReady(t *testing.T) {
	data := completeEKCH()
	cache := &memoryCache{hasActive: true, active: navdata.ActiveManifest{Candidate: navdata.ManifestCandidate{Airport: "EKCH", Version: data.Version}, Revision: 1}}
	clock := data.Provenance.ImportedAt.Add(time.Hour)
	m := newMaterializer(t, fixture.New(data), cache, configFor(data), &clock)
	m.deps.Cycles = unavailableCycle{}
	require.Error(t, m.Refresh(context.Background(), Request{Airport: "EKCH"}))
	health := m.Health("EKCH")
	require.False(t, health.CacheReady)
	require.False(t, health.TerminalValid)
	require.False(t, health.FragmentsValid)
}

func newPostgresMaterializer(t *testing.T, source *fixture.Source, cache interface {
	navdata.NavigationCandidateWriter
	navdata.NavigationManifestActivator
	navdata.ActiveManifestReader
	navdata.GeometryReader
}, data fixture.Dataset, clock *time.Time) *Materializer {
	t.Helper()
	m, err := New(Dependencies{Cycles: source, Airports: source, Runways: source, Procedures: source, Fixes: source, Routes: source, Cache: cache, Terminal: configFor(data), Now: func() time.Time { return *clock }})
	require.NoError(t, err)
	return m
}

type fixedCycle struct{ version navdata.DatasetVersion }

func (s fixedCycle) LatestVersion(context.Context) (navdata.DatasetVersion, error) {
	return s.version, nil
}
