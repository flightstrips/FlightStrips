package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
)

func TestHTTPOutcomeProjectionScopesAuthenticatedCID(t *testing.T) {
	id := uuid.NewString()
	ref := sessionRef(1)
	state := NewAggregate(ref)
	sessionID := int32(1)
	state.Ledger[id] = &pb.CommandOutcome{CommandId: id, Actor: &pb.Actor{Kind: pb.Actor_PILOT, Id: "123", SessionId: &sessionID}, Status: pb.CommandOutcome_ACCEPTED, AggregateRevision: 7}
	p := &Projection{states: map[string]*Aggregate{"fs.v1.state.session.1": state}, started: true, positionReady: true, presenceReady: true, checked: time.Now()}
	if got := p.Outcome(context.Background(), id, &pb.Actor{Id: "123"}); got.GetOutcome().GetStatus() != pb.CommandOutcome_ACCEPTED {
		t.Fatalf("own outcome: %v", got)
	}
	if got := p.Outcome(context.Background(), id, &pb.Actor{Id: "456"}); got.Status != pb.CommandReply_UNAUTHORIZED || got.Outcome != nil {
		t.Fatalf("other actor: %v", got)
	}
	if got := p.Outcome(context.Background(), uuid.NewString(), &pb.Actor{Id: "123"}); got.Status != pb.CommandReply_NOT_FOUND {
		t.Fatalf("unknown ID: %v", got)
	}
	state.Ledger[id].Status = pb.CommandOutcome_SUCCEEDED
	if got := p.Outcome(context.Background(), id, &pb.Actor{Id: "123"}); got.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("terminal outcome: %v", got)
	}
	controllerID := uuid.NewString()
	state.Ledger[controllerID] = &pb.CommandOutcome{CommandId: controllerID, Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "123", SessionId: &sessionID}, Status: pb.CommandOutcome_SUCCEEDED}
	if got := p.Outcome(context.Background(), controllerID, &pb.Actor{Id: "123"}); got.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("controller outcome: %v", got)
	}
	providerID := uuid.NewString()
	state.Ledger[providerID] = &pb.CommandOutcome{CommandId: providerID, Actor: &pb.Actor{Kind: pb.Actor_PROVIDER, Id: "123"}, Status: pb.CommandOutcome_SUCCEEDED}
	if got := p.Outcome(context.Background(), providerID, &pb.Actor{Id: "123"}); got.Status != pb.CommandReply_UNAUTHORIZED {
		t.Fatalf("provider outcome exposed: %v", got)
	}
}

func TestHTTPFlightProjectionReadsTypedSessionEntities(t *testing.T) {
	ref := sessionRef(1)
	state := NewAggregate(ref)
	state.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{}
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{}
	state.Indexes[pb.EntityKind_SESSION]["1"] = &pb.EntitySnapshot{Key: "1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 1, Airport: "EKCH"}}}}
	state.Indexes[pb.EntityKind_STRIP]["SAS101"] = &pb.EntitySnapshot{Key: "SAS101", Revision: 3, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "SAS101", AircraftType: "A320"}}}}
	p := &Projection{states: map[string]*Aggregate{"fs.v1.state.session.1": state}, started: true, positionReady: true, presenceReady: true, checked: time.Now()}
	flight, err := p.FindFlight(context.Background(), "sas101")
	if err != nil || flight.SessionID != 1 || flight.Airport != "EKCH" || flight.Strip.AircraftType != "A320" {
		t.Fatalf("flight: %v %v", flight, err)
	}
	flight.Strip.AircraftType = "B738"
	if state.Indexes[pb.EntityKind_STRIP]["SAS101"].GetValue().GetStrip().AircraftType != "A320" {
		t.Fatal("HTTP result aliases projection")
	}
}
