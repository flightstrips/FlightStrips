package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetInitialCFLForRunway_ReturnsConfiguredValue(t *testing.T) {
	orig := runwayInitialCFL
	t.Cleanup(func() { runwayInitialCFL = orig })

	runwayInitialCFL = map[string]int{
		"04R": 7000,
		"22L": 7000,
		"04L": 7000,
		"22R": 7000,
		"12":  4000,
		"30":  4000,
	}

	tests := []struct {
		runway   string
		wantCFL  int
		wantOk   bool
	}{
		{"04R", 7000, true},
		{"22L", 7000, true},
		{"04L", 7000, true},
		{"22R", 7000, true},
		{"12", 4000, true},
		{"30", 4000, true},
		{"99", 0, false},
		{"", 0, false},
	}

	for _, tc := range tests {
		cfl, ok := GetInitialCFLForRunway(tc.runway)
		assert.Equal(t, tc.wantOk, ok, "runway %q ok", tc.runway)
		assert.Equal(t, tc.wantCFL, cfl, "runway %q cfl", tc.runway)
	}
}

func TestGetTransitionAltitude_ReturnsConfiguredValue(t *testing.T) {
	orig := transitionAltitude
	t.Cleanup(func() { transitionAltitude = orig })

	transitionAltitude = 5000
	assert.Equal(t, 5000, GetTransitionAltitude())
}
