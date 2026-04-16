package pdc

import (
	"testing"

	"FlightStrips/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestBuildWebPDCStatus_HidesRequestedWithFaults(t *testing.T) {
	remarks := "NO SID"
	status := buildWebPDCStatus(&models.Strip{
		Callsign:          "SAS123",
		PdcState:          string(StateRequestedWithFaults),
		PdcRequestRemarks: &remarks,
	})

	assert.Equal(t, string(StateRequested), status.State)
	assert.Equal(t, remarks, valueOrEmpty(status.RequestRemarks))
	assert.False(t, status.CanSubmit)
	assert.False(t, status.RequiresPilotAction)
}
