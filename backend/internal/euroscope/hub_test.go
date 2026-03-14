package euroscope

import (
	"FlightStrips/internal/shared"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---- buildOfflineBroadcastMessage ----

func TestBuildOfflineBroadcastMessage_NoChanges(t *testing.T) {
	msg := buildOfflineBroadcastMessage("EKCH_DEL", nil)
	assert.Equal(t, "EKCH_DEL went offline.", msg)
}

func TestBuildOfflineBroadcastMessage_SingleChange_NoCoverage(t *testing.T) {
	changes := []shared.SectorChange{{SectorName: "CLR", ToPosition: ""}}
	msg := buildOfflineBroadcastMessage("EKCH_DEL", changes)
	assert.Equal(t, "EKCH_DEL went offline. Sector CLR has no coverage.", msg)
}

func TestBuildOfflineBroadcastMessage_SingleChange_WithTransfer(t *testing.T) {
	changes := []shared.SectorChange{{SectorName: "CLR", ToPosition: "EKCH_TWR"}}
	msg := buildOfflineBroadcastMessage("EKCH_DEL", changes)
	assert.Equal(t, "EKCH_DEL went offline. Sector CLR transferred to EKCH_TWR.", msg)
}

func TestBuildOfflineBroadcastMessage_MultipleChanges(t *testing.T) {
	changes := []shared.SectorChange{
		{SectorName: "CLR", ToPosition: "EKCH_TWR"},
		{SectorName: "GND", ToPosition: ""},
	}
	msg := buildOfflineBroadcastMessage("EKCH_DEL", changes)
	assert.Equal(t, "EKCH_DEL went offline. Sectors: CLR (to EKCH_TWR), GND (no coverage).", msg)
}

// ---- buildOnlineBroadcastMessage ----

func TestBuildOnlineBroadcastMessage_NoChanges(t *testing.T) {
	msg := buildOnlineBroadcastMessage("EKCH_DEL", nil)
	assert.Equal(t, "EKCH_DEL is now online.", msg)
}

func TestBuildOnlineBroadcastMessage_SingleChange_NoPreviousCoverage(t *testing.T) {
	changes := []shared.SectorChange{{SectorName: "CLR", FromPosition: ""}}
	msg := buildOnlineBroadcastMessage("EKCH_DEL", changes)
	assert.Equal(t, "EKCH_DEL is now online. Sector CLR now has coverage.", msg)
}

func TestBuildOnlineBroadcastMessage_SingleChange_WithTransfer(t *testing.T) {
	changes := []shared.SectorChange{{SectorName: "CLR", FromPosition: "EKCH_TWR"}}
	msg := buildOnlineBroadcastMessage("EKCH_DEL", changes)
	assert.Equal(t, "EKCH_DEL is now online. Sector CLR transferred from EKCH_TWR.", msg)
}

func TestBuildOnlineBroadcastMessage_MultipleChanges(t *testing.T) {
	changes := []shared.SectorChange{
		{SectorName: "CLR", FromPosition: "EKCH_TWR"},
		{SectorName: "GND", FromPosition: ""},
	}
	msg := buildOnlineBroadcastMessage("EKCH_DEL", changes)
	assert.Equal(t, "EKCH_DEL is now online. Sectors: CLR (from EKCH_TWR), GND (no previous coverage).", msg)
}
