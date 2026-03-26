package cdm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestConfigStore_DepartureRestrictionOverridesDefaultRate(t *testing.T) {
	t.Parallel()

	store := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{Rate: 20, RateLvo: 14, TaxiMinutes: 10}, nil)

	// Without any restriction, ConfigForAirport returns nil for an unknown airport.
	// Seed a config via mergeRatesLocked so the airport exists.
	store.mu.Lock()
	store.mergeRatesLocked([]CdmRate{{Airport: "EKCH"}})
	store.mu.Unlock()

	cfg := store.ConfigForAirport("EKCH")
	if cfg == nil {
		t.Fatal("expected config to exist after seeding")
	}
	if cfg.DefaultRate != 20 {
		t.Fatalf("expected default rate 20, got %d", cfg.DefaultRate)
	}

	// Apply a restriction that overrides the rate.
	store.applyDepartureRestrictions([]DepartureRestriction{
		{Airport: "EKCH", Rate: 12},
	})

	cfg = store.ConfigForAirport("EKCH")
	if cfg.DefaultRate != 12 {
		t.Fatalf("expected overridden rate 12, got %d", cfg.DefaultRate)
	}
	if cfg.DefaultRateLvo != 12 {
		t.Fatalf("expected overridden LVO rate 12, got %d", cfg.DefaultRateLvo)
	}

	// Clearing restrictions (empty list) restores original rate.
	store.applyDepartureRestrictions(nil)
	cfg = store.ConfigForAirport("EKCH")
	if cfg.DefaultRate != 20 {
		t.Fatalf("expected restored rate 20 after clearing restrictions, got %d", cfg.DefaultRate)
	}
}

func TestParseRateData_SourceFormat(t *testing.T) {
	t.Parallel()

	rates, err := parseRateData([]byte("EKCH:cfg:22L::dep:04L:22R:22R:24_18\n"))
	if err != nil {
		t.Fatalf("parseRateData returned error: %v", err)
	}

	if len(rates) != 1 {
		t.Fatalf("expected 1 rate entry, got %d", len(rates))
	}

	rate := rates[0]
	if rate.Airport != "EKCH" {
		t.Fatalf("expected airport EKCH, got %q", rate.Airport)
	}
	if len(rate.ArrRwyYes) != 1 || rate.ArrRwyYes[0] != "22L" {
		t.Fatalf("unexpected arrival runway matchers: %#v", rate.ArrRwyYes)
	}
	if len(rate.DepRwyYes) != 1 || rate.DepRwyYes[0] != "04L" {
		t.Fatalf("unexpected departure runway matchers: %#v", rate.DepRwyYes)
	}
	if len(rate.DependentRwy) != 1 || rate.DependentRwy[0] != "22R" {
		t.Fatalf("unexpected dependent runways: %#v", rate.DependentRwy)
	}
	if len(rate.Rates) != 1 || rate.Rates[0] != "24" {
		t.Fatalf("unexpected rates: %#v", rate.Rates)
	}
	if len(rate.RatesLvo) != 1 || rate.RatesLvo[0] != "18" {
		t.Fatalf("unexpected LVO rates: %#v", rate.RatesLvo)
	}
}

func TestParseSidIntervalData_SourceFormat(t *testing.T) {
	t.Parallel()

	intervals, err := parseSidIntervalData([]byte("EKCH,04L,MIKLA1A,NEXEN1A,2.5\n"))
	if err != nil {
		t.Fatalf("parseSidIntervalData returned error: %v", err)
	}

	if len(intervals) != 1 {
		t.Fatalf("expected 1 interval entry, got %d", len(intervals))
	}

	interval := intervals[0]
	if interval.Airport != "EKCH" || interval.Runway != "04L" || interval.Sid1 != "MIKLA1A" || interval.Sid2 != "NEXEN1A" || interval.Value != 2.5 {
		t.Fatalf("unexpected interval: %#v", interval)
	}
}

func TestParseTaxiZoneData_SourceFormat(t *testing.T) {
	t.Parallel()

	zones, err := parseTaxiZoneData([]byte("EKCH:04L:55.1:12.1:55.2:12.2:55.3:12.3:55.4:12.4:12:7,9\n"))
	if err != nil {
		t.Fatalf("parseTaxiZoneData returned error: %v", err)
	}

	if len(zones) != 1 {
		t.Fatalf("expected 1 taxi zone, got %d", len(zones))
	}

	zone := zones[0]
	if zone.Airport != "EKCH" || zone.Runway != "04L" || zone.Minutes != 12 {
		t.Fatalf("unexpected taxi zone: %#v", zone)
	}
	if len(zone.Polygon) != 4 {
		t.Fatalf("expected 4 polygon points, got %d", len(zone.Polygon))
	}
	if len(zone.RemoteTaxiMinutes) != 2 || zone.RemoteTaxiMinutes[0] != 7 || zone.RemoteTaxiMinutes[1] != 9 {
		t.Fatalf("unexpected remote taxi minutes: %#v", zone.RemoteTaxiMinutes)
	}
}

// ---- fetchBytes ----

func TestFetchBytes_ReadsLocalFile(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp("", "cdm_test_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())

	content := []byte("EKCH:cfg:22L::dep:04L:22R:22R:24_18\n")
	if _, err := f.Write(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	f.Close()

	store := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil)
	got, err := store.fetchBytes(context.Background(), f.Name())
	if err != nil {
		t.Fatalf("fetchBytes returned error: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("fetchBytes returned %q, want %q", string(got), string(content))
	}
}

func TestFetchBytes_HTTPURLDelegatesToHTTPClient(t *testing.T) {
	t.Parallel()

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	store := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, srv.Client())
	got, err := store.fetchBytes(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetchBytes returned error: %v", err)
	}
	if !called {
		t.Error("expected HTTP server to be called")
	}
	if string(got) != "hello" {
		t.Errorf("fetchBytes returned %q, want %q", string(got), "hello")
	}
}

// ---- SeedAirportConfig ----

func TestSeedAirportConfig_SetsRatesAndDeice(t *testing.T) {
	t.Parallel()

	store := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil)
	deice := CdmDeiceConfig{
		Light:  3,
		Medium: 5,
		Heavy:  7,
		Super:  9,
		Platform: []CdmDeicePlatformConfig{
			{Name: "A", Time: 5},
		},
	}

	store.SeedAirportConfig("EKCH", 40, 10, deice)

	cfg := store.ConfigForAirport("EKCH")
	if cfg == nil {
		t.Fatal("expected config to exist after seeding")
	}
	if cfg.DefaultRate != 40 {
		t.Errorf("DefaultRate = %d, want 40", cfg.DefaultRate)
	}
	if cfg.DefaultRateLvo != 10 {
		t.Errorf("DefaultRateLvo = %d, want 10", cfg.DefaultRateLvo)
	}
	if cfg.DeiceConfig.Light != 3 {
		t.Errorf("DeiceConfig.Light = %d, want 3", cfg.DeiceConfig.Light)
	}
	if cfg.DeiceConfig.Super != 9 {
		t.Errorf("DeiceConfig.Super = %d, want 9", cfg.DeiceConfig.Super)
	}
	if len(cfg.DeiceConfig.Platform) != 1 || cfg.DeiceConfig.Platform[0].Name != "A" {
		t.Errorf("DeiceConfig.Platform = %v, want [{A 5}]", cfg.DeiceConfig.Platform)
	}
}

func TestSeedAirportConfig_ZeroRateDoesNotOverride(t *testing.T) {
	t.Parallel()

	store := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{Rate: 20, RateLvo: 14}, nil)

	// Seed with zero rates — should keep defaults set during NewCdmConfigStore.
	store.SeedAirportConfig("EKCH", 0, 0, CdmDeiceConfig{})

	cfg := store.ConfigForAirport("EKCH")
	if cfg == nil {
		t.Fatal("expected config to exist after seeding")
	}
	if cfg.DefaultRate != 20 {
		t.Errorf("DefaultRate = %d, want 20 (default preserved)", cfg.DefaultRate)
	}
	if cfg.DefaultRateLvo != 14 {
		t.Errorf("DefaultRateLvo = %d, want 14 (default preserved)", cfg.DefaultRateLvo)
	}
}
