package pdc

import "time"

type MessageType string

const (
	MsgPDCRequest         MessageType = "PDC_REQUEST"
	MsgPDCStatus          MessageType = "PDC_STATUS"
	MsgPDCClearance       MessageType = "PDC_CLEARANCE"
	MsgWilco              MessageType = "WILCO"
	MsgUnable             MessageType = "UNABLE"
	MsgClearanceConfirm   MessageType = "CLEARANCE_CONFIRMED"
	MsgNoResponse         MessageType = "NO_RESPONSE"
	MsgRevertToVoice      MessageType = "REVERT_TO_VOICE"
	MsgFlightPlanNotHeld  MessageType = "FLIGHT_PLAN_NOT_HELD"
	MsgPDCUnavailable     MessageType = "PDC_UNAVAILABLE"
	MsgInvalidAircraftType MessageType = "INVALID_AIRCRAFT_TYPE"
	MsgRefuseNotSupported MessageType = "REFUSE_NOT_SUPPORTED"
	MsgUnknown            MessageType = "UNKNOWN"
)

type PDCRequest struct {
	Callsign    string
	Aircraft    string
	Departure   string
	Destination string
	Stand       string
	Atis        string
}

type Wilco struct {
	ResponseTo int32
}

type ClearanceState string

const (
	StateNone          ClearanceState = "NONE"
	StateRequested     ClearanceState = "REQUESTED"
	StateCleared       ClearanceState = "CLEARED"
	StateConfirmed     ClearanceState = "CONFIRMED"
	StateNoResponse    ClearanceState = "NO_RESPONSE"
	StateRevertToVoice ClearanceState = "REVERT_TO_VOICE"
	StateFailed        ClearanceState = "FAILED"
)

func (s ClearanceState) IsActive() bool {
	return s == StateRequested
}

type IncomingMessage struct {
	Type       MessageType
	From       string
	To         string
	Payload    string
	RawMessage string
}

type FlightSession struct {
	Callsign    string
	Aircraft    string
	Origin      string
	Destination string
	Stand       string
	Atis        string

	Runway        string
	SID           string
	Squawk        string
	NextFrequency string

	State      ClearanceState
	LastUpdate time.Time
}

var HoppieFlags = map[MessageType]string{
	MsgPDCRequest:          "N",  // pilot requests — telex, usually "N"
	MsgPDCStatus:           "NE", // status / standby ACK
	MsgPDCClearance:        "WU", // clearance sent
	MsgWilco:               "N",  // pilot response
	MsgClearanceConfirm:    "NE", // confirmation back to ATC
	MsgNoResponse:          "NE",
	MsgRevertToVoice:       "NE",
	MsgFlightPlanNotHeld:   "NE",
	MsgPDCUnavailable:      "NE",
	MsgInvalidAircraftType: "NE",
	MsgRefuseNotSupported:  "NE",
}
