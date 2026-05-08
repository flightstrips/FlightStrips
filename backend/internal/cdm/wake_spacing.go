package cdm

import "strings"

// wakeSeparationMinutes returns the required same-runway wake separation in minutes
// for a trailing departure behind a leading departure. Unsupported categories return 0.
func wakeSeparationMinutes(trailing, leading string) int {
	switch normalizeWakeCategory(trailing) {
	case "L":
		switch normalizeWakeCategory(leading) {
		case "L":
			return 1
		case "M", "H":
			return 2
		case "J":
			return 3
		}
	case "M":
		switch normalizeWakeCategory(leading) {
		case "L", "M":
			return 1
		case "H":
			return 2
		case "J":
			return 3
		}
	case "H":
		switch normalizeWakeCategory(leading) {
		case "L", "M", "H":
			return 1
		case "J":
			return 2
		}
	case "J":
		switch normalizeWakeCategory(leading) {
		case "L", "M", "H", "J":
			return 1
		}
	}

	return 0
}

func normalizeWakeCategory(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "L", "M", "H", "J":
		return normalized
	case "S", "SUPER":
		return "J"
	default:
		return ""
	}
}
