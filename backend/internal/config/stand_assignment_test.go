package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitializeStandAssignmentLoadsAircraftReferenceWhenEnabled(t *testing.T) {
	originalConfigDir := standAssignmentConfigDir
	standAssignmentConfigDir = func() string { return filepath.Join("..", "..", "config") }
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
}

func TestInitializeStandAssignmentDisabledDoesNotLoadAircraftReference(t *testing.T) {
	InitializeStandAssignment(true)
	state := InitializeStandAssignment(false)
	assert.False(t, state.Enabled)
	assert.False(t, state.Ready)
	assert.Nil(t, GetAircraftReference())
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
