package pdc

import (
	"testing"
	"time"

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
		AircraftType: stringPtrTest("A380/H"),
		Runway:       stringPtrTest("22L"),
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"})

	assert.Empty(t, faults)
}

func TestValidatePDCFlightPlan_SpecialRunwayAircraftStillRequiresConfiguredRunway(t *testing.T) {
	t.Parallel()

	service := &Service{}
	strip := &models.Strip{
		AircraftType: stringPtrTest("A380/H"),
		Runway:       stringPtrTest("22R"),
	}

	faults := service.validatePDCFlightPlan(strip, []string{"22R"})

	assert.Contains(t, faults, "Aircraft type A380/H is not allowed on runway 22R")
	assert.NotContains(t, faults, "Runway 22R is not an active departure runway")
}

func TestPDCStripValidationFaults_ExcludesEobtOnlyFaults(t *testing.T) {
	t.Parallel()

	sid := "BETUD"
	eobt := time.Now().UTC().Format("1504")
	strip := &models.Strip{
		Sid:     &sid,
		CdmData: &models.CdmData{Eobt: &eobt},
	}

	faults := PDCStripValidationFaults(strip, []string{"22R"})

	require.Len(t, faults, 1)
	assert.Equal(t, FlightPlanValidationFaultKindSID, faults[0].Kind)
	assert.Equal(t, "SID BETUD is not available via PDC", faults[0].Message)
}

func stringPtrTest(value string) *string {
	return &value
}
