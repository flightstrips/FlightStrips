package terminal

import (
	"context"
	"encoding/json"
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

func TestPathDecodesLegacyAndExplicitSTARFamilyKeys(t *testing.T) {
	var legacy Path
	require.NoError(t, json.Unmarshal([]byte(`{"feeder":"TESPI"}`), &legacy))
	require.Equal(t, navdata.FeederID("TESPI"), legacy.Feeder)
	require.Empty(t, legacy.STARFamily)

	var explicit Path
	require.NoError(t, json.Unmarshal([]byte(`{"starFamily":"TESPI","feederFix":"TNO","holdingToFeederSeconds":195}`), &explicit))
	require.Equal(t, navdata.FeederID("TESPI"), explicit.Feeder, "legacy runtime alias is populated")
	require.Equal(t, navdata.STARFamilyID("TESPI"), explicit.STARFamily)
	require.Equal(t, navdata.FixID("TNO"), explicit.FeederFix)
	require.NotNil(t, explicit.HoldingToFeederSeconds)
	require.EqualValues(t, 195, *explicit.HoldingToFeederSeconds)
}

func TestGoldenEKCHConfigurationValidatesAndBuildsCandidate(t *testing.T) {
	config := goldenConfig(t)
	refs := referencesFor(t, config)
	require.NoError(t, config.Validate(refs))
	wantHeadings := map[aman.RunwayGroupID]int{"ARRIVAL-04L": 217, "ARRIVAL-04R": 217, "ARRIVAL-12": 299, "ARRIVAL-22L": 37, "ARRIVAL-22R": 37, "ARRIVAL-30": 119}
	for _, path := range config.Paths {
		require.NotNil(t, path.PublishedHeadingMagneticDeg, path.Feeder)
		require.Equal(t, wantHeadings[path.RunwayGroup], *path.PublishedHeadingMagneticDeg, path.Feeder)
	}
	fragment, err := config.Candidate(refs, time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, fragment.Paths, len(config.Feeders)*len(config.RunwayGroups))
	require.Len(t, fragment.Holdings, len(config.OverlayHoldings))
	wantDigests := map[string]string{
		"TESPI/ARRIVAL-04L": "4f26f2897e15675cd8dbd0420246414863aa62df25d76dacc59a1db32583dd03",
		"TESPI/ARRIVAL-04R": "d67f57b85e18254ae527c8f1bec33779552573a832dc40a97858570de8a26f64",
		"TUDLO/ARRIVAL-04L": "52f61ed22a37b0f8bebae10d6663c29970615fd470e8a11b216ff6cf0ed17f6a",
		"TUDLO/ARRIVAL-04R": "36fd313e0add7fed25016562feaed53b31f82c75671e8baf8b588cf36b0b574f",
		"MONAK/ARRIVAL-04L": "fb83cbffef4ee76b89d1584565cad0697023c233541fa5c6d18dd2f4bcd73777",
		"MONAK/ARRIVAL-04R": "3853533b42021c172eba816cfef42c5a6d87a6ddba621e6e819ddd767f96c9f5",
		"TIDVU/ARRIVAL-04L": "5d313de9d246484642cc90f869a4bce084ecb60a400e79a99f4c97d07d38777f",
		"TIDVU/ARRIVAL-04R": "75f4e4785621ba49ed968fffefafe0e149e96c82313e766b215465f0d2198ed7",
		"ERNOV/ARRIVAL-04L": "cd2600a16cfb66f2d2d54bea4faa26b37a34245ca298043618239d8b8e3c5907",
		"ERNOV/ARRIVAL-04R": "ab1d046cf055fdb1df720e46eb2e9fdf40086295b81e8df5ea845bf4db5d8a5c",
		"TESPI/ARRIVAL-22L": "068f22bda6f6eaad6e652e8989e253940ca48b3c01bb76c9b47880e1282beb41",
		"TESPI/ARRIVAL-22R": "88874cc15f49b7751442f316593e262748bc0505949b15a2749537ea9dff347f",
		"TUDLO/ARRIVAL-22L": "9accd4566dd3960aa0c26e30bb799656787c9f7c2cfcae7c2dab733630bc2231",
		"TUDLO/ARRIVAL-22R": "c0796baa6ed03fcd74e7cfae00b43290f4f7b9a13404b8b7c7e633319687ae93",
		"MONAK/ARRIVAL-22L": "e6371d70e30a043179cca358d892b5ca27a834dd3fbfa29f29ae4c05b150f871",
		"MONAK/ARRIVAL-22R": "222536c1a3a9fbd28a7f28ae1a83f726d2ea3c167952ea6357463c17c1e0c1e3",
		"TIDVU/ARRIVAL-22L": "239684d01302d88a45d0d8a96ace3bc60136d92fb5553cc033ea384167cb2c59",
		"TIDVU/ARRIVAL-22R": "5927fe12ec45a282466b42a6158aa40f7b8d4d4d2ceccb511c81124b06c2f1b9",
		"ERNOV/ARRIVAL-22L": "ded96aa96756de2accbe9e402a3bdd6d2617ec5c4423e4685875a4aaec99967b",
		"ERNOV/ARRIVAL-22R": "0a7ee21fd5f96bae5176202e8b1d53732d0f941c1b621c84a6883b1d217349c5",
		"TESPI/ARRIVAL-12":  "7a455392c178f77ea3f7d71fcf67cda03514d0010d9b1928029150d45e973e1d",
		"TUDLO/ARRIVAL-12":  "023df13fc940e5bd8c4699cca741cf91faf9981678b2a22b05af8b3c8baf2162",
		"MONAK/ARRIVAL-12":  "dd5843e753b7a10cbf94c9c24ab118806d32986e8bd011943ec10e4c07fddfda",
		"TIDVU/ARRIVAL-12":  "69a3d71e36ef48dff19f58955bb7a1339b84dc6ce1bdd4143374892db0195428",
		"ERNOV/ARRIVAL-12":  "e03446b918cfacb895c826da3875f7cf4bf0c18e6ef15183877208783f208255",
		"TESPI/ARRIVAL-30":  "0ced02100055602ecfe088d473b5fb9e2e0ade6f367bdacb4e75a01a7c941cb4",
		"TUDLO/ARRIVAL-30":  "2efaf1bdff7b57d9de83949cd92408e1889c73a72ca416c18a9ef5107511b367",
		"MONAK/ARRIVAL-30":  "02a08d801f0bf0a85ddd2bb186c39f14b0518a37d0f794633eaaa0a23d9a31a9",
		"TIDVU/ARRIVAL-30":  "1addffbc2a33cf286594c5df69736844c4b94fcaa4827b12d45d5d5d999460cd",
		"ERNOV/ARRIVAL-30":  "7349c6ac3bc4769085576eddb974baebf2f65ea7bf800118bbf59f0f6bdc5abe",
	}
	for _, path := range fragment.Paths {
		key := string(path.STARFamily) + "/" + string(path.RunwayGroup)
		require.Len(t, path.HoldingIDs, 1)
		require.Equal(t, wantDigests[key], path.Digest, key)
		require.GreaterOrEqual(t, len(path.Legs), 2)
		require.Contains(t, path.Legs[len(path.Legs)-2].ID, "FINAL-APPROACH-FIX")
		require.Contains(t, path.Legs[len(path.Legs)-1].ID, "-RUNWAY")
		require.Contains(t, string(*path.Legs[len(path.Legs)-1].ToFix), "RWY-")
		require.NotNil(t, path.Legs[len(path.Legs)-1].ToPosition)
		for _, group := range config.RunwayGroups {
			if group.ID == path.RunwayGroup {
				require.Equal(t, "RWY-"+string(group.Runways[0]), string(*path.Legs[len(path.Legs)-1].ToFix))
			}
		}
		require.NotNil(t, path.PublishedHeadingMagneticDeg, path.Feeder)
		require.Equal(t, wantHeadings[path.RunwayGroup], *path.PublishedHeadingMagneticDeg, path.Feeder)
	}
}

func TestLegacyFeederConfigurationMaterializesWithoutExplicitMetadata(t *testing.T) {
	config := goldenConfig(t)
	require.NotEmpty(t, config.Paths)
	for i := range config.Paths {
		config.Paths[i].Feeder = navdata.FeederID(config.Paths[i].STARFamily)
		config.Paths[i].STARFamily = ""
		config.Paths[i].FeederFix = ""
		config.Paths[i].HoldingToFeederSeconds = nil
	}
	require.Equal(t, navdata.FeederID("TESPI"), config.Paths[0].Feeder)
	require.Empty(t, config.Paths[0].STARFamily)
	require.Empty(t, config.Paths[0].FeederFix)
	require.Nil(t, config.Paths[0].HoldingToFeederSeconds)

	fragment, err := config.Candidate(referencesFor(t, config), time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, navdata.FeederID("TESPI"), fragment.Paths[0].Feeder)
	require.Empty(t, fragment.Paths[0].STARFamily)
	require.Empty(t, fragment.Paths[0].FeederFix)
	require.Nil(t, fragment.Paths[0].HoldingToFeederDuration)
}

func TestCandidateMaterializesExplicitTerminalPathMetadata(t *testing.T) {
	config := goldenConfig(t)
	seconds := int64(3*60 + 15)
	config.Paths[0].HoldingToFeederSeconds = &seconds

	fragment, err := config.Candidate(referencesFor(t, config), time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	path := fragment.Paths[0]
	require.Equal(t, navdata.FeederID("TESPI"), path.Feeder, "legacy family remains populated")
	require.Equal(t, navdata.STARFamilyID("TESPI"), path.STARFamily)
	require.Equal(t, navdata.FixID("TNO"), path.FeederFix)
	require.NotNil(t, path.HoldingToFeederDuration)
	require.Equal(t, 3*time.Minute+15*time.Second, *path.HoldingToFeederDuration)
	require.NoError(t, path.Validate())
}

func TestGoldenEKCHConfigurationMatchesIndependentOfficialContent(t *testing.T) {
	config := goldenConfig(t)
	require.Equal(t, "EKCH-AIP-2609-V2", config.ConfigVersion)
	require.Equal(t, "2609", config.Dataset.Cycle)
	require.Equal(t, time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), config.ApplicabilityFrom)
	require.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), config.ApplicabilityUntil)
	require.Equal(t, config.ApplicabilityFrom, config.Dataset.EffectiveFrom)
	require.Equal(t, config.ApplicabilityUntil, config.Dataset.EffectiveUntil)
	require.Equal(t, []aman.RunwayGroupID{"ARRIVAL-04L", "ARRIVAL-04R", "ARRIVAL-22L", "ARRIVAL-22R", "ARRIVAL-12", "ARRIVAL-30"}, groupIDs(config.RunwayGroups))
	for _, group := range config.RunwayGroups {
		require.Equal(t, &SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}, group.SameSTARSpacing, group.ID)
	}

	wantFinals := map[navdata.RunwayID]struct{ latitude, longitude, course float64 }{
		"04L": {55.5922, 12.6035361111, 41.2}, "04R": {55.6031, 12.6330472222, 41.2},
		"22L": {55.6254111111, 12.6675805556, 221.2}, "22R": {55.6124777778, 12.6348916667, 221.2},
		"12": {55.62415, 12.6391166667, 123.2}, "30": {55.6138527778, 12.6669472222, 303.2},
	}
	wantFinalApproachFixes := map[navdata.RunwayID]navdata.FixID{
		"04L": "BUDIQ", "04R": "CATWU", "12": "CH12F", "22L": "EXTAR", "22R": "RUCCI", "30": "CH30F",
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
		require.Equal(t, wantFinalApproachFixes[runway], actual.FinalApproachFix, runway)
		require.NotNil(t, actual.Threshold.CourseTrueDeg, runway)
		require.Equal(t, want.course, *actual.Threshold.CourseTrueDeg, runway)
	}

	runways, err := config.Runways(context.Background(), navdata.DatasetVersion{Cycle: config.Dataset.Cycle, SourceRevision: "test", EffectiveFrom: config.Dataset.EffectiveFrom, EffectiveUntil: config.Dataset.EffectiveUntil}, "EKCH")
	require.NoError(t, err)
	lengths := map[navdata.RunwayID]float64{}
	for _, runway := range runways {
		lengths[runway.ID] = runway.LengthNM * 1852
	}
	require.Equal(t, 3001.0, lengths["04L"])
	require.Equal(t, 3302.0, lengths["04R"])
	require.Equal(t, 3302.0, lengths["22L"])
	require.Equal(t, 3571.0, lengths["22R"])
	require.Equal(t, 2800.0, lengths["12"])
	require.Equal(t, 2365.0, lengths["30"])

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
	wantFeederFixes := map[navdata.STARFamilyID]navdata.FixID{
		"TESPI": "TNO", "TUDLO": "KOR", "MONAK": "NEKSO", "TIDVU": "ESJAH", "ERNOV": "ERNOV",
	}
	wantHoldingToFeederSeconds := map[navdata.STARFamilyID]int64{
		"TESPI": 195, "TUDLO": 255, "MONAK": 200, "TIDVU": 140, "ERNOV": 0,
	}
	missingHoldingToFeederDuration := map[string]navdata.FixID{
		"MONAK/ARRIVAL-30": "KUBIS", "TIDVU/ARRIVAL-12": "WUPJA", "TIDVU/ARRIVAL-30": "WUPJA",
	}
	require.Len(t, config.Paths, len(wantPaths)+2*len(config.Feeders))
	for _, path := range config.Paths {
		group := string(path.RunwayGroup)
		if strings.HasPrefix(group, "ARRIVAL-04") {
			group = "ARRIVAL-04"
		} else if strings.HasPrefix(group, "ARRIVAL-22") {
			group = "ARRIVAL-22"
		}
		want, found := wantPaths[string(path.Feeder)+"/"+group]
		require.True(t, found, path.Feeder)
		require.Equal(t, want.fixes, path.Fixes)
		require.Equal(t, want.hold, path.SelectedHolding)
		require.Equal(t, navdata.STARFamilyID(path.Feeder), path.STARFamily)
		key := string(path.STARFamily) + "/" + string(path.RunwayGroup)
		if feederFix, missing := missingHoldingToFeederDuration[key]; missing {
			require.Equal(t, feederFix, path.FeederFix, key)
			require.Nil(t, path.HoldingToFeederSeconds, key)
		} else {
			require.Equal(t, wantFeederFixes[path.STARFamily], path.FeederFix, key)
			require.NotNil(t, path.HoldingToFeederSeconds, key)
			require.Equal(t, wantHoldingToFeederSeconds[path.STARFamily], *path.HoldingToFeederSeconds, key)
		}
	}
	require.Len(t, config.FixAliases, 1)
	require.Equal(t, navdata.FixID("CDA"), config.FixAliases[0].Alias)
	require.Equal(t, navdata.FixID("OLPIB"), config.FixAliases[0].Canonical)
	require.Equal(t, "NAVIAIR-AIP-DK", config.FixAliases[0].Source.ID)
	require.Equal(t, time.Date(2024, 11, 28, 0, 0, 0, 0, time.UTC), config.FixAliases[0].Source.EffectiveFrom)
	require.Equal(t, config.ApplicabilityUntil, config.FixAliases[0].Source.EffectiveUntil)
	require.Contains(t, config.FixAliases[0].Source.Document, "CDA withdrawn")
	require.Len(t, config.OverlayFixes, 1)
	require.Equal(t, navdata.FixID("CH632"), config.OverlayFixes[0].ID)
	require.InDelta(t, 55.9384649710, config.OverlayFixes[0].Position.LatitudeDeg, 0.0000001)
	require.InDelta(t, 12.4659798388, config.OverlayFixes[0].Position.LongitudeDeg, 0.0000001)
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

func TestSameSTARSpacingValidation(t *testing.T) {
	config := goldenConfig(t)
	refs := referencesFor(t, config)

	config.RunwayGroups[0].SameSTARSpacing = nil
	require.NoError(t, config.Validate(refs), "omitted spacing disables the rule")
	config.RunwayGroups[0].SameSTARSpacing = &SameSTARSpacing{Enabled: false}
	require.NoError(t, config.Validate(refs), "disabled spacing does not require thresholds")

	config.RunwayGroups[0].SameSTARSpacing = &SameSTARSpacing{Enabled: true}
	err := config.Validate(refs)
	require.ErrorContains(t, err, "sameStarSpacing.activationRatePerHour")
	require.ErrorContains(t, err, "sameStarSpacing.minimumEmptySlots")
}

func TestConfigurationRejectsTerminalSafetyViolations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Configuration, *ReferenceSet)
		want   string
	}{
		{"first fix differs from feeder", func(c *Configuration, _ *ReferenceSet) { c.Paths[0].Fixes[0] = "ROSBI" }, "paths[0].fixes[0]"},
		{"explicit feeder fix requires family", func(c *Configuration, _ *ReferenceSet) {
			c.Paths[0].STARFamily = ""
			c.Paths[0].FeederFix = "TNO"
		}, "paths[0].starFamily: is required"},
		{"explicit family requires feeder fix", func(c *Configuration, _ *ReferenceSet) {
			c.Paths[0].STARFamily = "TESPI"
			c.Paths[0].FeederFix = ""
		}, "paths[0].feederFix: is required"},
		{"explicit family matches legacy feeder", func(c *Configuration, _ *ReferenceSet) {
			c.Paths[0].STARFamily = "TUDLO"
			c.Paths[0].FeederFix = "TNO"
		}, "paths[0].starFamily: must match the legacy feeder"},
		{"feeder fix occurs on path", func(c *Configuration, _ *ReferenceSet) {
			c.Paths[0].STARFamily = "TESPI"
			c.Paths[0].FeederFix = "MISSING"
		}, "paths[0].feederFix: must occur exactly once"},
		{"feeder fix follows selected holding", func(c *Configuration, _ *ReferenceSet) {
			c.Paths[0].STARFamily = "TESPI"
			c.Paths[0].FeederFix = "TESPI"
		}, "paths[0].feederFix: must not precede the selected holding fix"},
		{"holding transit cannot be negative", func(c *Configuration, _ *ReferenceSet) {
			seconds := int64(-1)
			c.Paths[0].STARFamily = "TESPI"
			c.Paths[0].FeederFix = "TNO"
			c.Paths[0].HoldingToFeederSeconds = &seconds
		}, "paths[0].holdingToFeederSeconds: must be a non-negative"},
		{"duplicate overlay ID", func(c *Configuration, _ *ReferenceSet) {
			c.OverlayHoldings = append(c.OverlayHoldings, c.OverlayHoldings[0])
		}, "overlayHoldings[5].id"},
		{"duplicate overlay fix", func(c *Configuration, _ *ReferenceSet) {
			c.OverlayFixes = append(c.OverlayFixes, c.OverlayFixes[0])
		}, "overlayFixes[1].id"},
		{"invalid overlay fix coordinate", func(c *Configuration, _ *ReferenceSet) {
			c.OverlayFixes[0].Position.LatitudeDeg = 100
		}, "overlayFixes[0]: invalid_argument: coordinate is outside WGS84 bounds"},
		{"duplicate group runway", func(c *Configuration, _ *ReferenceSet) {
			c.RunwayGroups[0].Runways = append(c.RunwayGroups[0].Runways, "04L")
		}, "runwayGroups[0].runways[1]"},
		{"duplicate final runway", func(c *Configuration, _ *ReferenceSet) {
			c.RunwayGroups[0].FinalApproaches = append(c.RunwayGroups[0].FinalApproaches, c.RunwayGroups[0].FinalApproaches[0])
		}, "runwayGroups[0].finalApproaches[1].runway"},
		{"missing final runway", func(c *Configuration, _ *ReferenceSet) {
			c.RunwayGroups[0].FinalApproaches = nil
		}, "runwayGroups[0]: requires exactly one runway and final approach"},
		{"empty active runway group set", func(c *Configuration, _ *ReferenceSet) {
			c.ActiveRunwayGroupSets = append(c.ActiveRunwayGroupSets, nil)
		}, "activeRunwayGroupSets[8]: cannot be empty"},
		{"unknown active runway group", func(c *Configuration, _ *ReferenceSet) {
			c.ActiveRunwayGroupSets[0] = []aman.RunwayGroupID{"MISSING"}
		}, "activeRunwayGroupSets[0][0]: must name a configured runway group"},
		{"duplicate active runway group", func(c *Configuration, _ *ReferenceSet) {
			c.ActiveRunwayGroupSets[0] = []aman.RunwayGroupID{"ARRIVAL-04L", "ARRIVAL-04L"}
		}, "activeRunwayGroupSets[0][1]: must be unique within the set"},
		{"duplicate active runway group set", func(c *Configuration, _ *ReferenceSet) {
			c.ActiveRunwayGroupSets = append(c.ActiveRunwayGroupSets, []aman.RunwayGroupID{"ARRIVAL-04R", "ARRIVAL-04L"})
		}, "activeRunwayGroupSets[8]: duplicates another configured set"},
		{"absent selected holding", func(c *Configuration, _ *ReferenceSet) { c.Paths[0].SelectedHolding = "MISSING" }, "paths[0].selectedHolding: is missing"},
		{"off-path selected holding", func(c *Configuration, _ *ReferenceSet) { c.Paths[0].SelectedHolding = "EKCH-ERNOV-PRIMARY" }, "paths[0].selectedHolding: holding fix must occur"},
		{"conflicting overlay holding", func(c *Configuration, refs *ReferenceSet) {
			published := c.OverlayHoldings[0].canonical()
			c.OverlayHoldings[0].InboundCourseTrueDeg = 1
			refs.Procedures = []navdata.Procedure{{ID: "PUBLISHED", Airport: "EKCH", Kind: navdata.ProcedureSTAR, Holdings: []navdata.HoldingPattern{published}, Provenance: published.Provenance}}
		}, "overlayHoldings[0]: conflicts"},
		{"missing feeder group path", func(c *Configuration, _ *ReferenceSet) { c.Paths = c.Paths[1:] }, "paths: missing enabled path"},
		{"duplicate feeder group path", func(c *Configuration, _ *ReferenceSet) { c.Paths = append(c.Paths, c.Paths[0]) }, "paths[30]: duplicates feeder/runway group"},
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
	fragment, err := config.Candidate(refs, time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NotContains(t, fragment.Holdings, published, "published canonical holding replaces identical AIP fallback")
	require.Len(t, fragment.Holdings, len(config.OverlayHoldings)-1)
}

func TestCandidateUsesAirportScopedFixOverrideForERNOV22(t *testing.T) {
	config := goldenConfig(t)
	refs := referencesFor(t, config)
	setFixPosition(refs.Fixes, "CH632", 26.327641, -83.227989)

	fragment, err := config.Candidate(refs, time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	pathIndex := slices.IndexFunc(fragment.Paths, func(path navdata.TerminalPath) bool {
		return path.Feeder == "ERNOV" && path.RunwayGroup == "ARRIVAL-22L"
	})
	require.NotEqual(t, -1, pathIndex)
	path := fragment.Paths[pathIndex]
	require.Equal(t, navdata.FixID("CH632"), *path.Legs[0].ToFix)
	require.NotNil(t, path.Legs[0].ToPosition)
	require.InDelta(t, 55.9384649710, path.Legs[0].ToPosition.LatitudeDeg, 0.0000001)
	require.InDelta(t, 12.4659798388, path.Legs[0].ToPosition.LongitudeDeg, 0.0000001)
	require.Equal(t, navdata.FixID("CH632"), *path.Legs[1].FromFix)
	require.NotNil(t, path.Legs[1].FromPosition)
	require.Equal(t, *path.Legs[0].ToPosition, *path.Legs[1].FromPosition)
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
	fragment, err := config.Candidate(refs, time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
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
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-22L"), *selected.RunwayGroup)
	selected = config.ResolveRunwayGroup(SelectionInput{SessionRunwayGroup: &session})
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04R"), *selected.RunwayGroup)
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
	active.ActiveRunwayGroupSets[0][0] = "MUTATED"
	active.RunwayGroups[0].FinalApproaches[0].Threshold.Position.LatitudeDeg = 0
	active.Paths[0].Fixes[0] = "MUTATED"
	active.OverlayHoldings[0].MinimumAltitudeFt = intPtr(1)

	next := store.Active()
	require.Equal(t, aman.RunwayGroupID("04L"), next.RunwayGroups[0].Aliases[0])
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04L"), next.ActiveRunwayGroupSets[0][0])
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
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "config", "aman", "ekch-terminal-2609.json"))
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
	for _, group := range config.RunwayGroups {
		for _, final := range group.FinalApproaches {
			fixIDs = append(fixIDs, final.FinalApproachFix)
		}
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
	setFixPosition(fixes, "BUDIQ", 55.435267, 12.362403)
	setFixPosition(fixes, "CATWU", 55.431833, 12.3695)
	setFixPosition(fixes, "CH12F", 55.706753, 12.415008)
	setFixPosition(fixes, "EXTAR", 55.781472, 12.910667)
	setFixPosition(fixes, "RUCCI", 55.785053, 12.903611)
	setFixPosition(fixes, "CH30F", 55.531728, 12.888461)
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
