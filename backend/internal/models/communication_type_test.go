package models

import "testing"

func TestCanonicalCommunicationType(t *testing.T) {
	for input, expected := range map[string]string{"t": "T", "T": "T", "r": "R", "R": "R", "v": "V", "V": "V", "?": "", "": "", "\x00": ""} {
		t.Run(input, func(t *testing.T) {
			if actual := NormalizeCommunicationType(input); actual != expected {
				t.Fatalf("got %q, want %q", actual, expected)
			}
			strip := &Strip{CommunicationType: &input}
			if strip.IsTextOnly() != (expected == "T") {
				t.Fatal("text-only decision differs from canonical state")
			}
		})
	}
	if (*Strip)(nil).IsTextOnly() || (&Strip{}).IsTextOnly() {
		t.Fatal("missing communication type must not enable text workflow")
	}
}
