package pdc

import (
	"testing"

	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	pkgModels "FlightStrips/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidPDCSquawk_RejectsReservedCodes(t *testing.T) {
	t.Parallel()

	for _, squawk := range []string{"1000", "1234", "2000", "2200", "7000"} {
		assert.False(t, isValidPDCSquawk(squawk), squawk)
	}
}

func TestValidatePDCFlightPlan_FaultsWhenRunwayIsNotActiveDeparture(t *testing.T) {
	t.Parallel()

	service := &Service{}
	strip := &models.Strip{
		AircraftType: stringPtrTest("A320"),
		Runway:       stringPtrTest("22L"),
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"}, nil)

	assert.Contains(t, faults, "Runway 22L is not an active departure runway")
}

func TestValidatePDCFlightPlan_SpecialRunwayAircraftSkipsActiveDepartureFault(t *testing.T) {
	t.Parallel()

	service := &Service{}
	strip := &models.Strip{
		AircraftType: stringPtrTest("A388/H"),
		Runway:       stringPtrTest("22L"),
		Sid:          stringPtrTest("VEMBO2E"),
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"}, nil)

	assert.Empty(t, faults)
}

func TestValidatePDCFlightPlan_SpecialRunwayAircraftStillRequiresConfiguredRunway(t *testing.T) {
	t.Parallel()

	service := &Service{}
	strip := &models.Strip{
		AircraftType: stringPtrTest("A388/H"),
		Runway:       stringPtrTest("22R"),
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"}, nil)

	assert.Contains(t, faults, "Aircraft type A388/H is not allowed on runway 22R")
	assert.NotContains(t, faults, "Runway 22R is not an active departure runway")
}

func TestRunwayTypeValidationFault_ReturnsConfiguredAircraftRunwayFault(t *testing.T) {
	t.Parallel()

	fault := RunwayTypeValidationFault(&models.Strip{
		AircraftType: stringPtrTest("AN225"),
		Runway:       stringPtrTest("04L"),
	})

	require.NotNil(t, fault)
	assert.Equal(t, FlightPlanValidationFaultKindRunway, fault.Kind)
	assert.Equal(t, "Aircraft type AN225 is not allowed on runway 04L", fault.Message)
}

func TestPDCStripValidationFaults_IgnoresEobtOutsideFormerWindow(t *testing.T) {
	t.Parallel()

	eobt := "2359"
	strip := &models.Strip{
		CdmData: &models.CdmData{Eobt: &eobt},
		Sid:     stringPtrTest("VEMBO2E"),
	}

	faults := PDCStripValidationFaults(strip, []string{"22R"}, nil)

	require.Empty(t, faults)
}

func TestValidatePDCFlightPlan_FaultsWhenNoSIDOrVectors(t *testing.T) {
	t.Parallel()

	service := &Service{}
	strip := &models.Strip{
		AircraftType: stringPtrTest("A320"),
		Runway:       stringPtrTest("22R"),
		Sid:          stringPtrTest("   "),
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"}, nil)

	assert.Contains(t, faults, "No SID or vectored departure assigned")
}

func TestValidatePDCFlightPlan_NoRoutingFaultWithUsableSID(t *testing.T) {
	t.Parallel()

	service := &Service{}
	strip := &models.Strip{
		AircraftType: stringPtrTest("A320"),
		Runway:       stringPtrTest("22R"),
		Sid:          stringPtrTest("VEMBO2E"),
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"}, nil)

	assert.NotContains(t, faults, "No SID or vectored departure assigned")
}

func TestValidatePDCFlightPlan_NoRoutingFaultWithVectors(t *testing.T) {
	t.Parallel()

	service := &Service{}
	strip := &models.Strip{
		AircraftType:    stringPtrTest("A320"),
		Runway:          stringPtrTest("22R"),
		Heading:         int32PtrTest(180),
		ClearedAltitude: int32PtrTest(7000),
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"}, nil)

	assert.NotContains(t, faults, "No SID or vectored departure assigned")
}

func TestValidatePDCFlightPlan_FaultsWhenHeadingWithoutAltitude(t *testing.T) {
	t.Parallel()

	service := &Service{}
	strip := &models.Strip{
		AircraftType: stringPtrTest("A320"),
		Runway:       stringPtrTest("22R"),
		Heading:      int32PtrTest(180),
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"}, nil)

	assert.Contains(t, faults, "No SID or vectored departure assigned")
}

func TestValidatePDCFlightPlan_MandatoryRouteCreatesManualReviewFault(t *testing.T) {
	t.Parallel()
	t.Cleanup(config.SetFeatureFlagsForTest(config.FeatureFlagsConfig{MandatoryRouteClearanceFlow: true}))

	service := &Service{}
	route := "GOLGA DCT"
	runway := "22R"
	sid := "GOLGA2C"
	strip := &models.Strip{
		Route:  &route,
		Runway: &runway,
		Sid:    &sid,
		CdmData: &models.CdmData{
			EcfmpRestrictions: []models.EcfmpRestriction{
				{Type: "mandatory_route", Routes: []string{"VEDAR DCT"}},
			},
		},
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"}, pkgModels.AvailableSids{
		{Name: "VEDAR2C", Runway: "22R"},
	})

	require.Contains(t, faults, "Mandatory route VEDAR DCT requires controller review before PDC can be issued. Use SID VEDAR2C.")
}

func TestValidatePDCFlightPlan_MandatoryRouteResolvedSidAvoidsRoutingFault(t *testing.T) {
	t.Parallel()
	t.Cleanup(config.SetFeatureFlagsForTest(config.FeatureFlagsConfig{MandatoryRouteClearanceFlow: true}))

	service := &Service{}
	runway := "22R"
	strip := &models.Strip{
		Runway: &runway,
		CdmData: &models.CdmData{
			EcfmpRestrictions: []models.EcfmpRestriction{
				{Type: "mandatory_route", Routes: []string{"VEDAR DCT"}},
			},
		},
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"}, pkgModels.AvailableSids{
		{Name: "VEDAR2C", Runway: "22R"},
	})

	assert.Contains(t, faults, "Mandatory route VEDAR DCT requires controller review before PDC can be issued. Use SID VEDAR2C.")
	assert.NotContains(t, faults, "No SID or vectored departure assigned")
}

func TestResolveMandatoryRouteSID_PrefersCurrentVariantForRunway(t *testing.T) {
	t.Parallel()

	runway := "22R"
	sid := "GOLGA2C"
	strip := &models.Strip{
		Runway: &runway,
		Sid:    &sid,
	}

	resolved := resolveMandatoryRouteSID(strip, pkgModels.AvailableSids{
		{Name: "VEDAR2F", Runway: "22R"},
		{Name: "VEDAR2C", Runway: "22R"},
	}, "VEDAR")

	assert.Equal(t, "VEDAR2C", resolved)
}

func stringPtrTest(value string) *string {
	return &value
}

func int32PtrTest(value int32) *int32 {
	return &value
}
