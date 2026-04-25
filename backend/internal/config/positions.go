package config

import (
	"errors"
	"strings"
)

type Position struct {
	Name      string `yaml:"name"`
	Frequency string `yaml:"frequency"`
	Section   string `yaml:"section"`
}

func GetPositionBasedOnFrequency(frequency string) (*Position, error) {
	for _, pos := range positions {
		if pos.Frequency == frequency {
			return &pos, nil
		}
	}

	return nil, errors.New("unknown position")
}

// GetAirborneOwners returns the ordered list of airborne position names (priority = first).
func GetAirborneOwners() []string {
	return airborneOwners
}

func GetPositionByName(name string) (*Position, error) {
	for _, pos := range positions {
		if strings.EqualFold(pos.Name, name) {
			return &pos, nil
		}
	}

	return nil, errors.New("unknown position")
}

func CallsignHasOwnerPrefix(callsign string) bool {
	callsign = strings.ToUpper(strings.TrimSpace(callsign))
	if callsign == "" {
		return false
	}

	for _, prefix := range ownerCallsignPrefixes {
		if strings.HasPrefix(callsign, prefix) {
			return true
		}
	}

	return false
}

func identifierPrefix(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	prefix, _, _ := strings.Cut(value, "_")
	return prefix
}
