package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStripToModel_MapsEmbeddedManualAndValidationFields(t *testing.T) {
	validationStatus := &models.ValidationStatus{
		IssueType:      "PDC INVALID",
		Message:        "Missing runway",
		OwningPosition: "EKCH_DEL",
		Active:         true,
		ActivationKey:  "abc-123",
	}
	rawValidationStatus, err := json.Marshal(validationStatus)
	require.NoError(t, err)

	personsOnBoard := int32(123)
	fplType := "I"
	language := "EN"

	strip, err := stripToModel(database.Strip{
		ID:                 42,
		Callsign:           "SAS123",
		Session:            7,
		Origin:             "EKCH",
		Destination:        "ESSA",
		Bay:                "CLEARED",
		TrackingController: "119.805",
		EngineType:         "JET",
		IsManual:           true,
		PersonsOnBoard:     &personsOnBoard,
		FplType:            &fplType,
		Language:           &language,
		HasFp:              true,
		ValidationStatus:   rawValidationStatus,
	})
	require.NoError(t, err)

	require.True(t, strip.IsManual)
	require.Equal(t, &personsOnBoard, strip.PersonsOnBoard)
	require.Equal(t, &fplType, strip.FplType)
	require.Equal(t, &language, strip.Language)
	require.True(t, strip.HasFP)
	require.NotNil(t, strip.ValidationStatus)
	require.Equal(t, *validationStatus, *strip.ValidationStatus)
}

func TestStripToModel_MapsPersistedArrivalETAInputs(t *testing.T) {
	eta := models.ArrivalETA{Source: "LIVE", EOBT: "0800", EnrouteDuration: "0245"}
	distance := 34.5
	groundspeed := int32(130)
	eta.DistanceNM = &distance
	eta.Groundspeed = &groundspeed
	rawETA, err := json.Marshal(eta)
	require.NoError(t, err)

	strip, err := stripToModel(database.Strip{
		Callsign:    "SAS123",
		Session:     7,
		Origin:      "EGLL",
		Destination: "EKCH",
		ArrivalEta:  rawETA,
	})
	require.NoError(t, err)
	require.NotNil(t, strip.ArrivalETA)
	require.Equal(t, eta, *strip.ArrivalETA)
}
