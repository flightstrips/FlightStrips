package config

import (
	"FlightStrips/internal/sat"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultStandAssignmentICAOFileUsesConfigDirectory(t *testing.T) {
	t.Setenv("GRPLUGIN_ICAO_AIRCRAFT_JSON", "")
	assert.Equal(t, filepath.Join("config", "data", "ICAO_Aircraft.json"), defaultStandAssignmentICAOFile())
}

func TestInitializeStandAssignmentLoadsAircraftReferenceWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	ekchDir := filepath.Join(dir, "ekch")
	require.NoError(t, os.Mkdir(ekchDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "GRpluginAircraftInfo.txt"), []byte("A20N\t35.8\t37.57\t11.76\t79000\tA\t32N\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "ICAO_Aircraft.json"), []byte(`[{"ICAO":"A20N","Description":"L2J","WTC":"M","IATA":"32N"}]`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "GRpluginStands.txt"), []byte("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\n"), 0o644))
	copyCommittedAirlineAssignment(t, ekchDir)

	originalConfigDir := standAssignmentConfigDir
	originalICAOFile := standAssignmentICAOFile
	standAssignmentConfigDir = func() string { return dir }
	standAssignmentICAOFile = func() string { return filepath.Join(dir, "ekch", "ICAO_Aircraft.json") }
	t.Cleanup(func() {
		standAssignmentConfigDir = originalConfigDir
		standAssignmentICAOFile = originalICAOFile
		InitializeStandAssignment(false)
	})

	state := InitializeStandAssignment(true)
	require.True(t, state.Enabled)
	require.True(t, state.Ready)
	assert.Empty(t, state.Reason)

	facts, ok := GetAircraftReference().Lookup("32N")
	require.True(t, ok)
	assert.Equal(t, "A20N", facts.Type)
	engine, ok := GetAircraftEngineReference().Lookup("32N")
	assert.True(t, ok)
	assert.Equal(t, sat.EngineJet, engine)
	assert.Equal(t, sat.BorderStatusSchengen, GetAirportCountries().BorderStatus("EKCH"))
	assert.NotNil(t, GetStandCapabilities())
}

func TestInitializeStandAssignmentDisabledDoesNotLoadAircraftReference(t *testing.T) {
	dir := t.TempDir()
	originalConfigDir := standAssignmentConfigDir
	standAssignmentConfigDir = func() string { return dir }
	t.Cleanup(func() {
		standAssignmentConfigDir = originalConfigDir
		InitializeStandAssignment(false)
	})

	// Neither SAT file exists in this directory. Disabled mode must not try to
	// open either file, and must leave no stale registries from an earlier run.
	state := InitializeStandAssignment(false)
	assert.False(t, state.Enabled)
	assert.False(t, state.Ready)
	assert.Nil(t, GetAircraftReference())
	assert.Nil(t, GetStandCapabilities())
}

func TestInitializeStandAssignmentReportsMissingConfigurationWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	originalConfigDir := standAssignmentConfigDir
	standAssignmentConfigDir = func() string { return dir }
	t.Cleanup(func() {
		standAssignmentConfigDir = originalConfigDir
		InitializeStandAssignment(false)
	})

	state := InitializeStandAssignment(true)
	assert.True(t, state.Enabled)
	assert.False(t, state.Ready)
	assert.Contains(t, state.Reason, "load aircraft reference data")
	assert.Nil(t, GetAircraftReference())
	assert.Nil(t, GetStandCapabilities())
}

func TestInitializeStandAssignmentReportsInvalidAircraftReference(t *testing.T) {
	dir := t.TempDir()
	ekchDir := filepath.Join(dir, "ekch")
	require.NoError(t, os.Mkdir(ekchDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "GRpluginAircraftInfo.txt"), []byte("A20N\tinvalid\t37\t11\t79000\tA\n"), 0o644))

	originalConfigDir := standAssignmentConfigDir
	standAssignmentConfigDir = func() string { return dir }
	t.Cleanup(func() {
		standAssignmentConfigDir = originalConfigDir
		InitializeStandAssignment(false)
	})

	state := InitializeStandAssignment(true)
	assert.True(t, state.Enabled)
	assert.False(t, state.Ready)
	assert.Contains(t, state.Reason, "line 1: invalid wingspan \"invalid\"")
	assert.Nil(t, GetAircraftReference())
}

func TestInitializeStandAssignmentIgnoresUnknownBlockTargets(t *testing.T) {
	dir := t.TempDir()
	ekchDir := filepath.Join(dir, "ekch")
	require.NoError(t, os.Mkdir(ekchDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "GRpluginAircraftInfo.txt"), []byte("A20N\t35.8\t37.57\t11.76\t79000\tA\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "ICAO_Aircraft.json"), []byte(`[{"ICAO":"A20N","Description":"L2J","WTC":"M"}]`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "GRpluginStands.txt"), []byte("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\nBLOCKS:A99\n"), 0o644))
	copyCommittedAirlineAssignment(t, ekchDir)

	originalConfigDir := standAssignmentConfigDir
	originalICAOFile := standAssignmentICAOFile
	standAssignmentConfigDir = func() string { return dir }
	standAssignmentICAOFile = func() string { return filepath.Join(dir, "ekch", "ICAO_Aircraft.json") }
	t.Cleanup(func() {
		standAssignmentConfigDir = originalConfigDir
		standAssignmentICAOFile = originalICAOFile
		InitializeStandAssignment(false)
	})

	state := InitializeStandAssignment(true)
	assert.True(t, state.Enabled)
	assert.True(t, state.Ready)
	assert.Empty(t, state.Reason)
	stand, ok := GetStandCapabilities().Lookup("EKCH", "A1")
	assert.True(t, ok)
	assert.Empty(t, stand.Blocks)
}

func TestInitializeStandAssignmentReportsInvalidAirlineAssignment(t *testing.T) {
	dir := t.TempDir()
	ekchDir := filepath.Join(dir, "ekch")
	require.NoError(t, os.Mkdir(ekchDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "GRpluginAircraftInfo.txt"), []byte("A20N\t35.8\t37.57\t11.76\t79000\tA\t32N\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "ICAO_Aircraft.json"), []byte(`[{"ICAO":"A20N","Description":"L2J","WTC":"M","IATA":"32N"}]`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "GRpluginStands.txt"), []byte("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "airline_assignment.json"), []byte(`{
  "rules": [], "stand_groups": {}, "fallback_rules": {}
}`), 0o644))

	originalConfigDir := standAssignmentConfigDir
	originalICAOFile := standAssignmentICAOFile
	standAssignmentConfigDir = func() string { return dir }
	standAssignmentICAOFile = func() string { return filepath.Join(dir, "ekch", "ICAO_Aircraft.json") }
	t.Cleanup(func() {
		standAssignmentConfigDir = originalConfigDir
		standAssignmentICAOFile = originalICAOFile
		InitializeStandAssignment(false)
	})

	state := InitializeStandAssignment(true)
	assert.True(t, state.Enabled)
	assert.False(t, state.Ready)
	assert.Contains(t, state.Reason, "airline assignment")
	assert.Nil(t, GetAirlineAssignment())
}

func copyCommittedAirlineAssignment(t *testing.T, destination string) {
	t.Helper()
	data := []byte(`{
  "rules": [{
    "callsigns": ["TEST"],
    "stands": {"tier1": {"A1": 100}}
  }],
  "stand_groups": {"Test": ["A1"]},
  "fallback_rules": {
    "airliner_default": {"stands": {"tier1": {"Test": 100}}},
    "business_vip": {"stands": {"tier1": {"Test": 100}}},
    "cargo": {"stands": {"tier1": {"Test": 100}}},
    "military": {"stands": {"tier1": {"Test": 100}}},
    "military_helicopter": {"stands": {"tier1": {"Test": 100}}},
    "helicopter": {"stands": {"tier1": {"Test": 100}}},
    "ga_private": {"stands": {"tier1": {"Test": 100}}},
    "unknown": {"stands": {"tier1": {"Test": 100}}}
  }
}`)
	require.NoError(t, os.WriteFile(filepath.Join(destination, "airline_assignment.json"), data, 0o644))
}
