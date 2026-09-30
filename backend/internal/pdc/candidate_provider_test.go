package pdc

import (
	"strings"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

func TestParsedHoppieIdentityAndMalformedRecords(t *testing.T) {
	cases := []struct {
		kind        pb.PdcProviderMessage_Kind
		typ, packet string
	}{
		{pb.PdcProviderMessage_KIND_REQUEST, "telex", "REQUEST PREDEP CLEARANCE SAS101 A320 TO ENGM AT EKCH STAND A17 ATIS A please review"},
		{pb.PdcProviderMessage_KIND_REQUEST, "telex", "RCD 123\nSAS101-EKCH-GATE A17-ENGM\nATIS A\n-TYP/A320\n-RMK/please review"},
		{pb.PdcProviderMessage_KIND_WILCO, "cpdlc", "/data2/42/17/N/WILCO"},
		{pb.PdcProviderMessage_KIND_UNABLE, "cpdlc", "/data2/43/17/N/UNABLE"},
		{pb.PdcProviderMessage_KIND_MALFORMED, "cpdlc", "/data2/42//N/WILCO"},
		{pb.PdcProviderMessage_KIND_MALFORMED, "telex", "REQUEST PREDEP CLEARANCE incomplete"},
		{pb.PdcProviderMessage_KIND_NOT_SUPPORTED, "ping", "opaque provider text"},
	}
	for _, tc := range cases {
		t.Run(tc.typ+tc.packet[:3], func(t *testing.T) {
			raw := Message{From: "SAS101", Type: tc.typ, Packet: tc.packet, Raw: "SAS101 " + tc.typ + " {" + tc.packet + "}"}
			first, e := ParseProviderMessage("EKCH", raw)
			if e != nil {
				t.Fatal(e)
			}
			again, e := ParseProviderMessage("EKCH", raw)
			if e != nil || !proto.Equal(first, again) {
				t.Fatal("unstable parsed provider identity")
			}
			id, e := HoppieMessageCommandID(raw)
			if e != nil || id != first.MessageId || first.To != "EKCH" || first.Kind != tc.kind {
				t.Fatal(first, e)
			}
			bytes, _ := proto.Marshal(first)
			if strings.Contains(string(bytes), raw.Raw) || strings.Contains(string(bytes), "/data2/") {
				t.Fatal("raw packet persisted")
			}
			if tc.kind == pb.PdcProviderMessage_KIND_MALFORMED && first.ReasonCode == "" {
				t.Fatal("missing typed malformed reason")
			}
			if first.Request != nil && first.Request.Remarks != "please review" {
				t.Fatal("request remarks lost")
			}
		})
	}
}

func TestMalformedHoppieHeadersRetainOnlyTypedReason(t *testing.T) {
	client := NewClient("unused")
	for _, body := range []string{"ok {broken}", "ok {{bad header}}", "ok {SAS101 telex}", "ok garbage", "ok {}"} {
		messages := client.parseResponse(body)
		if len(messages) != 1 {
			t.Fatalf("malformed poll dropped: %q", body)
		}
		parsed, err := ParseProviderMessage("EKCH", messages[0])
		if err != nil || parsed.Kind != pb.PdcProviderMessage_KIND_MALFORMED || parsed.ReasonCode == "" {
			t.Fatalf("malformed poll record: %v %v", parsed, err)
		}
	}
}
