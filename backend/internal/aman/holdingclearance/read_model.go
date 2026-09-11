package holdingclearance

import (
	"slices"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

// ReadModel is the presentation-neutral list of authoritative en-route holds
// for one AMAN airport. Missing display facts remain explicit nil values.
type ReadModel struct {
	Entries []Entry
}

type Entry struct {
	FlightID        aman.FlightID
	Callsign        string
	Holding         string
	EAT             *time.Time
	ClearedAltitude *int32
	SourceStatus    aman.DataStatus
	ObservedAt      time.Time
}

func BuildReadModel(state aman.AirportState) ReadModel {
	airport := strings.ToUpper(strings.TrimSpace(state.Airport))
	result := ReadModel{Entries: make([]Entry, 0)}
	for _, flight := range state.Flights {
		clearance := flight.HoldingClearance
		if clearance == nil || clearance.Hold == "" || clearance.HoldType != aman.HoldingClearanceEnroute ||
			flight.LatestObservation == nil || strings.ToUpper(strings.TrimSpace(flight.LatestObservation.Destination)) != airport {
			continue
		}

		entry := Entry{
			FlightID: flight.ID, Callsign: flight.CurrentCallsign, Holding: clearance.Hold,
			ClearedAltitude: cloneAltitude(clearance.ClearedAltitude), SourceStatus: flight.DataStatus,
			ObservedAt: clearance.ObservedAt,
		}
		if clearance.HoldEAT != "" {
			if resolved, err := ResolveEATUTC(clearance.HoldEAT, clearance.ObservedAt); err == nil {
				entry.EAT = &resolved
			}
		}
		result.Entries = append(result.Entries, entry)
	}

	slices.SortFunc(result.Entries, func(left, right Entry) int {
		if comparison := strings.Compare(strings.ToUpper(strings.TrimSpace(left.Callsign)), strings.ToUpper(strings.TrimSpace(right.Callsign))); comparison != 0 {
			return comparison
		}
		return strings.Compare(string(left.FlightID), string(right.FlightID))
	})
	return result
}
