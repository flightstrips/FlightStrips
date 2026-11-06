package sat

import (
	"slices"
	"strings"
)

// AirportInputs contains immutable domain inputs, never reservations or workers.
// Aircraft registries may be shared by definitions referencing the same files.
type AirportInputs struct {
	Aircraft *AircraftRegistry
	Engines  *AircraftEngineRegistry
	Borders  *AirportCountryRegistry
	Stands   *StandCapabilityRegistry
	Policy   *AirlineAssignmentConfig
}

type AirportInputRegistry struct {
	airports map[string]AirportInputs
	stands   *StandCapabilityRegistry
}

func NewAirportInputRegistry(inputs map[string]AirportInputs) *AirportInputRegistry {
	r := &AirportInputRegistry{airports: make(map[string]AirportInputs), stands: &StandCapabilityRegistry{byAirport: make(map[string]map[string]Stand)}}
	airports := make([]string, 0, len(inputs))
	for airport := range inputs {
		airports = append(airports, airport)
	}
	slices.Sort(airports)
	for _, airport := range airports {
		input := inputs[airport]
		airport = strings.ToUpper(strings.TrimSpace(airport))
		r.airports[airport] = input
		if input.Stands == nil {
			continue
		}
		// Include exactly the selected airport even if an asset lists others.
		r.stands.byAirport[airport] = input.Stands.byAirport[airport]
		for _, stand := range input.Stands.ordered {
			if stand.Airport == airport {
				r.stands.ordered = append(r.stands.ordered, stand)
			}
		}
	}
	return r
}

func (r *AirportInputRegistry) Lookup(airport string) (AirportInputs, bool) {
	if r == nil {
		return AirportInputs{}, false
	}
	input, ok := r.airports[strings.ToUpper(strings.TrimSpace(airport))]
	return input, ok
}

func (r *AirportInputRegistry) Stands() *StandCapabilityRegistry {
	if r == nil {
		return nil
	}
	return r.stands
}

// ForAirport binds stand-name-only airline policies to exactly one airport.
func (r *StandCapabilityRegistry) ForAirport(airport string) *StandCapabilityRegistry {
	airport = strings.ToUpper(strings.TrimSpace(airport))
	selected := &StandCapabilityRegistry{byAirport: map[string]map[string]Stand{airport: r.byAirport[airport]}}
	for _, stand := range r.ordered {
		if stand.Airport == airport {
			selected.ordered = append(selected.ordered, stand)
		}
	}
	return selected
}

func (r *StandCapabilityRegistry) policyStandExists(name string) bool {
	if r == nil || len(r.byAirport) != 1 {
		return false
	}
	for airport := range r.byAirport {
		_, ok := r.Lookup(airport, name)
		return ok
	}
	return false
}
