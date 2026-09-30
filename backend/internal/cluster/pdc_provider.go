package cluster

import (
	"fmt"
	"strings"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// ValidatePdcProviderMessage validates parsed records at both the object and
// entity boundaries. Provider transport frames never enter the aggregate.
func ValidatePdcProviderMessage(m *pb.PdcProviderMessage) error {
	if m == nil || !canonicalUUID(m.MessageId) || m.From == "" && (m.Kind != pb.PdcProviderMessage_KIND_MALFORMED || m.ReasonCode != "MALFORMED_HEADER") || m.To == "" || m.From != strings.ToUpper(strings.TrimSpace(m.From)) || m.To != strings.ToUpper(strings.TrimSpace(m.To)) || m.Kind == pb.PdcProviderMessage_KIND_UNSPECIFIED || pb.PdcProviderMessage_Kind_name[int32(m.Kind)] == "" || pb.PdcProviderMessage_Transport_name[int32(m.Transport)] == "" || proto.Size(m) > MaxStateBytes {
		return fmt.Errorf("invalid parsed PDC message")
	}
	if m.Kind == pb.PdcProviderMessage_KIND_REQUEST && (m.Request == nil || m.Request.Callsign == "" || m.Request.Departure == "" || m.Request.Destination == "") {
		return fmt.Errorf("missing parsed PDC request")
	}
	if m.Kind != pb.PdcProviderMessage_KIND_REQUEST && m.Request != nil {
		return fmt.Errorf("unexpected PDC request fields")
	}
	if (m.Kind == pb.PdcProviderMessage_KIND_MALFORMED || m.Kind == pb.PdcProviderMessage_KIND_NOT_SUPPORTED) && m.ReasonCode == "" {
		return fmt.Errorf("PDC rejection reason missing")
	}
	if m.Kind == pb.PdcProviderMessage_KIND_CLEARANCE && (m.ClearanceText == nil || strings.TrimSpace(*m.ClearanceText) == "") {
		return fmt.Errorf("missing clearance prose")
	}
	if m.ClearanceText != nil && (m.Kind != pb.PdcProviderMessage_KIND_CLEARANCE || strings.HasPrefix(*m.ClearanceText, "/data2/")) {
		return fmt.Errorf("encoded PDC transport is not clearance prose")
	}
	if m.ProviderAcceptedAt != nil && m.ProviderAcceptedAt.CheckValid() != nil {
		return fmt.Errorf("invalid PDC acceptance timestamp")
	}
	return nil
}
