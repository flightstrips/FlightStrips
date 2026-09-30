package services

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestTrafficCandidateTwoReplicaNATS(t *testing.T) {
	h := newLifecycleHarness(t)
	h.now = time.Date(2026, 9, 30, 0, 5, 0, 0, time.UTC)
	owner := h.owner(sessionRef(h.id))
	writer := h.nodes[owner].candidate.Writer
	writer.Plan = cluster.PlanStrip
	clock := func(at time.Time) *timestamppb.Timestamp { return timestamppb.New(at) }
	fixtures := []*pb.Strip{
		{Callsign: "COUNT1", Departure: "EKCH", Destination: "EDDF", Bay: "STAND", Aldt: clock(h.now.Add(-15 * time.Minute))},
		{Callsign: "COUNT2", Departure: "EKCH", Destination: "EDDF", Bay: "CLEARED", Aobt: clock(h.now.Add(-time.Minute))},
		{Callsign: "COUNT3", Departure: "EKCH", Destination: "EDDF", Bay: "TAXI_TWR", Aldt: clock(h.now.Add(-15*time.Minute - time.Second))},
		{Callsign: "COUNT4", Departure: "EKCH", Destination: "EDDF", Bay: "PUSH", Aobt: clock(h.now.Add(time.Minute))},
	}
	for _, strip := range fixtures {
		request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(h.id), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "traffic-fixture"}, ExpectedEntityRevision: new(uint64), Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: strip.Callsign, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}}}}}}
		h.await("fixture acceptance", func() bool {
			reply := writer.Execute(h.ctx, request)
			if reply.Status != pb.CommandReply_COMMITTED && reply.Status != pb.CommandReply_UNAVAILABLE {
				t.Fatalf("fixture: %v", reply)
			}
			return lifecycleReply(reply) == nil
		})
	}
	var publications [2]int
	var candidates [2]*TrafficCandidate
	for i, node := range h.nodes {
		candidate, err := NewTrafficCandidate(node.candidate.Writer)
		if err != nil {
			t.Fatal(err)
		}
		candidate.Now = func() time.Time { return h.now }
		candidate.Record = func(_ context.Context, name, airport string, stand, taxi, arrivals, departures int64) {
			if name != "LIVE" || airport != "EKCH" || stand != 2 || taxi != 2 || arrivals != 1 || departures != 1 {
				t.Fatalf("wrong series %s/%s %d %d %d %d", name, airport, stand, taxi, arrivals, departures)
			}
			publications[i]++
		}
		candidates[i] = candidate
		node.work.Traffic = candidate.Traffic
	}
	if err := h.nodes[1-owner].work.Traffic(h.ctx, h.id); err == nil || publications[1-owner] != 0 {
		t.Fatal("nonowner published traffic")
	}
	h.await("owner publication", func() bool { return h.nodes[owner].work.Traffic(h.ctx, h.id) == nil })
	state := h.state()
	revision := state.Revision
	h.await("second owner publication", func() bool { return candidates[owner].Traffic(h.ctx, h.id) == nil })
	if h.state().Revision != revision {
		t.Fatal("gauge changed domain state")
	}
	h.nodes[owner].stop()
	h.nodes[owner].nc.Close()
	if err := candidates[owner].Traffic(h.ctx, h.id); err == nil {
		t.Fatal("unready owner published")
	}
	next := h.owner(sessionRef(h.id))
	if next == owner {
		t.Fatal("traffic owner did not move")
	}
	h.await("takeover publication", func() bool { return candidates[next].Traffic(h.ctx, h.id) == nil })
	if publications[owner] != 2 || publications[next] != 1 {
		t.Fatalf("unexpected publications %v", publications)
	}
}
