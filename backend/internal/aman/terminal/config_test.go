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

func TestConfigurationDecodesLegacyRunwayGroupSpacing(t *testing.T) {
	var config Configuration
	require.NoError(t, json.Unmarshal([]byte(`{"runwayGroups":[{"id":"SOUTH","sameStarSpacing":{"enabled":true,"activationRatePerHour":20,"minimumEmptySlots":1}}]}`), &config))
	require.Nil(t, config.STARFamilyPolicies)
	require.Equal(t, &SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}, config.RunwayGroups[0].SameSTARSpacing)
}

func TestHoldingSequencePolicyDefaultsAndValidates(t *testing.T) {
	var config Configuration
	require.NoError(t, json.Unmarshal([]byte(`{"starFamilyPolicies":[{"starFamily":"TESPI","sameStarSpacing":{}}]}`), &config))
	require.Equal(t, navdata.HoldingSequenceDisabled, config.STARFamilyPolicies[0].HoldingSequencePolicy.Effective())
	require.NoError(t, config.ValidateOperationalSettings())

	config.STARFamilyPolicies[0].HoldingSequencePolicy = navdata.HoldingSequenceDisabled
	require.NoError(t, config.ValidateOperationalSettings())
	config.STARFamilyPolicies[0].HoldingSequencePolicy = navdata.HoldingSequenceLowestAltitudeFirst
	require.NoError(t, config.ValidateOperationalSettings())
	config.STARFamilyPolicies[0].HoldingSequencePolicy = "highest_first"
	require.ErrorContains(t, config.ValidateOperationalSettings(), "starFamilyPolicies[0].holdingSequencePolicy: must be disabled or lowest_altitude_first")
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
	require.Equal(t, []navdata.TimelineMapping{
		{ID: 1, Left: starFamilyPtr("TESPI"), Right: starFamilyPtr("TUDLO")},
		{ID: 2, Left: starFamilyPtr("MONAK"), Right: starFamilyPtr("TIDVU")},
		{ID: 3, Left: starFamilyPtr("ERNOV")},
	}, fragment.TimelineMappings)
	require.Equal(t, []navdata.STARFamilyID{"ERNOV", "MONAK", "TESPI", "TIDVU", "TUDLO"}, starFamilyPolicyIDs(fragment.STARFamilyPolicies))
	for _, policy := range fragment.STARFamilyPolicies {
		require.Equal(t, navdata.SameSTARSpacingPolicy{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}, policy.SameSTARSpacing, policy.STARFamily)
		require.Equal(t, navdata.HoldingSequenceDisabled, policy.HoldingSequencePolicy, policy.STARFamily)
		require.Equal(t, 6*time.Minute, (time.Hour/time.Duration(policy.SameSTARSpacing.ActivationRatePerHour))*time.Duration(policy.SameSTARSpacing.MinimumEmptySlots+1))
	}
	require.Len(t, fragment.Paths, len(config.Feeders)*len(config.RunwayGroups))
	require.Len(t, fragment.Holdings, len(config.OverlayHoldings))
	wantDigests := map[string]string{
		"TESPI/ARRIVAL-04L": "f74aa4dc0b9fc4d2fb8a6d7aa824d80dafe4f3ba40d70f515111428ee41497cb",
		"TESPI/ARRIVAL-04R": "45641664b311a67659ef7024592aae39362127bfe7b6e5b6e026b13a91d53d20",
		"TUDLO/ARRIVAL-04L": "9d714da489f6d3d587b3bd7aed856ca2b36206186b692a3505dd1a1a557f68a0",
		"TUDLO/ARRIVAL-04R": "7235f574a69af3c4c9efc2adb663d116438afad8efff36d2de4a5f87ae30e7d9",
		"MONAK/ARRIVAL-04L": "ecea087bcd8160970cda9c512deb645114f7a441951823ec6489f3b0424ab630",
		"MONAK/ARRIVAL-04R": "a97166bb517993e8e7c918c868ab17b346b124fbdbedb58d405dc2833814269a",
		"TIDVU/ARRIVAL-04L": "fa216d0bfe7eaddf79a8dafc5541b48bd56bce6c4644ee1451482735b8193158",
		"TIDVU/ARRIVAL-04R": "3efacdca8b74f26a7cd424c923f278f65289aa0c06139c01155df88d794b73d0",
		"ERNOV/ARRIVAL-04L": "5f0de8e43394d9303cadaa20b175e2f4c48fd3b60bbd8bbaa415884a76814949",
		"ERNOV/ARRIVAL-04R": "5a448dd883c8278c72779c53169c3e5ddadb3ddaaad7ceebfbe69fff1b985fd7",
		"TESPI/ARRIVAL-22L": "bf90d1764b067ec69978e02c5dcca80782af41859f2526d6c42445ab5ec2cc66",
		"TESPI/ARRIVAL-22R": "9c276c5a0598be616eb950544188163d2fb90f46dc69bdd2eaa7cb1eecc70adb",
		"TUDLO/ARRIVAL-22L": "a5636832d2be28f8f141f4f47190a24be1a56121baec2ddf50acbd7c392d0893",
		"TUDLO/ARRIVAL-22R": "52c7750cf4bf03c8f355dafe16fabcab4897aa1cf0b2685f9dbe175c50c81f9e",
		"MONAK/ARRIVAL-22L": "e3f3933aa76fbb1e20efecbf6950280f153d72d642494a9d07ea95ac80703d9a",
		"MONAK/ARRIVAL-22R": "c2c6f1d6fa2c2932ed8a82f904778ab588005299d15a8728e86cad9fcb2734d9",
		"TIDVU/ARRIVAL-22L": "7470f1b389e4241abf8af6f2228037dd09cb6183c59311ad9e4933f7c9955425",
		"TIDVU/ARRIVAL-22R": "ed9e057aaabe28c85717886f0d3c525c1edb488ec2f43d1c1a4681c556836841",
		"ERNOV/ARRIVAL-22L": "f3c9bd4ca53b478c73fb8888c851579187c3942f0067256eec5714b663403a91",
		"ERNOV/ARRIVAL-22R": "49f7833e1d431ff9378d5bc5f3df79a4d205999d1890a5a7ee0e9d20a7484ba0",
		"TESPI/ARRIVAL-12":  "43af99620740b971807090010ecc07e0539492d1090c1a82ef8581b189c43ecc",
		"TUDLO/ARRIVAL-12":  "de49e7f55b60e040b797774b73884134f970ec53e28b7e2fdbb889b5c20f7ef8",
		"MONAK/ARRIVAL-12":  "3387899b28ac20197da11ad37c4ff80ddd7ff8a1099a2cedc1c1f9b31493dede",
		"TIDVU/ARRIVAL-12":  "207fafae184b0489b8cff8b34037b80ded137b9144d7428d98390c57f4a981e8",
		"ERNOV/ARRIVAL-12":  "50c73a94863dd8b98d35fbfe66a4304a8ff09dcc3740eddf0ffb4c7d1b725e10",
		"TESPI/ARRIVAL-30":  "67adcc7d4f67bdac5ef02896c772e78af9db50dc4a8b8e0409f6baad8e128665",
		"TUDLO/ARRIVAL-30":  "da15492e9d24ae99e26886965f79d40df5a040eecbb3210dcdfdc066a898aa38",
		"MONAK/ARRIVAL-30":  "ec5dfbf75780d35009bd7f8d5ec60008c56f1abc720d7e955e6d56f006b4a158",
		"TIDVU/ARRIVAL-30":  "b8fa8f045eac245ed1cc9ae982bc8e8af0927974a18165d24173c17ca905f73b",
		"ERNOV/ARRIVAL-30":  "990efa02594108e3a2b14db74bc38cbc8b3ece7665886268ee16b1c733b0dc54",
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
	require.Equal(t, "EKCH-AIP-2609-V4", config.ConfigVersion)
	require.Equal(t, "2609", config.Dataset.Cycle)
	require.Equal(t, time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), config.ApplicabilityFrom)
	require.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), config.ApplicabilityUntil)
	require.Equal(t, config.ApplicabilityFrom, config.Dataset.EffectiveFrom)
	require.Equal(t, config.ApplicabilityUntil, config.Dataset.EffectiveUntil)
	require.Equal(t, []aman.RunwayGroupID{"ARRIVAL-04L", "ARRIVAL-04R", "ARRIVAL-22L", "ARRIVAL-22R", "ARRIVAL-12", "ARRIVAL-30"}, groupIDs(config.RunwayGroups))
	for _, group := range config.RunwayGroups {
		require.Equal(t, &SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}, group.SameSTARSpacing, group.ID)
	}
	require.Equal(t, []navdata.STARFamilyID{"ERNOV", "MONAK", "TESPI", "TIDVU", "TUDLO"}, configuredSTARFamilyPolicyIDs(config.STARFamilyPolicies))
	for _, policy := range config.STARFamilyPolicies {
		require.Equal(t, SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}, policy.SameSTARSpacing, policy.STARFamily)
		require.Equal(t, navdata.HoldingSequenceDisabled, policy.HoldingSequencePolicy, policy.STARFamily)
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

func configuredSTARFamilyPolicyIDs(policies []STARFamilyPolicy) []navdata.STARFamilyID {
	result := make([]navdata.STARFamilyID, len(policies))
	for i, policy := range policies {
		result[i] = policy.STARFamily
	}
	return result
}

func starFamilyPolicyIDs(policies []navdata.STARFamilyPolicy) []navdata.STARFamilyID {
	result := make([]navdata.STARFamilyID, len(policies))
	for i, policy := range policies {
		result[i] = policy.STARFamily
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

func TestSTARFamilyPolicyValidationAndDeterministicDigest(t *testing.T) {
	config := goldenConfig(t)
	refs := referencesFor(t, config)
	baseline, err := config.Candidate(refs, time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	slices.Reverse(config.STARFamilyPolicies)
	reordered, err := config.Candidate(refs, time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, baseline.Digest, reordered.Digest)
	require.Equal(t, baseline.STARFamilyPolicies, reordered.STARFamilyPolicies)

	config.STARFamilyPolicies[0].HoldingSequencePolicy = navdata.HoldingSequenceLowestAltitudeFirst
	enabled, err := config.Candidate(refs, time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NotEqual(t, baseline.Digest, enabled.Digest)
	require.Equal(t, navdata.HoldingSequenceLowestAltitudeFirst, enabled.STARFamilyPolicies[4].HoldingSequencePolicy)

	config.STARFamilyPolicies[0].SameSTARSpacing = SameSTARSpacing{Enabled: true}
	err = config.Validate(refs)
	require.ErrorContains(t, err, "starFamilyPolicies[0].sameStarSpacing.activationRatePerHour")
	require.ErrorContains(t, err, "starFamilyPolicies[0].sameStarSpacing.minimumEmptySlots")

	config = goldenConfig(t)
	config.STARFamilyPolicies = config.STARFamilyPolicies[1:]
	require.ErrorContains(t, config.Validate(refs), "starFamilyPolicies: is missing configured STAR family ERNOV")
}

func TestTimelineMappingValidationAndDeterministicDigest(t *testing.T) {
	config := goldenConfig(t)
	refs := referencesFor(t, config)
	baseline, err := config.Candidate(refs, time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	slices.Reverse(config.TimelineMappings)
	reordered, err := config.Candidate(refs, time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, baseline.Digest, reordered.Digest)
	require.Equal(t, baseline.TimelineMappings, reordered.TimelineMappings)

	changedFamily := navdata.STARFamilyID("MONAK")
	config.TimelineMappings[0].Left = &changedFamily
	changed, err := config.Candidate(refs, time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NotEqual(t, baseline.Digest, changed.Digest)

	tests := []struct {
		name   string
		mutate func(*Configuration)
		want   string
	}{
		{"zero ID", func(c *Configuration) { c.TimelineMappings[0].ID = 0 }, "timelineMappings[0].id: must be greater than zero"},
		{"duplicate ID", func(c *Configuration) { c.TimelineMappings[1].ID = c.TimelineMappings[0].ID }, "timelineMappings[1].id: is duplicated"},
		{"empty mapping", func(c *Configuration) { c.TimelineMappings[0].Left, c.TimelineMappings[0].Right = nil, nil }, "timelineMappings[0]: must configure at least one side"},
		{"unknown family", func(c *Configuration) { c.TimelineMappings[0].Left = starFamilyPtr("UNKNOWN") }, "timelineMappings[0].left: must name a configured STAR family"},
		{"noncanonical family", func(c *Configuration) { c.TimelineMappings[0].Left = starFamilyPtr("tespi") }, "timelineMappings[0].left: must name a configured STAR family"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := goldenConfig(t)
			test.mutate(&candidate)
			require.ErrorContains(t, candidate.Validate(refs), test.want)
		})
	}
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
	*active.TimelineMappings[0].Left = "MUTATED"
	active.OverlayHoldings[0].MinimumAltitudeFt = intPtr(1)

	next := store.Active()
	require.Equal(t, aman.RunwayGroupID("04L"), next.RunwayGroups[0].Aliases[0])
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04L"), next.ActiveRunwayGroupSets[0][0])
	require.Equal(t, 55.5922, next.RunwayGroups[0].FinalApproaches[0].Threshold.Position.LatitudeDeg)
	require.Equal(t, navdata.FixID("TESPI"), next.Paths[0].Fixes[0])
	require.Equal(t, navdata.STARFamilyID("TESPI"), *next.TimelineMappings[0].Left)
	require.NotNil(t, next.OverlayHoldings[0].MinimumAltitudeFt)
	require.Equal(t, 5000, *next.OverlayHoldings[0].MinimumAltitudeFt)
}

func intPtr(value int) *int                                          { return &value }
func starFamilyPtr(value navdata.STARFamilyID) *navdata.STARFamilyID { return &value }

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
