// Package scenery lists the airport sceneries a pilot can declare in the EFB and
// the stands that do not exist (or are unusable) in each of them.
package scenery

import "strings"

// Scenery is one add-on (or the simulator default) for an airport.
type Scenery struct {
	ID   string
	Name string
	// ExcludedStands are never assigned to a pilot flying with this scenery.
	ExcludedStands []string
}

var catalog = map[string][]Scenery{
	"EKCH": {
		{ID: "flytampa", Name: "FlyTampa Copenhagen", ExcludedStands: []string{"E20", "E22", "E24", "E25", "E27", "E29", "E31", "E33", "E35", "E36"}},
		{ID: "simnord", Name: "SimNord Copenhagen"},
		{ID: "default", Name: "Default"},
	},
}

// Lookup returns the scenery with the given ID at an airport.
func Lookup(airport, id string) (Scenery, bool) {
	airport, id = normalize(airport), strings.ToLower(strings.TrimSpace(id))
	for _, candidate := range catalog[airport] {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return Scenery{}, false
}

// Supported reports whether the airport offers any scenery choice.
func Supported(airport string) bool {
	return len(catalog[normalize(airport)]) > 0
}

// ExcludedStands returns the stands a pilot with this scenery must not be
// given, as a set of upper-case stand names. It is empty for unknown sceneries.
func ExcludedStands(airport, id string) map[string]struct{} {
	declared, ok := Lookup(airport, id)
	if !ok || len(declared.ExcludedStands) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(declared.ExcludedStands))
	for _, stand := range declared.ExcludedStands {
		result[normalize(stand)] = struct{}{}
	}
	return result
}

func normalize(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }
