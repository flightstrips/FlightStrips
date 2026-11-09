package viff

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/cdm"
	"github.com/stretchr/testify/require"
)

type referenceReader struct {
	snapshot navdata.ActiveGeometrySnapshot
	err      error
}

func (r referenceReader) ActiveGeometrySnapshot(context.Context, navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	return r.snapshot, r.err
}

func routeFixture() (navdata.RouteQuery, referenceReader, time.Time, flight) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	version := navdata.DatasetVersion{Cycle: "2611", SourceRevision: "cached-terminal", EffectiveFrom: now.Add(-24 * time.Hour), EffectiveUntil: now.Add(24 * time.Hour)}
	entry := navdata.FixID("TESPI")
	query := navdata.RouteQuery{Version: version, Callsign: "TEST123", TerminalEntry: &entry, Origin: "ENGM", Destination: "EKCH", FiledRoute: "N0450F350 DCT FIRST/N0440F330 P729 TESPI TESPI1A"}
	refs := referenceReader{snapshot: navdata.ActiveGeometrySnapshot{Manifest: navdata.ManifestCandidate{Version: version}, Fixes: []navdata.Fix{{ID: entry, Position: navdata.Coordinate{LatitudeDeg: 55.898333, LongitudeDeg: 10.531111}}}}}
	f := flight{Callsign: "TEST123", Departure: "ENGM", Arrival: "EKCH", Path: []point{testPoint("ENGM", 60.2, 11.1), testPoint("FIRST", 58, 11), testPoint("MID", 57, 10.7), testPoint("TESPI", 55.898333, 10.531111), testPoint("WRONG-STAR", 54, 14), testPoint("EKCH", 55.617917, 12.655972)}}
	return query, refs, now, f
}

func testPoint(name string, lat, lon float64) point { return point{Name: name, Lat: &lat, Lon: &lon} }

func TestBulkArrivalsSharesRequestsAcrossFlightsAndSessions(t *testing.T) {
	q, refs, now, f := routeFixture()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "/ifps/arrAirport", r.URL.Path)
		require.Equal(t, "airport=EKCH&path=true", r.URL.RawQuery)
		require.Equal(t, "shared-key", r.Header.Get("x-api-key"))
		require.Equal(t, http.MethodGet, r.Method)
		second := f
		second.Callsign = "TEST456"
		require.NoError(t, json.NewEncoder(w).Encode([]flight{f, second}))
	}))
	defer server.Close()
	a, err := New(Config{BaseURL: server.URL, APIKey: "shared-key", References: refs, Now: func() time.Time { return now }})
	require.NoError(t, err)
	cdmClient := cdm.NewClient(cdm.WithBaseURL("http://must-not-be-called.invalid"))
	cdmClient.SetArrivalSource(a)
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%4 == 0 {
				_, err := cdmClient.IFPSByArrivalAirport(t.Context(), "ekch")
				errs <- err
				return
			}
			query := q
			if i%2 == 0 {
				query.Callsign = "TEST456"
			}
			_, err := a.Resolve(aman.WithSession(t.Context(), int32(i+1)), query)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, requests.Load())
	now = now.Add(RefreshInterval)
	_, err = a.Resolve(t.Context(), q)
	require.NoError(t, err)
	require.EqualValues(t, 2, requests.Load())
}

func TestGeometryUsesSelectedTerminal(t *testing.T) {
	q, refs, now, f := routeFixture()
	// Generated profile points can be co-located with actual named fixes.
	f.Path = append(f.Path[:2:2], append([]point{testPoint("MID(AR2)", 57, 10.7)}, f.Path[2:]...)...)
	g, err := geometry(q, f, refs.snapshot, now)
	require.NoError(t, err)
	require.NoError(t, g.Validate())
	require.Len(t, g.Legs, 3)
	require.Equal(t, navdata.FixID("TESPI"), *g.Legs[2].ToFix)
	require.Equal(t, *g.Legs[0].ToFix, *g.Legs[1].FromFix)
	require.Equal(t, *g.Legs[1].ToFix, *g.Legs[2].FromFix)
	require.Equal(t, "viff", g.Provenance.SourceID)
	require.Equal(t, q.Version, g.Version, "vIFF cannot invent an AIRAC cycle")
	for _, leg := range g.Legs {
		require.NotNil(t, leg.FromPosition)
		require.NotNil(t, leg.ToPosition)
		require.Positive(t, *leg.DistanceNM)
		require.NotEqual(t, navdata.FixID("EKCH"), *leg.ToFix)
		require.NotEqual(t, navdata.FixID("WRONG-STAR"), *leg.ToFix)
	}
}

func TestRejectsUnusableOrMismatchedPaths(t *testing.T) {
	cases := []struct {
		name string
		edit func(*navdata.RouteQuery, *flight, *referenceReader)
	}{
		{"missing path", func(_ *navdata.RouteQuery, f *flight, _ *referenceReader) { f.Path = nil }},
		{"only profile markers", func(_ *navdata.RouteQuery, f *flight, _ *referenceReader) {
			f.Path = []point{{Name: "T_O_C"}, {Name: "T_O_D"}}
		}},
		{"missing coordinate", func(_ *navdata.RouteQuery, f *flight, _ *referenceReader) { f.Path[1].Lat = nil }},
		{"null coordinate", func(_ *navdata.RouteQuery, f *flight, _ *referenceReader) { f.Path[1].Lon = nil }},
		{"invalid coordinate", func(_ *navdata.RouteQuery, f *flight, _ *referenceReader) { f.Path[1] = testPoint("FIRST", 91, 11) }},
		{"zero placeholder", func(_ *navdata.RouteQuery, f *flight, _ *referenceReader) { f.Path[1] = testPoint("FIRST", 0, 0) }},
		{"wrong origin", func(_ *navdata.RouteQuery, f *flight, _ *referenceReader) { f.Path[0].Name = "LFPO" }},
		{"wrong destination", func(_ *navdata.RouteQuery, f *flight, _ *referenceReader) { f.Path[len(f.Path)-1].Name = "ESSA" }},
		{"missing selected entry", func(_ *navdata.RouteQuery, f *flight, _ *referenceReader) { f.Path[3].Name = "TUDLO" }},
		{"wrong region entry", func(_ *navdata.RouteQuery, f *flight, _ *referenceReader) { f.Path[3] = testPoint("TESPI", 35, 120) }},
		{"uncached entry", func(_ *navdata.RouteQuery, _ *flight, r *referenceReader) { r.snapshot.Fixes = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, refs, now, f := routeFixture()
			tc.edit(&q, &f, &refs)
			_, err := geometry(q, f, refs.snapshot, now)
			require.Error(t, err)
		})
	}
}

func TestBulkFailuresAreBoundedAndDoNotServeExpiredResponses(t *testing.T) {
	q, refs, now, f := routeFixture()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			require.NoError(t, json.NewEncoder(w).Encode([]flight{f}))
			return
		}
		w.WriteHeader(530)
		_, _ = w.Write([]byte("secret vendor body"))
	}))
	defer server.Close()
	a, err := New(Config{BaseURL: server.URL, References: refs, Now: func() time.Time { return now }})
	require.NoError(t, err)
	_, err = a.Resolve(t.Context(), q)
	require.NoError(t, err)
	now = now.Add(RefreshInterval)
	for range 10 {
		_, err = a.Resolve(t.Context(), q)
		require.ErrorContains(t, err, "HTTP 530")
		require.NotContains(t, err.Error(), "secret")
	}
	require.Equal(t, 2, calls)
	now = now.Add(RefreshInterval)
	_, err = a.Resolve(t.Context(), q)
	require.Error(t, err)
	require.Equal(t, 3, calls)
}

func TestRateLimitsPauseBothCDMAndAMANUntilRetryAfter(t *testing.T) {
	for _, format := range []string{"seconds", "date"} {
		t.Run(format, func(t *testing.T) {
			q, refs, now, _ := routeFixture()
			var calls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				value := "900"
				if format == "date" {
					value = now.Add(15 * time.Minute).Format(http.TimeFormat)
				}
				w.Header().Set("Retry-After", value)
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			defer server.Close()
			a, err := New(Config{BaseURL: server.URL, References: refs, Now: func() time.Time { return now }})
			require.NoError(t, err)
			client := cdm.NewClient()
			client.SetArrivalSource(a)
			_, err = a.Resolve(t.Context(), q)
			require.Error(t, err)
			var backoff interface{ RetryDelay() time.Duration }
			require.ErrorAs(t, err, &backoff)
			require.Equal(t, 15*time.Minute, backoff.RetryDelay())
			start := now
			for i := 1; i < 30; i++ {
				now = start.Add(time.Duration(i) * 30 * time.Second)
				_, err = a.Resolve(t.Context(), q)
				require.Error(t, err)
				_, err = client.IFPSByArrivalAirport(t.Context(), "EKCH")
				require.Error(t, err)
			}
			require.Equal(t, 1, calls, "both consumers respect the same server backoff")
			require.ErrorAs(t, err, &backoff)
			require.Equal(t, 30*time.Second, backoff.RetryDelay(), "cached errors return the remaining backoff rather than restarting it")
			now = start.Add(15 * time.Minute)
			_, err = a.Resolve(t.Context(), q)
			require.Error(t, err)
			require.Equal(t, 2, calls)
		})
	}
}

func TestSharedResponseCannotBeMutatedByTimingConsumer(t *testing.T) {
	q, refs, now, f := routeFixture()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode([]flight{f}) }))
	defer server.Close()
	a, err := New(Config{BaseURL: server.URL, References: refs, Now: func() time.Time { return now }})
	require.NoError(t, err)
	data, err := a.Arrivals(t.Context(), "EKCH")
	require.NoError(t, err)
	clear(data)
	_, err = a.Resolve(t.Context(), q)
	require.NoError(t, err)
	data, err = a.Arrivals(t.Context(), "EKCH")
	require.NoError(t, err)
	require.True(t, json.Valid(data))
}

func TestOneMalformedFlightPathDoesNotDiscardOtherFlightsOrCDMTiming(t *testing.T) {
	q, refs, now, f := routeFixture()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		valid, err := json.Marshal(f)
		require.NoError(t, err)
		_, _ = w.Write([]byte(`[` + string(valid) + `,{"callsign":"BAD123","departure":"ENGM","arrival":"EKCH","ctot":"1230","path":[{"name":"BROKEN","lat":"not a coordinate","lon":12}]}]`))
	}))
	defer server.Close()
	a, err := New(Config{BaseURL: server.URL, References: refs, Now: func() time.Time { return now }})
	require.NoError(t, err)
	_, err = a.Resolve(t.Context(), q)
	require.NoError(t, err)
	q.Callsign = "BAD123"
	_, err = a.Resolve(t.Context(), q)
	require.ErrorContains(t, err, "path is missing")
	client := cdm.NewClient()
	client.SetArrivalSource(a)
	rows, err := client.IFPSByArrivalAirport(t.Context(), "EKCH")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "1230", rows[1].CTOT)
}

func TestGeometryUsesVIFFPathRegardlessOfFiledWaypoints(t *testing.T) {
	for _, route := range []string{
		"DCT LIBSO DCT TESPI",
		"TESPI FIRST",
		"FIRST NATA 5700N01042E/N0440F330 DCT TESPI",
		"DCT FIRST T_O_C T_O_D TESPI",
	} {
		t.Run(route, func(t *testing.T) {
			q, refs, now, f := routeFixture()
			want, err := geometry(q, f, refs.snapshot, now)
			require.NoError(t, err)
			q.FiledRoute = route
			got, err := geometry(q, f, refs.snapshot, now)
			require.NoError(t, err)
			require.Equal(t, want.Legs, got.Legs)
			require.Equal(t, want.TotalDistanceNM, got.TotalDistanceNM)
		})
	}
}

func TestGeometryFiltersProfileMarkersWithoutChangingSharedPath(t *testing.T) {
	q, refs, now, f := routeFixture()
	want, err := geometry(q, f, refs.snapshot, now)
	require.NoError(t, err)
	// Markers may lack coordinates or carry positions that would cause detours.
	f.Path = []point{
		f.Path[0], {Name: "T_O_C"}, f.Path[1],
		testPoint(" t_o_c ", 35, 120), f.Path[2],
		testPoint("T_O_D", 0, 0), f.Path[3],
		{Name: " t_o_d "}, f.Path[4], f.Path[5],
	}
	raw, err := json.Marshal(f)
	require.NoError(t, err)
	got, err := geometry(q, f, refs.snapshot, now)
	require.NoError(t, err)
	require.NoError(t, got.Validate())
	require.Equal(t, want.Legs, got.Legs)
	require.Equal(t, want.TotalDistanceNM, got.TotalDistanceNM)
	require.Equal(t, want.Digest, got.Digest)
	after, err := json.Marshal(f)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(after))
}

func TestFeedRejectsWrongIdentityAndMalformedResponses(t *testing.T) {
	for _, kind := range []string{"missing flight", "duplicate callsign", "wrong departure", "wrong arrival", "null", "object", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			q, refs, now, f := routeFixture()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "missing flight":
					f.Callsign = "OTHER"
				case "duplicate callsign":
					_ = json.NewEncoder(w).Encode([]flight{f, f})
					return
				case "wrong departure":
					f.Departure = "LFPO"
				case "wrong arrival":
					f.Arrival = "ESSA"
				case "null":
					_, _ = w.Write([]byte("null"))
					return
				case "object":
					_, _ = w.Write([]byte("{}"))
					return
				case "malformed":
					_, _ = w.Write([]byte("["))
					return
				}
				_ = json.NewEncoder(w).Encode([]flight{f})
			}))
			defer server.Close()
			a, err := New(Config{BaseURL: server.URL, References: refs, Now: func() time.Time { return now }})
			require.NoError(t, err)
			_, err = a.Resolve(t.Context(), q)
			require.Error(t, err)
		})
	}
}

func TestCancellationAndTerminalValidityDoNotMakeNetworkRequests(t *testing.T) {
	q, refs, now, _ := routeFixture()
	a, err := New(Config{References: refs, Now: func() time.Time { return now }})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = a.Resolve(ctx, q)
	require.ErrorIs(t, err, context.Canceled)
	now = q.Version.EffectiveUntil
	_, err = a.Resolve(t.Context(), q)
	require.ErrorContains(t, err, "current cached terminal dataset")
	now = q.Version.EffectiveFrom.Add(-time.Second)
	_, err = a.Resolve(t.Context(), q)
	require.Error(t, err)
	q.Callsign = ""
	_, err = a.Resolve(t.Context(), q)
	require.ErrorContains(t, err, "callsign")
	_, err = New(Config{References: referenceReader{err: errors.New("no cache")}, BaseURL: "bad"})
	require.Error(t, err)
	_, err = New(Config{})
	require.Error(t, err)
}

func TestAcquisitionContextDoesNotChangeExistingRouteSemanticKey(t *testing.T) {
	q, _, _, _ := routeFixture()
	want, err := q.Key()
	require.NoError(t, err)
	q.Callsign = "TEST456"
	q.TerminalEntry = nil
	got, err := q.Key()
	require.NoError(t, err)
	require.Equal(t, want, got)
	q.Callsign = aman.Callsign(strings.ToLower(string(q.Callsign)))
	require.Error(t, q.Validate())
}
