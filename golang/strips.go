package main

import (
	"github.com/google/uuid"
)

type WeightCategory string

const (
	WCUnknown    WeightCategory = "Unknown"
	WCLight      WeightCategory = "Light"
	WCMedium     WeightCategory = "Medium"
	WCHeavy      WeightCategory = "Heavy"
	WCSuperHeavy WeightCategory = "SuperHeavy"
)

type StripState string

const (
	SSNone    StripState = "None"
	SSStartup StripState = "Startup"
	SSPush    StripState = "Push"
	SSTaxi    StripState = "Taxi"
	SSDeice   StripState = "Deice"
	SSLineup  StripState = "Lineup"
	SSDepart  StripState = "Depart"
	SSArrival StripState = "Arrival"
)

type Strip struct {
	ID uuid.UUID

	Origin      string
	Destination string
	Alternative string

	Route string

	Remarks string

	//TODO: Should we be strict about our definitions
	AssignedSquawk string

	Squawk string

	SID string

	ClearedAltitude string

	//TODO: Is this the only one that is Nullable?
	Heading *string

	AircraftType string

	Runway string

	FinalAltitude string

	Capabilities string

	//TODO: This is enum in the original code and I believe that this is redundant.
	CommunicationType string

	AircraftCategory WeightCategory

	Stand string

	Sequence   *string
	StripState StripState
	Cleared    bool

	PositionFrequency *strong

	Bay string

	Position Position

	TOBT *string
	TSAT *string
	TTOT *string
	CTOT *string
	AOBT *string
	ASAT *string
}
