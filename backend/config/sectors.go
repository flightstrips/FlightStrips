package config

import (
	"slices"
)

type constraints struct {
	Departure bool `yaml:"departure"`
	Arrival   bool `yaml:"arrival"`
}

type Sector struct {
	Name         string       `yaml:"name"`
	NamePriority int          `yaml:"name_priority"`
	Region       *string      `yaml:"region"`
	Constraints  *constraints `yaml:"constraints"`
	Active       []string     `yaml:"active"`
	Owner        []string     `yaml:"owner"`
}

func GetControllerSectors(controllers []Position, active []string) map[string][]Sector {
	var result = make(map[string][]Sector)
	for _, c := range controllers {
		result[c.Name] = make([]Sector, 0)
	}

	for _, s := range sectors {
		if !isActive(s, active) {
			continue
		}

		for _, owner := range s.Owner {
			if _, ok := result[owner]; ok {
				result[owner] = append(result[owner], s)
				break
			}
		}
	}

	return result
}

func isActive(sector Sector, active []string) bool {
	for _, a := range active {
		if slices.Contains(sector.Active, a) {
			return true
		}
	}
	return false
}
