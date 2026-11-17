package postgres

import (
	"context"
	"testing"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/fixture"
	"FlightStrips/internal/pdc/testdata"
	"github.com/stretchr/testify/require"
)

func TestGeometryReadScopePinsRevisionUntilLocalActivation(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	repo := NewNavigationCache(pool)
	ctx := t.Context()
	manifest, candidate := writeNavigationFixture(t, ctx, repo, fixture.EKCH())
	_, err := repo.ActivateManifest(ctx, manifest)
	require.NoError(t, err)
	key, err := repo.PutRoute(ctx, candidate)
	require.NoError(t, err)
	scoped, err := repo.WithGeometryReadScope(ctx, "EKCH", []navdata.RouteKey{key, key})
	require.NoError(t, err)
	first, err := repo.ActiveGeometrySnapshot(scoped, "EKCH")
	require.NoError(t, err)
	manifest.ExpectedRevision = 1
	_, err = repo.ActivateManifest(ctx, manifest)
	require.NoError(t, err)
	pinned, err := repo.ActiveGeometrySnapshot(context.WithoutCancel(scoped), "EKCH")
	require.NoError(t, err)
	require.EqualValues(t, 1, pinned.ManifestRevision, "external activation cannot mix an event's revisions")
	fresh, err := repo.ActiveGeometrySnapshot(ctx, "EKCH")
	require.NoError(t, err)
	require.EqualValues(t, 2, fresh.ManifestRevision, "later events see activation")
	manifest.ExpectedRevision = 2
	_, err = repo.ActivateManifest(scoped, manifest)
	require.NoError(t, err)
	updated, err := repo.ActiveGeometrySnapshot(scoped, "EKCH")
	require.NoError(t, err)
	require.EqualValues(t, 3, updated.ManifestRevision, "route repair may expand the manifest within the event")
	version, err := repo.ActiveVersion(scoped, "EKCH")
	require.NoError(t, err)
	require.Equal(t, first.Manifest.Version, version)
	path, err := repo.TerminalPath(scoped, "EKCH", "SOK", "SOUTH")
	require.NoError(t, err)
	require.Equal(t, updated.TerminalPaths[0], path)
	route, err := repo.Route(scoped, key)
	require.NoError(t, err)
	require.Equal(t, candidate.Geometry, route)
	_, err = repo.Route(scoped, "missing")
	requireDomainErrorClass(t, err, aman.ErrorNotFound)
}

func TestGeometryReadScopeRejectsCorruptFragmentsAndRequestedRoutes(t *testing.T) {
	for _, kind := range []string{"airport", "fix", "procedure", "terminal", "route"} {
		t.Run(kind, func(t *testing.T) {
			pool, _ := testdata.SetupTestDB(t)
			repo := NewNavigationCache(pool)
			ctx := t.Context()
			manifest, candidate := writeNavigationFixture(t, ctx, repo, fixture.EKCH())
			_, err := repo.ActivateManifest(ctx, manifest)
			require.NoError(t, err)
			key, err := repo.PutRoute(ctx, candidate)
			require.NoError(t, err)
			// Fixed test table names, never input-derived SQL identifiers.
			tables := map[string]string{"airport": "aman_nav_airport_fragments", "fix": "aman_nav_fix_fragments", "procedure": "aman_nav_procedure_fragments", "terminal": "aman_nav_terminal_fragments", "route": "aman_nav_route_cache"}
			_, err = pool.Exec(ctx, "UPDATE "+tables[kind]+" SET payload='{}'::jsonb")
			require.NoError(t, err)
			scoped, err := repo.WithGeometryReadScope(ctx, "EKCH", []navdata.RouteKey{key})
			if kind == "route" {
				require.NoError(t, err, "an unrelated corrupt route must not break the airport snapshot")
				_, err = repo.Route(scoped, key)
			}
			requireDomainErrorClass(t, err, aman.ErrorCorruptData)
		})
	}
}
