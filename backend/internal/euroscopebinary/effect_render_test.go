package euroscopebinary

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
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
