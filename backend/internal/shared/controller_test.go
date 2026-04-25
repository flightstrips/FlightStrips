package shared

import (
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"testing"
)

func TestIsOperationalPositionController(t *testing.T) {
	t.Cleanup(config.SetOwnerCallsignPrefixesForTest([]string{"EKCH", "EKDK"}))

	tests := []struct {
		name       string
		controller *internalModels.Controller
		expected   bool
	}{
		{
			name:       "matching prefix is operational",
			controller: &internalModels.Controller{Callsign: "EKCH_A1_TWR"},
			expected:   true,
		},
		{
			name:       "observer is ignored",
			controller: &internalModels.Controller{Callsign: "EKCH_A__TWR", Observer: true},
			expected:   false,
		},
		{
			name:       "wrong prefix is ignored",
			controller: &internalModels.Controller{Callsign: "ESMS_TWR"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOperationalPositionController(tt.controller); got != tt.expected {
				t.Fatalf("IsOperationalPositionController(%+v) = %v, want %v", tt.controller, got, tt.expected)
			}
		})
	}
}

func TestIsOperationalControllerForPosition(t *testing.T) {
	t.Cleanup(config.SetOwnerCallsignPrefixesForTest([]string{"EKCH", "EKDK"}))

	position := &config.Position{Name: "EKDK_I_CTR", Frequency: "121.380"}
	controller := &internalModels.Controller{Callsign: "EKDK_1_CTR"}
	if !IsOperationalControllerForPosition(controller, position) {
		t.Fatal("expected controller to match config position")
	}
}
