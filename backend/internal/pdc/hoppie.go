package pdc

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func classify(raw string) MessageType {
	switch {
	case strings.Contains(raw, "REQUEST PREDEP CLEARANCE"):
		return MsgPDCRequest
	case strings.Contains(raw, "WILCO") || strings.Contains(raw, "ROGER"):
		return MsgWilco
	case strings.Contains(raw, "UNABLE"):
		return MsgUnable
	default:
		return MsgUnknown
	}
}

func parsePDCRequest(raw string) (*PDCRequest, error) {
	req := &PDCRequest{}

	reCallsign := regexp.MustCompile(`REQUEST PREDEP CLEARANCE\s+(\w+)`)
	reAircraft := regexp.MustCompile(`\b([A-Z0-9]{3,4})\b\s+TO`)
	reDest := regexp.MustCompile(`TO\s+([A-Z]{4})`)
	reDep := regexp.MustCompile(`AT\s+([A-Z]{4})`)
	reStand := regexp.MustCompile(`STAND\s+([A-Z0-9]+)`)
	reAtis := regexp.MustCompile(`ATIS\s+([A-Z])`)

	if m := reCallsign.FindStringSubmatch(raw); m != nil {
		req.Callsign = m[1]
	}
	if m := reAircraft.FindStringSubmatch(raw); m != nil {
		req.Aircraft = m[1]
	}
	if m := reDest.FindStringSubmatch(raw); m != nil {
		req.Destination = m[1]
	}
	if m := reDep.FindStringSubmatch(raw); m != nil {
		req.Departure = m[1]
	}
	if m := reStand.FindStringSubmatch(raw); m != nil {
		req.Stand = m[1]
	}
	if m := reAtis.FindStringSubmatch(raw); m != nil {
		req.Atis = m[1]
	}

	if req.Callsign == "" || req.Destination == "" || req.Departure == "" {
		return nil, errors.New("invalid PDC request")
	}

	return req, nil
}

func ParseWilcoMessage(payload string) (*Wilco, error) {
	reResponseTo := regexp.MustCompile(`/data2/(\d+)/(\d+)/(\w+)/(\w+)`)
	m := reResponseTo.FindStringSubmatch(payload)
	if len(m) != 5 {
		return nil, errors.New("invalid Wilco message")
	}

	responseTo, _ := strconv.Atoi(m[2])
	wilco := &Wilco{}
	wilco.ResponseTo = int32(responseTo)

	return wilco, nil
}

// ParseIncomingMessage parses a raw message and returns an IncomingMessage
func ParseIncomingMessage(from, to, raw string) (*IncomingMessage, error) {
	msgType := classify(raw)
	return &IncomingMessage{
		Type:       msgType,
		From:       from,
		To:         to,
		Payload:    raw,
		RawMessage: raw,
	}, nil
}

func buildClearanceConfirm(sequence int32, origin, callsign string) string {
	payload := fmt.Sprintf(
		"ATC REQUEST STATUS . . FSM %s %s %s @%s@ CDA RECEIVED @CLEARANCE CONFIRMED",
		time.Now().Format("1504"),
		time.Now().Format("020106"),
		origin,
		callsign,
	)
	return buildHoppieMessage(sequence, payload, MsgClearanceConfirm)
}

func buildRequestAck(sequence int32, origin, callsign string) string {
	payload := fmt.Sprintf(
		"DEPART REQUEST STATUS . FSM %s %s %s @%s@ RCD RECEIVED @REQUEST BEING PROCESSED @STANDBY",
		time.Now().Format("1504"),
		time.Now().Format("020106"),
		origin,
		callsign,
	)
	return buildHoppieMessage(sequence, payload, MsgPDCStatus)
}

type ClearanceOptions struct {
	Callsign    string
	Origin      string
	Destination string
	Atis        string

	Runway        string
	Squawk        string
	NextFrequency string

	Sequence    int32
	PdcSequence int32

	SID string

	Vectors string
	Heading string
	ClimbTo string

	Remarks string
}

func buildPDCClearance(options ClearanceOptions) string {
	now := time.Now()
	timeStr := now.Format("1504")   // HHMM
	dateStr := now.Format("020106") // DDMMYY

	sb := strings.Builder{}
	sb.Grow(256)

	sb.WriteString(options.Origin)
	sb.WriteString(" PDC ")
	sb.WriteString(strconv.Itoa(int(options.PdcSequence)))
	sb.WriteString(" . . . . . CLD ")
	sb.WriteString(timeStr)
	sb.WriteString(" ")
	sb.WriteString(dateStr)
	sb.WriteString(" ")
	sb.WriteString(options.Origin)
	sb.WriteString(" ")
	sb.WriteString(" PDC ")
	sb.WriteString(strconv.Itoa(int(options.PdcSequence)))
	sb.WriteString(" @")
	sb.WriteString(options.Callsign)
	sb.WriteString("@ CLRD TO: @")
	sb.WriteString(options.Destination)
	sb.WriteString("@ RWY: @")
	sb.WriteString(options.Runway)
	sb.WriteString("@ ")

	if options.Heading != "" {
		sb.WriteString("HDG: @")
		sb.WriteString(options.Heading)
		sb.WriteString("@ ")
	}

	if options.ClimbTo != "" {
		sb.WriteString("CLIMB TO: @")
		sb.WriteString(options.ClimbTo)
		sb.WriteString("@ ")
	}

	if options.Vectors != "" {
		sb.WriteString("VECTORS: @")
		sb.WriteString(options.Vectors)
		sb.WriteString("@ ")
	}

	if options.SID != "" {
		sb.WriteString("SID: @")
		sb.WriteString(options.SID)
		sb.WriteString("@ ")
	}

	sb.WriteString("SQK: @")
	sb.WriteString(options.Squawk)
	sb.WriteString("@ ATIS @")
	sb.WriteString(options.Atis)
	sb.WriteString("@ NEXT FRQ: @")
	sb.WriteString(options.NextFrequency)
	sb.WriteString("@")

	if options.Remarks != "" {
		sb.WriteString(" @")
		sb.WriteString(options.Remarks)
		sb.WriteString("@")
	}

	return buildHoppieMessage(options.Sequence, sb.String(), MsgPDCClearance)
}

func getFlagsForMessage(msgType MessageType) string {
	if flag, ok := HoppieFlags[msgType]; ok {
		return flag
	}
	return "N" // default fallback
}

func buildHoppieMessage(sequence int32, payload string, msgType MessageType) string {
	flags := getFlagsForMessage(msgType)
	return fmt.Sprintf("/data2/%d//%s/%s", sequence, flags, payload)
}

func buildHoppieResponseMessage(sequence, responseTo int32, payload string, msgType MessageType) string {
	flags := getFlagsForMessage(msgType)
	return fmt.Sprintf("/data2/%d/%d/%s/%s", sequence, responseTo, flags, payload)
}

func buildNoResponseMessage(sequence, responseTo int32, airport, callsign string) string {
	now := time.Now()
	timeStr := now.Format("1504")   // HHMM
	dateStr := now.Format("020106") // DDMMYY
	payload := fmt.Sprintf("ATC REQUEST STATUS . . FSM %s %s %s @%s@ ACK NOT RECEIVED @CLEARANCE CANCELLED @REVERT TO VOICE PROCEDURES", timeStr, dateStr, airport, callsign)
	return buildHoppieResponseMessage(sequence, responseTo, payload, MsgNoResponse)
}

func buildRevertToVoice(sequence int32) string {
	payload := "ERROR @REVERT TO VOICE PROCEDURES"
	return buildHoppieMessage(sequence, payload, MsgRevertToVoice)
}

func buildFlightPlanNotHeld(sequence int32, airport, callsign string) string {
	now := time.Now()
	timeStr := now.Format("1504")
	dateStr := now.Format("020106")
	payload := fmt.Sprintf("DEPART REQUEST STATUS . FSM %s %s %s @%s@ RCD REJECTED @FLIGHT PLAN NOT HELD @REVERT TO VOICE PROCEDURES", timeStr, dateStr, airport, callsign)
	return buildHoppieMessage(sequence, payload, MsgFlightPlanNotHeld)
}

func buildPDCUnavailable(sequence int32) string {
	payload := "ERROR @REVERT TO VOICE PROCEDURES"
	return buildHoppieMessage(sequence, payload, MsgPDCUnavailable)
}

func buildInvalidAircraftType(sequence int32, airport, callsign string) string {
	now := time.Now()
	timeStr := now.Format("1504")
	dateStr := now.Format("020106")
	payload := fmt.Sprintf("DEPART REQUEST STATUS . FSM %s %s %s @%s@ RCD REJECTED @TYPE MISMATCH @UPDATE RCD AND RESEND", timeStr, dateStr, airport, callsign)
	return buildHoppieMessage(sequence, payload, MsgInvalidAircraftType)
}

func buildRefuseNotSupported(sequence int32) string {
	payload := "CONTACT ATC BY VOICE @REFUSE NOT SUPPORTED BY DATALINK"
	return buildHoppieMessage(sequence, payload, MsgRefuseNotSupported)
}
