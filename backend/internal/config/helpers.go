package config

import "slices"

func hasAnyActive(active []string, required []string) bool {
	if len(required) == 0 {
		return true
	}

	for _, a := range required {
		if slices.Contains(active, a) {
			return true
		}
	}

	return false
}

// scoreActive returns how many of required are present in active.
// An empty required list always matches with score 0.
// If requireAll is true, all entries in required must be present or -1 is returned.
// Returns -1 if required is non-empty and nothing (or not enough) matches.
func scoreActive(required []string, active []string, requireAll bool) int {
	if len(required) == 0 {
		return 0
	}
	score := 0
	for _, a := range required {
		if slices.Contains(active, a) {
			score++
		}
	}
	if score == 0 || (requireAll && score < len(required)) {
		return -1
	}
	return score
}
