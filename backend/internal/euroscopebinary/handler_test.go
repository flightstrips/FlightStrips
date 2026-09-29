package euroscopebinary

import (
	"testing"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
)

func TestBackendSyncOnlyMaterializesObservedFlightPlans(t *testing.T) {
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: 42}}}
	state := cluster.NewAggregate(ref)
	state.Entities["PLANNED"] = &pb.EntitySnapshot{Key: "PLANNED", Value: &pb.EntityRecord{
		Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "PLANNED", Bay: "CLEARED"}}}}
	state.Entities["OBSERVED"] = &pb.EntitySnapshot{Key: "OBSERVED", Value: &pb.EntityRecord{
		Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "OBSERVED", Bay: "CLEARED", HasFlightPlan: true}}}}
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{
		"PLANNED": state.Entities["PLANNED"], "OBSERVED": state.Entities["OBSERVED"]}
	sync := backendSync(state).GetBackendSync()
	if len(sync.Strips) != 1 || sync.Strips[0].Callsign != "OBSERVED" {
		t.Fatalf("backend sync materialized an unobserved planning strip: %v", sync.Strips)
	}
}
