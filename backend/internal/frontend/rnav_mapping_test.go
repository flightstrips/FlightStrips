package frontend

import (
	"testing"

	"FlightStrips/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestMapStripToFrontendModelDerivesRNAVCapabilityFromAircraftInfoAndRemarks(t *testing.T) {
	aircraftType := "A20N/M-SDE2FGHIRWY/LB1"
	remarks := "REG/OYABC PBN/A1B1C1D1S1S2"
	rawCapabilities := "?"

	strip := MapStripToFrontendModel(&models.Strip{
		Callsign:     "SAS123",
		AircraftType: &aircraftType,
		Remarks:      &remarks,
		Capabilities: &rawCapabilities,
	})

	assert.Equal(t, "1", strip.Capabilities)
}

func TestMapStripToFrontendModelRNAVIsNilWhenEquipmentMarkerRAbsent(t *testing.T) {
	aircraftType := "A20N/M-SDE2FGHIWY/LB1"
	remarks := "PBN/A1B1C1D1S1S2"
	rawCapabilities := "1"

	strip := MapStripToFrontendModel(&models.Strip{
		Callsign:     "SAS123",
		AircraftType: &aircraftType,
		Remarks:      &remarks,
		Capabilities: &rawCapabilities,
	})

	assert.Equal(t, "NIL", strip.Capabilities)
}

func TestMapStripToFrontendModelMapsStoredSpokenCallsign(t *testing.T) {
	spokenCallsign := "ALPACA"

	strip := MapStripToFrontendModel(&models.Strip{
		Callsign:       "WLF166",
		SpokenCallsign: &spokenCallsign,
	})

	assert.Equal(t, "ALPACA", strip.SpokenCallsign)
}

func TestMapStripToFrontendModelLeavesSpokenCallsignEmptyWhenMissing(t *testing.T) {
	strip := MapStripToFrontendModel(&models.Strip{
		Callsign: "SAS123",
	})

	assert.Equal(t, "", strip.SpokenCallsign)
}
