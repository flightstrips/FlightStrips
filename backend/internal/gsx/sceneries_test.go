package gsx

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gsx_sceneries.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	cfg, err := LoadSceneryConfig(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("a missing config must not fail startup: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config")
	}
}

func TestLoadRejectsBadConfig(t *testing.T) {
	if _, err := LoadSceneryConfig(writeConfig(t, `{"release_points":{}}`)); err == nil {
		t.Error("expected an error when icao is missing")
	}
	if _, err := LoadSceneryConfig(writeConfig(t, `{"icao":`)); err == nil {
		t.Error("expected an error on malformed JSON")
	}
}
