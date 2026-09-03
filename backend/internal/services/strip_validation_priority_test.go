package services

import (
	"testing"

	"FlightStrips/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestValidationPriorityOrder(t *testing.T) {
	t.Parallel()

	expected := [][]string{
		{landingClearanceValidationIssueType},
		{duplicateSquawkValidationIssueType},
		{pdcInvalidValidationIssueType, pdcCustomValidationIssueType},
		{wrongSquawkValidationIssueType},
		{runwayTypeValidationIssueType},
		{taxiwayTypeValidationIssueType},
		{ctotValidationIssueType},
		{models.ValidationIssueTypeStandAssignment},
	}

	assert.Equal(t, expected, validationPriorityOrder)
}

func TestValidationCandidateInhibition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		current   *models.ValidationStatus
		candidate string
		inhibited bool
	}{
		{name: "no current caution", candidate: wrongSquawkValidationIssueType},
		{name: "same caution refreshes", current: &models.ValidationStatus{IssueType: wrongSquawkValidationIssueType}, candidate: wrongSquawkValidationIssueType},
		{name: "higher caution inhibits lower", current: &models.ValidationStatus{IssueType: duplicateSquawkValidationIssueType}, candidate: wrongSquawkValidationIssueType, inhibited: true},
		{name: "acknowledged unresolved caution still inhibits", current: &models.ValidationStatus{IssueType: duplicateSquawkValidationIssueType, Active: false}, candidate: wrongSquawkValidationIssueType, inhibited: true},
		{name: "higher candidate replaces lower advisory", current: &models.ValidationStatus{IssueType: models.ValidationIssueTypeStandAssignment}, candidate: wrongSquawkValidationIssueType},
		{name: "stand advisory cannot inhibit PDC", current: &models.ValidationStatus{IssueType: models.ValidationIssueTypeStandAssignment}, candidate: pdcInvalidValidationIssueType},
		{name: "runway caution replaces taxiway caution", current: &models.ValidationStatus{IssueType: taxiwayTypeValidationIssueType}, candidate: runwayTypeValidationIssueType},
		{name: "taxiway caution inhibited by runway caution", current: &models.ValidationStatus{IssueType: runwayTypeValidationIssueType}, candidate: taxiwayTypeValidationIssueType, inhibited: true},
		{name: "same-tier PDC presentations may replace", current: &models.ValidationStatus{IssueType: pdcCustomValidationIssueType}, candidate: pdcInvalidValidationIssueType},
		{name: "unknown current caution is protected", current: &models.ValidationStatus{IssueType: "NEW SAFETY CAUTION"}, candidate: wrongSquawkValidationIssueType, inhibited: true},
		{name: "unknown candidate requires priority decision", current: &models.ValidationStatus{IssueType: wrongSquawkValidationIssueType}, candidate: "NEW SAFETY CAUTION", inhibited: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.inhibited, validationCandidateIsInhibited(tt.current, tt.candidate))
		})
	}
}
