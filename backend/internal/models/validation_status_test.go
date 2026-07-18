package models

import "testing"

func TestValidationStatusIsBlocking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status *ValidationStatus
		want   bool
	}{
		{name: "missing status", status: nil, want: false},
		{name: "inactive validation", status: &ValidationStatus{IssueType: "WRONG SQUAWK"}, want: false},
		{name: "active operational validation", status: &ValidationStatus{IssueType: "WRONG SQUAWK", Active: true}, want: true},
		{name: "active stand assignment advisory", status: &ValidationStatus{IssueType: ValidationIssueTypeStandAssignment, Active: true}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.status.IsBlocking(); got != test.want {
				t.Fatalf("IsBlocking() = %t, want %t", got, test.want)
			}
		})
	}
}
