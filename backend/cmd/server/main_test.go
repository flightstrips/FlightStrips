package main

import "testing"

func TestEnvBool(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		fallback bool
		expected bool
	}{
		{name: "unset defaults false", value: "", fallback: false, expected: false},
		{name: "true", value: "true", fallback: false, expected: true},
		{name: "false", value: "false", fallback: true, expected: false},
		{name: "malformed uses false fallback", value: "enabled", fallback: false, expected: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TEST_ENV_BOOL", test.value)
			if got := envBool("TEST_ENV_BOOL", test.fallback); got != test.expected {
				t.Fatalf("envBool() = %v, want %v", got, test.expected)
			}
		})
	}
}

func TestStandAssignmentFlagDefaultsFalseRegardlessOfEnvironment(t *testing.T) {
	t.Setenv("ENABLE_STAND_ASSIGNMENT", "")
	if got := envBool("ENABLE_STAND_ASSIGNMENT", false); got {
		t.Fatal("ENABLE_STAND_ASSIGNMENT should default to false when unset")
	}

	t.Setenv("ENVIRONMENT", "production")
	if got := envBool("ENABLE_STAND_ASSIGNMENT", false); got {
		t.Fatal("ENABLE_STAND_ASSIGNMENT should remain false in production unless explicitly enabled")
	}
}

func TestEFBFlagDefaultsFalseRegardlessOfEnvironment(t *testing.T) {
	t.Setenv("ENABLE_EFB", "")
	if got := envBool("ENABLE_EFB", false); got {
		t.Fatal("ENABLE_EFB should default to false when unset")
	}

	t.Setenv("ENVIRONMENT", "production")
	if got := envBool("ENABLE_EFB", false); got {
		t.Fatal("ENABLE_EFB should remain false in production unless explicitly enabled")
	}
}
