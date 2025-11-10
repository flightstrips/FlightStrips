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
