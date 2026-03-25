package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- matchesControllers ----

func TestMatchesControllers_NoMutation(t *testing.T) {
	// matchesControllers must not mutate the required slice between calls.
	online := []*Position{
		{Name: "EKCH_B_GND", Frequency: "B_FREQ", Section: "GND"},
	}
	variant := []string{"EKCH_A_GND", "EKCH_B_GND"}

	// First call — should match (both B is online)
	matchesControllers(online, variant, false)

	// Second call on same slice — must still see both entries (no corruption from first call).
	assert.Equal(t, []string{"EKCH_A_GND", "EKCH_B_GND"}, variant, "required slice must not be mutated")
}

func TestMatchesControllers_OfflineAll(t *testing.T) {
	// offline=true: all required positions must be absent from controllers.
	controllers := []*Position{
		{Name: "EKCH_A_GND", Frequency: "A_FREQ", Section: "GND"},
	}
	// A is online — so "A is offline" condition fails.
	assert.False(t, matchesControllers(controllers, []string{"EKCH_A_GND"}, true))
	// B is not in controllers, so "B is offline" is satisfied.
	assert.True(t, matchesControllers(controllers, []string{"EKCH_B_GND"}, true))
}

func TestMatchesControllers_EmptyRequired(t *testing.T) {
	controllers := []*Position{{Name: "X", Frequency: "F", Section: "TWR"}}
	assert.True(t, matchesControllers(controllers, []string{}, false))
	assert.True(t, matchesControllers(controllers, []string{}, true))
}

// ---- GetLayouts — EKCH apron single-controller AAAD ----

func makePos(name, freq, section string) *Position {
	return &Position{Name: name, Frequency: freq, Section: section}
}

// ekchApronLayouts mirrors the relevant subset of the EKCH layouts config used in apron tests.
func ekchApronLayouts() map[string][]LayoutVariant {
	return map[string][]LayoutVariant{
		"EKCH_A_GND": {
			{Online: []string{"EKCH_B_GND", "EKCH_C_GND"}, Offline: []string{}, Layout: "AA"},
			{Online: []string{"EKCH_B_GND"}, Offline: []string{"EKCH_C_GND"}, Layout: "AA"},
			{Online: []string{}, Offline: []string{"EKCH_B_GND", "EKCH_C_GND"}, Layout: "AAAD"},
		},
		"EKCH_B_GND": {
			{Online: []string{"EKCH_A_GND", "EKCH_C_GND"}, Offline: []string{}, Layout: "SEQPLN"},
			{Online: []string{"EKCH_A_GND"}, Offline: []string{"EKCH_C_GND"}, Layout: "AD"},
			{Online: []string{"EKCH_C_GND"}, Offline: []string{"EKCH_A_GND"}, Layout: "AA"},
			{Online: []string{}, Offline: []string{"EKCH_A_GND", "EKCH_C_GND"}, Layout: "AAAD"},
		},
		"EKCH_C_GND": {
			{Online: []string{"EKCH_A_GND", "EKCH_B_GND"}, Offline: []string{}, Layout: "AD"},
			{Online: []string{"EKCH_B_GND"}, Offline: []string{"EKCH_A_GND"}, Layout: "AD"},
			{Online: []string{"EKCH_A_GND"}, Offline: []string{"EKCH_B_GND"}, Layout: "AD"},
			{Online: []string{}, Offline: []string{"EKCH_A_GND", "EKCH_B_GND"}, Layout: "AAAD"},
		},
	}
}

// setupApronLayouts sets the global layouts var to the EKCH apron test data and restores it on cleanup.
func setupApronLayouts(t *testing.T) {
	t.Helper()
	original := layouts
	layouts = ekchApronLayouts()
	t.Cleanup(func() { layouts = original })
}

func TestGetLayouts_SingleApronA_GetsAAD(t *testing.T) {
	setupApronLayouts(t)
	// Only EKCH_A_GND online — should get AAAD.
	controllers := []*Position{
		makePos("EKCH_A_GND", "121.630", "GND"),
	}
	active := []string{"04R", "04L"}
	result := GetLayouts(controllers, active)
	require.NotNil(t, result["121.630"], "EKCH_A_GND should have a layout")
	assert.Equal(t, "AAAD", *result["121.630"])
}

func TestGetLayouts_SingleApronB_GetsAAD(t *testing.T) {
	setupApronLayouts(t)
	// Only EKCH_B_GND online — should get AAAD.
	controllers := []*Position{
		makePos("EKCH_B_GND", "121.905", "GND"),
	}
	active := []string{"04R", "04L"}
	result := GetLayouts(controllers, active)
	require.NotNil(t, result["121.905"], "EKCH_B_GND should have a layout")
	assert.Equal(t, "AAAD", *result["121.905"])
}

func TestGetLayouts_SingleApronC_GetsAAD(t *testing.T) {
	setupApronLayouts(t)
	// Only EKCH_C_GND online — should get AAAD.
	controllers := []*Position{
		makePos("EKCH_C_GND", "121.730", "GND"),
	}
	active := []string{"04R", "04L"}
	result := GetLayouts(controllers, active)
	require.NotNil(t, result["121.730"], "EKCH_C_GND should have a layout")
	assert.Equal(t, "AAAD", *result["121.730"])
}

func TestGetLayouts_ApronAandB_SplitViews(t *testing.T) {
	setupApronLayouts(t)
	// EKCH_A_GND + EKCH_B_GND online → AA and AD split.
	controllers := []*Position{
		makePos("EKCH_A_GND", "121.630", "GND"),
		makePos("EKCH_B_GND", "121.905", "GND"),
	}
	active := []string{"04R", "04L"}
	result := GetLayouts(controllers, active)
	require.NotNil(t, result["121.630"], "EKCH_A_GND should have a layout")
	assert.Equal(t, "AA", *result["121.630"])
	require.NotNil(t, result["121.905"], "EKCH_B_GND should have a layout")
	assert.Equal(t, "AD", *result["121.905"])
}

func TestGetLayouts_AllThreeAprons_SplitViews(t *testing.T) {
	setupApronLayouts(t)
	// All three apron controllers online.
	controllers := []*Position{
		makePos("EKCH_A_GND", "121.630", "GND"),
		makePos("EKCH_B_GND", "121.905", "GND"),
		makePos("EKCH_C_GND", "121.730", "GND"),
	}
	active := []string{"04R", "04L"}
	result := GetLayouts(controllers, active)
	require.NotNil(t, result["121.630"])
	assert.Equal(t, "AA", *result["121.630"])
	require.NotNil(t, result["121.905"])
	assert.Equal(t, "SEQPLN", *result["121.905"])
	require.NotNil(t, result["121.730"])
	assert.Equal(t, "AD", *result["121.730"])
}

func TestGetLayouts_NoMutationAcrossCalls(t *testing.T) {
	setupApronLayouts(t)
	// Calling GetLayouts multiple times must produce consistent results.
	controllers := []*Position{
		makePos("EKCH_A_GND", "121.630", "GND"),
	}
	active := []string{"04R", "04L"}

	result1 := GetLayouts(controllers, active)
	result2 := GetLayouts(controllers, active)

	require.NotNil(t, result1["121.630"])
	require.NotNil(t, result2["121.630"])
	assert.Equal(t, *result1["121.630"], *result2["121.630"], "layout must be the same across repeated calls")
}
