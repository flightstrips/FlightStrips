package cluster

import (
	"context"
	"fmt"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAtisSessionApplicationRejectsStaleFeedAndWeather(t *testing.T) {
	store, _ := stripFixture(t)
	adapter := AtisSessionAdapter{Writer: Writer{Store: store, NodeID: "node-a"}}
	version := func(feed, metar int64) string { return fmt.Sprintf("%020d:%064x:%020d", feed, feed, metar) }
	value := &pb.Atis{Airport: "EKCH", ArrivalCode: "A", DepartureCode: "B", Metar: "EKCH 301200Z CAVOK", ObservedAt: timestamppb.Now(), SourceRevision: version(2, 100)}
	ctx := context.Background()
	first := adapter.Apply(ctx, 1, value)
	if first.Status != pb.CommandReply_COMMITTED || first.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("first ATIS: %v", first)
	}
	if retry := adapter.Apply(ctx, 1, proto.Clone(value).(*pb.Atis)); retry.GetStreamSequence() != first.GetStreamSequence() {
		t.Fatalf("duplicate ATIS event: %v", retry)
	}
	older := proto.Clone(value).(*pb.Atis)
	older.SourceRevision = version(1, 200)
	if reply := adapter.Apply(ctx, 1, older); reply.GetOutcome().GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("older feed accepted: %v", reply)
	}
	newer := proto.Clone(value).(*pb.Atis)
	newer.SourceRevision, newer.Metar = version(2, 200), "EKCH 301300Z CAVOK"
	if reply := adapter.Apply(ctx, 1, newer); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("new METAR: %v", reply)
	}
	stale := proto.Clone(value).(*pb.Atis)
	stale.SourceRevision = version(2, 150)
	if reply := adapter.Apply(ctx, 1, stale); reply.GetOutcome().GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("older METAR accepted: %v", reply)
	}
}
