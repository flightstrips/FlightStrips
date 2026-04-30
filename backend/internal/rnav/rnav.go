package rnav

import (
	"errors"
	"regexp"
	"strings"
)

const NilCapability = "NIL"

var pbnTokenPattern = regexp.MustCompile(`(?i)^PBN/[A-Z0-9]+$`)

var saveRemarksByCapability = map[string]string{
	"10": "PBN/A1",
	"5":  "PBN/A1B1",
	"2":  "PBN/A1B1C1",
	"1":  "PBN/A1B1C1D1S1S2",
}

// DeriveCapability returns the CLX RNAV value derived from the ICAO aircraft
// equipment marker and the first PBN/... token in remarks.
func DeriveCapability(aircraftInfo, remarks string) string {
	if !HasEquipmentMarkerR(aircraftInfo) {
		return NilCapability
	}

	pbn := firstPBNSuffix(remarks)
	if pbn == "" {
		return NilCapability
	}

	switch {
	case strings.Contains(pbn, "D1"):
		return "1"
	case strings.Contains(pbn, "C1"):
		return "2"
	case strings.Contains(pbn, "B1"):
		return "5"
	case strings.Contains(pbn, "A1"):
		return "10"
	default:
		return NilCapability
	}
}

// BuildUpdate applies the selected CLX RNAV value to remarks and, for non-NIL
// values, ensures the ICAO equipment marker R is present.
func BuildUpdate(aircraftInfo, remarks, capability string) (string, string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(capability))
	if normalized == "" {
		normalized = NilCapability
	}

	if normalized == NilCapability {
		return aircraftInfo, replacePBNTokens(remarks, ""), nil
	}

	pbn, ok := saveRemarksByCapability[normalized]
	if !ok {
		return "", "", errors.New("unsupported RNAV capability")
	}

	return AddEquipmentMarkerR(aircraftInfo), replacePBNTokens(remarks, pbn), nil
}

func HasEquipmentMarkerR(aircraftInfo string) bool {
	equipment, ok := equipmentSegment(aircraftInfo)
	if !ok {
		return false
	}
	return strings.Contains(strings.ToUpper(equipment), "R")
}

func AddEquipmentMarkerR(aircraftInfo string) string {
	if HasEquipmentMarkerR(aircraftInfo) {
		return aircraftInfo
	}

	token, suffix := splitAircraftInfoToken(aircraftInfo)
	if token == "" {
		return "/M-R" + suffix
	}

	return addEquipmentMarkerRToToken(token) + suffix
}

func addEquipmentMarkerRToToken(token string) string {
	dash := strings.Index(token, "-")
	if dash >= 0 {
		end := segmentEnd(token, dash+1)
		if end <= dash+1 {
			return token
		}
		if !hasWakeTurbulenceCategoryBeforeDash(token, dash) {
			token = token[:dash] + "/M" + token[dash:]
			dash += 2
			end += 2
		}
		return token[:end] + "R" + token[end:]
	}

	firstSlash := strings.Index(token, "/")
	if firstSlash < 0 {
		return token + "/M-R"
	}

	segmentStart := firstSlash + 1
	segmentEnd := segmentEnd(token, segmentStart)
	if isWakeTurbulenceCategory(token[segmentStart:segmentEnd]) {
		return token[:segmentEnd] + "-R" + token[segmentEnd:]
	}

	return token[:firstSlash] + "/M-R" + token[firstSlash:]
}

func firstPBNSuffix(remarks string) string {
	for _, token := range strings.Fields(remarks) {
		if pbnTokenPattern.MatchString(token) {
			return strings.TrimPrefix(strings.ToUpper(token), "PBN/")
		}
	}
	return ""
}

func replacePBNTokens(remarks, replacement string) string {
	fields := strings.Fields(remarks)
	if len(fields) == 0 {
		return replacement
	}

	result := make([]string, 0, len(fields)+1)
	replaced := false
	for _, field := range fields {
		if pbnTokenPattern.MatchString(field) {
			if replacement != "" && !replaced {
				result = append(result, replacement)
				replaced = true
			}
			continue
		}
		result = append(result, field)
	}

	if replacement != "" && !replaced {
		result = append(result, replacement)
	}
	return strings.Join(result, " ")
}

func equipmentSegment(aircraftInfo string) (string, bool) {
	start, end, ok := equipmentBounds(aircraftInfo)
	if !ok {
		return "", false
	}
	return aircraftInfo[start:end], true
}

func equipmentBounds(aircraftInfo string) (int, int, bool) {
	if aircraftInfo == "" {
		return 0, 0, false
	}

	start := strings.Index(aircraftInfo, "-")
	if start >= 0 {
		start++
		end := segmentEnd(aircraftInfo, start)
		if start >= end {
			return 0, 0, false
		}
		return start, end, true
	}

	for slash := strings.Index(aircraftInfo, "/"); slash >= 0 && slash < len(aircraftInfo)-1; {
		start = slash + 1
		end := segmentEnd(aircraftInfo, start)
		segment := aircraftInfo[start:end]
		if !isWakeTurbulenceCategory(segment) {
			return start, end, true
		}
		next := strings.Index(aircraftInfo[end:], "/")
		if next < 0 {
			break
		}
		slash = end + next
	}

	return 0, 0, false
}

func segmentEnd(value string, start int) int {
	end := len(value)
	for i := start; i < len(value); i++ {
		if value[i] == '/' || value[i] == ' ' || value[i] == '\t' {
			return i
		}
	}
	return end
}

func isWakeTurbulenceCategory(segment string) bool {
	switch strings.ToUpper(segment) {
	case "L", "M", "H", "J":
		return true
	default:
		return false
	}
}

func splitAircraftInfoToken(aircraftInfo string) (string, string) {
	trimmed := strings.TrimSpace(aircraftInfo)
	if trimmed == "" {
		return "", ""
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] == ' ' || trimmed[i] == '\t' {
			return trimmed[:i], trimmed[i:]
		}
	}
	return trimmed, ""
}

func hasWakeTurbulenceCategoryBeforeDash(token string, dash int) bool {
	if dash <= 0 {
		return false
	}

	slash := strings.LastIndex(token[:dash], "/")
	if slash < 0 || slash == dash-1 {
		return false
	}

	return isWakeTurbulenceCategory(token[slash+1 : dash])
}
