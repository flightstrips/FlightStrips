package models

import "strings"

// NormalizeCommunicationType is the backend's canonical communication state.
// EuroScope may send lowercase values; unknown/unassigned values stay empty.
func NormalizeCommunicationType(value string) string {
	switch value = strings.ToUpper(value); value {
	case "T", "R", "V":
		return value
	default:
		return ""
	}
}

func (s *Strip) IsTextOnly() bool {
	return s != nil && s.CommunicationType != nil && NormalizeCommunicationType(*s.CommunicationType) == "T"
}
