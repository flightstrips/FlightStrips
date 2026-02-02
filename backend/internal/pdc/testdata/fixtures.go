package testdata

import (
	"FlightStrips/internal/database"
)

// Message represents a Hoppie ACARS message (avoiding import cycle)
type Message struct {
	From   string
	To     string
	Type   string
	Packet string
	Raw    string
}

// ValidStrip returns a test strip matching a PDC request
func ValidStrip() database.Strip {
	return database.Strip{
		ID:             1,
		Version:        1,
		Callsign:       "SAS123",
		Session:        1,
		Origin:         "EKCH",
		Destination:    "ESSA",
		AircraftType:   ptr("A320"),
		Runway:         ptr("22L"),
		Sid:            ptr("VEMBO2E"),
		Squawk:         ptr("2401"),
		AssignedSquawk: ptr("2401"),
		Bay:            "NOT_CLEARED",
		PdcState:       "",
	}
}

// ValidPDCRequest returns a test PDC request message
func ValidPDCRequest() string {
	return "REQUEST PREDEP CLEARANCE SAS123 A320 TO ESSA AT EKCH STAND A10 ATIS A"
}

// PDCWilcoMessage returns a WILCO response message
func PDCWilcoMessage(callsign string) Message {
	return Message{
		From:   callsign,
		To:     "EKCH",
		Type:   "cpdlc",
		Packet: "/data2/1/2/N/WILCO",
		Raw:    callsign + " EKCH cpdlc {/data2/1/2/N/WILCO}",
	}
}

// PDCUnableMessage returns an UNABLE response message
func PDCUnableMessage(callsign string) Message {
	return Message{
		From:   callsign,
		To:     "EKCH",
		Type:   "cpdlc",
		Packet: "/data2/1/2/N/UNABLE",
		Raw:    callsign + " EKCH cpdlc {/data2/1/2/N/UNABLE}",
	}
}

// PDCRequestMessage returns a PDC request message
func PDCRequestMessage(callsign string) Message {
	return Message{
		From:   callsign,
		To:     "EKCH",
		Type:   "telex",
		Packet: ValidPDCRequest(),
		Raw:    callsign + " EKCH telex {" + ValidPDCRequest() + "}",
	}
}
