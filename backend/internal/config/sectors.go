package config

import (
	"FlightStrips/internal/vatsim"
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
	Key          string       `yaml:"key"`
	NamePriority int          `yaml:"name_priority"`
	Region       []string     `yaml:"region"`
	Constraints  *constraints `yaml:"constraints"`
	Active       []string     `yaml:"active"`
	RequireAll   bool         `yaml:"require_all"`
	Owner        []string     `yaml:"owner"`
}

type ControllerCoverage struct {
	Name               string
	Frequency          string
	CoveredFrequencies []string
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
	coverage := make([]ControllerCoverage, 0, len(controllers))
	for _, controller := range controllers {
		coverage = append(coverage, ControllerCoverage{
			Name:      controller.Name,
			Frequency: controller.Frequency,
		})
	}

	return GetControllerSectorsWithCoverage(coverage, active)
}

func GetControllerSectorsWithCoverage(controllers []ControllerCoverage, active []string) map[string][]Sector {
	var result = make(map[string][]Sector)
	directLookup := make(map[string]string)
	primaryNameByFrequency := make(map[string]string)
	coverageByFrequency := make(map[string][]string)

	for _, c := range controllers {
		primaryFrequency := vatsim.NormalizeFrequency(c.Frequency)
		if primaryFrequency == "" {
			continue
		}

		result[primaryFrequency] = make([]Sector, 0)
		directLookup[c.Name] = primaryFrequency
		primaryNameByFrequency[primaryFrequency] = c.Name
		for _, coveredFrequency := range c.CoveredFrequencies {
			normalizedCoveredFrequency := vatsim.NormalizeFrequency(coveredFrequency)
			if normalizedCoveredFrequency == "" {
				continue
			}
			coverageByFrequency[normalizedCoveredFrequency] = appendUniqueFrequency(coverageByFrequency[normalizedCoveredFrequency], primaryFrequency)
		}
	}

	for frequency := range coverageByFrequency {
		slices.Sort(coverageByFrequency[frequency])
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
		if frequency, ok := resolveSectorOwnerFrequency(s.Owner, directLookup, primaryNameByFrequency, coverageByFrequency); ok {
			result[frequency] = append(result[frequency], s)
			continue
		}
		if frequency, ok := resolveSectorOwnerFrequency(airborneOwners, directLookup, primaryNameByFrequency, coverageByFrequency); ok {
			result[frequency] = append(result[frequency], s)
		}
	}

	return result
}

func resolveSectorOwnerFrequency(owners []string, directLookup map[string]string, primaryNameByFrequency map[string]string, coverageByFrequency map[string][]string) (string, bool) {
	for _, owner := range owners {
		if frequency, ok := directLookup[owner]; ok {
			return frequency, true
		}
	}

	for _, owner := range owners {
		position, err := GetPositionByName(owner)
		if err != nil {
			continue
		}

		frequencies := coverageByFrequency[vatsim.NormalizeFrequency(position.Frequency)]
		if len(frequencies) > 0 {
			if frequency, ok := pickCoveredPrimaryFrequency(owners, directLookup, primaryNameByFrequency, frequencies); ok {
				return frequency, true
			}
			return frequencies[0], true
		}
	}

	return "", false
}

func pickCoveredPrimaryFrequency(owners []string, directLookup map[string]string, primaryNameByFrequency map[string]string, frequencies []string) (string, bool) {
	for _, owner := range owners {
		frequency, ok := directLookup[owner]
		if !ok || !slices.Contains(frequencies, frequency) {
			continue
		}
		if strings.EqualFold(primaryNameByFrequency[frequency], owner) {
			return frequency, true
		}
	}

	return "", false
}

func appendUniqueFrequency(frequencies []string, frequency string) []string {
	if slices.Contains(frequencies, frequency) {
		return frequencies
	}

	return append(frequencies, frequency)
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

func GetSectorDisplayFrequency(active []string, sectorRef string, isArrival bool) (string, bool) {
	sector, ok := getSectorByIdentifier(active, sectorRef, isArrival)
	if !ok {
		return "", false
	}

	for _, owner := range sector.Owner {
		position, err := GetPositionByName(owner)
		if err != nil || strings.TrimSpace(position.Frequency) == "" {
			continue
		}

		return position.Frequency, true
	}

	return "", false
}

func IsArrivalTowerOwner(owner string, active []string) bool {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return false
	}

	type scored struct {
		sector Sector
		score  int
	}

	bestByKey := make(map[string]scored)
	for _, sector := range sectors {
		if !strings.EqualFold(sector.Name, "TE") && !strings.EqualFold(sector.Name, "TW") {
			continue
		}
		score := matchScore(sector, active)
		if score < 0 {
			continue
		}
		key := sector.KeyOrName()
		if prev, ok := bestByKey[key]; !ok || score > prev.score {
			bestByKey[key] = scored{sector: sector, score: score}
		}
	}

	for _, entry := range bestByKey {
		for _, candidate := range entry.sector.Owner {
			if strings.EqualFold(candidate, owner) {
				return true
			}
		}
	}

	return false
}

func sectorMatchesIdentifier(sector Sector, sectorRef string) bool {
	return strings.EqualFold(sector.KeyOrName(), sectorRef) || strings.EqualFold(sector.Name, sectorRef)
}

func getSectorByIdentifier(active []string, sectorRef string, isArrival bool) (Sector, bool) {
	bestScore := -1
	var best Sector
	found := false

	for _, sector := range sectors {
		if !sectorMatchesIdentifier(sector, sectorRef) {
			continue
		}
		if sector.Constraints != nil && (sector.Constraints.Arrival != isArrival || sector.Constraints.Departure != !isArrival) {
			continue
		}

		score := matchScore(sector, active)
		if score < 0 {
			continue
		}
		if !found || score > bestScore {
			bestScore = score
			best = sector
			found = true
		}
	}

	return best, found
}
