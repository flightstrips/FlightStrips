package main

import (
	"FlightStrips/internal/aman"
	"os"
	"testing"
	"time"
)

func TestEnvBool(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		fallback bool
		expected bool
	}{
		{name: "unset defaults false", value: "", fallback: false, expected: false},
		{name: "true", value: "true", fallback: false, expected: true},
		{name: "false", value: "false", fallback: true, expected: false},
		{name: "malformed uses false fallback", value: "enabled", fallback: false, expected: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TEST_ENV_BOOL", test.value)
			if got := envBool("TEST_ENV_BOOL", test.fallback); got != test.expected {
				t.Fatalf("envBool() = %v, want %v", got, test.expected)
			}
		})
	}
}

func TestStandAssignmentFlagDefaultsFalseRegardlessOfEnvironment(t *testing.T) {
	t.Setenv("ENABLE_STAND_ASSIGNMENT", "")
	if got := envBool("ENABLE_STAND_ASSIGNMENT", false); got {
		t.Fatal("ENABLE_STAND_ASSIGNMENT should default to false when unset")
	}

	t.Setenv("ENVIRONMENT", "production")
	if got := envBool("ENABLE_STAND_ASSIGNMENT", false); got {
		t.Fatal("ENABLE_STAND_ASSIGNMENT should remain false in production unless explicitly enabled")
	}
}

func TestStandAssignmentEuroscopeMessagesDefaultToDisabled(t *testing.T) {
	t.Setenv("ENABLE_STAND_ASSIGNMENT_ES_MESSAGES", "")
	if got := envBool("ENABLE_STAND_ASSIGNMENT_ES_MESSAGES", false); got {
		t.Fatal("ENABLE_STAND_ASSIGNMENT_ES_MESSAGES should default to false")
	}

	t.Setenv("ENABLE_STAND_ASSIGNMENT_ES_MESSAGES", "true")
	if got := envBool("ENABLE_STAND_ASSIGNMENT_ES_MESSAGES", false); !got {
		t.Fatal("ENABLE_STAND_ASSIGNMENT_ES_MESSAGES should enable messages when explicitly set true")
	}
}

func TestEFBFlagDefaultsFalseRegardlessOfEnvironment(t *testing.T) {
	t.Setenv("ENABLE_EFB", "")
	if got := envBool("ENABLE_EFB", false); got {
		t.Fatal("ENABLE_EFB should default to false when unset")
	}

	t.Setenv("ENVIRONMENT", "production")
	if got := envBool("ENABLE_EFB", false); got {
		t.Fatal("ENABLE_EFB should remain false in production unless explicitly enabled")
	}
}

func TestStandAssignmentAircraftFilePreservesExplicitConfiguration(t *testing.T) {
	if got := standAssignmentAircraftFile(" C:/sector/ICAO_Aircraft.json "); got != "C:/sector/ICAO_Aircraft.json" {
		t.Fatalf("explicit aircraft file = %q", got)
	}
	if got := standAssignmentAircraftFile(""); got != "" {
		t.Fatalf("empty aircraft file = %q, want empty so config selects its default", got)
	}
}

func TestAMANConfigFromEnvDefaultsDisabled(t *testing.T) {
	for _, key := range []string{"AMAN_MODE", "AMAN_ENABLED_AIRPORTS", "AMAN_FMP_ROLES", "AMAN_TERMINAL_GEOMETRY_PATH", "AMAN_NAVIGATION_SOURCE", "AMAN_RECONCILIATION_INTERVAL", "AMAN_SURVEILLANCE_INTERVAL", "ENABLE_AMAN_EUROSCOPE_GAIN_LOSE_TAGS"} {
		t.Setenv(key, "")
	}
	config, err := amanConfigFromEnv()
	if err != nil {
		t.Fatalf("amanConfigFromEnv() error = %v", err)
	}
	if config.Mode != aman.ModeDisabled {
		t.Fatalf("AMAN mode = %q, want disabled", config.Mode)
	}
}

func TestAMANConfigFromEnvParsesConfiguredRuntime(t *testing.T) {
	geometry, err := os.CreateTemp(t.TempDir(), "terminal-*.geojson")
	if err != nil {
		t.Fatal(err)
	}
	if err := geometry.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AMAN_MODE", "authoritative")
	t.Setenv("AMAN_ENABLED_AIRPORTS", "EKCH,EKRN")
	t.Setenv("AMAN_FMP_ROLES", "EKCH_APP,EKCH_CTR")
	t.Setenv("AMAN_TERMINAL_GEOMETRY_PATH", geometry.Name())
	t.Setenv("AMAN_NAVIGATION_SOURCE", "airacnet")
	t.Setenv("AMAN_RECONCILIATION_INTERVAL", "21s")
	t.Setenv("AMAN_SURVEILLANCE_INTERVAL", "34s")
	t.Setenv("ENABLE_AMAN_EUROSCOPE_GAIN_LOSE_TAGS", "true")

	config, err := amanConfigFromEnv()
	if err != nil {
		t.Fatalf("amanConfigFromEnv() error = %v", err)
	}
	if config.Mode != aman.ModeAuthoritative || len(config.EnabledAirports) != 2 || len(config.FMPRoles) != 2 || config.FMPRoles[0] != "EKCH_APP" || config.ReconciliationInterval != 21*time.Second || config.SurveillanceInterval != 34*time.Second || !config.EnableEuroScopeGainLoseTags {
		t.Fatalf("unexpected AMAN config: %#v", config)
	}
}

func TestAMANConfigFromEnvRejectsInvalidTiming(t *testing.T) {
	t.Setenv("AMAN_RECONCILIATION_INTERVAL", "later")
	if _, err := amanConfigFromEnv(); err == nil {
		t.Fatal("amanConfigFromEnv() succeeded with invalid timing")
	}
}
