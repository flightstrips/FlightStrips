package sector

import (
	"os"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

// Synthetic geometry; no vendor sector data is redistributed in the tests.
const sectorFixture = `; AIRAC 2611/1
[INFO]
Synthetic 2611/1
[VOR]
NAV 113.000 N055.00.00.000 E010.00.00.000
[NDB]
NDB 300.000 N055.10.00.000 E010.10.00.000
[FIXES]
START N055.00.00.000 E011.00.00.000
MID N055.30.00.000 E011.30.00.000
JOIN N056.00.00.000 E012.00.00.000
FINAL N055.45.00.000 E012.15.00.000
[AIRPORT]
EKCH 000.000 N055.37.00.000 E012.39.00.000 D
[HIGH AIRWAY]
L1 N055.00.00.000 E011.00.00.000 N055.30.00.000 E011.30.00.000
L1 N055.30.00.000 E011.30.00.000 N056.00.00.000 E012.00.00.000
[GEO]
ignored display text
`

func testConfig() terminal.Configuration {
	return terminal.Configuration{Airport: "EKCH", Dataset: terminal.DatasetCompatibility{Cycle: "2611", EffectiveFrom: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC), EffectiveUntil: time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)}, Paths: []terminal.Path{{RunwayGroup: "22L", Fixes: []navdata.FixID{"JOIN", "FINAL"}}}}
}
func testNow() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) }
func testQuery(a *Adapter, route string) navdata.RouteQuery {
	entry, group := navdata.FixID("JOIN"), aman.RunwayGroupID("22L")
	return navdata.RouteQuery{Version: a.version, Origin: "ESSA", Destination: "EKCH", FiledRoute: route, TerminalEntry: &entry, RunwayGroup: &group}
}

func TestSectorCoordinatesRoutesAndAirways(t *testing.T) {
	a, err := Read(strings.NewReader("\ufeff"+sectorFixture), testConfig(), testNow)
	require.NoError(t, err)
	require.Equal(t, 6, a.FixCount())
	for _, route := range []string{"N0450F350 START L1 JOIN", "START/N0450F350 L1 JOIN STAR1A", "START DCT MID DCT JOIN"} {
		q := testQuery(a, route)
		g, err := a.Resolve(t.Context(), q)
		require.NoError(t, err, route)
		require.NoError(t, g.Validate())
		require.Equal(t, "sector", g.Provenance.SourceID)
		require.Len(t, g.Legs, 2)
		require.Equal(t, navdata.FixID("MID"), *g.Legs[0].ToFix)
		require.Equal(t, navdata.FixID("JOIN"), *g.Legs[1].ToFix)
		require.Greater(t, g.TotalDistanceNM, 60.0)
	}
	p, err := coordinate("S012.30.30.500", "W001.15.00.000")
	require.NoError(t, err)
	require.InDelta(t, -12.50847222, p.LatitudeDeg, 0.00000001)
	require.Equal(t, -1.25, p.LongitudeDeg)
}

func TestSectorRejectsUnresolvedOrDisconnectedRoute(t *testing.T) {
	for _, route := range []string{"START UNKNOWN JOIN", "START L99 JOIN", "START L1 DCT JOIN", "START L1", "START DCT FINAL", "DCT JOIN", "NAV L1 JOIN"} {
		a, err := Read(strings.NewReader(sectorFixture), testConfig(), testNow)
		require.NoError(t, err)
		_, err = a.Resolve(t.Context(), testQuery(a, route))
		require.Error(t, err, route)
	}
	for _, extra := range []string{
		"[FIXES]\nSTART N051.00.00.000 E009.00.00.000\n",
		"[LOW AIRWAY]\nL1 N055.00.00.000 E011.00.00.000 N056.00.00.000 E012.00.00.000\n",
	} {
		a, err := Read(strings.NewReader(sectorFixture+extra), testConfig(), testNow)
		require.NoError(t, err)
		_, err = a.Resolve(t.Context(), testQuery(a, "START L1 JOIN"))
		require.Error(t, err)
	}
}

func TestSectorSupportsNamedAirwayEndpointsAndIgnoresReportingLabels(t *testing.T) {
	input := sectorFixture + "[FIXES]\nREPORT WEST N055.20.00.000 E011.20.00.000\n[LOW AIRWAY]\nL2 START START MID MID\nL2 MID MID JOIN JOIN\n"
	a, err := Read(strings.NewReader(input), testConfig(), testNow)
	require.NoError(t, err)
	_, exists := a.fixes["REPORT"]
	require.False(t, exists, "a reporting label must never create a truncated waypoint")
	g, err := a.Resolve(t.Context(), testQuery(a, "START L2 JOIN"))
	require.NoError(t, err)
	require.Len(t, g.Legs, 2)
}

func TestSectorRejectsBadDataAndCycle(t *testing.T) {
	for _, input := range []string{
		strings.ReplaceAll(sectorFixture, "2611", "2610"),
		strings.Replace(sectorFixture, "N055.00.00.000", "N055.60.00.000", 1),
		strings.Replace(sectorFixture, "N055.00.00.000", "N091.00.00.000", 1),
		strings.Replace(sectorFixture, "N055.00.00.000", "E055.00.00.000", 1),
	} {
		_, err := Read(strings.NewReader(input), testConfig(), testNow)
		require.Error(t, err)
	}
	a, err := Read(strings.NewReader(sectorFixture), testConfig(), func() time.Time { return testConfig().Dataset.EffectiveUntil })
	require.NoError(t, err)
	_, err = a.LatestVersion(t.Context())
	require.ErrorContains(t, err, "not currently effective")
	_, err = a.Resolve(t.Context(), testQuery(a, "START DCT JOIN"))
	require.Error(t, err)
}

// Opt-in acceptance check against a user's local file; the file is never copied
// into the repository. All normal tests use independently authored fixtures.
func TestLocalSectorTerminalCompatibility(t *testing.T) {
	path := os.Getenv("FLIGHTSTRIPS_TEST_SECTOR_FILE")
	if path == "" {
		t.Skip("set FLIGHTSTRIPS_TEST_SECTOR_FILE to check a local SCT")
	}
	config, err := terminal.LoadFile("../../../../config/aman/ekch-terminal-2611.json")
	require.NoError(t, err)
	a, err := Load(path, config, testNow)
	require.NoError(t, err)
	t.Logf("Parsed %d named fixes", a.FixCount())
	airport, err := a.Airport(t.Context(), a.version, "EKCH")
	require.NoError(t, err)
	runways, err := config.Runways(t.Context(), a.version, "EKCH")
	require.NoError(t, err)
	var fixes []navdata.Fix
	for id, position := range a.fixes {
		if !a.ambiguous[id] {
			fixes = append(fixes, navdata.Fix{ID: id, Position: position, Provenance: a.provenance})
		}
	}
	_, err = config.Candidate(terminal.ReferenceSet{Version: a.version, Airport: airport, Runways: runways, Fixes: fixes}, testNow())
	require.NoError(t, err)
	entry, group := navdata.FixID("ERNOV"), aman.RunwayGroupID("ARRIVAL-22L")
	q := navdata.RouteQuery{Version: a.version, Origin: "ESSA", Destination: "EKCH", FiledRoute: "DCT ERNOV", TerminalEntry: &entry, RunwayGroup: &group}
	// Select a real, distinct waypoint from this file for a local DCT replay.
	for _, id := range []navdata.FixID{"TESPI", "TUDLO", "MONAK"} {
		if _, exists := a.fixes[id]; exists {
			q.FiledRoute = string(id) + " DCT ERNOV"
			break
		}
	}
	geometry, err := a.Resolve(t.Context(), q)
	require.NoError(t, err)
	require.NoError(t, geometry.Validate())
}
