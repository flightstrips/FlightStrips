package config

import (
	"slices"
	"strings"
	"time"

	"FlightStrips/internal/vatsim"
)

// CoverageParticipant is operational evidence, not strip/sector ownership or
// plugin command authority. Key identifies a controller within one environment.
type CoverageParticipant struct {
	Key, Callsign, PrimaryFrequency string
	TunedFrequencies                []string
	Observer                        bool
}

// CoverageAirports intersects a deployment declaration with enabled definitions
// and trusted server participation authority. A declaration never grants access.
func (r *AirportRegistry) CoverageAirports(declared []string, authorized func(string) bool) []*AirportDefinition {
	result := []*AirportDefinition{}
	if r == nil || authorized == nil {
		return result
	}
	for _, airport := range r.Airports() {
		definition, _ := r.Lookup(airport)
		if definition.MembershipPolicy() == "remaining-responsibilities/v1" && slices.Contains(declared, airport) && authorized(airport) {
			result = append(result, definition)
		}
	}
	return result
}

func (d *AirportDefinition) RelinquishGrace() time.Duration {
	value := d.document.Coverage.Membership.RelinquishGraceSeconds
	if value == nil {
		return 0
	}
	return time.Duration(*value * float64(time.Second))
}

// CoverageResponsibilities uses the existing coverage/v1 ordering: carriage of
// the primary duty frequency, configured primary providers, carriage of fallback
// frequencies, then configured callsign fallback. Ties use primary frequency
// followed by controller key, preserving the existing frequency ordering.
// Airport participation deliberately has no runway, strip or sector prerequisite:
// all configured duties can require participation before runway setup arrives.
func (d *AirportDefinition) CoverageResponsibilities(participants []CoverageParticipant) map[string][]string {
	participants = slices.Clone(participants)
	slices.SortFunc(participants, func(a, b CoverageParticipant) int {
		if order := strings.Compare(vatsim.NormalizeFrequency(a.PrimaryFrequency), vatsim.NormalizeFrequency(b.PrimaryFrequency)); order != 0 {
			return order
		}
		return strings.Compare(a.Key, b.Key)
	})
	positions := map[string]Position{}
	for _, position := range d.document.Positions {
		positions[position.Name] = position
	}
	matching := d.document.Coverage.Matching
	identity := func(c CoverageParticipant) string {
		for _, position := range d.document.Positions {
			if strings.EqualFold(c.Callsign, position.Name) {
				return position.Name
			}
		}
		for _, position := range d.document.Positions {
			if vatsim.NormalizeFrequency(c.PrimaryFrequency) != "" && vatsim.NormalizeFrequency(c.PrimaryFrequency) == vatsim.NormalizeFrequency(position.Frequency) {
				return position.Name
			}
		}
		return ""
	}
	primary := func(c CoverageParticipant, name string) bool {
		p, ok := positions[name]
		return ok && !c.Observer && matching.PrimaryFrequency && vatsim.NormalizeFrequency(c.PrimaryFrequency) != "" && vatsim.NormalizeFrequency(c.PrimaryFrequency) == vatsim.NormalizeFrequency(p.Frequency)
	}
	carried := func(c CoverageParticipant, name string) bool {
		p, ok := positions[name]
		if !ok || c.Observer {
			return false
		}
		if primary(c, name) {
			return true
		}
		if matching.TunedFrequencies {
			for _, frequency := range c.TunedFrequencies {
				if vatsim.NormalizeFrequency(frequency) != "" && vatsim.NormalizeFrequency(frequency) == vatsim.NormalizeFrequency(p.Frequency) {
					return true
				}
			}
		}
		return false
	}
	selectProvider := func(providers []string) string {
		if len(providers) == 0 {
			return ""
		}
		pickCarried := func(names []string) string {
			for _, name := range names {
				var candidates []CoverageParticipant
				for _, c := range participants {
					if carried(c, name) {
						candidates = append(candidates, c)
					}
				}
				// Prefer the configured provider among controllers carrying the same
				// frequency, matching existing primary identity tie resolution.
				for _, provider := range providers {
					for _, c := range candidates {
						if identity(c) == provider {
							return c.Key
						}
					}
				}
				if len(candidates) > 0 {
					return candidates[0].Key
				}
			}
			return ""
		}
		if key := pickCarried(providers[:1]); key != "" {
			return key
		}
		for _, provider := range providers {
			for _, c := range participants {
				if primary(c, provider) && identity(c) == provider {
					return c.Key
				}
			}
		}
		if key := pickCarried(providers); key != "" {
			return key
		}
		if matching.CallsignFallback {
			for _, provider := range providers {
				for _, c := range participants {
					if !c.Observer && strings.EqualFold(c.Callsign, provider) {
						return c.Key
					}
				}
			}
		}
		return ""
	}
	result := map[string][]string{}
	for _, duty := range d.document.Coverage.Responsibilities {
		key := selectProvider(duty.Providers)
		if key == "" {
			key = selectProvider(d.document.Coverage.AirborneFallback)
		}
		if key != "" {
			result[key] = append(result[key], duty.ID)
		}
	}
	return result
}
