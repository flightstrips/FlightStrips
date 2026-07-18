package terminal

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"

	"github.com/stretchr/testify/require"
)

func TestGoldenEKCHConfigurationValidatesAndBuildsCandidate(t *testing.T) {
	config := goldenConfig(t)
	refs := referencesFor(t, config)
	require.NoError(t, config.Validate(refs))
	fragment, err := config.Candidate(refs, time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, fragment.Paths, len(config.Feeders)*len(config.RunwayGroups))
	require.Len(t, fragment.Holdings, len(config.OverlayHoldings))
	for _, path := range fragment.Paths {
		require.Len(t, path.HoldingIDs, 1)
		require.NotEmpty(t, path.Digest)
	}
}

func TestGoldenEKCHConfigurationMatchesIndependentOfficialContent(t *testing.T) {
	config := goldenConfig(t)
	require.Equal(t, "EKCH-AIP-2607-V1", config.ConfigVersion)
	require.Equal(t, "2607", config.Dataset.Cycle)
	require.Equal(t, time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), config.ApplicabilityFrom)
	require.Equal(t, time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC), config.ApplicabilityUntil)
	require.Equal(t, config.ApplicabilityFrom, config.Dataset.EffectiveFrom)
	require.Equal(t, config.ApplicabilityUntil, config.Dataset.EffectiveUntil)
	require.Equal(t, []aman.RunwayGroupID{"ARRIVAL-04", "ARRIVAL-22", "ARRIVAL-12", "ARRIVAL-30"}, groupIDs(config.RunwayGroups))

	wantFinals := map[navdata.RunwayID]struct{ latitude, longitude, course float64 }{
		"04L": {55.5922, 12.6035361111, 41.2}, "04R": {55.6031, 12.6330472222, 41.2},
		"22L": {55.6254111111, 12.6675805556, 221.2}, "22R": {55.6124777778, 12.6348916667, 221.2},
		"12": {55.62415, 12.6391166667, 123.2}, "30": {55.6138527778, 12.6669472222, 303.2},
	}
	actualFinals := map[navdata.RunwayID]FinalApproachDefinition{}
	for _, group := range config.RunwayGroups {
		for _, final := range group.FinalApproaches {
			actualFinals[final.Runway] = final
		}
	}
	require.Len(t, actualFinals, len(wantFinals))
	for runway, want := range wantFinals {
		actual := actualFinals[runway]
		require.Equal(t, want.latitude, actual.Threshold.Position.LatitudeDeg, runway)
		require.Equal(t, want.longitude, actual.Threshold.Position.LongitudeDeg, runway)
		require.Equal(t, want.course, actual.CourseTrueDeg, runway)
		require.NotNil(t, actual.Threshold.CourseTrueDeg, runway)
		require.Equal(t, want.course, *actual.Threshold.CourseTrueDeg, runway)
	}

	wantHoldings := map[navdata.HoldingID]struct {
		fix     navdata.FixID
		course  float64
		turn    navdata.TurnDirection
		seconds int64
		minimum int
		maximum int
		hasMax  bool
		speed   int
	}{
		"EKCH-TIDVU-PRIMARY-LOW": {"TIDVU", 298, navdata.TurnRight, 90, 5000, 0, false, 230},
		"EKCH-OLPIB-PRIMARY-LOW": {"OLPIB", 34, navdata.TurnRight, 60, 3500, 14000, true, 230},
		"EKCH-LUGAS-PRIMARY-LOW": {"LUGAS", 77, navdata.TurnLeft, 60, 3500, 14000, true, 230},
		"EKCH-ROSBI-PRIMARY-LOW": {"ROSBI", 107, navdata.TurnLeft, 60, 3500, 14000, true, 230},
		"EKCH-ERNOV-PRIMARY":     {"ERNOV", 183, navdata.TurnLeft, 90, 10000, 0, false, 230},
	}
	require.Len(t, config.OverlayHoldings, len(wantHoldings))
	for _, holding := range config.OverlayHoldings {
		want, found := wantHoldings[holding.ID]
		require.True(t, found, holding.ID)
		require.Equal(t, want.fix, holding.Fix)
		require.Equal(t, want.course, holding.InboundCourseTrueDeg)
		require.Equal(t, want.turn, holding.TurnDirection)
		require.NotNil(t, holding.LegTimeSeconds)
		require.Equal(t, want.seconds, *holding.LegTimeSeconds)
		require.NotNil(t, holding.MinimumAltitudeFt)
		require.Equal(t, want.minimum, *holding.MinimumAltitudeFt)
		if want.hasMax {
			require.NotNil(t, holding.MaximumAltitudeFt)
			require.Equal(t, want.maximum, *holding.MaximumAltitudeFt)
		} else {
			require.Nil(t, holding.MaximumAltitudeFt)
		}
		require.NotNil(t, holding.MaximumSpeedKt)
		require.Equal(t, want.speed, *holding.MaximumSpeedKt)
		require.Equal(t, navdata.HoldingManual, holding.Termination)
		require.Equal(t, "NAVIAIR-AIP-DK", holding.Provenance.SourceID)
		require.Equal(t, "AD2-EKCH-17-AMDT-12-25", holding.Provenance.SourceRevision)
	}

	wantPaths := map[string]struct {
		fixes []navdata.FixID
		hold  navdata.HoldingID
	}{
		"TESPI/ARRIVAL-04": {fixIDs("TESPI", "ROSBI", "TNO", "CH750", "CH742", "CH734", "CH727", "ERPUK"), "EKCH-ROSBI-PRIMARY-LOW"},
		"TUDLO/ARRIVAL-04": {fixIDs("TUDLO", "LUGAS", "KOR", "CH751", "CH740", "CH734", "CH727", "ERPUK"), "EKCH-LUGAS-PRIMARY-LOW"},
		"MONAK/ARRIVAL-04": {fixIDs("MONAK", "OLPIB", "NEKSO", "CH731", "CH724", "DOPEM"), "EKCH-OLPIB-PRIMARY-LOW"},
		"TIDVU/ARRIVAL-04": {fixIDs("TIDVU", "ESJAH", "CH743", "CH737", "CH731", "CH724", "DOPEM"), "EKCH-TIDVU-PRIMARY-LOW"},
		"ERNOV/ARRIVAL-04": {fixIDs("ERNOV", "CH744", "CH727", "ERPUK"), "EKCH-ERNOV-PRIMARY"},
		"TESPI/ARRIVAL-22": {fixIDs("TESPI", "ROSBI", "TNO", "CH653", "CH645", "CH638", "CH631", "CH626", "ABEGI"), "EKCH-ROSBI-PRIMARY-LOW"},
		"TUDLO/ARRIVAL-22": {fixIDs("TUDLO", "LUGAS", "KOR", "CH654", "CH645", "CH638", "CH631", "CH626", "ABEGI"), "EKCH-LUGAS-PRIMARY-LOW"},
		"MONAK/ARRIVAL-22": {fixIDs("MONAK", "OLPIB", "NEKSO", "CH643", "CH636", "CH630", "CH625", "ADOVI"), "EKCH-OLPIB-PRIMARY-LOW"},
		"TIDVU/ARRIVAL-22": {fixIDs("TIDVU", "ESJAH", "CH641", "CH636", "CH630", "CH625", "ADOVI"), "EKCH-TIDVU-PRIMARY-LOW"},
		"ERNOV/ARRIVAL-22": {fixIDs("ERNOV", "CH632", "ABEGI"), "EKCH-ERNOV-PRIMARY"},
		"TESPI/ARRIVAL-12": {fixIDs("TESPI", "ROSBI", "TNO", "CH553", "CH542", "CH533", "CH525", "AGTIC"), "EKCH-ROSBI-PRIMARY-LOW"},
		"TUDLO/ARRIVAL-12": {fixIDs("TUDLO", "LUGAS", "KOR", "CH543", "CH533", "CH525", "AGTIC"), "EKCH-LUGAS-PRIMARY-LOW"},
		"MONAK/ARRIVAL-12": {fixIDs("MONAK", "OLPIB", "NEKSO", "CH546", "CH530", "CH525", "AGTIC"), "EKCH-OLPIB-PRIMARY-LOW"},
		"TIDVU/ARRIVAL-12": {fixIDs("TIDVU", "WUPJA", "CH545", "CH535", "CH524", "FEDJO"), "EKCH-TIDVU-PRIMARY-LOW"},
		"ERNOV/ARRIVAL-12": {fixIDs("ERNOV", "CH532", "CH524", "FEDJO"), "EKCH-ERNOV-PRIMARY"},
		"TESPI/ARRIVAL-30": {fixIDs("TESPI", "ROSBI", "TNO", "CH969", "CH949", "CH941", "CH932", "CH925", "HOFFO"), "EKCH-ROSBI-PRIMARY-LOW"},
		"TUDLO/ARRIVAL-30": {fixIDs("TUDLO", "LUGAS", "KOR", "CH947", "CH930", "COPHO"), "EKCH-LUGAS-PRIMARY-LOW"},
		"MONAK/ARRIVAL-30": {fixIDs("MONAK", "OLPIB", "KUBIS", "CH930", "COPHO"), "EKCH-OLPIB-PRIMARY-LOW"},
		"TIDVU/ARRIVAL-30": {fixIDs("TIDVU", "WUPJA", "CH940", "CH932", "CH925", "HOFFO"), "EKCH-TIDVU-PRIMARY-LOW"},
		"ERNOV/ARRIVAL-30": {fixIDs("ERNOV", "CH956", "CH949", "CH941", "CH932", "CH925", "HOFFO"), "EKCH-ERNOV-PRIMARY"},
	}
	require.Len(t, config.Paths, len(wantPaths))
	for _, path := range config.Paths {
		want, found := wantPaths[string(path.Feeder)+"/"+string(path.RunwayGroup)]
		require.True(t, found, path.Feeder)
		require.Equal(t, want.fixes, path.Fixes)
		require.Equal(t, want.hold, path.SelectedHolding)
	}
	require.Len(t, config.FixAliases, 1)
	require.Equal(t, navdata.FixID("CDA"), config.FixAliases[0].Alias)
	require.Equal(t, navdata.FixID("OLPIB"), config.FixAliases[0].Canonical)
	require.Equal(t, "NAVIAIR-AIP-DK", config.FixAliases[0].Source.ID)
	require.Equal(t, time.Date(2024, 11, 28, 0, 0, 0, 0, time.UTC), config.FixAliases[0].Source.EffectiveFrom)
	require.Equal(t, config.ApplicabilityUntil, config.FixAliases[0].Source.EffectiveUntil)
	require.Contains(t, config.FixAliases[0].Source.Document, "CDA withdrawn")
}

func groupIDs(groups []RunwayGroup) []aman.RunwayGroupID {
	result := make([]aman.RunwayGroupID, len(groups))
	for i, group := range groups {
		result[i] = group.ID
	}
	return result
}

func fixIDs(values ...string) []navdata.FixID {
	result := make([]navdata.FixID, len(values))
	for i, value := range values {
		result[i] = navdata.FixID(value)
	}
	return result
}

func TestEKCHConfigurationReportsEveryInvalidFieldPath(t *testing.T) {
	config := goldenConfig(t)
	config.Sources[0].Document = ""
	config.Paths = config.Paths[1:]
	config.Paths[0].Fixes = append(config.Paths[0].Fixes, config.Paths[0].Fixes[0])
	config.Paths[0].SelectedHolding = "MISSING"
	config.RunwayGroups[0].Runways[0] = "MISSING"
	refs := referencesFor(t, config)
	refs.Version.Cycle = "2604"
	err := config.Validate(refs)
	var all ValidationErrors
	require.True(t, errors.As(err, &all))
	message := err.Error()
	for _, path := range []string{"sources[0]", "paths: missing enabled path", "paths[0].fixes", "paths[0].selectedHolding", "runwayGroups[0].runways[0]", "dataset"} {
		require.Contains(t, message, path)
	}
}

func TestConfigurationRejectsTerminalSafetyViolations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Configuration, *ReferenceSet)
		want   string
	}{
		{"first fix differs from feeder", func(c *Configuration, _ *ReferenceSet) { c.Paths[0].Fixes[0] = "ROSBI" }, "paths[0].fixes[0]"},
		{"duplicate overlay ID", func(c *Configuration, _ *ReferenceSet) {
			c.OverlayHoldings = append(c.OverlayHoldings, c.OverlayHoldings[0])
		}, "overlayHoldings[5].id"},
		{"duplicate group runway", func(c *Configuration, _ *ReferenceSet) {
			c.RunwayGroups[0].Runways = append(c.RunwayGroups[0].Runways, "04L")
		}, "runwayGroups[0].runways[2]"},
		{"duplicate final runway", func(c *Configuration, _ *ReferenceSet) {
			c.RunwayGroups[0].FinalApproaches = append(c.RunwayGroups[0].FinalApproaches, c.RunwayGroups[0].FinalApproaches[0])
		}, "runwayGroups[0].finalApproaches[2].runway"},
		{"missing final runway", func(c *Configuration, _ *ReferenceSet) {
			c.RunwayGroups[0].FinalApproaches = c.RunwayGroups[0].FinalApproaches[:1]
		}, "runwayGroups[0].runways[1]: requires exactly one final approach"},
		{"absent selected holding", func(c *Configuration, _ *ReferenceSet) { c.Paths[0].SelectedHolding = "MISSING" }, "paths[0].selectedHolding: is missing"},
		{"off-path selected holding", func(c *Configuration, _ *ReferenceSet) { c.Paths[0].SelectedHolding = "EKCH-ERNOV-PRIMARY" }, "paths[0].selectedHolding: holding fix must occur"},
		{"conflicting overlay holding", func(c *Configuration, refs *ReferenceSet) {
			published := c.OverlayHoldings[0].canonical()
			c.OverlayHoldings[0].InboundCourseTrueDeg = 1
			refs.Procedures = []navdata.Procedure{{ID: "PUBLISHED", Airport: "EKCH", Kind: navdata.ProcedureSTAR, Holdings: []navdata.HoldingPattern{published}, Provenance: published.Provenance}}
		}, "overlayHoldings[0]: conflicts"},
		{"missing feeder group path", func(c *Configuration, _ *ReferenceSet) { c.Paths = c.Paths[1:] }, "paths: missing enabled path"},
		{"duplicate feeder group path", func(c *Configuration, _ *ReferenceSet) { c.Paths = append(c.Paths, c.Paths[0]) }, "paths[20]: duplicates feeder/runway group"},
		{"cycle", func(c *Configuration, _ *ReferenceSet) {
			c.Paths[0].Fixes = append(c.Paths[0].Fixes, c.Paths[0].Fixes[0])
		}, "paths[0].fixes[8]: forms a cycle"},
		{"final intercept", func(_ *Configuration, refs *ReferenceSet) { setFixPosition(refs.Fixes, "ERPUK", 56, 13) }, "does not connect plausibly to final approach"},
		{"alias collision", func(c *Configuration, _ *ReferenceSet) {
			c.FixAliases = append(c.FixAliases, FixAlias{Alias: "OLPIB", Canonical: "TIDVU", Source: c.FixAliases[0].Source})
		}, "fixAliases[1].alias: collides"},
		{"dataset mismatch", func(_ *Configuration, refs *ReferenceSet) { refs.Version.Cycle = "2604" }, "dataset: does not match active dataset"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := goldenConfig(t)
			refs := referencesFor(t, config)
			test.mutate(&config, &refs)
			require.ErrorContains(t, config.Validate(refs), test.want)
		})
	}
}

func TestPublishedAndOverlayHoldingsNormalizeEquivalently(t *testing.T) {
	config := goldenConfig(t)
	refs := referencesFor(t, config)
	published := config.OverlayHoldings[0].canonical()
	refs.Procedures = []navdata.Procedure{{ID: "TIDVU3C", Airport: "EKCH", Kind: navdata.ProcedureSTAR, Holdings: []navdata.HoldingPattern{published}, Provenance: published.Provenance}}
	require.NoError(t, config.Validate(refs))
	first, err := navdata.HoldingDigest(published)
	require.NoError(t, err)
	second, err := navdata.HoldingDigest(config.OverlayHoldings[0].canonical())
	require.NoError(t, err)
	require.Equal(t, first, second)
	fragment, err := config.Candidate(refs, time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NotContains(t, fragment.Holdings, published, "published canonical holding replaces identical AIP fallback")
	require.Len(t, fragment.Holdings, len(config.OverlayHoldings)-1)
}

func TestLegacyCDAFixAliasNormalizesToCurrentOLPIB(t *testing.T) {
	config := goldenConfig(t)
	refs := referencesFor(t, config)
	for index := range config.Paths {
		if config.Paths[index].Feeder == "MONAK" {
			config.Paths[index].Fixes[1] = "CDA"
			break
		}
	}
	require.NoError(t, config.Validate(refs))
	fragment, err := config.Candidate(refs, time.Date(2026, 7, 9, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	for _, path := range fragment.Paths {
		for _, leg := range path.Legs {
			require.NotEqual(t, navdata.FixID("CDA"), *leg.ToFix)
		}
	}
}

func TestResolveRunwayGroupUsesOnlyServerConfiguration(t *testing.T) {
	config := goldenConfig(t)
	explicit := aman.RunwayGroupID("22L")
	session := aman.RunwayGroupID("04R")
	selected := config.ResolveRunwayGroup(SelectionInput{ExplicitFMP: &explicit, SessionRunwayGroup: &session})
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-22"), *selected.RunwayGroup)
	selected = config.ResolveRunwayGroup(SelectionInput{SessionRunwayGroup: &session})
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04"), *selected.RunwayGroup)
	selected = config.ResolveRunwayGroup(SelectionInput{})
	require.Nil(t, selected.RunwayGroup)
	require.Contains(t, selected.DegradedReason, "server-authoritative")
}

func TestReloadIsAtomic(t *testing.T) {
	config := goldenConfig(t)
	refs := referencesFor(t, config)
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	bad := filepath.Join(dir, "bad.json")
	encoded, err := os.ReadFile(goldenConfigPath(t))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(good, encoded, 0o600))
	broken := strings.Replace(string(encoded), "EKCH-TIDVU-PRIMARY-LOW", "MISSING", 1)
	require.NoError(t, os.WriteFile(bad, []byte(broken), 0o600))
	var store Store
	require.NoError(t, store.Reload(good, refs))
	require.Equal(t, config.ConfigVersion, store.Active().ConfigVersion)
	require.Error(t, store.Reload(bad, refs))
	require.Equal(t, config.ConfigVersion, store.Active().ConfigVersion)
}

func TestActiveReturnsDefensiveConfigurationClone(t *testing.T) {
	config := goldenConfig(t)
	refs := referencesFor(t, config)
	dir := t.TempDir()
	path := filepath.Join(dir, "terminal.json")
	encoded, err := os.ReadFile(goldenConfigPath(t))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, encoded, 0o600))
	var store Store
	require.NoError(t, store.Reload(path, refs))

	active := store.Active()
	active.RunwayGroups[0].Aliases[0] = "MUTATED"
	active.RunwayGroups[0].FinalApproaches[0].Threshold.Position.LatitudeDeg = 0
	active.Paths[0].Fixes[0] = "MUTATED"
	active.OverlayHoldings[0].MinimumAltitudeFt = intPtr(1)

	next := store.Active()
	require.Equal(t, aman.RunwayGroupID("04"), next.RunwayGroups[0].Aliases[0])
	require.Equal(t, 55.5922, next.RunwayGroups[0].FinalApproaches[0].Threshold.Position.LatitudeDeg)
	require.Equal(t, navdata.FixID("TESPI"), next.Paths[0].Fixes[0])
	require.NotNil(t, next.OverlayHoldings[0].MinimumAltitudeFt)
	require.Equal(t, 5000, *next.OverlayHoldings[0].MinimumAltitudeFt)
}

func intPtr(value int) *int { return &value }

func TestTerminalValidationHasNoSourceNetworkOrEuroScopeDependency(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(file), "config.go"))
	require.NoError(t, err)
	for _, forbidden := range []string{"net/http", "navdata/fixture", "navdata/airacnet", "internal/euroscope"} {
		require.NotContains(t, string(contents), forbidden)
	}
}

func goldenConfig(t *testing.T) Configuration {
	value, err := LoadFile(goldenConfigPath(t))
	require.NoError(t, err)
	return value
}
func goldenConfigPath(t *testing.T) string {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "config", "aman", "ekch-terminal-2607.json"))
}

func referencesFor(t *testing.T, config Configuration) ReferenceSet {
	t.Helper()
	version := navdata.DatasetVersion{Cycle: config.Dataset.Cycle, EffectiveFrom: config.Dataset.EffectiveFrom, EffectiveUntil: config.Dataset.EffectiveUntil, SourceRevision: "fixture-cache"}
	provenance := navdata.Provenance{SourceID: "fixture-cache", SourceRevision: "fixture-cache", ImportedAt: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC), EffectiveFrom: config.Dataset.EffectiveFrom, EffectiveUntil: config.Dataset.EffectiveUntil}
	runways := []navdata.Runway{}
	for _, group := range config.RunwayGroups {
		for _, definition := range group.FinalApproaches {
			final := definition.canonical()
			runways = append(runways, navdata.Runway{ID: final.Runway, Airport: "EKCH", Threshold: final.Threshold, LengthNM: 2, Provenance: provenance})
		}
	}
	fixIDs := []navdata.FixID{}
	for _, path := range config.Paths {
		fixIDs = append(fixIDs, path.Fixes...)
	}
	for _, holding := range config.OverlayHoldings {
		fixIDs = append(fixIDs, holding.Fix)
	}
	fixes := []navdata.Fix{}
	for index, id := range fixIDs {
		if slices.ContainsFunc(fixes, func(f navdata.Fix) bool { return f.ID == id }) {
			continue
		}
		fixes = append(fixes, navdata.Fix{ID: id, Position: navdata.Coordinate{LatitudeDeg: 55.1 + float64(index)/100, LongitudeDeg: 12.1 + float64(index)/100}, Provenance: provenance})
	}
	// Merge points are positioned behind their associated final courses so the
	// golden fixture exercises final-intercept continuity instead of bypassing it.
	setFixPosition(fixes, "ERPUK", 55.45, 12.40)
	setFixPosition(fixes, "DOPEM", 55.44, 12.42)
	setFixPosition(fixes, "ABEGI", 55.76, 12.85)
	setFixPosition(fixes, "ADOVI", 55.75, 12.84)
	setFixPosition(fixes, "AGTIC", 55.682428, 12.176869)
	setFixPosition(fixes, "FEDJO", 55.837475, 12.351667)
	setFixPosition(fixes, "HOFFO", 55.570833, 13.073333)
	setFixPosition(fixes, "COPHO", 55.416558, 12.902131)
	return ReferenceSet{Version: version, Airport: navdata.Airport{ID: "EKCH", Name: "Copenhagen", Position: navdata.Coordinate{LatitudeDeg: 55.61, LongitudeDeg: 12.65}, Provenance: provenance}, Runways: runways, Fixes: fixes}
}
func setFixPosition(fixes []navdata.Fix, id navdata.FixID, lat, lon float64) {
	for i := range fixes {
		if fixes[i].ID == id {
			fixes[i].Position = navdata.Coordinate{LatitudeDeg: lat, LongitudeDeg: lon}
			return
		}
	}
}
