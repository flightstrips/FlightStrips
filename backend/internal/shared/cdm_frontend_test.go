package shared

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFrontendCdmDataEvent_HidesMandatoryRouteRestrictionWhenFeatureDisabled(t *testing.T) {
	t.Cleanup(config.SetFeatureFlagsForTest(config.FeatureFlagsConfig{}))

	event := BuildFrontendCdmDataEvent("SAS123", (&models.CdmData{
		EcfmpRestrictions: []models.EcfmpRestriction{
			{Type: "mandatory_route", Routes: []string{"VEDAR DCT"}},
			{Type: "ground_stop", Reason: "Weather"},
		},
	}).Normalize())

	require.Len(t, event.EcfmpRestrictions, 1)
	assert.Equal(t, "ground_stop", event.EcfmpRestrictions[0].Type)
}

func TestBuildFrontendCdmDataEvent_IncludesMandatoryRouteRestrictionWhenFeatureEnabled(t *testing.T) {
	t.Cleanup(config.SetFeatureFlagsForTest(config.FeatureFlagsConfig{MandatoryRouteClearanceFlow: true}))

	event := BuildFrontendCdmDataEvent("SAS123", (&models.CdmData{
		EcfmpRestrictions: []models.EcfmpRestriction{
			{Type: "mandatory_route", Routes: []string{"VEDAR DCT"}},
		},
	}).Normalize())

	require.Len(t, event.EcfmpRestrictions, 1)
	assert.Equal(t, "mandatory_route", event.EcfmpRestrictions[0].Type)
}
