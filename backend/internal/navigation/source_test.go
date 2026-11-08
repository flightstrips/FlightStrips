package navigation

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/aman/materializer"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/fixture"
	"FlightStrips/internal/aman/navdata/viff"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/aman/trajectory"
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"github.com/stretchr/testify/require"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVIFFStartupRoutesAndRestartWithoutAIRACNet(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	data := fixture.EKCH()
	now := data.Provenance.ImportedAt.Add(time.Hour)
	clock := func() time.Time { return now }
	course := 221.2
	config := terminal.Configuration{
		SchemaVersion: terminal.SchemaVersion, ConfigVersion: "fixture-terminal", Airport: "EKCH",
		ApplicabilityFrom: data.Version.EffectiveFrom, ApplicabilityUntil: data.Version.EffectiveUntil,
		Dataset:      terminal.DatasetCompatibility{Cycle: data.Version.Cycle, EffectiveFrom: data.Version.EffectiveFrom, EffectiveUntil: data.Version.EffectiveUntil},
		Sources:      []terminal.Source{{ID: "fixture-aip", Document: "fixture", EffectiveFrom: data.Version.EffectiveFrom, EffectiveUntil: data.Version.EffectiveUntil}},
		RunwayGroups: []terminal.RunwayGroup{{ID: "SOUTH", Runways: []navdata.RunwayID{"22L"}, FinalApproaches: []terminal.FinalApproachDefinition{{Runway: "22L", FinalApproachFix: "KEMAX", Threshold: terminal.ThresholdDefinition{Position: terminal.CoordinateDefinition{LatitudeDeg: 55.6254111111, LongitudeDeg: 12.6675805556}, CourseTrueDeg: &course}, CourseTrueDeg: course, PhysicalLengthM: 3302, Provenance: terminal.ProvenanceDefinition{SourceID: data.Provenance.SourceID, SourceRevision: data.Provenance.SourceRevision, ImportedAt: data.Provenance.ImportedAt, EffectiveFrom: data.Version.EffectiveFrom, EffectiveUntil: data.Version.EffectiveUntil}}}}},
		Feeders:      []terminal.Feeder{{ID: "SOK"}}, Paths: []terminal.Path{{Feeder: "SOK", RunwayGroup: "SOUTH", Fixes: []navdata.FixID{"SOK", "KEMAX"}, MergeFix: "KEMAX", SelectedHolding: "SOK-HF"}},
	}
	cache := postgres.NewNavigationCache(pool)
	seedSource := fixture.New(data)
	seed, err := materializer.New(materializer.Dependencies{Cycles: seedSource, Airports: seedSource, Runways: config, Procedures: seedSource, Fixes: seedSource, Routes: seedSource, Cache: cache, Terminal: config, Now: clock})
	require.NoError(t, err)
	require.NoError(t, seed.Refresh(t.Context(), materializer.Request{Airport: "EKCH"}))
	seedCalls := seedSource.Calls()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "/ifps/arrAirport", r.URL.Path)
		require.Equal(t, "airport=EKCH&path=true", r.URL.RawQuery)
		_, _ = w.Write([]byte(`[{"callsign":"TEST123","departure":"ENGM","arrival":"EKCH","ctot":"1230","atot":"1225","path":[{"name":"ENGM","lat":56,"lon":12},{"name":"ENROUTE","lat":55.9,"lon":12.1},{"name":"SOK","lat":55.7,"lon":12.2},{"name":"OTHER-STAR","lat":54,"lon":15},{"name":"EKCH","lat":55.618,"lon":12.656}]}]`))
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != u.Host {
			return nil, fmt.Errorf("external provider blocked: %s", r.URL.Host)
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	cfg := Config{Source: SourceVIFF, VIFFBaseURL: server.URL, HTTPClient: client, Now: clock}
	importer, arrivals, err := assembleMaterializer(cfg, config, cache, nil)
	require.NoError(t, err)
	require.NoError(t, importer.Refresh(t.Context(), materializer.Request{Airport: "EKCH"}))
	require.True(t, importer.Health("EKCH").CacheReady)
	require.Zero(t, requests.Load(), "startup validates the terminal cache without HTTP")
	cdmClient := cdm.NewClient(cdm.WithBaseURL("http://must-not-be-called.invalid"))
	cdmClient.SetArrivalSource(arrivals)
	rows, err := cdmClient.IFPSByArrivalAirport(t.Context(), "EKCH")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "1230", rows[0].CTOT)
	require.Equal(t, "1225", rows[0].ATOT)
	require.EqualValues(t, 1, requests.Load(), "CDM's existing poll fills the same cache used by AMAN")
	source := &Source{Materializer: importer, Geometry: cache, Terminal: config, routeSource: SourceVIFF}
	entry := navdata.FixID("SOK")
	query := navdata.RouteQuery{Version: data.Version, Callsign: "TEST123", TerminalEntry: &entry, Origin: "ENGM", Destination: "EKCH", FiledRoute: "DCT SOK"}
	key, err := source.MaterializeRoute(t.Context(), query, "old-airac-resolver")
	require.NoError(t, err)
	route, err := cache.Route(t.Context(), key)
	require.NoError(t, err)
	require.Equal(t, "viff", route.Provenance.SourceID)
	require.True(t, route.Revisioned)
	require.Equal(t, entry, *route.Legs[len(route.Legs)-1].ToFix)
	wantKey, err := (navdata.RouteCandidate{Query: query, ResolverVersion: viff.ResolverVersion, SchemaVersion: navdata.CanonicalSchemaVersion, Geometry: route}).PersistenceKey()
	require.NoError(t, err)
	require.Equal(t, wantKey, key)
	_, err = source.MaterializeRoute(t.Context(), query, "ignored")
	require.NoError(t, err)
	require.EqualValues(t, 1, requests.Load(), "materializing another route shares the airport response")
	filed, err := trajectory.ReadFiledRoute(t.Context(), trajectory.Readers{Geometry: cache, Snapshot: cache}, "EKCH", key, "", "SOK", "SOUTH")
	require.NoError(t, err)
	require.Greater(t, len(filed.Legs), len(route.Legs), "the selected cached terminal continuation is attached")
	for _, leg := range filed.Legs {
		require.NotEqual(t, navdata.FixID("OTHER-STAR"), leg.To)
	}
	// A fresh materializer and repository reuse the durable manifest and route.
	freshCache := postgres.NewNavigationCache(pool)
	fresh, _, err := assembleMaterializer(cfg, config, freshCache, nil)
	require.NoError(t, err)
	require.NoError(t, fresh.Refresh(t.Context(), materializer.Request{Airport: "EKCH"}))
	recovered, err := freshCache.Route(t.Context(), key)
	require.NoError(t, err)
	require.Equal(t, route.Digest, recovered.Digest)
	require.Equal(t, seedCalls, seedSource.Calls())
	require.EqualValues(t, 1, requests.Load())
}
