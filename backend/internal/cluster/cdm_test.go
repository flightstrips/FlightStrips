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

func TestPilotTobtPlanCommitsTypedPendingEffect(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	if _, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Departure: "EKCH", Destination: "ENGM", Bay: "CLEARED"}, 0); err != nil {
		t.Fatal(err)
	}
	state, err := (Writer{Store: store}).load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	session := state.Entities["1"].GetValue().GetSession()
	session.Master = &pb.MasterTerm{Cid: "master", ConnectionId: uuid.NewString(), Epoch: 2}
	id := uuid.NewString()
	revision := uint64(0)
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_PILOT, Id: "123", SessionId: func() *int32 { value := int32(1); return &value }()}, ExpectedEntityRevision: &revision, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Cdm{Cdm: &pb.CdmAction{Callsign: "SAS101", Change: &pb.CdmAction_SetTobt{SetTobt: &pb.SetTobt{Value: timestamppb.New(time.Date(2026, 9, 29, 10, 30, 0, 0, time.UTC))}}}}}}}
	change, status, _, err := PlanCdm(ctx, request, state)
	if err != nil || status != pb.CommandReply_COMMITTED || len(change.Changes) != 2 || len(change.Effects) != 1 || change.Effects[0].CommandId != id || change.Effects[0].GetCdm().Value != "1030" {
		t.Fatalf("CDM plan: %v %v %v", change, status, err)
	}
	set := request.GetClient().GetCdm().GetSetTobt()
	set.Value, set.HhmmUtc = nil, "1030"
	if change, status, _, err = PlanCdm(ctx, request, state); err != nil || status != pb.CommandReply_COMMITTED || change.Effects[0].GetCdm().Value != "1030" {
		t.Fatalf("HTTP clock plan: %v %v", status, err)
	}
	request.Actor.Id = ""
	if _, status, _, err = PlanCdm(ctx, request, state); err == nil || status != pb.CommandReply_UNAUTHORIZED {
		t.Fatalf("unauthorized pilot: %v %v", status, err)
	}
}

func TestPilotTobtCommandDeduplicatesPendingEffect(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	if _, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Departure: "EKCH", Destination: "ENGM", Bay: "CLEARED"}, 0); err != nil {
		t.Fatal(err)
	}
	reader := Writer{Store: store}
	state, err := reader.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	session := proto.Clone(state.Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession()).(*pb.Session)
	session.Master = &pb.MasterTerm{Cid: "master", ConnectionId: "connection-1", Epoch: 1, OwnerEpoch: 1}
	revision := state.Indexes[pb.EntityKind_SESSION]["1"].Revision
	seed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test-master"}, ExpectedEntityRevision: &revision, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}}}}}}
	if reply := (Writer{Store: store, NodeID: "node-a", Plan: PlanSystemEntity}).Execute(ctx, seed); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("master seed: %v", reply)
	}
	zero := uint64(0)
	sessionID := int32(1)
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_PILOT, Id: "123", SessionId: &sessionID}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Cdm{Cdm: &pb.CdmAction{Callsign: "SAS101", Change: &pb.CdmAction_SetTobt{SetTobt: &pb.SetTobt{HhmmUtc: "1030"}}}}}}}
	first := strips.Store.Execute(ctx, req)
	if first.Status != pb.CommandReply_PENDING || first.GetOutcome().GetStatus() != pb.CommandOutcome_ACCEPTED {
		t.Fatalf("TOBT commit: %v", first)
	}
	before := store.commits
	second := strips.Store.Execute(ctx, req)
	if second.GetStreamSequence() != first.GetStreamSequence() || store.commits != before {
		t.Fatalf("retry committed another effect: %v", second)
	}
}
