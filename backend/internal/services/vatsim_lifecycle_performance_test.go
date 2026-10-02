package services

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

func TestLifecycleCandidateStandDiffPreservesOperationalStrip(t *testing.T) {
	stands, policy := lifecyclePolicyFixture(t)
	now := time.Now().UTC()
	ref := sessionRef(42)
	state := cluster.NewAggregate(ref)
	strip := &pb.Strip{Callsign: "SAS123", Departure: "EKCH", Destination: "EDDF", AircraftType: "A320", Bay: "DEP_HIDDEN", VatsimOnly: true, VatsimCid: "12345", Heading: proto.Int32(270), Aldt: timestamppb.New(now.Add(-time.Hour)), OwnerCid: "controller", ControllerModifiedFields: []string{"heading"}}
	state.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"42": {Key: "42", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 42, Airport: "EKCH", Name: "LIVE"}}}}}
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{"SAS123": {Key: "SAS123", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}}}
	candidate := &VatsimLifecycleCandidate{Writer: cluster.Writer{Projection: &cluster.Projection{}}, Stands: cluster.StandState{Stands: stands, Policy: policy}, AllowPrefiles: true}
	request := &pb.CommandRequest{CommandId: uuid.NewString(), Aggregate: ref}
	page := &pb.VatsimPage{SnapshotAt: timestamppb.New(now), Flights: []*pb.VatsimFlight{lifecycleFlight("SAS123", "EKCH", "EDDF", "prefile")}}
	before := proto.Clone(strip).(*pb.Strip)
	change, err := candidate.plan(context.Background(), request, state, "SAS123", true, 1, "source", page, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var updated *pb.Strip
	for _, entity := range change.Changes {
		if entity.Key == strip.Callsign && entity.GetUpsert().GetStrip() != nil {
			updated = entity.GetUpsert().GetStrip()
		}
	}
	if updated == nil || updated.Stand == "" {
		t.Fatalf("stand allocation did not update strip: %v", change)
	}
	stand, revision := updated.Stand, updated.Revision
	updated.Stand, updated.Revision = before.Stand, before.Revision
	if !proto.Equal(updated, before) {
		t.Fatalf("stand diff changed operational fields: %v", updated)
	}
	if !proto.Equal(strip, before) {
		t.Fatal("planning mutated input strip")
	}
	updated.Stand, updated.Revision = stand, revision
	// Repeating the preflight against this accepted allocation is a no-op; all
	// protobuf values in the reusable input must remain untouched by planning.
	state.Indexes[pb.EntityKind_STRIP][strip.Callsign] = &pb.EntitySnapshot{Key: strip.Callsign, Revision: revision, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: updated}}}
	state.Indexes[pb.EntityKind_STAND_ASSIGNMENT] = map[string]*pb.EntitySnapshot{}
	for _, entity := range change.Changes {
		if assignment := entity.GetUpsert().GetStandAssignment(); assignment != nil {
			state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][entity.Key] = &pb.EntitySnapshot{Key: entity.Key, Revision: entity.Revision, Value: entity.GetUpsert()}
		}
	}
	for _, workflow := range change.Workflows {
		state.Workflows[workflow.WorkflowId] = workflow
	}
	snapshotInputs := map[*pb.EntitySnapshot]*pb.EntitySnapshot{}
	for _, entities := range state.Indexes {
		for _, entity := range entities {
			snapshotInputs[entity] = proto.Clone(entity).(*pb.EntitySnapshot)
		}
	}
	noOp, err := candidate.plan(context.Background(), request, state, "SAS123", true, 1, "source", page, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(noOp.Changes)+len(noOp.Workflows)+len(noOp.Effects) != 0 {
		t.Fatalf("unchanged allocation generated changes: %v", noOp)
	}
	for entity, before := range snapshotInputs {
		if !proto.Equal(entity, before) {
			t.Fatal("no-op preflight mutated reusable snapshot input")
		}
	}
}
