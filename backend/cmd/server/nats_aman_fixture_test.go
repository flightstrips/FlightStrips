package main

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Same typed provider fixture as Task20's production policy test. HTTP JSON is
// parsed by real adapters; durable navigation/provider values remain Protobuf.
func newAMANHTTPFixture(t *testing.T) (string, string) {
	from := time.Now().UTC().Truncate(24 * time.Hour).Add(-24 * time.Hour)
	until := from.Add(28 * 24 * time.Hour)
	course := 221.2
	minute := int64(60)
	provenance := terminal.ProvenanceDefinition{SourceID: "fixture-aip", SourceRevision: "fixture-r1", ImportedAt: from, EffectiveFrom: from, EffectiveUntil: until}
	config := terminal.Configuration{SchemaVersion: terminal.SchemaVersion, ConfigVersion: "runtime-" + uuid.NewString(), Airport: "EKCH", ApplicabilityFrom: from, ApplicabilityUntil: until,
		Dataset: terminal.DatasetCompatibility{Cycle: "2610", EffectiveFrom: from, EffectiveUntil: until}, Sources: []terminal.Source{{ID: "fixture-aip", Document: "local HTTP fixture", EffectiveFrom: from, EffectiveUntil: until}},
		RunwayGroups: []terminal.RunwayGroup{{ID: "SOUTH", Aliases: []aman.RunwayGroupID{"22L"}, Runways: []navdata.RunwayID{"22L"}, FinalApproaches: []terminal.FinalApproachDefinition{{Runway: "22L", FinalApproachFix: "KEMAX", Threshold: terminal.ThresholdDefinition{Position: terminal.CoordinateDefinition{LatitudeDeg: 55.6254, LongitudeDeg: 12.6676}, CourseTrueDeg: &course}, CourseTrueDeg: course, PhysicalLengthM: 3302, Provenance: provenance}}}},
		Feeders:      []terminal.Feeder{{ID: "SOK"}}, Paths: []terminal.Path{{Feeder: "SOK", RunwayGroup: "SOUTH", Fixes: []navdata.FixID{"SOK", "KEMAX"}, MergeFix: "KEMAX", SelectedHolding: "SOK-HF"}},
		OverlayHoldings: []terminal.HoldingDefinition{{ID: "SOK-HF", Fix: "SOK", InboundCourseTrueDeg: 90, TurnDirection: navdata.TurnRight, LegTimeSeconds: &minute, Termination: navdata.HoldingManual, Provenance: provenance}}}
	data, err := json.Marshal(config)
	require.NoError(t, err)
	filename := filepath.Join(t.TempDir(), "terminal.json")
	require.NoError(t, os.WriteFile(filename, data, 0600))
	var airacCalls, windCalls atomic.Int32
	point := func(id string, lat, lon float64) any {
		return map[string]any{"identifier": id, "coordinates": map[string]any{"lat": lat, "lon": lon}}
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var value any
		switch {
		case r.URL.Path == "/api/v1/airac/current":
			airacCalls.Add(1)
			value = map[string]any{"data": map[string]any{"cycle": "2610", "effective_date": from.Format(time.RFC3339), "expiration_date": until.Format(time.RFC3339)}}
		case r.URL.Path == "/api/v1/airports/EKCH":
			value = map[string]any{"data": map[string]any{"icao": "EKCH", "name": "Copenhagen", "coordinates": map[string]any{"lat": 55.618, "lon": 12.656}}}
		case r.URL.Path == "/api/v1/procedures":
			kind := r.URL.Query().Get("type")
			id := map[string]string{"SID": "KEMAX3A", "STAR": "SOK1P", "APP": "ILS22L"}[kind]
			if id == "" {
				id = "SOK1P"
				kind = "STAR"
			}
			value = map[string]any{"data": []any{map[string]any{"airport": "EKCH", "identifier": id, "type": map[string]any{"code": kind}, "runway": "22L"}}, "pagination": map[string]any{"has_more": false}}
		case strings.HasPrefix(r.URL.Path, "/api/v1/procedures/EKCH/"):
			id := path.Base(r.URL.Path)
			kind := map[string]string{"KEMAX3A": "SID", "SOK1P": "STAR", "ILS22L": "APP"}[id]
			value = map[string]any{"data": map[string]any{"airport": "EKCH", "identifier": id, "type": map[string]any{"code": kind}, "available_runways": []string{"22L"}, "segments": []any{map[string]any{"sequence": 10, "path_terminator": "TF", "fix_identifier": "SOK"}, map[string]any{"sequence": 20, "path_terminator": "TF", "fix_identifier": "KEMAX"}}}}
		case strings.HasPrefix(r.URL.Path, "/api/v1/waypoints/"):
			id := path.Base(r.URL.Path)
			lat, lon := 55.4, 11.5
			if id == "KEMAX" {
				lat, lon = 55.71, 12.8
			}
			value = map[string]any{"data": point(id, lat, lon)}
		case r.URL.Path == "/api/v1/routes/parse":
			value = map[string]any{"data": map[string]any{"total_distance": 40, "segments": []any{map[string]any{"from": point("EHAM", 52.3086, 4.7639), "to": point("SOK", 55.4, 11.5), "distance": 30, "bearing": 45}, map[string]any{"from": point("SOK", 55.4, 11.5), "to": point("KEMAX", 55.71, 12.8), "distance": 10, "bearing": 90}}, "errors": []any{}}}
		case r.URL.Path == "/wind":
			windCalls.Add(1)
			hourly := map[string]any{"time": []string{time.Now().UTC().Truncate(time.Hour).Format("2006-01-02T15:04"), time.Now().UTC().Truncate(time.Hour).Add(time.Hour).Format("2006-01-02T15:04")}}
			for i, level := range []int{1000, 850, 700, 500, 300, 250, 200, 150} {
				hourly[fmt.Sprintf("wind_speed_%dhPa", level)] = []float64{20, 20}
				hourly[fmt.Sprintf("wind_direction_%dhPa", level)] = []float64{270, 270}
				hourly[fmt.Sprintf("geopotential_height_%dhPa", level)] = []float64{float64(100 + i*1800), float64(100 + i*1800)}
			}
			samples := strings.Count(r.URL.Query().Get("latitude"), ",") + 1
			values := make([]any, samples)
			for i := range values {
				values[i] = map[string]any{"hourly": hourly}
			}
			if samples == 1 {
				value = values[0]
			} else {
				value = values
			}
		default:
			http.NotFound(w, r)
			return
		}
		if envelope, ok := value.(map[string]any); ok && r.URL.Path != "/wind" {
			envelope["status"] = "success"
		}
		_ = json.NewEncoder(w).Encode(value)
	}))
	t.Cleanup(provider.Close)
	return filename, provider.URL
}
