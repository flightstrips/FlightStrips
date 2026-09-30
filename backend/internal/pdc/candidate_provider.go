package pdc

import (
	"fmt"
	"strconv"
	"strings"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// ParseProviderMessage is the only raw Hoppie frame boundary in the candidate.
// The hash-derived UUID preserves provider identity; malformed content is
// retained as a reason, never as the original packet.
func ParseProviderMessage(station string, raw Message) (*pb.PdcProviderMessage, error) {
	id, err := HoppieMessageCommandID(raw)
	if err != nil {
		return nil, err
	}
	m := &pb.PdcProviderMessage{MessageId: id, From: strings.ToUpper(strings.TrimSpace(raw.From)), To: strings.ToUpper(strings.TrimSpace(station)), Kind: pb.PdcProviderMessage_KIND_NOT_SUPPORTED, ReasonCode: "NOT_SUPPORTED"}
	if m.From == "" {
		m.Kind, m.ReasonCode = pb.PdcProviderMessage_KIND_MALFORMED, "MALFORMED_HEADER"
		return m, cluster.ValidatePdcProviderMessage(m)
	}
	switch strings.ToLower(raw.Type) {
	case "cpdlc":
		m.Transport = pb.PdcProviderMessage_TRANSPORT_CPDLC
	case "telex":
		m.Transport = pb.PdcProviderMessage_TRANSPORT_TELEX
	}
	packet := strings.TrimSpace(raw.Packet)
	malformed := func() { m.Kind = pb.PdcProviderMessage_KIND_MALFORMED; m.ReasonCode = "MALFORMED" }
	if packet == "" {
		malformed()
		return m, cluster.ValidatePdcProviderMessage(m)
	}
	if len(raw.Raw) > cluster.MaxStateBytes || len(packet) > cluster.MaxStateBytes {
		malformed()
		return m, cluster.ValidatePdcProviderMessage(m)
	}
	if m.Transport == pb.PdcProviderMessage_TRANSPORT_CPDLC {
		fields := strings.SplitN(packet, "/", 6)
		if len(fields) != 6 || fields[0] != "" || fields[1] != "data2" {
			malformed()
			return m, cluster.ValidatePdcProviderMessage(m)
		}
		m.Sequence, err = strconv.ParseUint(fields[2], 10, 64)
		if err != nil || m.Sequence == 0 {
			malformed()
			return m, cluster.ValidatePdcProviderMessage(m)
		}
		if fields[3] != "" {
			response, e := strconv.ParseUint(fields[3], 10, 64)
			if e != nil || response == 0 {
				malformed()
				return m, cluster.ValidatePdcProviderMessage(m)
			}
			m.ResponseTo = &response
		}
		packet = fields[5]
	}
	if m.Transport != pb.PdcProviderMessage_TRANSPORT_UNSPECIFIED {
		switch classify(packet) {
		case MsgPDCRequest:
			req, e := parsePDCRequest(packet)
			if e != nil {
				malformed()
				break
			}
			m.Kind = pb.PdcProviderMessage_KIND_REQUEST
			m.ReasonCode = ""
			m.Request = &pb.HoppiePdcRequest{Callsign: req.Callsign, AircraftType: req.Aircraft, Departure: req.Departure, Destination: req.Destination, Stand: req.Stand, Atis: req.Atis, Remarks: req.Remarks}
		case MsgWilco, MsgUnable:
			if m.ResponseTo == nil || m.Transport != pb.PdcProviderMessage_TRANSPORT_CPDLC {
				malformed()
				break
			}
			m.Kind = pb.PdcProviderMessage_KIND_WILCO
			if classify(packet) == MsgUnable {
				m.Kind = pb.PdcProviderMessage_KIND_UNABLE
			}
			m.ReasonCode = ""
		}
	}
	return m, cluster.ValidatePdcProviderMessage(m)
}

func renderProviderMessage(m *pb.PdcProviderMessage) (string, error) {
	if err := cluster.ValidatePdcProviderMessage(m); err != nil {
		return "", err
	}
	var payload, flag string
	flag = "NE"
	switch m.Kind {
	case pb.PdcProviderMessage_KIND_CLEARANCE:
		payload = *m.ClearanceText
		flag = "WU"
	case pb.PdcProviderMessage_KIND_STATUS:
		payload = "RCD RECEIVED @REQUEST BEING PROCESSED @STANDBY"
	case pb.PdcProviderMessage_KIND_CONFIRMED:
		payload = "CLEARANCE CONFIRMED"
	case pb.PdcProviderMessage_KIND_NO_RESPONSE:
		payload = "ACK NOT RECEIVED @CLEARANCE CANCELLED @REVERT TO VOICE PROCEDURES"
	case pb.PdcProviderMessage_KIND_REVERT_TO_VOICE, pb.PdcProviderMessage_KIND_UNAVAILABLE:
		payload = "ERROR @REVERT TO VOICE PROCEDURES"
	case pb.PdcProviderMessage_KIND_FLIGHT_PLAN_NOT_HELD:
		payload = "RCD REJECTED @FLIGHT PLAN NOT HELD @REVERT TO VOICE PROCEDURES"
		if m.ReasonCode == "ALREADY_CLEARED" {
			payload = "RCD REJECTED @CLEARANCE ALREADY ISSUED @REVERT TO VOICE PROCEDURES"
		}
	case pb.PdcProviderMessage_KIND_INVALID_AIRCRAFT_TYPE:
		payload = "RCD REJECTED @TYPE MISMATCH @UPDATE RCD AND RESEND"
	case pb.PdcProviderMessage_KIND_NOT_SUPPORTED, pb.PdcProviderMessage_KIND_MALFORMED:
		payload = "MESSAGE NOT SUPPORTED @REVERT TO VOICE PROCEDURES"
	default:
		return "", fmt.Errorf("unsupported outbound PDC kind")
	}
	response := ""
	if m.ResponseTo != nil {
		response = strconv.FormatUint(*m.ResponseTo, 10)
	}
	return fmt.Sprintf("/data2/%d/%s/%s/%s", m.Sequence, response, flag, payload), nil
}

func copyProvider(m *pb.PdcProviderMessage) *pb.PdcProviderMessage {
	return proto.Clone(m).(*pb.PdcProviderMessage)
}
