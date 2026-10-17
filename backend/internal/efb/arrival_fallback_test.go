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
	config, err := terminal.LoadFile(filepath.Join("..", "..", "config", "aman", "ekch-terminal-2610.json"))
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
		{name: "assigned STAR retains version", destination: "EKCH", star: "MONAK5A", wantStar: "MONAK5A", wantRunway: "22L", terminal: "ADOVI", expected: true},
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
