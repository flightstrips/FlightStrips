package navigation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/materializer"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/fixture"
	"FlightStrips/internal/aman/navdata/sector"
	"FlightStrips/internal/aman/navdata/viff"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"github.com/stretchr/testify/require"
)

func TestVIFFPrimaryFallsBackForControllerAmendmentsUntilFeedCatchesUp(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	cache := postgres.NewNavigationCache(pool)
	terminal := sectorTestConfig()
	now := fixture.EKCH().Provenance.ImportedAt.Add(time.Hour)
	path := filepath.Join(t.TempDir(), "synthetic.sct")
	require.NoError(t, os.WriteFile(path, []byte(localTestSector+"[FIXES]\nEDIT N055.54.00.000 E012.30.00.000\n"), 0600))
	var caughtUp atomic.Bool
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "/ifps/arrAirport", r.URL.Path)
		type point struct {
			Name string  `json:"name"`
			Lat  float64 `json:"lat"`
			Lon  float64 `json:"lon"`
		}
		type flight struct {
			Callsign  string  `json:"callsign"`
			Departure string  `json:"departure"`
			Arrival   string  `json:"arrival"`
			Path      []point `json:"path"`
		}
		old := []point{{"ENGM", 56.5, 12}, {"ENROUTE", 56, 12}, {"SOK", 55.7, 12.2}, {"EKCH", 55.618, 12.656}}
		edited := old
		if caughtUp.Load() {
			edited = []point{old[0], old[1], {"EDIT", 55.9, 12.5}, old[2], old[3]}
		}
		require.NoError(t, json.NewEncoder(w).Encode([]flight{{"EDIT123", "ENGM", "EKCH", edited}, {"OTHER456", "ENGM", "EKCH", old}}))
	}))
	defer server.Close()
	cfg := Config{Source: SourceVIFF, SectorFilePath: path, VIFFBaseURL: server.URL, HTTPClient: server.Client(), Now: func() time.Time { return now }}
	importer, arrivals, err := assembleMaterializer(cfg, terminal, cache, nil)
	require.NoError(t, err)
	require.NoError(t, importer.Refresh(t.Context(), materializer.Request{Airport: "EKCH"}))
	version, err := cache.ActiveVersion(t.Context(), "EKCH")
	require.NoError(t, err)
	entry, group := navdata.FixID("SOK"), aman.RunwayGroupID("SOUTH")
	q := navdata.RouteQuery{Version: version, Callsign: "EDIT123", Origin: "ENGM", Destination: "EKCH", FiledRoute: "ENROUTE DCT SOK", TerminalEntry: &entry, RunwayGroup: &group}
	source := &Source{Materializer: importer, Geometry: cache, Terminal: terminal, routeSource: SourceVIFF}
	materialize := func(query navdata.RouteQuery) (navdata.RouteKey, navdata.RouteGeometry) {
		t.Helper()
		key, err := source.MaterializeRoute(t.Context(), query, "test")
		require.NoError(t, err)
		g, err := cache.Route(t.Context(), key)
		require.NoError(t, err)
		return key, g
	}
	oldKey, original := materialize(q)
	require.Equal(t, "viff", original.Provenance.SourceID)
	rawBefore, err := arrivals.Arrivals(t.Context(), "EKCH")
	require.NoError(t, err)
	// EuroScope changes ITEM15 while vIFF still returns HTTP 200 and its old
	// bulk path. The new semantic key must store locally amended geometry.
	q.FiledRoute = "ENROUTE DCT EDIT DCT SOK"
	editedKey, amended := materialize(q)
	require.NotEqual(t, oldKey, editedKey)
	require.Equal(t, "sector", amended.Provenance.SourceID)
	require.Equal(t, navdata.FixID("EDIT"), *amended.Legs[0].ToFix)
	_, stillAmended := materialize(q)
	require.Equal(t, amended.Digest, stillAmended.Digest, "repeated stale HTTP success must not replace the amendment")
	other := q
	other.Callsign, other.FiledRoute = "OTHER456", "ENROUTE DCT SOK"
	_, unaffected := materialize(other)
	require.Equal(t, "viff", unaffected.Provenance.SourceID)
	require.EqualValues(t, 1, requests.Load(), "controller edits must reuse the paced shared bulk feed")
	rawAfter, err := arrivals.Arrivals(t.Context(), "EKCH")
	require.NoError(t, err)
	require.Equal(t, rawBefore, rawAfter, "local amendments must not mutate CDM's shared arrivals")
	// A restarted resolver still recognizes the stale route without relying on
	// an in-memory 'previous flight plan' baseline.
	fresh, _, err := assembleMaterializer(cfg, terminal, cache, nil)
	require.NoError(t, err)
	source.Materializer = fresh
	_, restarted := materialize(q)
	require.Equal(t, "sector", restarted.Provenance.SourceID)
	// The primary resumes as soon as its normally paced feed matches ITEM15.
	caughtUp.Store(true)
	now = now.Add(viff.RefreshInterval)
	_, recovered := materialize(q)
	require.Equal(t, "viff", recovered.Provenance.SourceID)
	require.Equal(t, navdata.FixID("EDIT"), *recovered.Legs[1].ToFix)
}

func TestVIFFStaleRouteDoesNotHideUnresolvableControllerAmendment(t *testing.T) {
	// Covered by the same provider-success path as real vIFF, without needing
	// an HTTP request when the amendment is already known to be unresolvable.
	config := sectorTestConfig()
	path := filepath.Join(t.TempDir(), "synthetic.sct")
	require.NoError(t, os.WriteFile(path, []byte(localTestSector), 0600))
	now := fixture.EKCH().Provenance.ImportedAt.Add(time.Hour)
	local, err := sector.Load(path, config, func() time.Time { return now })
	require.NoError(t, err)
	version, err := local.LatestVersion(t.Context())
	require.NoError(t, err)
	entry := navdata.FixID("SOK")
	q := navdata.RouteQuery{Version: version, Origin: "ENGM", Destination: "EKCH", FiledRoute: "ENROUTE DCT SOK", TerminalEntry: &entry}
	old, err := local.Resolve(t.Context(), q)
	require.NoError(t, err)
	routes := &fallbackRoutes{local: local, checkFiledRoute: true, primary: resolverFunc(func(context.Context, navdata.RouteQuery) (navdata.RouteGeometry, error) { return old, nil })}
	q.FiledRoute = "ENROUTE DCT UNKNOWN DCT SOK"
	_, err = routes.Resolve(t.Context(), q)
	require.ErrorContains(t, err, "does not match current filed waypoints")
	require.ErrorContains(t, err, "UNKNOWN")
}

func TestVIFFPrefersMatchingOnlineRouteWithPartialSectorCoverage(t *testing.T) {
	config := sectorTestConfig()
	now := fixture.EKCH().Provenance.ImportedAt.Add(time.Hour)
	local, err := sector.Read(strings.NewReader(localTestSector), config, func() time.Time { return now })
	require.NoError(t, err)
	version, err := local.LatestVersion(t.Context())
	require.NoError(t, err)
	entry := navdata.FixID("SOK")
	q := navdata.RouteQuery{Version: version, Origin: "ENGM", Destination: "EKCH", FiledRoute: "ENROUTE DCT SOK", TerminalEntry: &entry}
	online, err := local.Resolve(t.Context(), q)
	require.NoError(t, err)
	online.Provenance.SourceID = "viff"
	routes := &fallbackRoutes{local: local, checkFiledRoute: true, primary: resolverFunc(func(context.Context, navdata.RouteQuery) (navdata.RouteGeometry, error) { return online, nil })}
	q.FiledRoute = "MISSING1A ENROUTE DCT SOK"
	partial, err := local.Resolve(t.Context(), q)
	require.NoError(t, err)
	require.True(t, navdata.HasRouteGaps(partial))
	preferred, err := routes.Resolve(t.Context(), q)
	require.NoError(t, err)
	require.Equal(t, "viff", preferred.Provenance.SourceID)
}
