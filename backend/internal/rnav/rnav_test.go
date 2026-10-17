package rnav

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveCapabilityRequiresEquipmentMarkerR(t *testing.T) {
	assert.Equal(t, NilCapability, DeriveCapability("B738/M-SDE2FGHIWY/LB1", "PBN/A1B1C1D1"))
	assert.Equal(t, "1", DeriveCapability("B738/M-SDE2FGHIRWY/LB1", "PBN/A1B1C1D1S1S2"))
}

func TestEffectiveCapabilityAssumesRNAV1ForListedAircraft(t *testing.T) {
	for _, aircraft := range []string{
		"A320", "A359", "A3N9", "B736", "B737", "B738", "B739", "B38M",
		"B744", "B748", "B752", "B753", "B762", "B763", "B764", "B77W",
		"B789", "BCS3", "AT72", "AT8A", "DH8D", "CRJ9", "E190", "E75L", "MD82", "MD88", "MD8A",
	} {
		t.Run(aircraft, func(t *testing.T) {
			for _, remarks := range []string{"", "PBN/A1", "PBN/A1B1", "PBN/A1B1C1", "PBN/A1B1C1D1"} {
				assert.Equal(t, "1", EffectiveCapability(aircraft+"/M-R/LB1", remarks))
			}
			assert.Equal(t, "1", EffectiveCapability(aircraft, ""))
		})
	}
}

func TestEffectiveCapabilityDoesNotMatchOtherAircraft(t *testing.T) {
	for _, aircraft := range []string{"A20N", "B735", "B734", "B743", "B741", "B761", "BCS1", "DH8C", "C172", "A3200", "AT7", "CRJ", "E19", "E1900", "E!90", "MD90", "MD8", "MD820"} {
		assert.Equal(t, NilCapability, EffectiveCapability(aircraft, ""), aircraft)
		assert.Equal(t, "2", EffectiveCapability(aircraft+"/M-R", "PBN/C1"), aircraft)
	}
	assert.Equal(t, "1", EffectiveCapability("  b738/m-s  ", ""))
}

func TestDeriveCapabilityUsesFirstPBNTokenAndPriority(t *testing.T) {
	aircraftInfo := "A20N/M-SDE2FGHIRWY/LB1"

	assert.Equal(t, "1", DeriveCapability(aircraftInfo, "REG/OYABC PBN/A1B1C1D1S1S2 RMK/OK"))
	assert.Equal(t, "2", DeriveCapability(aircraftInfo, "PBN/A1B1C1 PBN/A1B1C1D1"))
	assert.Equal(t, "5", DeriveCapability(aircraftInfo, "PBN/A1B1"))
	assert.Equal(t, "10", DeriveCapability(aircraftInfo, "PBN/A1"))
	assert.Equal(t, NilCapability, DeriveCapability(aircraftInfo, "PBN/Z9 RMK/NO_SUPPORTED_CAPABILITY"))
}

func TestBuildUpdateNilRemovesPBNAndPreservesAircraftInfo(t *testing.T) {
	aircraftInfo := "B738/M-SDE2FGHIRWY/LB1"
	updatedAircraftInfo, updatedRemarks, err := BuildUpdate(aircraftInfo, "REG/OYABC PBN/A1B1C1 RMK/OK", NilCapability)

	require.NoError(t, err)
	assert.Equal(t, aircraftInfo, updatedAircraftInfo)
	assert.Equal(t, "REG/OYABC RMK/OK", updatedRemarks)
}

func TestBuildUpdateNonNilReplacesPBNAndAddsEquipmentMarkerR(t *testing.T) {
	updatedAircraftInfo, updatedRemarks, err := BuildUpdate("B738/M-SDE2FGHIWY/LB1", "REG/OYABC PBN/A1 RMK/OK", "1")

	require.NoError(t, err)
	assert.Equal(t, "B738/M-SDE2FGHIWYR/LB1", updatedAircraftInfo)
	assert.Equal(t, "REG/OYABC PBN/A1B1C1D1S1S2 RMK/OK", updatedRemarks)
}

func TestBuildUpdateNonNilAppendsPBNWhenMissing(t *testing.T) {
	updatedAircraftInfo, updatedRemarks, err := BuildUpdate("B738", "REG/OYABC", "5")

	require.NoError(t, err)
	assert.Equal(t, "B738/M-R", updatedAircraftInfo)
	assert.Equal(t, "REG/OYABC PBN/A1B1", updatedRemarks)
}

func TestBuildUpdateNonNilDoesNotTreatWakeCategoryAsEquipment(t *testing.T) {
	updatedAircraftInfo, updatedRemarks, err := BuildUpdate("B738/M", "", "10")

	require.NoError(t, err)
	assert.Equal(t, "B738/M-R", updatedAircraftInfo)
	assert.Equal(t, "PBN/A1", updatedRemarks)
	assert.Equal(t, "10", DeriveCapability(updatedAircraftInfo, updatedRemarks))
	assert.Equal(t, NilCapability, DeriveCapability("B738/M", updatedRemarks))
}

func TestBuildUpdateNonNilPreservesExistingWakeCategoryWhenAddingEquipment(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected string
	}{
		{name: "heavy suffix", input: "A388/H", expected: "A388/H-R"},
		{name: "light suffix with surveillance", input: "C172/L/LB1", expected: "C172/L-R/LB1"},
		{name: "missing wake category before surveillance", input: "B738/LB1", expected: "B738/M-R/LB1"},
		{name: "missing wake category before equipment", input: "B738-SDE2FGHIWY/LB1", expected: "B738/M-SDE2FGHIWYR/LB1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			updatedAircraftInfo, updatedRemarks, err := BuildUpdate(tc.input, "", "2")

			require.NoError(t, err)
			assert.Equal(t, tc.expected, updatedAircraftInfo)
			assert.Equal(t, "PBN/A1B1C1", updatedRemarks)
			assert.Equal(t, "2", DeriveCapability(updatedAircraftInfo, updatedRemarks))
		})
	}
}

func TestBuildUpdateRejectsUnsupportedCapability(t *testing.T) {
	_, _, err := BuildUpdate("B738/M-SDE2FGHIWY/LB1", "", "3")
	assert.Error(t, err)
}
