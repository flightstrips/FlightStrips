package sat

import "strings"

// FlightDirection determines which route endpoint supplies border status.
type FlightDirection string

const (
	Arrival   FlightDirection = "ARRIVAL"
	Departure FlightDirection = "DEPARTURE"
)

// AirportCountryRegistry is a read-only airport/prefix-to-border-status map.
// Prefixes cover the global feed, while exact airport codes can override a
// prefix when an airport needs special handling.
type AirportCountryRegistry struct {
	byCode   map[string]BorderStatus
	prefixes map[string]BorderStatus
}

var defaultAirportBorderStatuses = map[string]BorderStatus{
	"BI": BorderStatusSchengen, "EB": BorderStatusSchengen, "ED": BorderStatusSchengen,
	"EE": BorderStatusSchengen, "EF": BorderStatusSchengen, "EH": BorderStatusSchengen,
	"EK": BorderStatusSchengen, "EL": BorderStatusSchengen, "EN": BorderStatusSchengen, "EP": BorderStatusSchengen,
	"ES": BorderStatusSchengen, "ET": BorderStatusSchengen, "EV": BorderStatusSchengen,
	"EY": BorderStatusSchengen, "GC": BorderStatusSchengen, "LB": BorderStatusSchengen,
	"LD": BorderStatusSchengen, "LE": BorderStatusSchengen, "LF": BorderStatusSchengen,
	"LG": BorderStatusSchengen, "LH": BorderStatusSchengen, "LI": BorderStatusSchengen,
	"LJ": BorderStatusSchengen, "LK": BorderStatusSchengen, "LM": BorderStatusSchengen,
	"LO": BorderStatusSchengen, "LP": BorderStatusSchengen, "LR": BorderStatusSchengen,
	"LS": BorderStatusSchengen, "LZ": BorderStatusSchengen,

	"C": BorderStatusNonSchengen, "EG": BorderStatusNonSchengen, "EI": BorderStatusNonSchengen,
	"K": BorderStatusNonSchengen, "LT": BorderStatusNonSchengen, "LU": BorderStatusNonSchengen,
	"LW": BorderStatusNonSchengen, "RJ": BorderStatusNonSchengen, "RK": BorderStatusNonSchengen,
	"U": BorderStatusNonSchengen, "V": BorderStatusNonSchengen, "Y": BorderStatusNonSchengen,
	"ZB": BorderStatusNonSchengen, "ZG": BorderStatusNonSchengen, "ZK": BorderStatusNonSchengen,
	"ZL": BorderStatusNonSchengen, "ZM": BorderStatusNonSchengen, "ZP": BorderStatusNonSchengen,
	"ZS": BorderStatusNonSchengen, "ZU": BorderStatusNonSchengen, "ZW": BorderStatusNonSchengen,
	"ZY": BorderStatusNonSchengen,
}

// NewAirportCountryRegistry returns the repository-owned border mapping.
func NewAirportCountryRegistry() *AirportCountryRegistry {
	prefixes := make(map[string]BorderStatus, len(defaultAirportBorderStatuses))
	for prefix, status := range defaultAirportBorderStatuses {
		prefixes[prefix] = status
	}
	return &AirportCountryRegistry{
		byCode:   make(map[string]BorderStatus),
		prefixes: prefixes,
	}
}

func (r *AirportCountryRegistry) statusForAirport(airport string) BorderStatus {
	if r == nil {
		return BorderStatusUnknown
	}
	code := strings.ToUpper(strings.TrimSpace(airport))
	if status, ok := r.byCode[code]; ok {
		return status
	}
	for length := len(code) - 1; length >= 1; length-- {
		if status, ok := r.prefixes[code[:length]]; ok {
			return status
		}
	}
	return BorderStatusUnknown
}

// BorderStatus returns the status of an airport, or BorderStatusUnknown when
// its airport code has no repository-owned mapping.
func (r *AirportCountryRegistry) BorderStatus(airport string) BorderStatus {
	return r.statusForAirport(airport)
}
