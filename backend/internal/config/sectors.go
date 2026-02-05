package config

import (
	"errors"
	"slices"
	"strings"
)

type constraints struct {
	Departure bool `yaml:"departure"`
	Arrival   bool `yaml:"arrival"`
}

type Sector struct {
	Name         string       `yaml:"name"`
	NamePriority int          `yaml:"name_priority"`
	Region       []string     `yaml:"region"`
	Constraints  *constraints `yaml:"constraints"`
	Active       []string     `yaml:"active"`
	Owner        []string     `yaml:"owner"`
}

func GetSectorFromRegion(region *Region, isArrival bool) (string, error) {
	for _, sector := range sectors {
		if sector.Constraints != nil && (sector.Constraints.Arrival != isArrival || sector.Constraints.Departure != !isArrival) {
			continue
		}
		for _, r := range sector.Region {
			if strings.EqualFold(r, region.Name) {
				return sector.Name, nil
			}
		}
	}

	return "", errors.New("sector not found")
}

func GetControllerSectors(controllers []*Position, active []string) map[string][]Sector {
	var result = make(map[string][]Sector)
	for _, c := range controllers {
		result[c.Frequency] = make([]Sector, 0)
	}

	var lookup = make(map[string]string)
	for _, c := range controllers {
		lookup[c.Name] = c.Frequency
	}

	for _, s := range sectors {
		if !isActive(s, active) {
			continue
		}

		for _, owner := range s.Owner {
			if _, ok := result[lookup[owner]]; ok {
				result[lookup[owner]] = append(result[lookup[owner]], s)
				break
			}
		}
	}

	return result
}

func isActive(sector Sector, active []string) bool {
	if len(sector.Active) == 0 {
		return true
	}
	for _, a := range active {
		if slices.Contains(sector.Active, a) {
			return true
		}
	}
	return false
}
