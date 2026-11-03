package airacnet

import (
	"encoding/json"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/trajectory"
	"github.com/stretchr/testify/require"
)

func TestSAS43MRecoversFromRecordedBadGeometryWithoutNewClearance(t *testing.T) {
	body, err := os.ReadFile("testdata/sas43m-route.json")
	require.NoError(t, err)
	var response routeResponse
	require.NoError(t, json.Unmarshal(body, &response))
	server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/airac/current":
			writeJSON(w, cycleJSON())
		case "/api/v1/navaids/SPY":
			writeJSON(w, map[string]any{"data": map[string]any{"identifier": "SPY", "type": map[string]any{"code": "DME"}, "coordinates": map[string]any{"lat": 0, "lon": 0}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	adapter := testAdapter(t, server.URL, Config{})
	version, err := adapter.LatestVersion(t.Context())
	require.NoError(t, err)
	query := routeQuery(version)
	query.Origin, query.ArrivalProcedure = "EBBR", nil
	original, err := adapter.routeGeometry(query, response.Data)
	require.NoError(t, err)
	require.InDelta(t, 8463.4, original.TotalDistanceNM, .1)
	require.NoError(t, adapter.resolveRouteDetours(t.Context(), &response.Data))
	route, err := adapter.routeGeometry(query, response.Data)
	require.NoError(t, err)
	group := aman.RunwayGroupID("ARRIVAL-04L")
	tudlo, kor, runway := navdata.FixID("TUDLO"), navdata.FixID("KOR"), navdata.FixID("RWY-04L")
	snapshot := navdata.ActiveGeometrySnapshot{
		Manifest: navdata.ManifestCandidate{Airport: "EKCH", Version: version, TerminalDigest: "terminal"}, ManifestRevision: 243,
		Fixes: []navdata.Fix{
			{ID: tudlo, Position: navdata.Coordinate{LatitudeDeg: 55.275833, LongitudeDeg: 10.647778}},
			{ID: kor, Position: navdata.Coordinate{LatitudeDeg: 55.439364, LongitudeDeg: 11.631531}},
			{ID: runway, Position: navdata.Coordinate{LatitudeDeg: 55.6, LongitudeDeg: 12.6}},
		},
		TerminalPaths: []navdata.TerminalPath{{Airport: "EKCH", FeederFix: kor, RunwayGroup: group, Legs: []navdata.ProcedureLeg{
			{ID: "TO-KOR", PathTerminator: navdata.PathTF, FromFix: &tudlo, ToFix: &kor},
			{ID: "TO-RUNWAY", PathTerminator: navdata.PathTF, FromFix: &kor, ToFix: &runway},
		}}},
	}
	at := time.Date(2026, 11, 2, 19, 22, 6, 0, time.UTC)
	priorAt := time.Date(2026, 11, 2, 18, 48, 8, 0, time.UTC)
	track := 61.86525366453225
	input := trajectory.Input{Airport: "EKCH", FeederFix: kor, RunwayGroup: group, FlightPlanRevision: 3,
		Prior:       &aman.RouteProgress{GeometryDigest: original.Digest, ManifestRevision: 243, TerminalDigest: "terminal", FlightPlanRevision: 3, RunwayGroupID: group, LegIndex: 5, RejoinLegIndex: 10, AlongTrackNM: 109.82127251462116, ObservedAt: &priorAt, RecoveryCandidateFix: "KOR", RecoveryCandidateSamples: 2},
		Observation: aman.SurveillanceFact{LatitudeDegrees: 55.09381, LongitudeDegrees: 10.47194, ObservedAt: &at, TrackTrueDegrees: &track},
	}
	for sample := 0; sample < 3; sample++ {
		observedAt := at.Add(time.Duration(sample) * 10 * time.Second)
		input.Observation.ObservedAt = &observedAt
		input.Observation.LatitudeDegrees = 55.09381 + float64(sample)*.009
		input.Observation.LongitudeDegrees = 10.47194 + float64(sample)*.026
		result := trajectory.Reduce(snapshot, route, input, trajectory.Config{ReferenceTime: observedAt, MaxObservationAge: 2 * time.Minute})
		if sample < 2 {
			require.Nil(t, result.DistanceToGoNM)
			require.NotNil(t, result.Recovery)
			input.Recovery = result.Recovery
		} else {
			require.NotNil(t, result.DistanceToGoNM)
			require.Less(t, *result.DistanceToGoNM, 150.0)
			require.True(t, result.ReacquiredProgress)
			require.NotContains(t, result.Reasons, "FORWARD_PROGRESS_OUT_OF_RANGE")
		}
	}
}

func TestSAS43MRouteResolutionRevalidates304AndAcceptsLaterCorrection(t *testing.T) {
	body, err := os.ReadFile("testdata/sas43m-route.json")
	require.NoError(t, err)
	var corrected atomic.Bool
	var conditional atomic.Int32
	server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/airac/current":
			writeJSON(w, cycleJSON())
		case "/api/v1/routes/parse":
			if r.Header.Get("If-None-Match") == "bad-spy" {
				conditional.Add(1)
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", "bad-spy")
			_, _ = w.Write(body)
		case "/api/v1/navaids/SPY":
			candidates := []any{
				map[string]any{"identifier": "SPY", "region": "EH", "type": map[string]any{"code": "DME"}, "coordinates": map[string]any{"lat": 0, "lon": 0}},
				map[string]any{"identifier": "SPY", "region": "PA", "coordinates": map[string]any{"lat": 57.157, "lon": -170.232992}},
			}
			if corrected.Load() {
				// Synthetic valid source coordinate; production must never hardcode it.
				candidates = append(candidates, map[string]any{"identifier": "SPY", "region": "EH", "coordinates": map[string]any{"lat": 52.5, "lon": 4.8}})
			}
			writeJSON(w, map[string]any{"data": candidates})
		case "/api/v1/airways/N873":
			fix := map[string]any{"fix_identifier": "SPY"}
			if corrected.Load() {
				fix["fix_coordinates"] = map[string]any{"lat": 52.5, "lon": 4.8}
			}
			writeJSON(w, map[string]any{"data": map[string]any{"segments": []any{map[string]any{"fix_identifier": "EKROS"}, fix}}})
		default:
			t.Errorf("unexpected regional lookup %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	adapter := testAdapter(t, server.URL, Config{})
	version, err := adapter.LatestVersion(t.Context())
	require.NoError(t, err)
	query := routeQuery(version)
	query.Origin, query.ArrivalProcedure, query.FiledRoute = "EBBR", nil, "NIK L191 VICOT N873 SPY DCT DOSUR P729 TUDLO"
	first, err := adapter.Resolve(t.Context(), query)
	require.NoError(t, err)
	require.True(t, navdata.HasRouteGaps(first))
	require.Equal(t, navdata.CoveragePartial, first.Coverage)
	require.Contains(t, first.Unresolved, "AMBIGUOUS_ROUTE_FIX:SPY")
	require.Less(t, first.TotalDistanceNM, 200.0, "bad excursion is never treated as usable distance")
	second, err := adapter.Resolve(t.Context(), query)
	require.NoError(t, err)
	require.Equal(t, first.Digest, second.Digest)
	corrected.Store(true)
	repaired, err := adapter.Resolve(t.Context(), query)
	require.NoError(t, err)
	require.False(t, navdata.HasRouteGaps(repaired))
	require.Empty(t, navdata.SuspectRouteDetours(repaired.Legs))
	require.Equal(t, navdata.CoverageComplete, repaired.Coverage)
	require.NotEqual(t, first.Digest, repaired.Digest)
	require.InDelta(t, 52.5, repaired.Legs[5].ToPosition.LatitudeDeg, .00001)
	require.InDelta(t, repaired.Legs[5].ToPosition.LongitudeDeg, repaired.Legs[6].FromPosition.LongitudeDeg, .00001)
	require.Less(t, repaired.TotalDistanceNM, 500.0)
	require.EqualValues(t, 2, conditional.Load())
}

func TestSAS43MRouteDoesNotChooseBetweenMultiplePlausibleSPYCandidates(t *testing.T) {
	body, err := os.ReadFile("testdata/sas43m-route.json")
	require.NoError(t, err)
	server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/airac/current":
			writeJSON(w, cycleJSON())
		case "/api/v1/routes/parse":
			_, _ = w.Write(body)
		case "/api/v1/navaids/SPY":
			writeJSON(w, map[string]any{"data": []any{
				map[string]any{"identifier": "SPY", "coordinates": map[string]any{"lat": 52.5, "lon": 4.8}},
				map[string]any{"identifier": "SPY", "coordinates": map[string]any{"lat": 52.6, "lon": 4.9}},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	adapter := testAdapter(t, server.URL, Config{})
	version, err := adapter.LatestVersion(t.Context())
	require.NoError(t, err)
	query := routeQuery(version)
	query.ArrivalProcedure = nil
	result, err := adapter.Resolve(t.Context(), query)
	require.NoError(t, err)
	require.True(t, navdata.HasRouteGaps(result))
}

func TestMissingIntendedFixCannotSelectNearbyWrongRegion(t *testing.T) {
	body, err := os.ReadFile("testdata/sas43m-route.json")
	require.NoError(t, err)
	for _, knownRegion := range []string{"", "EH"} {
		t.Run("native_region_"+knownRegion, func(t *testing.T) {
			var response routeResponse
			require.NoError(t, json.Unmarshal(body, &response))
			response.Data.Segments[5].To.Region = knownRegion
			server := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/navaids/SPY" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				writeJSON(w, map[string]any{"data": []any{
					map[string]any{"identifier": "SPY", "region": "EH", "type": map[string]any{"code": "DME"}, "coordinates": map[string]any{"lat": 0, "lon": 0}},
					map[string]any{"identifier": "SPY", "region": "EB", "coordinates": map[string]any{"lat": 52.5, "lon": 4.8}},
				}})
			})
			adapter := testAdapter(t, server.URL, Config{})
			require.NoError(t, adapter.resolveRouteDetours(t.Context(), &response.Data))
			require.Equal(t, "SPY", response.Data.Segments[5].GapFix)
			require.Equal(t, "SPY", response.Data.Segments[6].GapFix)
		})
	}
}
