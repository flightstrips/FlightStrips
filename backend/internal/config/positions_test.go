package config

import "testing"

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
