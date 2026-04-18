package vatsim

import "testing"

func TestNormalizeFrequency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "already normalized", input: "118.105", want: "118.105"},
		{name: "short decimal", input: "121.9", want: "121.900"},
		{name: "hz frequency", input: "118105000", want: "118.105"},
		{name: "invalid", input: "abc", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := NormalizeFrequency(tt.input); got != tt.want {
				t.Fatalf("NormalizeFrequency(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
