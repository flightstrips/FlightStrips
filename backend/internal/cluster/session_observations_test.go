package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSessionObservationsReplicateWithoutChangingStripVersion(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	adapter := SessionObservations{Store: strips.Store}
	if _, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "CLEARED"}, 0); err != nil {
		t.Fatal(err)
	}
	before, _, err := strips.ByCallsign(ctx, 1, "SAS101")
	if err != nil {
		t.Fatal(err)
	}
	messageID := uuid.NewString()
	if _, err := adapter.SendMessage(ctx, 1, messageID, "cid-1", "hello", []string{"TWR"}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.SendMessage(ctx, 1, messageID, "cid-1", "hello", []string{"TWR"}); err != nil {
		t.Fatal(err)
	}
	atis := &pb.Atis{Airport: "EKCH", Metar: "EKCH 291900Z 21005KT CAVOK", ArrivalCode: "A", DepartureCode: "B", ObservedAt: timestamppb.Now()}
	if _, err := adapter.PutAtis(ctx, 1, uuid.NewString(), atis, 0); err != nil {
		t.Fatal(err)
	}
	clx := &pb.ClxOverride{Callsign: "SAS101", Key: "SID", Actor: "cid-1", CreatedAt: timestamppb.Now()}
	if _, err := adapter.PutClxOverride(ctx, 1, uuid.NewString(), clx, 0); err != nil {
		t.Fatal(err)
	}
	deadline := &pb.SessionDeadline{Id: "disconnect-SAS101", Kind: "aircraft-disconnect", Callsign: "SAS101", DueAt: timestamppb.New(time.Now().Add(time.Minute)), SourceRevision: 1}
	if _, err := adapter.PutDeadline(ctx, 1, uuid.NewString(), deadline, 0); err != nil {
		t.Fatal(err)
	}
	if pending, revision, err := adapter.AircraftDisconnectPending(ctx, 1, "SAS101"); err != nil || pending == nil || revision != 1 {
		t.Fatalf("disconnect deadline unavailable across adapter: %v %d %v", pending, revision, err)
	}
	state, err := adapter.List(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.EntitiesByKind(pb.EntityKind_FRONTEND_MESSAGE)) != 1 || state.Indexes[pb.EntityKind_ATIS]["EKCH"].GetValue().GetAtis().ArrivalCode != "A" ||
		state.Indexes[pb.EntityKind_CLX_OVERRIDE]["SAS101.SID"] == nil || state.Indexes[pb.EntityKind_SESSION_DEADLINE]["disconnect-SAS101"] == nil {
		t.Fatalf("missing replicated session state: %v", state.Indexes)
	}
	after, _, err := strips.ByCallsign(ctx, 1, "SAS101")
	if err != nil || after.Revision != before.Revision {
		t.Fatalf("observation changed strip revision: %v %v", after, err)
	}
	replayed, err := (Writer{Store: store}).load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []pb.EntityKind{pb.EntityKind_FRONTEND_MESSAGE, pb.EntityKind_ATIS, pb.EntityKind_CLX_OVERRIDE, pb.EntityKind_SESSION_DEADLINE} {
		one, two := state.EntitiesByKind(kind), replayed.EntitiesByKind(kind)
		if len(one) != len(two) {
			t.Fatalf("kind %v diverged on replay", kind)
		}
		for i := range one {
			if !proto.Equal(one[i], two[i]) {
				t.Fatalf("kind %v diverged on replay", kind)
			}
		}
	}
}

func TestSessionMessageHistoryRetainsNewestHundred(t *testing.T) {
	_, strips := stripFixture(t)
	adapter := SessionObservations{Store: strips.Store}
	for i := 0; i < 101; i++ {
		if _, err := adapter.SendMessage(context.Background(), 1, uuid.NewString(), "cid-1", "message", nil); err != nil {
			t.Fatal(err)
		}
	}
	state, err := adapter.List(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	messages := state.EntitiesByKind(pb.EntityKind_FRONTEND_MESSAGE)
	if len(messages) != 100 {
		t.Fatalf("message history has %d entries", len(messages))
	}
	for _, entry := range messages {
		id := entry.GetValue().GetFrontendMessage().Id
		if id < 2 || id > 101 {
			t.Fatalf("expired message %d was retained", id)
		}
	}
}
