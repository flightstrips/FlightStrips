package euroscope

import (
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestScopedTypedEnvelope(t *testing.T) {
	context := &MembershipContext{InstanceId: "process-a", TransportGeneration: 7, MembershipId: "membership-b", SessionId: 42, Airport: "EKCH", Environment: "SWEATBOX_123", Generation: 8, MasterEpoch: 9}
	data, err := (ScopedMessage{Context: context, Message: GenerateSquawkEvent{Callsign: "SAS123"}}).Marshal()
	require.NoError(t, err)
	var envelope Envelope
	require.NoError(t, proto.Unmarshal(data, &envelope))
	require.True(t, proto.Equal(context, envelope.Context))
	require.Equal(t, "SAS123", envelope.GetGenerateSquawk().Callsign)
	kind, payload, err := UnmarshalEnvelope(data)
	require.NoError(t, err)
	require.Equal(t, GenerateSquawk, kind)
	var sync GenerateSquawkEvent
	require.NoError(t, proto.Unmarshal(payload, &sync))
	require.Equal(t, "SAS123", sync.Callsign)
}
