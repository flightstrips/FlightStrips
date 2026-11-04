package euroscope

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/shared"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestAutomaticPDCEffectsSelectExactMasterWithDuplicateCID(t *testing.T) {
	hub := membershipTestHub()
	first := membershipTestRoot(hub, "123", "first")
	second := membershipTestRoot(hub, "123", "second")
	slave := membershipTestMember(hub, first, 1, "AAAA", "LIVE", "slave")
	master := membershipTestMember(hub, second, 1, "AAAA", "LIVE", "master")
	// These are the actual PDC shared dispatch paths, with its resolved master
	// CID. Automatic routing retains the known master pointer despite CID reuse.
	for _, event := range []es.OutgoingMessage{
		es.ClearedFlagEvent{Callsign: "SAS123", Cleared: true},
		es.RouteEvent{Callsign: "SAS123", Route: "DCT TEST"},
		es.SidEvent{Callsign: "SAS123", Sid: "TEST1A"},
	} {
		require.NoError(t, shared.DispatchEuroscopeCommand(context.Background(), hub, 1, "123", event, nil))
		wire, err := (<-second.send).Marshal()
		require.NoError(t, err)
		var envelope es.Envelope
		require.NoError(t, proto.Unmarshal(wire, &envelope))
		require.True(t, proto.Equal(master.wireContext(), envelope.Context))
		require.Equal(t, es.CommandOrigin_COMMAND_ORIGIN_AUTOMATIC, envelope.CommandOrigin)
	}
	// Automatic departure-lifecycle pilot messages use the same scoped
	// executor; a CID round trip here would discard the exact master again.
	require.True(t, hub.SendPrivateMessageFromDelivery(1, "SAS123", "MOVE TO A1"))
	wire, err := (<-second.send).Marshal()
	require.NoError(t, err)
	var delivery es.Envelope
	require.NoError(t, proto.Unmarshal(wire, &delivery))
	require.True(t, proto.Equal(master.wireContext(), delivery.Context))
	require.Equal(t, es.CommandOrigin_COMMAND_ORIGIN_AUTOMATIC, delivery.CommandOrigin)
	require.Equal(t, "MOVE TO A1", delivery.GetSendPrivateMessage().Message)
	require.Empty(t, first.send)
	_, err = hub.CaptureCommandBinding(1, "123")
	require.Error(t, err, "human selection still requires an exact own instance")
	// A captured automatic master binding must not silently become a slave
	// executor or rebind when another same-CID instance takes over.
	binding, err := hub.ResolveAutomaticCommandBinding(1, "123")
	require.NoError(t, err)
	hub.setMasterClient(slave)
	consumeAutomaticTestElection(t, first, second)
	require.Error(t, binding.Send(es.ClearedFlagEvent{Callsign: "SAS123", Cleared: true}))
	require.Empty(t, first.send)
	require.Empty(t, second.send)
}

func TestAutomaticSquawkRetainsExactMasterThroughQueue(t *testing.T) {
	hub := membershipTestHub()
	first := membershipTestRoot(hub, "123", "first")
	second := membershipTestRoot(hub, "123", "second")
	slave := membershipTestMember(hub, first, 1, "AAAA", "LIVE", "slave")
	master := membershipTestMember(hub, second, 1, "AAAA", "LIVE", "master")
	hub.squawkThrottle = newSquawkThrottle(time.Hour, nil, hub.dispatchGenerateSquawkRequest)
	t.Cleanup(func() {
		hub.squawkThrottle.mu.Lock()
		defer hub.squawkThrottle.mu.Unlock()
		for _, state := range hub.squawkThrottle.states {
			if state.timer != nil {
				state.timer.Stop()
			}
		}
	})
	hub.SendGenerateSquawk(1, "", "FIRST")
	wire, err := (<-second.send).Marshal()
	require.NoError(t, err)
	var envelope es.Envelope
	require.NoError(t, proto.Unmarshal(wire, &envelope))
	require.True(t, proto.Equal(master.wireContext(), envelope.Context))
	require.Equal(t, es.CommandOrigin_COMMAND_ORIGIN_AUTOMATIC, envelope.CommandOrigin)
	require.Equal(t, "FIRST", envelope.GetGenerateSquawk().Callsign)
	require.Empty(t, first.send)
	hub.SendGenerateSquawk(1, "", "QUEUED")
	hub.SendGenerateSquawk(1, "", "QUEUED")
	require.Len(t, hub.squawkThrottle.states[1].pending, 1)
	hub.setMasterClient(slave)
	consumeAutomaticTestElection(t, first, second)
	hub.squawkThrottle.flush(1)
	require.Empty(t, first.send, "queued automatic work cannot rebind to replacement master")
	require.Empty(t, second.send, "former master cannot execute stale queued work")
	hub.SendGenerateSquawk(1, "123", "HUMAN")
	require.Empty(t, first.send)
	require.Empty(t, second.send)
}

func consumeAutomaticTestElection(t *testing.T, roots ...*Client) {
	t.Helper()
	for _, root := range roots {
		wire, err := (<-root.send).Marshal()
		require.NoError(t, err)
		var envelope es.Envelope
		require.NoError(t, proto.Unmarshal(wire, &envelope))
		require.NotNil(t, envelope.GetMasterInfo())
	}
}
