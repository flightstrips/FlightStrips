package config

import (
	"strings"
	"testing"
)

func TestGetConfigDir(t *testing.T) {
	if got := GetConfigDir(); got != "config" {
		t.Errorf("GetConfigDir() = %q, want %q", got, "config")
	}
}

func TestGetCdmConfig_ReturnsEmptyWhenNotLoaded(t *testing.T) {
	original := cdmConfig
	t.Cleanup(func() { cdmConfig = original })

	cdmConfig = CdmConfig{}
	cfg := GetCdmConfig()
	if cfg.Rate != 0 || cfg.RateLvo != 0 || cfg.RateUri != "" {
		t.Errorf("expected zero CdmConfig, got %+v", cfg)
	}
}

func TestLoadAirportConfig_ParsesCdmSection(t *testing.T) {
	original := cdmConfig
	t.Cleanup(func() { cdmConfig = original })

	yaml := `
latitude: 55.6
longitude: 12.6
transition_altitude: 5000
airborne_altitude_agl: 200
cdm:
  rate: 40
  rateLvo: 10
  rateUri: ekch/rate.txt
  sidIntervalUri: ekch/sidInterval.txt
  taxizonesUri: ekch/taxizones.txt
  deice:
    light: 3
    medium: 5
    heavy: 7
    super: 9
    platform:
      - name: A
        time: 5
      - name: B
        time: 6
`
	if err := loadAirportConfig(strings.NewReader(yaml)); err != nil {
		t.Fatalf("loadAirportConfig error: %v", err)
	}

	cfg := GetCdmConfig()

	if cfg.Rate != 40 {
		t.Errorf("Rate = %d, want 40", cfg.Rate)
	}
	if cfg.RateLvo != 10 {
		t.Errorf("RateLvo = %d, want 10", cfg.RateLvo)
	}
	if cfg.RateUri != "ekch/rate.txt" {
		t.Errorf("RateUri = %q, want %q", cfg.RateUri, "ekch/rate.txt")
	}
	if cfg.SidIntervalUri != "ekch/sidInterval.txt" {
		t.Errorf("SidIntervalUri = %q, want %q", cfg.SidIntervalUri, "ekch/sidInterval.txt")
	}
	if cfg.TaxizonesUri != "ekch/taxizones.txt" {
		t.Errorf("TaxizonesUri = %q, want %q", cfg.TaxizonesUri, "ekch/taxizones.txt")
	}
	if cfg.Deice.Light != 3 {
		t.Errorf("Deice.Light = %d, want 3", cfg.Deice.Light)
	}
	if cfg.Deice.Medium != 5 {
		t.Errorf("Deice.Medium = %d, want 5", cfg.Deice.Medium)
	}
	if cfg.Deice.Heavy != 7 {
		t.Errorf("Deice.Heavy = %d, want 7", cfg.Deice.Heavy)
	}
	if cfg.Deice.Super != 9 {
		t.Errorf("Deice.Super = %d, want 9", cfg.Deice.Super)
	}
	if len(cfg.Deice.Platform) != 2 {
		t.Fatalf("len(Deice.Platform) = %d, want 2", len(cfg.Deice.Platform))
	}
	if cfg.Deice.Platform[0].Name != "A" || cfg.Deice.Platform[0].Time != 5 {
		t.Errorf("Platform[0] = %+v, want {A 5}", cfg.Deice.Platform[0])
	}
	if cfg.Deice.Platform[1].Name != "B" || cfg.Deice.Platform[1].Time != 6 {
		t.Errorf("Platform[1] = %+v, want {B 6}", cfg.Deice.Platform[1])
	}
}
