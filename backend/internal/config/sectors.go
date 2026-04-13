package config

import (
	"errors"
	"strings"
)

type constraints struct {
	Departure bool `yaml:"departure"`
	Arrival   bool `yaml:"arrival"`
}

type Sector struct {
	Name         string       `yaml:"name"`
	Key          string       `yaml:"key"`
	NamePriority int          `yaml:"name_priority"`
	Region       []string     `yaml:"region"`
	Constraints  *constraints `yaml:"constraints"`
	Active       []string     `yaml:"active"`
	RequireAll   bool         `yaml:"require_all"`
	Owner        []string     `yaml:"owner"`
}

func (s Sector) KeyOrName() string {
	if strings.TrimSpace(s.Key) != "" {
		return s.Key
	}
	return s.Name
}

func GetSectorFromRegion(region *Region, isArrival bool) (string, error) {
	for _, sector := range sectors {
		if sector.Constraints != nil && (sector.Constraints.Arrival != isArrival || sector.Constraints.Departure != !isArrival) {
			continue
		}
		for _, r := range sector.Region {
			if strings.EqualFold(r, region.Name) {
				return sector.KeyOrName(), nil
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

	// Group sectors by name, then pick the most specific match per group.
	type scored struct {
		sector Sector
		score  int
	}
	byKey := make(map[string]scored)
	for _, s := range sectors {
		score := matchScore(s, active)
		if score < 0 {
			continue
		}
		key := s.KeyOrName()
		if prev, seen := byKey[key]; !seen || score > prev.score {
			byKey[key] = scored{s, score}
		}
	}

	for _, entry := range byKey {
		s := entry.sector
		for _, owner := range s.Owner {
			if _, ok := result[lookup[owner]]; ok {
				result[lookup[owner]] = append(result[lookup[owner]], s)
				break
			}
		}
	}

	return result
}

// matchScore returns how many of the sector's active runways are in the
// current active set. A sector with an empty active list always matches
// with score 0. Returns -1 if the sector has no match at all.
func matchScore(sector Sector, active []string) int {
	return scoreActive(sector.Active, active, sector.RequireAll)
}

func GetSectorDisplayName(sectorRef string) string {
	for _, sector := range sectors {
		if sectorMatchesIdentifier(sector, sectorRef) {
			return sector.Name
		}
	}

	return sectorRef
}

func sectorMatchesIdentifier(sector Sector, sectorRef string) bool {
	return strings.EqualFold(sector.KeyOrName(), sectorRef) || strings.EqualFold(sector.Name, sectorRef)
}
