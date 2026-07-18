package airacnet

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/contracttest"
	"FlightStrips/internal/aman/navdata/fixture"
	pdctestdata "FlightStrips/internal/pdc/testdata"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestAdapterPassesSharedSourceResolverContract(t *testing.T) {
	server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/airac/current":
			writeJSON(w, cycleJSON())
		case "/api/v1/airports/EKCH":
			writeJSON(w, map[string]any{"data": map[string]any{"icao": "EKCH", "name": "Copenhagen", "coordinates": map[string]any{"lat": 55.618, "lon": 12.656}}})
		case "/api/v1/procedures":
			writeJSON(w, procedurePage(r.URL.Query(), false))
		case "/api/v1/procedures/EKCH/KEMAX3A":
			writeJSON(w, detailJSON("EKCH", "KEMAX3A", "SID", "TF", "KEMAX"))
		case "/api/v1/procedures/EKCH/SOK1P":
			writeJSON(w, detailJSON("EKCH", "SOK1P", "STAR", "TF", "SOK"))
		case "/api/v1/procedures/EKCH/ILS22L":
			writeJSON(w, detailJSON("EKCH", "ILS22L", "APP", "TF", "ROSBI"))
		case "/api/v1/waypoints/KEMAX", "/api/v1/waypoints/SOK":
			writeJSON(w, map[string]any{"data": map[string]any{"identifier": r.URL.Path[len("/api/v1/waypoints/"):], "coordinates": map[string]any{"lat": 55.8, "lon": 12.4}}})
		case "/api/v1/routes/parse":
			require.Equal(t, "KEMAX SOK1P", r.URL.Query().Get("route"))
			require.Equal(t, "22L", r.URL.Query().Get("arrival_runway"))
			writeJSON(w, routeJSON())
		default:
			t.Errorf("unexpected endpoint %s", r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	})
	checkpoints := NewMemoryCheckpoints()
	adapter := testAdapter(t, server.URL, Config{Checkpoints: checkpoints})
	version, err := adapter.LatestVersion(context.Background())
	require.NoError(t, err)
	query := routeQuery(version)
	expected, err := expectedRoute(query)
	require.NoError(t, err)
	contracttest.Run(t, adapter, contracttest.Suite{
		Version: version, Airport: "EKCH",
		SIDQuery:      navdata.ProcedureQuery{Version: version, Airport: "EKCH", Kinds: []navdata.ProcedureKind{navdata.ProcedureSID}},
		STARQuery:     navdata.ProcedureQuery{Version: version, Airport: "EKCH", Kinds: []navdata.ProcedureKind{navdata.ProcedureSTAR}},
		ApproachQuery: navdata.ProcedureQuery{Version: version, Airport: "EKCH", Kinds: []navdata.ProcedureKind{navdata.ProcedureApproach}},
		FixQuery:      navdata.FixQuery{Version: version, Identifiers: []navdata.FixID{"KEMAX", "SOK"}}, RouteQuery: query, RouteDigest: expected.Digest,
	})
}

func TestProcedureFiltersPaginationAndUndocumentedHoldsRemainPartial(t *testing.T) {
	var calls []url.Values
	var mu sync.Mutex
	server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/airac/current":
			writeJSON(w, cycleJSON())
		case "/api/v1/procedures":
			mu.Lock()
			calls = append(calls, r.URL.Query())
			mu.Unlock()
			writeJSON(w, procedurePage(r.URL.Query(), true))
		case "/api/v1/procedures/EKCH/SOK1P":
			writeJSON(w, detailJSON("EKCH", "SOK1P", "STAR", "HF", "SOK"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	checkpoints := NewMemoryCheckpoints()
	adapter := testAdapter(t, server.URL, Config{Checkpoints: checkpoints})
	version, err := adapter.LatestVersion(context.Background())
	require.NoError(t, err)
	set, err := adapter.Procedures(context.Background(), navdata.ProcedureQuery{Version: version, Airport: "EKCH", Kinds: []navdata.ProcedureKind{navdata.ProcedureSTAR}, Runways: []navdata.RunwayID{"22L"}, Identifiers: []navdata.ProcedureID{"SOK1P"}})
	require.NoError(t, err)
	require.Equal(t, navdata.CoveragePartial, set.Coverage)
	require.Len(t, set.Procedures, 1)
	require.Equal(t, navdata.PathUnsupported, set.Procedures[0].Legs[0].PathTerminator)
	require.Empty(t, set.Procedures[0].Holdings, "AIRAC.NET documents no holding definition schema")
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, calls, 2, "page based pagination")
	for _, query := range calls {
		require.Equal(t, "STAR", query.Get("type"))
		require.Equal(t, "SOK1P", query.Get("identifier"))
		require.Equal(t, "22L", query.Get("runway"))
	}
	checkpoint, found, err := checkpoints.Load(context.Background(), "/api/v1/procedures?airport=EKCH&identifier=SOK1P&page=1&per_page=100&runway=22L&type=STAR")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 2, checkpoint.NextPage)
}

func TestHoldingTerminatorsWithoutOfficialDefinitionsAreAlwaysIncomplete(t *testing.T) {
	for _, terminator := range []string{"HA", "HF", "HM"} {
		t.Run(terminator, func(t *testing.T) {
			server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/airac/current":
					writeJSON(w, cycleJSON())
				case "/api/v1/procedures":
					writeJSON(w, procedurePage(r.URL.Query(), false))
				case "/api/v1/procedures/EKCH/SOK1P":
					writeJSON(w, detailJSON("EKCH", "SOK1P", "STAR", terminator, "SOK"))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			adapter := testAdapter(t, server.URL, Config{})
			version, err := adapter.LatestVersion(context.Background())
			require.NoError(t, err)
			set, err := adapter.Procedures(context.Background(), navdata.ProcedureQuery{Version: version, Airport: "EKCH", Kinds: []navdata.ProcedureKind{navdata.ProcedureSTAR}})
			require.NoError(t, err)
			require.Equal(t, navdata.CoveragePartial, set.Coverage)
			require.Empty(t, set.Procedures[0].Holdings)
			require.Equal(t, navdata.PathUnsupported, set.Procedures[0].Legs[0].PathTerminator)
		})
	}
}

func TestConditionalRetryTimeoutCancellationRedactionAndConcurrency(t *testing.T) {
	t.Run("conditional", func(t *testing.T) {
		var calls atomic.Int32
		server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.Header().Set("ETag", `"cycle"`)
				w.Header().Set("Last-Modified", "Thu, 16 Jul 2026 00:00:00 GMT")
				writeJSON(w, cycleJSON())
				return
			}
			require.Equal(t, `"cycle"`, r.Header.Get("If-None-Match"))
			w.WriteHeader(http.StatusNotModified)
		})
		adapter := testAdapter(t, server.URL, Config{})
		first, err := adapter.LatestVersion(context.Background())
		require.NoError(t, err)
		second, err := adapter.LatestVersion(context.Background())
		require.NoError(t, err)
		require.True(t, first.Equal(second))
		require.Equal(t, int32(2), calls.Load())
	})
	t.Run("rate retry", func(t *testing.T) {
		var calls atomic.Int32
		server := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			writeJSON(w, cycleJSON())
		})
		adapter := testAdapter(t, server.URL, Config{Retries: 1, RetryBackoff: time.Millisecond})
		_, err := adapter.LatestVersion(context.Background())
		require.NoError(t, err)
		require.Equal(t, int32(2), calls.Load())
	})
	t.Run("server retry", func(t *testing.T) {
		var calls atomic.Int32
		server := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			writeJSON(w, cycleJSON())
		})
		adapter := testAdapter(t, server.URL, Config{Retries: 1, RetryBackoff: time.Millisecond})
		_, err := adapter.LatestVersion(context.Background())
		require.NoError(t, err)
		require.Equal(t, int32(2), calls.Load())
	})
	t.Run("timeout cancellation and redaction", func(t *testing.T) {
		server := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			writeJSON(w, cycleJSON())
		})
		adapter := testAdapter(t, server.URL, Config{Timeout: 10 * time.Millisecond})
		_, err := adapter.LatestVersion(context.Background())
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = adapter.LatestVersion(ctx)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("redacts vendor body", func(t *testing.T) {
		server := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"token":"secret"}`))
		})
		adapter := testAdapter(t, server.URL, Config{})
		_, err := adapter.LatestVersion(context.Background())
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	})
	t.Run("bounded fix concurrency", func(t *testing.T) {
		var current, maximum atomic.Int32
		server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/airac/current" {
				writeJSON(w, cycleJSON())
				return
			}
			now := current.Add(1)
			for {
				previous := maximum.Load()
				if now <= previous || maximum.CompareAndSwap(previous, now) {
					break
				}
			}
			time.Sleep(15 * time.Millisecond)
			current.Add(-1)
			id := r.URL.Path[len("/api/v1/waypoints/"):]
			writeJSON(w, map[string]any{"data": map[string]any{"identifier": id, "coordinates": map[string]any{"lat": 1, "lon": 1}}})
		})
		adapter := testAdapter(t, server.URL, Config{MaxConcurrent: 2})
		version, err := adapter.LatestVersion(context.Background())
		require.NoError(t, err)
		_, err = adapter.Fixes(context.Background(), navdata.FixQuery{Version: version, Identifiers: []navdata.FixID{"ONE", "TWO", "THREE"}})
		require.NoError(t, err)
		require.LessOrEqual(t, maximum.Load(), int32(2))
	})
}

func TestMalformedAndVersionMismatchFailClosed(t *testing.T) {
	server := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not json")) })
	adapter := testAdapter(t, server.URL, Config{})
	_, err := adapter.LatestVersion(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "malformed")

	server = newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, cycleJSON()) })
	adapter = testAdapter(t, server.URL, Config{})
	wrong := navdata.DatasetVersion{Cycle: "9999", SourceRevision: "airac.net-9999", EffectiveFrom: time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC), EffectiveUntil: time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)}
	_, err = adapter.Airport(context.Background(), wrong, "EKCH")
	var domain *aman.DomainError
	require.ErrorAs(t, err, &domain)
	require.Equal(t, navdata.ErrorDatasetMismatch, domain.Class)
}

func TestPostgresCheckpointsSurviveAdapterRestartAndConditionalResponse(t *testing.T) {
	db := &fakeCheckpointDB{values: map[string]Checkpoint{}}
	store := &PostgresCheckpoints{db: db}
	var calls atomic.Int32
	server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("ETag", `"cycle"`)
			writeJSON(w, cycleJSON())
			return
		}
		require.Equal(t, `"cycle"`, r.Header.Get("If-None-Match"))
		w.WriteHeader(http.StatusNotModified)
	})
	first := testAdapter(t, server.URL, Config{Checkpoints: store})
	version, err := first.LatestVersion(context.Background())
	require.NoError(t, err)
	second := testAdapter(t, server.URL, Config{Checkpoints: store})
	again, err := second.LatestVersion(context.Background())
	require.NoError(t, err)
	require.True(t, version.Equal(again))
	require.Equal(t, int32(2), calls.Load())

	_, err = New(Config{})
	require.Error(t, err, "production configuration must choose a durable checkpoint store")
}

func TestPostgresCheckpointMigrationSurvivesRestartAnd304(t *testing.T) {
	pool, _ := pdctestdata.SetupTestDB(t)
	store := NewPostgresCheckpoints(pool)
	var calls atomic.Int32
	server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("ETag", `"cycle"`)
			w.Header().Set("Last-Modified", "Thu, 16 Jul 2026 00:00:00 GMT")
			writeJSON(w, cycleJSON())
			return
		}
		require.Equal(t, `"cycle"`, r.Header.Get("If-None-Match"))
		w.WriteHeader(http.StatusNotModified)
	})
	first := testAdapter(t, server.URL, Config{Checkpoints: store})
	version, err := first.LatestVersion(context.Background())
	require.NoError(t, err)
	second := testAdapter(t, server.URL, Config{Checkpoints: NewPostgresCheckpoints(pool)})
	again, err := second.LatestVersion(context.Background())
	require.NoError(t, err)
	require.True(t, version.Equal(again))
	require.Equal(t, int32(2), calls.Load())
	checkpoint, found, err := store.Load(context.Background(), "/api/v1/airac/current?")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, `"cycle"`, checkpoint.ETag)
	require.NotEmpty(t, checkpoint.Body)
}

func TestAmbiguousWaypointIsNotArbitrarilySelected(t *testing.T) {
	server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/airac/current" {
			writeJSON(w, cycleJSON())
			return
		}
		writeJSON(w, map[string]any{"data": []any{map[string]any{"identifier": "DUP", "coordinates": map[string]any{"lat": 1, "lon": 1}}, map[string]any{"identifier": "DUP", "coordinates": map[string]any{"lat": 2, "lon": 2}}}})
	})
	adapter := testAdapter(t, server.URL, Config{})
	version, err := adapter.LatestVersion(context.Background())
	require.NoError(t, err)
	_, err = adapter.Fixes(context.Background(), navdata.FixQuery{Version: version, Identifiers: []navdata.FixID{"DUP"}})
	var domain *aman.DomainError
	require.ErrorAs(t, err, &domain)
	require.Equal(t, navdata.ErrorIncompleteGeometry, domain.Class)
}

func TestRouteKeyPrecedesTransformsAndWarmCacheNeedsNoNetwork(t *testing.T) {
	var calls atomic.Int32
	server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/api/v1/airac/current":
			writeJSON(w, cycleJSON())
		case "/api/v1/routes/parse":
			require.Equal(t, "KEMAX P60 TUDLO SOK1P", r.URL.Query().Get("route"))
			require.Equal(t, "22L", r.URL.Query().Get("arrival_runway"))
			writeJSON(w, routeJSON())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	adapter := testAdapter(t, server.URL, Config{})
	version, err := adapter.LatestVersion(context.Background())
	require.NoError(t, err)
	query := routeQuery(version)
	query.FiledRoute = "DCT KEMAX P60 TUDLO"
	key, err := query.Key()
	require.NoError(t, err)
	geometry, err := adapter.Resolve(context.Background(), query)
	require.NoError(t, err)
	cache, err := navdata.NewCache(map[navdata.AirportID]navdata.DatasetVersion{"EKCH": version}, map[navdata.RouteKey]navdata.RouteGeometry{key: geometry}, nil)
	require.NoError(t, err)
	before := calls.Load()
	server.Close()
	warm, err := cache.Route(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, geometry.Digest, warm.Digest)
	require.Equal(t, before, calls.Load())
	expected, err := expectedRoute(query)
	require.NoError(t, err)
	require.Equal(t, expected.Digest, geometry.Digest)
	data := fixture.EKCH()
	data.Version = version
	data.Provenance = expected.Provenance
	data.Routes = map[navdata.RouteKey]navdata.RouteGeometry{key: expected}
	replacement := fixture.New(data)
	equivalent, err := replacement.Resolve(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, geometry.Digest, equivalent.Digest)
	require.Empty(t, equivalent.HoldingIDs)
}

func TestErrorEnvelopeAndPermanentTransportFailureFailClosed(t *testing.T) {
	server := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSONRaw(w, map[string]any{"status": "error", "message": "secret vendor reason"})
	})
	adapter := testAdapter(t, server.URL, Config{})
	_, err := adapter.LatestVersion(context.Background())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
	var requests atomic.Int32
	adapter = testAdapter(t, server.URL, Config{Retries: 2, HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { requests.Add(1); return nil, errors.New("permanent") })}})
	_, err = adapter.LatestVersion(context.Background())
	require.Error(t, err)
	require.Equal(t, int32(1), requests.Load())
}

func TestVendorAdapterCannotLeakIntoRuntimeOrCanonicalPackages(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	internal := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))
	for _, directory := range []string{"predictor", "sequence", "frontend", "repository", "aman"} {
		root := filepath.Join(internal, directory)
		err := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() && filepath.Clean(name) == filepath.Join(internal, "aman", "navdata", "airacnet") {
				return filepath.SkipDir
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			contents, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			require.NotContains(t, string(contents), "FlightStrips/internal/aman/navdata/airacnet", name)
			return nil
		})
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		require.NoError(t, err)
	}
}

type fakeCheckpointDB struct {
	mu     sync.Mutex
	values map[string]Checkpoint
}

func (db *fakeCheckpointDB) QueryRow(_ context.Context, _ string, values ...any) pgx.Row {
	db.mu.Lock()
	defer db.mu.Unlock()
	value, found := db.values[values[0].(string)]
	if !found {
		return fakeCheckpointRow{err: pgx.ErrNoRows}
	}
	return fakeCheckpointRow{value: value}
}
func (db *fakeCheckpointDB) Exec(_ context.Context, _ string, values ...any) (pgconn.CommandTag, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.values[values[0].(string)] = Checkpoint{ETag: values[1].(string), LastModified: values[2].(string), NextPage: values[3].(int), Body: append([]byte(nil), values[4].([]byte)...)}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

type fakeCheckpointRow struct {
	value Checkpoint
	err   error
}

func (r fakeCheckpointRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*string) = r.value.ETag
	*dest[1].(*string) = r.value.LastModified
	*dest[2].(*int) = r.value.NextPage
	*dest[3].(*[]byte) = append([]byte(nil), r.value.Body...)
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func newAPIServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}
func testAdapter(t *testing.T, baseURL string, config Config) *Adapter {
	t.Helper()
	if config.BaseURL == "" {
		config.BaseURL = baseURL + "/api/v1"
	}
	if config.Checkpoints == nil {
		config.Checkpoints = NewMemoryCheckpoints()
	}
	config.Now = func() time.Time { return time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC) }
	adapter, err := New(config)
	require.NoError(t, err)
	return adapter
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	encoded, _ := json.Marshal(value)
	var envelope map[string]any
	_ = json.Unmarshal(encoded, &envelope)
	envelope["status"] = "success"
	_ = json.NewEncoder(w).Encode(envelope)
}
func writeJSONRaw(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func cycleJSON() any {
	return map[string]any{"data": map[string]any{"cycle": "2608", "effective_date": "2026-07-16T00:00:00+00:00", "expiration_date": "2026-08-13T00:00:00+00:00"}}
}
func procedurePage(query url.Values, paged bool) any {
	page := query.Get("page")
	if paged && page == "1" {
		return map[string]any{"data": []any{}, "pagination": map[string]any{"has_more": true}}
	}
	kind, id := query.Get("type"), query.Get("identifier")
	if kind == "" {
		kind = "STAR"
	}
	if id == "" {
		switch kind {
		case "SID":
			id = "KEMAX3A"
		case "APP":
			id = "ILS22L"
		default:
			id = "SOK1P"
		}
	}
	return map[string]any{"data": []any{map[string]any{"airport": "EKCH", "identifier": id, "type": map[string]any{"code": kind}, "runway": "22L"}}, "pagination": map[string]any{"has_more": false}}
}
func detailJSON(airport, identifier, kind, terminator, fix string) any {
	return map[string]any{"data": map[string]any{"airport": airport, "identifier": identifier, "type": map[string]any{"code": kind}, "available_runways": []string{"22L"}, "segments": []any{map[string]any{"sequence": 10, "path_terminator": terminator, "fix_identifier": fix}}}}
}
func routeJSON() any {
	return map[string]any{"data": map[string]any{"total_distance": 100.5, "segments": []any{map[string]any{"from": map[string]any{"identifier": "EHAM"}, "to": map[string]any{"identifier": "KEMAX"}, "distance": 80.0, "bearing": 45.0}, map[string]any{"from": map[string]any{"identifier": "KEMAX"}, "to": map[string]any{"identifier": "EKCH"}, "distance": 20.5, "bearing": 90.0}}, "errors": []any{map[string]any{"type": "airway_not_found"}}}}
}
func routeQuery(version navdata.DatasetVersion) navdata.RouteQuery {
	arrival, runway := navdata.ProcedureID("SOK1P"), navdata.RunwayID("22L")
	group := aman.RunwayGroupID("SOUTH")
	return navdata.RouteQuery{Version: version, Origin: "EHAM", Destination: "EKCH", FiledRoute: "DCT KEMAX", ArrivalProcedure: &arrival, Runway: &runway, RunwayGroup: &group}
}
func expectedRoute(query navdata.RouteQuery) (navdata.RouteGeometry, error) {
	from, to, ekch := navdata.FixID("EHAM"), navdata.FixID("KEMAX"), navdata.FixID("EKCH")
	firstCourse, secondCourse, firstDistance, secondDistance := 45.0, 90.0, 80.0, 20.5
	provenance := navdata.Provenance{SourceID: "airac.net", SourceRevision: query.Version.SourceRevision, ImportedAt: time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC), EffectiveFrom: query.Version.EffectiveFrom, EffectiveUntil: query.Version.EffectiveUntil}
	geometry := navdata.RouteGeometry{Version: query.Version, TotalDistanceNM: 100.5, Coverage: navdata.CoveragePartial, Unresolved: []string{"AIRWAY_NOT_FOUND"}, Provenance: provenance, Legs: []navdata.ProcedureLeg{{ID: "ROUTE-0001", PathTerminator: navdata.PathTF, FromFix: &from, ToFix: &to, CourseTrueDeg: &firstCourse, DistanceNM: &firstDistance}, {ID: "ROUTE-0002", PathTerminator: navdata.PathTF, FromFix: &to, ToFix: &ekch, CourseTrueDeg: &secondCourse, DistanceNM: &secondDistance}}}
	digest, err := navdata.RouteGeometryDigest(query, geometry)
	geometry.Digest = digest
	return geometry, err
}
