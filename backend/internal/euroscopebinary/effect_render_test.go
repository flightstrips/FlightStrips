package euroscopebinary

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"testing"
)

func TestStandEffectRendersClaimedImmutableAction(t *testing.T) {
	effect := &pb.EffectRecord{CommandId: "stand-command", TargetCid: "777777", OwnerEpoch: 3, MasterEpoch: 5, Status: pb.EffectRecord_DISPATCH_CLAIMED, Payload: &pb.EffectRecord_SetFlightPlan{SetFlightPlan: &pb.SetFlightPlanEffect{Callsign: "SAS123", Field: "STAND", Value: "A1"}}}
	frame, err := EffectRenderer(cluster.EffectSecrets{})(42, effect)
	if err != nil || frame.GetStand().GetStand() != "A1" || frame.GetStand().GetCallsign() != "SAS123" || frame.CommandId != effect.CommandId || frame.OwnerEpoch != 3 || frame.MasterEpoch != 5 {
		t.Fatalf("stand effect lost its action or fences: %v %v", frame, err)
	}
	effect.Status = pb.EffectRecord_WAITING
	if _, err := EffectRenderer(cluster.EffectSecrets{})(42, effect); err == nil {
		t.Fatal("unclaimed effect rendered")
	}
}
