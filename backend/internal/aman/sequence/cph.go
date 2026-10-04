package sequence

import "time"

// CPHSeparationRules is shared by committed sequencing and provisional ground
// arrival planning so both apply the same wake spacing.
func CPHSeparationRules() []SeparationRule {
	categories := []WakeCategory{"L", "M", "H", "J"}
	rules := make([]SeparationRule, 0, len(categories)*len(categories))
	for _, leading := range categories {
		for _, trailing := range categories {
			gap := time.Duration(0)
			if leading == "H" {
				gap = 120 * time.Second
			}
			if leading == "J" || trailing == "L" {
				gap = 180 * time.Second
			}
			rules = append(rules, SeparationRule{Leading: leading, Trailing: trailing, Minimum: gap})
		}
	}
	return rules
}
