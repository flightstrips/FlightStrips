package cdm

import "strings"

func deiceTypeToMinutes(deiceType string) int {
	switch strings.ToUpper(strings.TrimSpace(deiceType)) {
	case "L":
		return 7
	case "M":
		return 10
	case "H":
		return 13
	case "J":
		return 18
	default:
		return 0
	}
}
