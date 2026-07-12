package sat

import (
	"slices"
	"strings"
)

// FlightFact identifies a fact that could not be resolved.
type FlightFact string

const (
	FlightFactAircraftType FlightFact = "AIRCRAFT_TYPE"
	FlightFactEngineType   FlightFact = "ENGINE_TYPE"
	FlightFactWTC          FlightFact = "WTC"
	FlightFactBorder       FlightFact = "BORDER"
)

// FlightCompatibilityInput contains only flight facts. It deliberately does
// not include stand or airline preference data.
type FlightCompatibilityInput struct {
	Direction      FlightDirection
	Origin         string
	Destination    string
	AircraftType   string
	LiveEngineType string
	WTC            string
}

// FlightCompatibilityFacts is the immutable, normalized value consumed by
// later compatibility and airline eligibility code. Unknown values are
// represented explicitly by the UNKNOWN constants and listed in UnknownFacts.
type FlightCompatibilityFacts struct {
	Direction      FlightDirection
	Origin         string
	Destination    string
	BorderEndpoint string
	Aircraft       Aircraft
	AircraftKnown  bool
	EngineType     EngineType
	WTC            string
	BorderStatus   BorderStatus
	unknownFacts   []FlightFact
}

// ResolveFlightCompatibilityFacts normalizes all input facts without
// mutating the input or the registries. A valid live EuroScope engine value
// takes precedence over the repo mapping; an empty live value uses the map.
func ResolveFlightCompatibilityFacts(input FlightCompatibilityInput, aircraft *AircraftRegistry, engines *AircraftEngineRegistry, borders *AirportCountryRegistry) FlightCompatibilityFacts {
	facts := FlightCompatibilityFacts{
		Direction:    input.Direction,
		Origin:       strings.ToUpper(strings.TrimSpace(input.Origin)),
		Destination:  strings.ToUpper(strings.TrimSpace(input.Destination)),
		EngineType:   EngineUnknown,
		WTC:          "UNKNOWN",
		BorderStatus: BorderStatusUnknown,
	}
	unknown := make([]FlightFact, 0, 4)

	if aircraft != nil {
		if resolved, ok := aircraft.Lookup(input.AircraftType); ok {
			facts.Aircraft = resolved
			facts.AircraftKnown = true
		} else {
			unknown = append(unknown, FlightFactAircraftType)
		}
	} else {
		unknown = append(unknown, FlightFactAircraftType)
	}

	if live := strings.TrimSpace(input.LiveEngineType); live != "" {
		if engine, ok := ParseEngineType(live); ok {
			facts.EngineType = engine
		} else {
			unknown = append(unknown, FlightFactEngineType)
		}
	} else if engines != nil {
		if engine, ok := engines.Lookup(input.AircraftType); ok {
			facts.EngineType = engine
		} else {
			unknown = append(unknown, FlightFactEngineType)
		}
	} else {
		unknown = append(unknown, FlightFactEngineType)
	}

	if wtc := strings.ToUpper(strings.TrimSpace(input.WTC)); wtc != "" {
		if validWTC(wtc) {
			facts.WTC = wtc
		} else {
			unknown = append(unknown, FlightFactWTC)
		}
	} else if engines != nil {
		if wtc, ok := engines.LookupWTC(input.AircraftType); ok {
			facts.WTC = wtc
		} else {
			unknown = append(unknown, FlightFactWTC)
		}
	} else {
		unknown = append(unknown, FlightFactWTC)
	}

	borderEndpoint := ""
	switch input.Direction {
	case Arrival:
		borderEndpoint = facts.Origin
	case Departure:
		borderEndpoint = facts.Destination
	}
	facts.BorderEndpoint = borderEndpoint
	if borderEndpoint != "" && borders != nil {
		facts.BorderStatus = borders.BorderStatus(borderEndpoint)
	}
	if facts.BorderStatus == BorderStatusUnknown {
		unknown = append(unknown, FlightFactBorder)
	}

	facts.unknownFacts = unknown
	return facts
}

// UnknownFactKinds returns a copy of the unresolved fact kinds.
func (f FlightCompatibilityFacts) UnknownFactKinds() []FlightFact {
	return slices.Clone(f.unknownFacts)
}

// Complete reports whether all facts required for automatic compatibility are
// known. Unknown facts never pass an explicit stand restriction.
func (f FlightCompatibilityFacts) Complete() bool {
	return len(f.unknownFacts) == 0
}
