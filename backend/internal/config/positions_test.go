package config

import "testing"

func TestGetPositionBasedOnFrequency_NormalizesTrailingZeros(t *testing.T) {
	t.Cleanup(SetPositionsForTest([]Position{
		{Name: "EKCH_A_GND", Frequency: "121.600", Section: "GND"},
		{Name: "EKCH_C_TWR", Frequency: "118.580", Section: "TWR"},
	}))

	tests := []struct {
		name      string
		frequency string
		wantName  string
	}{
		{name: "ground short decimal", frequency: "121.6", wantName: "EKCH_A_GND"},
		{name: "tower short decimal", frequency: "118.58", wantName: "EKCH_C_TWR"},
		{name: "already normalized", frequency: "118.580", wantName: "EKCH_C_TWR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			position, err := GetPositionBasedOnFrequency(tt.frequency)
			if err != nil {
				t.Fatalf("expected position for %q, got error: %v", tt.frequency, err)
			}
			if position.Name != tt.wantName {
				t.Fatalf("expected %q for %q, got %q", tt.wantName, tt.frequency, position.Name)
			}
		})
	}
}

func TestCallsignHasOwnerPrefix(t *testing.T) {
	t.Cleanup(SetOwnerCallsignPrefixesForTest([]string{"EKCH", "EKDK"}))

	tests := []struct {
		name       string
		callsign   string
		expectedOk bool
	}{
		{name: "matches numbered variant", callsign: "EKCH_1_TWR", expectedOk: true},
		{name: "matches suffixed variant", callsign: "EKCH_A1_TWR", expectedOk: true},
		{name: "matches double underscore variant", callsign: "EKCH_A__TWR", expectedOk: true},
		{name: "matches ctr prefix", callsign: "ekdk_w_ctr", expectedOk: true},
		{name: "rejects different prefix", callsign: "ESMS_TWR", expectedOk: false},
		{name: "rejects blank callsign", callsign: "", expectedOk: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := CallsignHasOwnerPrefix(tt.callsign); got != tt.expectedOk {
				t.Fatalf("CallsignHasOwnerPrefix(%q) = %v, want %v", tt.callsign, got, tt.expectedOk)
			}
		})
	}
}
