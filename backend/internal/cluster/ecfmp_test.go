package cluster

import (
	"context"
	"fmt"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
)

func TestEcfmpSessionApplicationRejectsStaleSourceAndStrip(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	if _, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS123", Bay: "CLEARED", Departure: "EKCH"}, 0); err != nil {
		t.Fatal(err)
	}
	adapter := EcfmpSessionAdapter{Writer: Writer{Store: store, NodeID: "node-a"}}
	current, err := adapter.Strips(ctx, 1)
	if err != nil || len(current) != 1 {
		t.Fatalf("strips: %v %v", current, err)
	}
	v2 := fmt.Sprintf("%020d:%064x", 2, 2)
	first := adapter.Apply(ctx, 1, current[0], v2, []*pb.EcfmpRestriction{{MeasureId: 42, Kind: "mandatory_route", Routes: []string{"DCT ABC"}}})
	if first.Status != pb.CommandReply_COMMITTED {
		t.Fatalf("first application: %v", first)
	}
	retry := adapter.Apply(ctx, 1, current[0], v2, []*pb.EcfmpRestriction{{MeasureId: 42, Kind: "mandatory_route", Routes: []string{"DCT ABC"}}})
	if retry.Status != pb.CommandReply_COMMITTED || retry.GetStreamSequence() != first.GetStreamSequence() {
		t.Fatalf("retry: %v", retry)
	}
	v1 := fmt.Sprintf("%020d:%064x", 1, 1)
	stale := adapter.Apply(ctx, 1, current[0], v1, nil)
	if stale.GetOutcome().GetStatus() != pb.CommandOutcome_FAILED || stale.GetOutcome().GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("stale source accepted: %v", stale)
	}
	oldStrip := *current[0]
	oldStrip.Revision--
	if reply := adapter.Apply(ctx, 1, &oldStrip, v2, nil); reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("stale strip accepted: %v", reply)
	}
}
