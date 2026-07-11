package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitializeStandAssignmentLoadsAircraftReferenceWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	ekchDir := filepath.Join(dir, "ekch")
	require.NoError(t, os.Mkdir(ekchDir, 0o755))
	aircraftData, err := os.ReadFile(filepath.Join("..", "..", "config", "ekch", "GRpluginAircraftInfo.txt"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "GRpluginAircraftInfo.txt"), aircraftData, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "GRpluginStands.txt"), []byte("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\n"), 0o644))

	originalConfigDir := standAssignmentConfigDir
	standAssignmentConfigDir = func() string { return dir }
	t.Cleanup(func() {
		standAssignmentConfigDir = originalConfigDir
		InitializeStandAssignment(false)
	})

	state := InitializeStandAssignment(true)
	require.True(t, state.Enabled)
	require.True(t, state.Ready)
	assert.Empty(t, state.Reason)

	facts, ok := GetAircraftReference().Lookup("32N")
	require.True(t, ok)
	assert.Equal(t, "A20N", facts.Type)
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
	require.NoError(t, os.WriteFile(filepath.Join(ekchDir, "GRpluginStands.txt"), []byte("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\nBLOCKS:A99\n"), 0o644))

	originalConfigDir := standAssignmentConfigDir
	standAssignmentConfigDir = func() string { return dir }
	t.Cleanup(func() {
		standAssignmentConfigDir = originalConfigDir
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
