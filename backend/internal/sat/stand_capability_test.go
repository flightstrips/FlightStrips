package sat

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadStandCapabilitiesParsesCapabilitiesAndKeepsVariants(t *testing.T) {
	data := `
;; comments and whitespace are ignored
STAND:ekch:A1:N055.37.42.710:E012.38.33.450:30
BLOCKS:A2,A3:36
SCHENGEN
WTC:lm
NOTWTC:h
ENGINETYPE:jt
NOTENGINETYPE:p
WINGSPAN:44
LENGTH:40
WIDTH:30
HEIGHT:12
MTOW:90000
CODE:ABC
ATYP:B77*, A20N
NOTATYP:CRJ*
MANUAL
AREA:north,south
PRIORITY:+2
CALLSIGN:SAS
NOTCALLSIGN:ICE
NOTADEP:U***

STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
NON-SCHENGEN
WTC:H
ATYP:B738*

STAND:EKCH:A2:N055.37.42.000:E012.38.33.000:20
WINGSPAN:20

STAND:EKCH:A3:N055.37.43.000:E012.38.34.000:20
`

	registry, err := LoadStandCapabilities(strings.NewReader(data))
	require.NoError(t, err)

	stand, ok := registry.Lookup("EKCH", "a1")
	require.True(t, ok)
	assert.Equal(t, []string{"A2", "A3"}, stand.Blocks)
	require.Len(t, stand.Variants, 2)

	schengen := stand.Variants[0]
	assert.Equal(t, StandBorderSchengen, schengen.BorderClass)
	assert.InDelta(t, 55.6285306, schengen.Latitude, 0.0000001)
	assert.InDelta(t, 12.642625, schengen.Longitude, 0.0000001)
	assert.Equal(t, 30.0, schengen.Radius)
	assert.Equal(t, []string{"LM"}, schengen.WTC)
	assert.Equal(t, []string{"H"}, schengen.NotWTC)
	assert.Equal(t, []string{"JT"}, schengen.EngineTypes)
	assert.Equal(t, []string{"P"}, schengen.NotEngineTypes)
	assert.Equal(t, 44.0, schengen.Wingspan)
	assert.Equal(t, 40.0, schengen.Length)
	assert.Equal(t, 30.0, schengen.Width)
	assert.Equal(t, 12.0, schengen.Height)
	assert.Equal(t, 90000.0, schengen.MTOW)
	assert.Equal(t, "ABC", schengen.Code)
	assert.Equal(t, []string{"B77*", "A20N"}, schengen.AircraftTypes)
	assert.Equal(t, []string{"CRJ*"}, schengen.NotAircraftTypes)
	assert.True(t, schengen.Manual)
	assert.Equal(t, []string{"NORTH", "SOUTH"}, schengen.Areas)

	nonSchengen := stand.Variants[1]
	assert.Equal(t, StandBorderNonSchengen, nonSchengen.BorderClass)
	assert.Equal(t, []string{"B738*"}, nonSchengen.AircraftTypes)

	stand, ok = registry.Lookup("EKCH", "A2")
	require.True(t, ok)
	assert.Len(t, stand.Variants, 1)
	assert.Equal(t, StandBorderAny, stand.Variants[0].BorderClass)
}

func TestLoadStandCapabilitiesRejectsUnknownCapabilities(t *testing.T) {
	registry, err := LoadStandCapabilities(strings.NewReader("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\nFUELTYPE:J\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 2: unknown stand capability directive \"FUELTYPE\"")
	assert.Nil(t, registry)
}

func TestLoadStandCapabilitiesRejectsUnknownBlockTargets(t *testing.T) {
	registry, err := LoadStandCapabilities(strings.NewReader("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\nBLOCKS:A99,A2\nSTAND:EKCH:A2:N055.37.42.000:E012.38.33.000:30\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stand EKCH:A1 references unknown BLOCKS target \"A99\"")
	assert.Nil(t, registry)
}

func TestLoadStandCapabilitiesRejectsConflictingGeometry(t *testing.T) {
	_, err := LoadStandCapabilities(strings.NewReader("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\nSTAND:EKCH:A1:N055.37.42.711:E012.38.33.450:30\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 2: conflicting geometry for stand EKCH:A1 (first declared on line 1)")
}

func TestLoadCommittedEKCHStandFileValidatesAllBlockReferences(t *testing.T) {
	registry, err := LoadStandCapabilityFile(filepath.Join("..", "..", "config", "ekch", "GRpluginStands.txt"))
	require.NoError(t, err)

	e20, ok := registry.Lookup("EKCH", "E20")
	require.True(t, ok)
	assert.Empty(t, e20.Blocks)

	e71, ok := registry.Lookup("EKCH", "E71")
	require.True(t, ok)
	assert.Equal(t, []string{"E70", "E72", "E73", "E74"}, e71.Blocks)
}
