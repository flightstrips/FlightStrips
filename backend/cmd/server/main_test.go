package main

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/navigation"
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestInitializeEnvironmentConfiguresLoggingFromDotEnv(t *testing.T) {
	originalLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	originalLogLevel, hadLogLevel := os.LookupEnv("LOG_LEVEL")
	if err := os.Unsetenv("LOG_LEVEL"); err != nil {
		t.Fatalf("unset LOG_LEVEL: %v", err)
	}
	t.Cleanup(func() {
		if hadLogLevel {
			_ = os.Setenv("LOG_LEVEL", originalLogLevel)
		} else {
			_ = os.Unsetenv("LOG_LEVEL")
		}
	})

	t.Chdir(t.TempDir())
	if err := os.WriteFile(".env", []byte("LOG_LEVEL=ERROR\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	initializeEnvironment()

	handler := slog.Default().Handler()
	if handler.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("logger enabled warnings below the .env ERROR level")
	}
	if !handler.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("logger did not enable errors at the .env ERROR level")
	}
}

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
	for _, key := range []string{"AMAN_MODE", "AMAN_SOURCE_MODE", "AMAN_ENABLED_AIRPORTS", "AMAN_RECONCILIATION_INTERVAL", "AMAN_SURVEILLANCE_INTERVAL", "ENABLE_AMAN_EUROSCOPE_GAIN_LOSE_TAGS", "ENABLE_AMAN_HOLDING_EAT_WRITEBACK"} {
		t.Setenv(key, "")
	}
	config, err := amanConfigFromEnv()
	if err != nil {
		t.Fatalf("amanConfigFromEnv() error = %v", err)
	}
	if config.Mode != aman.ModeDisabled {
		t.Fatalf("AMAN mode = %q, want disabled", config.Mode)
	}
	if config.SourceMode != aman.ObservationSourceHybrid {
		t.Fatalf("AMAN source mode = %q, want hybrid", config.SourceMode)
	}
}

func TestAMANConfigFromEnvParsesConfiguredRuntime(t *testing.T) {
	t.Setenv("AMAN_MODE", "authoritative")
	t.Setenv("AMAN_SOURCE_MODE", "euroscope")
	t.Setenv("AMAN_ENABLED_AIRPORTS", "EKCH,EKRN")
	t.Setenv("AMAN_RECONCILIATION_INTERVAL", "21s")
	t.Setenv("AMAN_SURVEILLANCE_INTERVAL", "34s")
	t.Setenv("ENABLE_AMAN_EUROSCOPE_GAIN_LOSE_TAGS", "true")
	t.Setenv("ENABLE_AMAN_HOLDING_EAT_WRITEBACK", "true")

	config, err := amanConfigFromEnv()
	if err != nil {
		t.Fatalf("amanConfigFromEnv() error = %v", err)
	}
	if config.Mode != aman.ModeAuthoritative || config.SourceMode != aman.ObservationSourceEuroScope || len(config.EnabledAirports) != 2 || config.ReconciliationInterval != 21*time.Second || config.SurveillanceInterval != 34*time.Second || !config.EnableEuroScopeGainLoseTags || !config.EnableHoldingEATWriteback {
		t.Fatalf("unexpected AMAN config: %#v", config)
	}
}

func TestNavigationConfigFromEnvParsesConfiguredSource(t *testing.T) {
	t.Setenv("NAVIGATION_SOURCE", " AIRACNET ")

	config, err := navigationConfigFromEnv()
	if err != nil {
		t.Fatalf("navigationConfigFromEnv() error = %v", err)
	}
	if config.Source != "airacnet" || config.TerminalGeometryPath != navigation.DefaultTerminalGeometryPath {
		t.Fatalf("unexpected navigation config: %#v", config)
	}
}

func TestNavigationConfigFromEnvDefaultsDisabled(t *testing.T) {
	t.Setenv("NAVIGATION_SOURCE", "")
	config, err := navigationConfigFromEnv()
	if err != nil {
		t.Fatalf("navigationConfigFromEnv() error = %v", err)
	}
	if config.Enabled() || config.TerminalGeometryPath != "" {
		t.Fatalf("unexpected disabled navigation config: %#v", config)
	}
}

func TestAMANConfigFromEnvRejectsInvalidTiming(t *testing.T) {
	t.Setenv("AMAN_RECONCILIATION_INTERVAL", "later")
	if _, err := amanConfigFromEnv(); err == nil {
		t.Fatal("amanConfigFromEnv() succeeded with invalid timing")
	}
}

func TestReleaseVersionUsesBuildIdentityAndEnvironmentOverride(t *testing.T) {
	previous := buildVersion
	t.Cleanup(func() { buildVersion = previous })
	buildVersion = "1.2.3+source-revision"
	t.Setenv("OTEL_SERVICE_VERSION", "")
	if got := releaseVersion(); got != buildVersion {
		t.Fatalf("build identity: %s", got)
	}
	t.Setenv("OTEL_SERVICE_VERSION", "release-override")
	if got := releaseVersion(); got != "release-override" {
		t.Fatalf("environment override: %s", got)
	}
}
