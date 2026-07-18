package postgres

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/fixture"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNavigationCachePersistsCompleteManifestAndWarmRoute(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	data := fixture.EKCH()
	repo := NewNavigationCache(pool)
	manifest, route := writeNavigationFixture(t, ctx, repo, data)

	revision, err := repo.ActivateManifest(ctx, manifest)
	require.NoError(t, err)
	require.EqualValues(t, 1, revision)
	version, err := repo.ActiveVersion(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, data.Version, version)
	path, err := repo.TerminalPath(ctx, "EKCH", "SOK", "SOUTH")
	require.NoError(t, err)
	require.Equal(t, data.TerminalPaths[0], path)

	// A reconstructed component reads the same canonical bytes without the
	// fixture source being involved, proving the warm cache survives restart.
	key, err := repo.PutRoute(ctx, route)
	require.NoError(t, err)
	warm, err := NewNavigationCache(pool).Route(ctx, key)
	require.NoError(t, err)
	require.Equal(t, route.Geometry, warm)
}

func TestNavigationCachePromotesCandidateFragmentWithoutChangingRevision(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	data := fixture.EKCH()
	repo := NewNavigationCache(pool)
	fragment := newAirportFragment(t, data)
	fragment.State = navdata.ValidationCandidate
	fragment.ValidatedAt = nil
	digest, err := repo.PutAirportFragment(ctx, fragment)
	require.NoError(t, err)
	validated := newAirportFragment(t, data)
	validated.Digest = digest
	_, err = repo.PutAirportFragment(ctx, validated)
	require.NoError(t, err)
	var state string
	require.NoError(t, pool.QueryRow(ctx, `SELECT validation_state FROM aman_nav_airport_fragments WHERE digest=$1`, digest).Scan(&state))
	require.Equal(t, string(navdata.ValidationValidated), state)
}

func TestNavigationCachePartialProcedureRefreshActivatesAtomicallyAndRetainsPrevious(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	data := fixture.EKCH()
	repo := NewNavigationCache(pool)
	first, _ := writeNavigationFixture(t, ctx, repo, data)
	_, err := repo.ActivateManifest(ctx, first)
	require.NoError(t, err)

	oldStarDigest := firstProcedureDigest(t, data, navdata.ProcedureSTAR)
	star := procedureByKind(data, navdata.ProcedureSTAR)
	star.Procedures[0].Legs[0].ID = "STAR-HOLD-R2"
	updated := newProcedureFragment(t, data, navdata.ProcedureSTAR, star.Procedures)
	updatedDigest, err := repo.PutProcedureFragment(ctx, updated)
	require.NoError(t, err)
	second := first
	second.ExpectedRevision = 1
	second.ProcedureDigests = replaceDigest(second.ProcedureDigests, oldStarDigest, updatedDigest)
	_, err = repo.ActivateManifest(ctx, second)
	require.NoError(t, err)

	var activeAirport, activeFix string
	require.NoError(t, pool.QueryRow(ctx, `SELECT m.airport_digest,m.fix_digest FROM aman_nav_active_manifests a JOIN aman_nav_manifests m ON m.manifest_id=a.manifest_id WHERE a.airport='EKCH'`).Scan(&activeAirport, &activeFix))
	require.Equal(t, first.AirportDigest, activeAirport)
	require.Equal(t, first.FixDigest, activeFix)
	require.Contains(t, second.ProcedureDigests, updatedDigest)

	// A third activation followed by retention preserves the active and
	// immediately previous complete manifests, including their fragments.
	third := second
	third.ExpectedRevision = 2
	_, err = repo.ActivateManifest(ctx, third)
	require.NoError(t, err)
	require.NoError(t, repo.PruneNavigationCache(ctx, "EKCH"))
	var manifests int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM aman_nav_manifests WHERE airport='EKCH'`).Scan(&manifests))
	require.Equal(t, 2, manifests)
	_, err = repo.ActiveVersion(ctx, "EKCH")
	require.NoError(t, err)
}

func TestNavigationCacheRejectsCorruptCandidateWithoutReplacingActiveManifest(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	data := fixture.EKCH()
	repo := NewNavigationCache(pool)
	manifest, route := writeNavigationFixture(t, ctx, repo, data)
	_, err := repo.ActivateManifest(ctx, manifest)
	require.NoError(t, err)

	bad := manifest
	bad.ExpectedRevision = 1
	bad.FixDigest = "missing-fragment"
	_, err = repo.ActivateManifest(ctx, bad)
	requireDomainErrorClass(t, err, aman.ErrorCorruptData)
	version, err := repo.ActiveVersion(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, data.Version, version)

	key, err := repo.PutRoute(ctx, route)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE aman_nav_route_cache SET payload='{}'::jsonb WHERE cache_key=$1`, key)
	require.NoError(t, err)
	_, err = repo.Route(ctx, key)
	requireDomainErrorClass(t, err, aman.ErrorCorruptData)
	_, err = repo.ActiveVersion(ctx, "EKCH")
	require.NoError(t, err, "a corrupt route must not damage the manifest")
}

func TestNavigationCacheCompareAndSwapAndRoutePutRaces(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	data := fixture.EKCH()
	repo := NewNavigationCache(pool)
	manifest, route := writeNavigationFixture(t, ctx, repo, data)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() { defer wait.Done(); <-start; _, err := repo.ActivateManifest(ctx, manifest); errs <- err }()
	}
	close(start)
	wait.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
			continue
		}
		var domain *aman.DomainError
		require.True(t, errors.As(err, &domain))
		require.Equal(t, aman.ErrorRevisionConflict, domain.Class)
		conflict++
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)

	routeStart := make(chan struct{})
	routeErrors := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-routeStart
			_, err := NewNavigationCache(pool).PutRoute(ctx, route)
			routeErrors <- err
		}()
	}
	close(routeStart)
	wait.Wait()
	close(routeErrors)
	for err := range routeErrors {
		require.NoError(t, err)
	}
}

func TestNavigationCacheCanonicalSchemaContainsNoHTTPMetadata(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "migrations", "0036-add-aman-navigation-cache.sql"))
	require.NoError(t, err)
	lower := string(migration)
	for _, forbidden := range []string{"etag", "last-modified", "cache-control", "cursor", "retry", "http", "url"} {
		require.NotContains(t, lower, forbidden)
	}
	repositorySource, err := os.ReadFile(filepath.Join(filepath.Dir(sourceFile), "navigation_cache.go"))
	require.NoError(t, err)
	for _, forbidden := range []string{"net/http", "airac", "fixture", "RouteResolver", "CycleSource"} {
		require.NotContains(t, string(repositorySource), forbidden)
	}
}

func TestNavigationRoutePersistenceKeyIncludesSemanticResolverAndSchemaVersions(t *testing.T) {
	data := fixture.EKCH()
	var geometry navdata.RouteGeometry
	for _, route := range data.Routes {
		geometry = route
	}
	base := navdata.RouteCandidate{Query: fixtureRouteQuery(data), Geometry: geometry, CreatedAt: data.Provenance.ImportedAt, ResolverVersion: "resolver-v1", SchemaVersion: navdata.CanonicalSchemaVersion}
	first, err := base.PersistenceKey()
	require.NoError(t, err)
	resolverChanged := base
	resolverChanged.ResolverVersion = "resolver-v2"
	second, err := resolverChanged.PersistenceKey()
	require.NoError(t, err)
	schemaChanged := base
	schemaChanged.SchemaVersion = "navdata/v2"
	third, err := schemaChanged.PersistenceKey()
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	require.NotEqual(t, first, third)
}

func writeNavigationFixture(t *testing.T, ctx context.Context, repo *navigationCache, data fixture.Dataset) (navdata.ManifestCandidate, navdata.RouteCandidate) {
	t.Helper()
	airport := newAirportFragment(t, data)
	airportDigest, err := repo.PutAirportFragment(ctx, airport)
	require.NoError(t, err)
	fixes := newFixFragment(t, data)
	fixDigest, err := repo.PutFixFragment(ctx, fixes)
	require.NoError(t, err)
	digests := make([]string, 0, 3)
	for _, kind := range []navdata.ProcedureKind{navdata.ProcedureSID, navdata.ProcedureSTAR, navdata.ProcedureApproach} {
		fragment := procedureByKind(data, kind)
		candidate := newProcedureFragment(t, data, kind, fragment.Procedures)
		digest, err := repo.PutProcedureFragment(ctx, candidate)
		require.NoError(t, err)
		digests = append(digests, digest)
	}
	terminal := newTerminalFragment(t, data)
	terminalDigest, err := repo.PutTerminalFragment(ctx, terminal)
	require.NoError(t, err)
	var geometry navdata.RouteGeometry
	for _, value := range data.Routes {
		geometry = value
	}
	query := fixtureRouteQuery(data)
	return navdata.ManifestCandidate{Airport: "EKCH", Version: data.Version, AirportDigest: airportDigest, ProcedureDigests: digests, FixDigest: fixDigest, TerminalDigest: terminalDigest}, navdata.RouteCandidate{Query: query, ResolverVersion: "fixture-resolver-v1", SchemaVersion: navdata.CanonicalSchemaVersion, Geometry: geometry, CreatedAt: data.Provenance.ImportedAt.Add(time.Minute)}
}

func newAirportFragment(t *testing.T, data fixture.Dataset) navdata.CandidateAirportFragment {
	t.Helper()
	validated := data.Provenance.ImportedAt.Add(time.Minute)
	airport := data.Airports["EKCH"]
	runways := []navdata.Runway{{ID: "22L", Airport: "EKCH", Threshold: navdata.Threshold{Position: airport.Position}, LengthNM: 2, Provenance: data.Provenance}}
	digest, err := navdata.CanonicalFragmentDigest(navdata.CanonicalSchemaVersion, data.Version, data.Provenance, struct {
		Airport navdata.Airport
		Runways []navdata.Runway
	}{airport, runways})
	require.NoError(t, err)
	return navdata.CandidateAirportFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: data.Version, Airport: airport, Runways: runways, Provenance: data.Provenance, ImportedAt: data.Provenance.ImportedAt, ValidatedAt: &validated, State: navdata.ValidationValidated, Digest: digest}
}
func newFixFragment(t *testing.T, data fixture.Dataset) navdata.CandidateFixFragment {
	t.Helper()
	validated := data.Provenance.ImportedAt.Add(time.Minute)
	fixes := make([]navdata.Fix, 0, len(data.Fixes))
	for _, fix := range data.Fixes {
		fixes = append(fixes, fix)
	} // fixture map order is normalized below
	for i := range fixes {
		for j := i + 1; j < len(fixes); j++ {
			if fixes[j].ID < fixes[i].ID {
				fixes[i], fixes[j] = fixes[j], fixes[i]
			}
		}
	}
	digest, err := navdata.CanonicalFragmentDigest(navdata.CanonicalSchemaVersion, data.Version, data.Provenance, struct{ Fixes []navdata.Fix }{fixes})
	require.NoError(t, err)
	return navdata.CandidateFixFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: data.Version, Fixes: fixes, Provenance: data.Provenance, ImportedAt: data.Provenance.ImportedAt, ValidatedAt: &validated, State: navdata.ValidationValidated, Digest: digest}
}
func procedureByKind(data fixture.Dataset, kind navdata.ProcedureKind) navdata.CandidateProcedureFragment {
	values := []navdata.Procedure{}
	for _, value := range data.Procedures {
		if value.Kind == kind {
			values = append(values, value)
		}
	}
	return navdata.CandidateProcedureFragment{Airport: "EKCH", Kind: kind, Procedures: values}
}
func newProcedureFragment(t *testing.T, data fixture.Dataset, kind navdata.ProcedureKind, procedures []navdata.Procedure) navdata.CandidateProcedureFragment {
	t.Helper()
	validated := data.Provenance.ImportedAt.Add(time.Minute)
	digest, err := navdata.CanonicalFragmentDigest(navdata.CanonicalSchemaVersion, data.Version, data.Provenance, struct {
		Airport    navdata.AirportID
		Kind       navdata.ProcedureKind
		Procedures []navdata.Procedure
	}{"EKCH", kind, procedures})
	require.NoError(t, err)
	return navdata.CandidateProcedureFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: data.Version, Airport: "EKCH", Kind: kind, Procedures: procedures, Provenance: data.Provenance, ImportedAt: data.Provenance.ImportedAt, ValidatedAt: &validated, State: navdata.ValidationValidated, Digest: digest}
}
func firstProcedureDigest(t *testing.T, data fixture.Dataset, kind navdata.ProcedureKind) string {
	return newProcedureFragment(t, data, kind, procedureByKind(data, kind).Procedures).Digest
}
func newTerminalFragment(t *testing.T, data fixture.Dataset) navdata.CandidateTerminalFragment {
	t.Helper()
	validated := data.Provenance.ImportedAt.Add(time.Minute)
	digest, err := navdata.CanonicalFragmentDigest(navdata.CanonicalSchemaVersion, data.Version, data.Provenance, struct {
		Airport       navdata.AirportID
		ConfigVersion string
		Paths         []navdata.TerminalPath
	}{"EKCH", "fixture-config-v1", data.TerminalPaths})
	require.NoError(t, err)
	return navdata.CandidateTerminalFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: data.Version, Airport: "EKCH", ConfigVersion: "fixture-config-v1", Paths: data.TerminalPaths, Provenance: data.Provenance, ImportedAt: data.Provenance.ImportedAt, ValidatedAt: &validated, State: navdata.ValidationValidated, Digest: digest}
}
func replaceDigest(values []string, old, replacement string) []string {
	result := append([]string(nil), values...)
	for i, value := range result {
		if value == old {
			result[i] = replacement
		}
	}
	return result
}
func fixtureRouteQuery(data fixture.Dataset) navdata.RouteQuery {
	procedure := navdata.ProcedureID("SOK1P")
	runway := navdata.RunwayID("22L")
	group := aman.RunwayGroupID("SOUTH")
	return navdata.RouteQuery{Version: data.Version, Origin: "ENGM", Destination: "EKCH", FiledRoute: "DCT KEMAX", ArrivalProcedure: &procedure, Runway: &runway, RunwayGroup: &group}
}
