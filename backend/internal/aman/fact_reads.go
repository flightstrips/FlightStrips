package aman

import "context"

// FactFlightReader resolves one active identity without decoding the airport
// aggregate. Missing or ambiguous identities remain explicit domain errors.
type FactFlightReader interface {
	FindActiveFactFlight(context.Context, string, string) (FlightID, error)
}

type HoldingFactSnapshot struct {
	FlightID  FlightID
	VATSIMCID string
	Clearance *HoldingClearance
}

// HoldingFactReader is only a no-change preflight. Mutations still reload and
// commit the complete revision-checked airport aggregate.
type HoldingFactReader interface {
	LoadHoldingFactSnapshots(context.Context, string, []FlightID) ([]HoldingFactSnapshot, error)
}
