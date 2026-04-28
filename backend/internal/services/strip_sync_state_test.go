package services

import (
	"testing"

	"FlightStrips/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestSyncStripChanged_ReturnsFalseForEquivalentSyncState(t *testing.T) {
	remarks := "NIL"
	route := "DCT"
	alternate := ""
	runway := ""
	assignedSquawk := "1234"
	squawk := "1234"
	sid := "NOVEN1A"
	aircraftType := "A20N"
	requestedAltitude := int32(5000)
	clearedAltitude := int32(5000)
	heading := int32(180)
	lat := 55.0
	lon := 12.0
	altitude := int32(1000)
	aircraftCategory := "M"
	stand := "A1"
	capabilities := "SDFGHIRWY"
	communicationType := "VOICE"
	groundState := "TAXI"
	eobt := "1200"

	existing := &models.Strip{
		Origin:             "EKCH",
		Destination:        "ESSA",
		Alternative:        &alternate,
		Remarks:            &remarks,
		Route:              &route,
		Runway:             &runway,
		AssignedSquawk:     &assignedSquawk,
		Squawk:             &squawk,
		Sid:                &sid,
		AircraftType:       &aircraftType,
		RequestedAltitude:  &requestedAltitude,
		ClearedAltitude:    &clearedAltitude,
		Heading:            &heading,
		AircraftCategory:   &aircraftCategory,
		Stand:              &stand,
		Capabilities:       &capabilities,
		CommunicationType:  &communicationType,
		State:              &groundState,
		CdmData:            models.NewLegacyCdmData(&eobt, nil, nil, nil, nil, nil, &eobt, nil),
		Bay:                "TAXI",
		TrackingController: "EKCH_APP",
		EngineType:         "JET",
		PositionLatitude:   &lat,
		PositionLongitude:  &lon,
		PositionAltitude:   &altitude,
	}

	updated := &models.Strip{
		Origin:             "EKCH",
		Destination:        "ESSA",
		Alternative:        &alternate,
		Remarks:            &remarks,
		Route:              &route,
		Runway:             &runway,
		AssignedSquawk:     &assignedSquawk,
		Squawk:             &squawk,
		Sid:                &sid,
		AircraftType:       &aircraftType,
		RequestedAltitude:  &requestedAltitude,
		ClearedAltitude:    &clearedAltitude,
		Heading:            &heading,
		AircraftCategory:   &aircraftCategory,
		Stand:              &stand,
		Capabilities:       &capabilities,
		CommunicationType:  &communicationType,
		State:              &groundState,
		CdmData:            models.NewLegacyCdmData(&eobt, nil, nil, nil, nil, nil, &eobt, nil),
		Bay:                "TAXI",
		TrackingController: "EKCH_APP",
		EngineType:         "JET",
		PositionLatitude:   &lat,
		PositionLongitude:  &lon,
		PositionAltitude:   &altitude,
	}

	assert.False(t, syncStripChanged(existing, updated))
}
