package efb

import (
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/metar"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func arrivalTestConfig(t *testing.T) terminal.Configuration {
	t.Helper()
	config, err := terminal.LoadFile(filepath.Join("..", "..", "config", "aman", "ekch-terminal-2611.json"))
	if err != nil {
		t.Fatal(err)
	}
	config.ApplicabilityFrom, config.ApplicabilityUntil = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	return config
}

func TestArrivalRunwayFromATIS(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"EKCH ARR ATIS C\nRWY IN USE 22L\nWIND 040/12 KT", "22L"},
		{"ARRIVAL RUNWAY 04L. DEPARTURE RUNWAY 04R", "04L"},
		{"ILS APPROACH RWY 22R", "22R"},
		{"LANDING RWY 30", "30"},
		{"RUNWAY 22L CONDITION REPORT AT 1750Z", ""},
		{"ARRIVAL RUNWAYS 04L AND 04R", ""},
		{"RWY IN USE 22L\nRWY IN USE 04L", ""},
		{"DEPARTURE RUNWAY 04R", ""},
		{"DEPARTURE RWY IN USE 04R", ""},
		{"DEPARTURE RWY IN USE 04R\nRWY IN USE 04R", "04R"},
		{"ATIS C WIND 040/12 KT QNH 1022", ""},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got := arrivalRunwayFromATIS(tc.text)
			if tc.want == "" && got != nil || tc.want != "" && (got == nil || *got != tc.want) {
				t.Fatalf("runway=%v, want %q", got, tc.want)
			}
		})
	}
}

func TestFiledArrivalFeederUsesOnlyFinalWaypoint(t *testing.T) {
	config := arrivalTestConfig(t)
	for _, tc := range []struct{ route, want string }{
		{"N0450F350 DCT TUDLO/N0250F100 DCT EKCH", "TUDLO"},
		{"DCT MONAK", "MONAK"},
		{"DCT TESPI N0250F100", "TESPI"},
		{"DCT TIDVU", "TIDVU"},
		{"DCT ERNOV", "ERNOV"},
		{"TUDLO DCT UNKNOWN", ""},
		{"TUDLO DCT 5500N01200E", ""},
		{"DCT EKCH", ""},
		{"", ""},
	} {
		t.Run(tc.route, func(t *testing.T) {
			got := filedArrivalFeeder(config, tc.route, "EKCH")
			if tc.want == "" && got != nil || tc.want != "" && (got == nil || *got != tc.want) {
				t.Fatalf("feeder=%v, want %q", got, tc.want)
			}
		})
	}
}

func TestArrivalGuidanceFallbackAndPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, star, runway, destination, wantStar, wantRunway, terminal string
		stale, expired                                                  bool
		expected                                                        bool
	}{
		{name: "route and ATIS", destination: "EKCH", wantStar: "TUDLO", wantRunway: "22L", terminal: "ABEGI", expected: true},
		{name: "ATC overrides both", destination: "EKCH", star: "MONAK5A", runway: "04L", wantStar: "MONAK5A", wantRunway: "04L", terminal: "DOPEM"},
		{name: "assigned STAR retains version", destination: "EKCH", star: "MONAK5C", wantStar: "MONAK5C", wantRunway: "22L", terminal: "ADOVI", expected: true},
		{name: "stale ATIS", destination: "EKCH", stale: true, wantStar: "TUDLO", expected: true},
		{name: "other destination", destination: "EDDF"},
		{name: "expired AIRAC", destination: "EKCH", expired: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := arrivalTestConfig(t)
			if tc.expired {
				config.ApplicabilityUntil = time.Now().Add(-time.Second)
			}
			api := NewWebAPI(WebAPIConfig{Terminal: config})
			route := "N0450F350 DCT TUDLO DCT EKCH"
			got := snapshot{Phase: "ARRIVAL", Destination: tc.destination, Route: &route, STAR: nonEmptyString(&tc.star), Runway: nonEmptyString(&tc.runway),
				ATIS: &metar.ATIS{Text: []string{"RWY IN USE 22L"}, Stale: tc.stale}}
			api.addArrivalGuidance(context.Background(), &got)
			value := func(v *string) string {
				if v == nil {
					return ""
				}
				return *v
			}
			if value(got.STAR) != tc.wantStar || value(got.Runway) != tc.wantRunway || value(got.TerminalFix) != tc.terminal || got.ArrivalGuidanceExpected != tc.expected {
				t.Fatalf("unexpected guidance: %+v", got)
			}
			if tc.terminal != "" && (got.PublishedHeading == nil || got.PublishedHoldingFix == nil || got.PublishedHoldingDetail == nil) {
				t.Fatal("published arrival fields must be populated together")
			}
			if tc.expired && len(got.DataErrors) == 0 {
				t.Fatal("expired data must have an explicit warning")
			}
		})
	}
}

func TestNetworkArrivalIncludesAuthoritativeStripGuidance(t *testing.T) {
	star, runway := "MONAK5A", "04L"
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Terminal: arrivalTestConfig(t),
		Network: networkTestFlight(t, true, time.Now(), "SAS99X", "EDDF", "EKCH"),
		Viff:    &networkTimingStub{}, ATIS: &atisLookupStub{},
		Flights: &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{Callsign: "SAS99X", Origin: "EDDF", Destination: "EKCH", Star: &star, Runway: &runway}}},
	})
	req := httptest.NewRequest(http.MethodGet, "/efb/flight", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleFlight(rec, req)
	var got snapshot
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}

	if got.STAR == nil || *got.STAR != star || got.Runway == nil || *got.Runway != runway || got.TerminalFix == nil || *got.TerminalFix != "DOPEM" || got.ArrivalGuidanceExpected {
		t.Fatalf("ATC guidance must override ATIS and route: %s", rec.Body.String())
	}
}

func TestNetworkArrivalFallbackWithoutStrip(t *testing.T) {
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Terminal: arrivalTestConfig(t),
		Network: networkTestFlightWithRoute(t, true, time.Now(), "SAS99X", "EDDF", "EKCH", "DCT TUDLO/N0250F100 DCT EKCH"),
		Viff:    &networkTimingStub{}, ATIS: &atisLookupStub{},
		Flights: &finderStub{err: pdc.ErrWebStripNotFound},
	})
	req := httptest.NewRequest(http.MethodGet, "/efb/flight", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleFlight(rec, req)
	var got snapshot
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if got.STAR == nil || *got.STAR != "TUDLO" || got.Runway == nil || *got.Runway != "22L" || got.TerminalFix == nil || *got.TerminalFix != "ABEGI" || !got.ArrivalGuidanceExpected || got.Stand != nil {
		t.Fatalf("expected explicitly inferred guidance, not a fabricated STAR version or stand: %s", rec.Body.String())
	}
}

type metarLookupStub struct{ raw, airport string }

func (s *metarLookupStub) GetMETAR(_ context.Context, airport string) string {
	s.airport = airport
	return s.raw
}

func TestMETARWind(t *testing.T) {
	for _, tc := range []struct {
		raw              string
		direction, speed int
		ok               bool
	}{
		{"EKCH 191150Z 24012KT 9999 FEW030 08/04 Q1012 NOSIG", 240, 12, true},
		{"EKCH 191150Z 05015G28KT CAVOK 08/04 Q1012", 50, 15, true},
		{"EKCH 191150Z 12006MPS 9999 08/04 Q1012", 120, 12, true},
		{"EKCH 191150Z VRB02KT CAVOK 08/04 Q1012", 0, 0, false},
		{"EKCH 191150Z 00000KT CAVOK 08/04 Q1012", 0, 0, false},
		{"", 0, 0, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			direction, speed, ok := metarWind(tc.raw)
			if direction != tc.direction || speed != tc.speed || ok != tc.ok {
				t.Fatalf("wind=%d/%d ok=%v, want %d/%d ok=%v", direction, speed, ok, tc.direction, tc.speed, tc.ok)
			}
		})
	}
}

func TestHeadwindRunway(t *testing.T) {
	config := arrivalTestConfig(t)
	for _, tc := range []struct {
		direction, speed int
		want             string
	}{
		{240, 12, "22L"},
		{200, 8, "22L"},
		{40, 10, "04L"},
		{80, 10, "04L"},
		{120, 25, "12"},
		{120, 15, "04L"},
		{300, 15, "22L"},
		{90, 25, "04L"},
		{90, 30, "12"},
		{270, 30, "30"},
		{300, 25, "30"},
		{360, 5, "04L"},
	} {
		got := headwindRunway(config, tc.direction, tc.speed)
		if got == nil || *got != tc.want {
			t.Fatalf("wind %03d/%d: runway=%v, want %s", tc.direction, tc.speed, got, tc.want)
		}
	}
}

func TestArrivalGuidanceUsesWindWhenATISHasNoRunway(t *testing.T) {
	for _, tc := range []struct {
		name, metar, wantRunway string
		atis                    *metar.ATIS
		expected                bool
	}{
		{name: "no ATIS online", metar: "EKCH 191150Z 22015KT 9999 Q1012", wantRunway: "22L", expected: true},
		{name: "stale ATIS", atis: &metar.ATIS{Text: []string{"RWY IN USE 22L"}, Stale: true}, metar: "EKCH 191150Z 04010KT 9999 Q1012", wantRunway: "04L", expected: true},
		{name: "ATIS wins over wind", atis: &metar.ATIS{Text: []string{"RWY IN USE 22R"}}, metar: "EKCH 191150Z 04010KT 9999 Q1012", wantRunway: "22R", expected: true},
		{name: "variable wind is not guessed", metar: "EKCH 191150Z VRB02KT 9999 Q1012", expected: true},
		{name: "no METAR", expected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &metarLookupStub{raw: tc.metar}
			api := NewWebAPI(WebAPIConfig{Terminal: arrivalTestConfig(t), METAR: lookup})
			route := "N0450F350 DCT TUDLO DCT EKCH"
			got := snapshot{Phase: "ARRIVAL", Destination: "EKCH", Route: &route, ATIS: tc.atis}
			api.addArrivalGuidance(context.Background(), &got)
			runway := ""
			if got.Runway != nil {
				runway = *got.Runway
			}
			if runway != tc.wantRunway || got.ArrivalGuidanceExpected != tc.expected || got.STAR == nil || *got.STAR != "TUDLO" {
				t.Fatalf("unexpected guidance: runway=%q expected=%v star=%v", runway, got.ArrivalGuidanceExpected, got.STAR)
			}
			if tc.wantRunway != "" && got.TerminalFix == nil {
				t.Fatal("a wind-derived runway must still resolve the published terminal fix")
			}
		})
	}
}

type staleATISStub struct{}

func (staleATISStub) GetATIS(string, bool) *metar.ATIS {
	return &metar.ATIS{Code: "C", Text: []string{"RWY IN USE 22L"}, Stale: true}
}

func TestFlightIncludesMETARWhenATISIsOffline(t *testing.T) {
	const raw = "EKCH 191150Z 24012KT 9999 FEW030 08/04 Q1012 NOSIG"
	for _, tc := range []struct {
		name      string
		atis      ATISLookup
		wantMETAR bool
	}{
		{name: "no ATIS online", wantMETAR: true},
		{name: "stale ATIS", atis: staleATISStub{}, wantMETAR: true},
		{name: "fresh ATIS", atis: &atisLookupStub{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &metarLookupStub{raw: raw}
			api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Terminal: arrivalTestConfig(t), METAR: lookup, ATIS: tc.atis,
				Network: networkTestFlightWithRoute(t, true, time.Now(), "SAS99X", "EDDF", "EKCH", "DCT TUDLO DCT EKCH"),
				Viff:    &networkTimingStub{}, Flights: &finderStub{err: pdc.ErrWebStripNotFound},
			})
			req := httptest.NewRequest(http.MethodGet, "/efb/flight", nil)
			req.Header.Set("Authorization", "Bearer token")
			rec := httptest.NewRecorder()
			api.handleFlight(rec, req)
			var got snapshot
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			if tc.wantMETAR && (got.METAR == nil || *got.METAR != raw || lookup.airport != "EKCH") {
				t.Fatalf("expected the destination METAR as ATIS fallback: %s", rec.Body.String())
			}
			if !tc.wantMETAR && got.METAR != nil {
				t.Fatalf("a fresh ATIS must not be replaced by the METAR: %s", rec.Body.String())
			}
		})
	}
}
