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
		variants, ok := layouts[controller.Name]
		if !ok {
			continue
		}

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

	return result
}

func matchesControllers(controllers []*Position, required []string, offline bool) bool {
	if len(required) == 0 {
		return true
	}

	for _, c := range controllers {
		if strings.Contains(c.Name, "_GND") {
			if index := slices.Index(required, "_GND"); index != -1 {
				if offline {
					return false
				}
				required = slices.Delete(required, index, index+1)
			}
		}
		if index := slices.Index(required, c.Name); index != -1 {
			if offline {
				return false
			}
			required = slices.Delete(required, index, index+1)
		}

		if len(required) == 0 {
			return true
		}
	}

	return offline
}
