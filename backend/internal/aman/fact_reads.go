package aman

import "context"

// FactFlightReader resolves one active identity without decoding the airport
// aggregate. Missing or ambiguous identities remain explicit domain errors.
type FactFlightReader interface {
	FindActiveFactFlight(context.Context, string, string) (Callsign, error)
}

type HoldingFactSnapshot struct {
	Callsign  Callsign
	Clearance *HoldingClearance
}

// HoldingFactReader is only a no-change preflight. Mutations still reload and
// commit the complete revision-checked airport aggregate.
type HoldingFactReader interface {
	LoadHoldingFactSnapshots(context.Context, string, []Callsign) ([]HoldingFactSnapshot, error)
}
