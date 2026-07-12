package sat

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAircraftReferenceParsesSixAndSevenColumnRecords(t *testing.T) {
	registry, err := LoadAircraftReference(strings.NewReader("a20n\t35.8\t37.57\t11.76\t79000\ta\t32n/32q\nB738\t35.79\t39.5\t12.5\t79015\tA\n"))
	require.NoError(t, err)

	canonical, ok := registry.Lookup("A20N")
	require.True(t, ok)
	assert.Equal(t, Aircraft{
		Type:           "A20N",
		WingspanMetres: 35.8,
		LengthMetres:   37.57,
		HeightMetres:   11.76,
		MTOWKilograms:  79000,
		UseCode:        AircraftUseCodeA,
		Aliases:        []string{"32N", "32Q"},
	}, canonical)

	alias, ok := registry.Lookup(" 32n ")
	require.True(t, ok)
	assert.Equal(t, canonical, alias)

	withoutAliases, ok := registry.Lookup("b738")
	require.True(t, ok)
	assert.Empty(t, withoutAliases.Aliases)

	withEquipment, ok := registry.Lookup("A20N/M-SDE3FGHIRWY/LB1")
	require.True(t, ok)
	assert.Equal(t, canonical, withEquipment)
}

func TestLoadAircraftReferenceRejectsInvalidRows(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		message string
	}{
		{
			name:    "duplicate canonical type",
			data:    "A20N\t35\t37\t11\t79000\tA\nA20N\t35\t37\t11\t79000\tA\n",
			message: "line 2: duplicate canonical type \"A20N\"",
		},
		{
			name:    "malformed number",
			data:    "A20N\twide\t37\t11\t79000\tA\n",
			message: "line 1: invalid wingspan \"wide\"",
		},
		{
			name:    "unknown use code",
			data:    "A20N\t35\t37\t11\t79000\tZ\n",
			message: "line 1: unknown use code \"Z\"",
		},
		{
			name:    "malformed row",
			data:    "A20N\t35\t37\n",
			message: "line 1: malformed row: expected 6 or 7 tab-separated columns, got 3",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadAircraftReference(strings.NewReader(test.data))
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.message)
		})
	}
}

func TestLoadAircraftReferenceRejectsConflictingAliases(t *testing.T) {
	registry, err := LoadAircraftReference(strings.NewReader("A20N\t35\t37\t11\t79000\tA\t32N\nB738\t35\t39\t12\t79000\tA\t32N\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 2: conflicting alias \"32N\" already declared on line 1")
	assert.Nil(t, registry)
}

func TestLoadAircraftReferenceLoadsCommittedEKCHFile(t *testing.T) {
	registry, err := LoadAircraftReferenceFile(filepath.Join("..", "..", "config", "ekch", "GRpluginAircraftInfo.txt"))
	require.NoError(t, err)

	a20n, ok := registry.Lookup("A20N")
	require.True(t, ok)
	alias, ok := registry.Lookup("32N")
	require.True(t, ok)
	assert.Equal(t, a20n, alias)
}
