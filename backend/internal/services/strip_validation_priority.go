package services

import internalModels "FlightStrips/internal/models"

// validationPriorityOrder is the strip-validation master-caution order.
// Earlier tiers inhibit later tiers. Issue types in the same tier are
// alternative presentations of the same operational condition and may replace
// one another. Unknown issue types are protected from replacement so adding a
// new validator requires an explicit priority decision.
var validationPriorityOrder = [][]string{
	{landingClearanceValidationIssueType},
	{duplicateSquawkValidationIssueType},
	{pdcInvalidValidationIssueType, pdcCustomValidationIssueType},
	{wrongSquawkValidationIssueType},
	{runwayTypeValidationIssueType},
	{taxiwayTypeValidationIssueType},
	{ctotValidationIssueType},
	{internalModels.ValidationIssueTypeStandAssignment},
}

func validationPriorityTier(issueType string) (int, bool) {
	for tier, issueTypes := range validationPriorityOrder {
		for _, candidate := range issueTypes {
			if issueType == candidate {
				return tier, true
			}
		}
	}
	return 0, false
}

// validationCandidateIsInhibited reports whether the currently presented
// validation must remain visible instead of the candidate. Inactive but still
// unresolved validations keep their priority after acknowledgement.
func validationCandidateIsInhibited(current *internalModels.ValidationStatus, candidateIssueType string) bool {
	if current == nil || current.IssueType == candidateIssueType {
		return false
	}

	currentTier, currentKnown := validationPriorityTier(current.IssueType)
	candidateTier, candidateKnown := validationPriorityTier(candidateIssueType)
	if !currentKnown || !candidateKnown {
		return true
	}

	return currentTier < candidateTier
}
