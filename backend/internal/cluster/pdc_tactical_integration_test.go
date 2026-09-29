package cluster

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

func TestNATSPdcTacticalConcurrencyAndRestart(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	port := 4222
	if value := os.Getenv("NATS_INTEGRATION_PORT_BASE"); value != "" {
		parsedPort, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		port = parsedPort
	}
	cfg := natsresources.Config{URLs: []string{fmt.Sprintf("nats://bootstrap:bootstrap-local-only@127.0.0.1:%d", port), fmt.Sprintf("nats://bootstrap:bootstrap-local-only@127.0.0.1:%d", port+1), fmt.Sprintf("nats://bootstrap:bootstrap-local-only@127.0.0.1:%d", port+2)}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	admin, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err := natscluster.WaitForQuorum(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := natsresources.Bootstrap(ctx, admin, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.URLs = []string{fmt.Sprintf("nats://backend:backend-local-only@127.0.0.1:%d", port)}
	nc, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream(nats.MaxWait(3 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	store := NATSStore{JS: js}
	random := uuid.New()
	id := int32(100000 + binary.BigEndian.Uint32(random[:4])%1000000000)
	ref := sessionRef(id)
	subject, _ := Subject(ref)
	claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, err := proto.Marshal(claim)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(ctx, subject, 0, data); err != nil {
		t.Fatal(err)
	}
	firstCtx, stopFirst := context.WithCancel(ctx)
	defer stopFirst()
	secondCtx, stopSecond := context.WithCancel(ctx)
	defer stopSecond()
	first, second := startProjection(t, firstCtx, nc, cfg), startProjection(t, secondCtx, nc, cfg)
	writers := []Writer{{Store: store, NodeID: "node-a", Projection: first, Plan: PlanStrip}, {Store: store, NodeID: "node-a", Projection: second, Plan: PlanStrip}}
	zero := uint64(0)
	seed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: fmt.Sprint(id), Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: id, Airport: "EKCH", Name: "LIVE", NextStripId: 1, NextTacticalId: 1, NextMessageId: 1}}}}}}}}
	if reply := writers[0].Execute(ctx, seed); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(reply)
	}
	strips := StripState{Store: LocalLifecycleStore{Writer: writers[0]}}
	if reply, err := strips.Put(ctx, id, &pb.Strip{Callsign: "SAS101", Bay: "NOT_CLEARED", Departure: "EKCH"}, 0); err != nil {
		t.Fatalf("strip: %v %v", reply, err)
	}
	if reply, err := strips.Put(ctx, id, &pb.Strip{Callsign: "SAS102", Bay: "NOT_CLEARED", Departure: "EKCH"}, 0); err != nil {
		t.Fatalf("second strip: %v %v", reply, err)
	}
	controller := ControllerSector{Store: LocalLifecycleStore{Writer: writers[0]}}
	if reply, err := controller.PutController(ctx, id, &pb.Controller{Cid: "cid-1", Callsign: "EKCH_A_TWR", Position: "118.100"}, 0); err != nil {
		t.Fatalf("controller: %v %v", reply, err)
	}
	_, deltas, stopDeltas, err := first.SubscribeInitial(ref)
	if err != nil {
		t.Fatal(err)
	}
	defer stopDeltas()
	actor := &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1", SessionId: &id}
	create := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: actor, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_Client{Client: tacticalCreate("TAXI", "CROSSING", "")}}
	var replies [2]*pb.CommandReply
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			replies[i], _ = (PdcTactical{Store: LocalLifecycleStore{Writer: writers[i]}}).Execute(ctx, create)
		}(i)
	}
	wg.Wait()
	if replies[0] == nil || replies[1] == nil || replies[0].GetStreamSequence() != replies[1].GetStreamSequence() || replies[0].GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("concurrent tactical create: %v %v", replies[0], replies[1])
	}
	secondTactical := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: actor, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_Client{Client: tacticalCreate("DEPART", "START", "")}}
	if reply, err := (PdcTactical{Store: LocalLifecycleStore{Writer: writers[0]}}).Execute(ctx, secondTactical); err != nil {
		t.Fatalf("timer strip create: %v %v", reply, err)
	}
	one := uint64(1)
	startTimer := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: actor, ExpectedEntityRevision: &one, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Tactical{Tactical: &pb.TacticalAction{StripId: 2, Change: &pb.TacticalAction_StartTimer{StartTimer: &pb.StartTacticalTimer{}}}}}}}
	if reply, err := (PdcTactical{Store: LocalLifecycleStore{Writer: writers[0]}}).Execute(ctx, startTimer); err != nil {
		t.Fatalf("timer start: %v %v", reply, err)
	}
	select {
	case delta := <-deltas:
		found := false
		for _, change := range delta.Changes {
			if change.GetUpsert().GetTacticalStrip() != nil {
				found = true
			}
		}
		if !found {
			t.Fatalf("tactical create missing from typed FrontendDelta: %v", delta)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	state, err := first.Read(ref)
	if err != nil {
		t.Fatal(err)
	}
	session := proto.Clone(state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].GetValue().GetSession()).(*pb.Session)
	session.Master = &pb.MasterTerm{Cid: "cid-1", ConnectionId: "connection-1", Epoch: 1, OwnerEpoch: 1}
	sessionRevision := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].Revision
	masterUpdate := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test-master"}, ExpectedEntityRevision: &sessionRevision, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: fmt.Sprint(id), Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}}}}}}
	if reply := writers[0].Execute(ctx, masterUpdate); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("master: %v", reply)
	}
	pilot := &pb.Actor{Kind: pb.Actor_PILOT, Id: "SAS101", SessionId: &id}
	requestPdc := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: pilot, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Pdc{Pdc: &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{}}}}}}}
	if reply, err := (PdcTactical{Store: LocalLifecycleStore{Writer: writers[0]}}).Execute(ctx, requestPdc); err != nil {
		t.Fatalf("PDC request: %v %v", reply, err)
	}
	pendingPdc := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_PILOT, Id: "SAS102", SessionId: &id}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Pdc{Pdc: &pb.PdcAction{Callsign: "SAS102", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{}}}}}}}
	if reply, err := (PdcTactical{Store: LocalLifecycleStore{Writer: writers[0]}}).Execute(ctx, pendingPdc); err != nil {
		t.Fatalf("pending PDC request: %v %v", reply, err)
	}
	issue := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: actor, ExpectedEntityRevision: &one, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Pdc{Pdc: &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{Clearance: "CLEARED TO EKCH"}}}}}}}
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			replies[i], _ = (PdcTactical{Store: LocalLifecycleStore{Writer: writers[i]}}).Execute(ctx, issue)
		}(i)
	}
	wg.Wait()
	if replies[0] == nil || replies[1] == nil || replies[0].GetStreamSequence() != replies[1].GetStreamSequence() || replies[0].GetOutcome().GetStatus() != pb.CommandOutcome_ACCEPTED {
		t.Fatalf("concurrent PDC issue: %v %v", replies[0], replies[1])
	}
	stopFirst()
	stopSecond()
	restarted := startProjection(t, ctx, nc, cfg)
	adapter := PdcTactical{Store: LocalLifecycleStore{Writer: Writer{Store: store, Projection: restarted}}}
	tactical, _, err := adapter.Tactical(ctx, id, 1)
	if err != nil || tactical.Id != 1 || tactical.Sequence != 1000 {
		t.Fatalf("restart: %v %v", tactical, err)
	}
	timer, _, err := adapter.Tactical(ctx, id, 2)
	if err != nil || timer.TimerStartedAt == nil {
		t.Fatalf("tactical timer replay: %v %v", timer, err)
	}
	pending, _, err := adapter.Pdc(ctx, id, "SAS102")
	if err != nil || pending.State != "REQUESTED" || pending.RequestedAt == nil {
		t.Fatalf("pending PDC replay: %v %v", pending, err)
	}
	pdc, _, err := adapter.Pdc(ctx, id, "SAS101")
	if err != nil || pdc.State != "CLEARED" || pdc.Deadline == nil {
		t.Fatalf("PDC replay: %v %v", pdc, err)
	}
	after, err := restarted.Read(ref)
	if err != nil || after.Effects[issue.CommandId] == nil || after.Indexes[pb.EntityKind_SESSION_DEADLINE]["pdc.SAS101"] == nil {
		t.Fatalf("PDC effect/deadline replay: %v %v", after, err)
	}
}
