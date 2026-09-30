package euroscopebinary

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestRenderAmanHoldingEATAndWithdrawal(t *testing.T) {
	for _, eat := range []string{"1422", ""} {
		effect := &pb.EffectRecord{CommandId: "aman-command", OwnerEpoch: 3, MasterEpoch: 4, Status: pb.EffectRecord_DISPATCH_CLAIMED, Payload: &pb.EffectRecord_AmanHoldingEat{AmanHoldingEat: &pb.AmanHoldingEatEffect{Callsign: "SAS123", Hold: "TESPI", HoldType: "enroute", Eat: eat, Airport: "EKCH", SourceRevision: 17}}}
		frame, err := EffectRenderer(cluster.EffectSecrets{})(123, effect)
		require.NoError(t, err)
		require.Equal(t, "SAS123", frame.GetHold().Callsign)
		require.Equal(t, eat, frame.GetHold().HoldEat)
		require.Equal(t, effect.CommandId, frame.CommandId)
		require.Equal(t, uint64(3), frame.OwnerEpoch)
		require.Equal(t, uint64(4), frame.MasterEpoch)
	}
}

func TestPdcTypedStateAndClearedEffects(t *testing.T) {
	for _, action := range []string{"STATE_CHANGE", "SET_CLEARED_FLAG", "ISSUE", "REVERT_TO_VOICE"} {
		payload := &pb.PdcEffect{Callsign: "SAS123", Action: action, Clearance: "CLRD TO: ENGM", State: proto.String("CONFIRMED"), Remarks: proto.String(""), Cleared: proto.Bool(true)}
		frame, err := EffectRenderer(cluster.EffectSecrets{})(42, &pb.EffectRecord{CommandId: "pdc-command", Status: pb.EffectRecord_DISPATCH_CLAIMED, OwnerEpoch: 3, MasterEpoch: 4, Payload: &pb.EffectRecord_Pdc{Pdc: payload}})
		if err != nil || frame.CommandId != "pdc-command" || frame.OwnerEpoch != 3 || frame.MasterEpoch != 4 {
			t.Fatalf("PDC effect: %v %v", frame, err)
		}
		if action == "STATE_CHANGE" && frame.GetPdcStateChange().GetState() != "CONFIRMED" {
			t.Fatal("state omitted")
		}
		if action == "SET_CLEARED_FLAG" && !frame.GetClearedFlag().GetCleared() {
			t.Fatal("cleared omitted")
		}
	}
}

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
