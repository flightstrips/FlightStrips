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
	require.Equal(t, []navdata.STARFamilyID{"ERNOV", "MONAK", "TESPI", "TIDVU", "TUDLO"}, starFamilyPolicyIDs(fragment.STARFamilyPolicies))
	for _, policy := range fragment.STARFamilyPolicies {
		require.Equal(t, navdata.SameSTARSpacingPolicy{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}, policy.SameSTARSpacing, policy.STARFamily)
		require.Equal(t, navdata.HoldingSequenceDisabled, policy.HoldingSequencePolicy, policy.STARFamily)
		require.Equal(t, 6*time.Minute, (time.Hour/time.Duration(policy.SameSTARSpacing.ActivationRatePerHour))*time.Duration(policy.SameSTARSpacing.MinimumEmptySlots+1))
	}
	require.Len(t, fragment.Paths, len(config.Feeders)*len(config.RunwayGroups))
	require.Len(t, fragment.Holdings, len(config.OverlayHoldings))
	wantDigests := map[string]string{
		"TESPI/ARRIVAL-04L": "bef2855dd861ec6465a9ef44c0ae801e1f8869be46dcc3d812547a2816186105",
		"TESPI/ARRIVAL-04R": "34be26cb5b3af5f70d0d782c00e9881235df9c9abdb8e28ca1c7a131f4a04377",
		"TUDLO/ARRIVAL-04L": "9ea0e127a1006323ef0e4a0b7ee59109625e9d46c004ca239c191dd4de0ff7e8",
		"TUDLO/ARRIVAL-04R": "023328771f957877e49c2f5754d3c647b257cd37bcf09525515ca5fb1059e089",
		"MONAK/ARRIVAL-04L": "ff5f58f8fa8dc7c4dc165dd0a366dcf76e0cd6c172418044192177183de4e0a3",
		"MONAK/ARRIVAL-04R": "01c85f22224c3f7521f792c0f23d76397d959eb0a7d71ea7b215fef4a6938e4b",
		"TIDVU/ARRIVAL-04L": "70de7dab937ff463e7f6179892875f66f27b5da0d74ae8de8405ba65ebf9feb4",
		"TIDVU/ARRIVAL-04R": "0e71cc7cfefa04a0cc9d358b5222b65db070bd46ac0eff47079e55be3f8964f0",
		"ERNOV/ARRIVAL-04L": "80c2ae1c3fa3dfd0d64c1fce81d02759fc435091bcd1fa1b2693dcf48e2184d8",
		"ERNOV/ARRIVAL-04R": "19e168c5c99c3f62c35f5156969dc2fd0fa13b5c01aeb815598bf85ae9018404",
		"TESPI/ARRIVAL-22L": "ada4fce932980c6fa5e55552199c4c72c864a9a8b58c24354eddc5dcf92d29e9",
		"TESPI/ARRIVAL-22R": "4b0cd1fb26e70a1cdde5ec114c0e5e12989dd162cc5979c3f44a2813025ae679",
		"TUDLO/ARRIVAL-22L": "063c7769ad6bc7383466799ff4bd64cc1ffe6fb49f0935a75d4d6c6817660339",
		"TUDLO/ARRIVAL-22R": "33d7f009107383d4a513b99f7ee70744bd98cff720dc1e49f5345d79f7668ada",
		"MONAK/ARRIVAL-22L": "41a6de8bc4c6d483b8ce23ad7ca6c96d30347f64050f5beb7e2830b3183dc94d",
		"MONAK/ARRIVAL-22R": "db41dc36a6177ebcf16876f76978d0b9a4a4675a8c333b29a03ccb11a433d593",
		"TIDVU/ARRIVAL-22L": "92a414e51ca021a6e6c9e7a5423ce7e6a6baebfa763b08ebc288cef2827d1979",
		"TIDVU/ARRIVAL-22R": "28f0cfb4df13d8fc7e14c298b9deba643e9e359edbb30ccec3b929ab24bf0685",
		"ERNOV/ARRIVAL-22L": "21e83108f5f82603a5b6a7b79f02c07a7386f185fe139fa91b1d84f013046839",
		"ERNOV/ARRIVAL-22R": "4645e486a2432a39df28e1b24ccf08357f5efc72e9bb3e04399e2490c4929e08",
		"TESPI/ARRIVAL-12":  "1bbfe2a6105caee099762059e7bcabd9b636588192808e07ffbbfccde5f7fa2f",
		"TUDLO/ARRIVAL-12":  "416b9f32c38d45f4aa36018be23714755d5d456e1234dab7e2609f63ae421ce3",
		"MONAK/ARRIVAL-12":  "8eae1b2641bd8b798a38a8344924c3017ffbd293302fa26f3e6767ef2c4e957a",
		"TIDVU/ARRIVAL-12":  "f84984e0189c9e61864e29e11c7f2fe436ee370b751b6ee46f23be00e6c964b5",
		"ERNOV/ARRIVAL-12":  "a32b4083500a6fe6a0898f344c746de11af99a39c001f22b7c9e2d12a17109a8",
		"TESPI/ARRIVAL-30":  "140832cad3e8f76caef4cf6489e80bab827369046c49e45153ac0a8006f67e4f",
		"TUDLO/ARRIVAL-30":  "31a620968a5c8e2c00376d675fdee5327a39707eeaf920af32a680db775b705c",
		"MONAK/ARRIVAL-30":  "a0fc4c1f1e7b9394aec4a3f47c4dbbf648e822a0be9e1c27311ba662ad8da5f7",
		"TIDVU/ARRIVAL-30":  "f08cf455ee79e161bd1b404471acbd7c9941bbe8dfc921b1bb311a544f1f47e7",
		"ERNOV/ARRIVAL-30":  "169b23a9883deba4b076774803cd98463ea971bed41d309da4cbe90e4363727c",
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
	require.Equal(t, "EKCH-AIP-2609-V3", config.ConfigVersion)
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
