package shared

import (
	"FlightStrips/internal/models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFrontendCdmDataEvent_IncludesMandatoryRouteRestrictionWhenFeatureEnabled(t *testing.T) {
	event := BuildFrontendCdmDataEvent("SAS123", (&models.CdmData{
		EcfmpRestrictions: []models.EcfmpRestriction{
			{Type: "mandatory_route", Routes: []string{"VEDAR DCT"}},
		},
	}).Normalize())

	require.Len(t, event.EcfmpRestrictions, 1)
	assert.Equal(t, "mandatory_route", event.EcfmpRestrictions[0].Type)
}
