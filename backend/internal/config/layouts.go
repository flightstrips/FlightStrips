package config

import (
	"slices"
	"strings"
)

type LayoutVariant struct {
	Online  []string `yaml:"online"`
	Offline []string `yaml:"offline"`
	Active  []string `yaml:"active,omitempty"`
	Layout  string   `yaml:"layout"`
}

func GetLayouts(controllers []*Position, active []string) map[string]*string {
	var result = make(map[string]*string)
	for _, c := range controllers {
		result[c.Frequency] = nil
	}

	for _, controller := range controllers {
		if variants, ok := layouts[controller.Name]; ok {
			for _, v := range variants {
				if !hasAnyActive(active, v.Active) {
					continue
				}

				if !matchesControllers(controllers, v.Online, false) || !matchesControllers(controllers, v.Offline, true) {
					continue
				}

				result[controller.Frequency] = &v.Layout
			}
		}

		if result[controller.Frequency] == nil && airborneFallbackLayout != "" && isAirborneOwner(controller.Name) {
			layout := airborneFallbackLayout
			result[controller.Frequency] = &layout
		}
	}

	return result
}

// GetLayoutsByFrequency resolves each controller's configured role from its
// primary frequency before applying the normal layout rules.
func GetLayoutsByFrequency(controllers []*Position, active []string) map[string]*string {
	positions := make([]*Position, 0, len(controllers))
	for _, controller := range controllers {
		if controller == nil {
			continue
		}
		position := *controller
		if configuredPosition, err := GetPositionBasedOnFrequency(controller.Frequency); err == nil {
			position.Name = configuredPosition.Name
			position.Section = configuredPosition.Section
		}
		positions = append(positions, &position)
	}
	return GetLayouts(positions, active)
}

func matchesControllers(controllers []*Position, required []string, offline bool) bool {
	if len(required) == 0 {
		return true
	}

	// Work on a copy to avoid mutating the original slice stored in global config state.
	remaining := slices.Clone(required)

	for _, c := range controllers {
		if strings.Contains(c.Name, "_GND") {
			if index := slices.Index(remaining, "_GND"); index != -1 {
				if offline {
					return false
				}
				remaining = slices.Delete(remaining, index, index+1)
			}
		}
		if index := slices.Index(remaining, c.Name); index != -1 {
			if offline {
				return false
			}
			remaining = slices.Delete(remaining, index, index+1)
		}

		if len(remaining) == 0 {
			return true
		}
	}

	return offline
}

func isAirborneOwner(positionName string) bool {
	for _, owner := range airborneOwners {
		if owner == positionName {
			return true
		}
	}

	return false
}
