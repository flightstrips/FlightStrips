package pdc

import (
	"testing"

	"FlightStrips/internal/models"

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

	faults := service.validatePDCFlightPlan(strip, []string{"22R"})

	assert.Contains(t, faults, "Runway 22L is not an active departure runway")
}

func TestValidatePDCFlightPlan_SpecialRunwayAircraftSkipsActiveDepartureFault(t *testing.T) {
	t.Parallel()

	service := &Service{}
	strip := &models.Strip{
		AircraftType: stringPtrTest("A388/H"),
		Runway:       stringPtrTest("22L"),
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"})

	assert.Empty(t, faults)
}

func TestValidatePDCFlightPlan_SpecialRunwayAircraftStillRequiresConfiguredRunway(t *testing.T) {
	t.Parallel()

	service := &Service{}
	strip := &models.Strip{
		AircraftType: stringPtrTest("A388/H"),
		Runway:       stringPtrTest("22R"),
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"})

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
	}

	faults := PDCStripValidationFaults(strip, []string{"22R"})

	require.Empty(t, faults)
}

func stringPtrTest(value string) *string {
	return &value
}
