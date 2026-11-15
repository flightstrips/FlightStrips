package euroscope

import (
	"testing"
	"time"

	"FlightStrips/internal/shared"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestVacsRelayPinsOwnMembershipAndCorrelatesOnlyItsResult(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "own")
	own := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "own-member")
	otherRoot := membershipTestRoot(hub, "123", "other-instance")
	other := membershipTestMember(hub, otherRoot, 1, "AAAA", "LIVE", "other-member")
	require.Same(t, other, hub.getMasterClient(1))
	own.vacsRelay.Store(true)
	binding := &commandBinding{hub: hub, client: own, session: 1, cid: "123"}
	var results []fe.VacsResultEvent
	deliver := func(result fe.VacsResultEvent) { results = append(results, result) }
	require.NoError(t, binding.SendVacsRequest(es.VacsRequestEvent{RequestId: "browser-request", Action: "answer", CallId: "call"}, deliver))
	require.Empty(t, otherRoot.send)
	bytes, err := (<-root.send).Marshal()
	require.NoError(t, err)
	var wire es.Envelope
	require.NoError(t, proto.Unmarshal(bytes, &wire))
	require.Equal(t, own.membership.MembershipId, wire.Context.MembershipId)
	require.Equal(t, es.CommandOrigin_COMMAND_ORIGIN_HUMAN, wire.CommandOrigin)
	request := wire.GetVacsRequest()
	require.NotEqual(t, "browser-request", request.RequestId)
	require.Equal(t, "call", request.CallId)
	response := &es.VacsResultEvent{RequestId: request.RequestId, State: &es.VacsState{Status: "incoming", Sequence: 1, Waiting: 2}}
	handlers := shared.NewMessageHandlers[es.EventType, *Client]()
	handlers.Add(es.VacsResult, handleVacsResult)
	// A second instance, even under the same CID and session, cannot answer.
	require.Error(t, handlers.Handle(t.Context(), other, authorityMessage(t, es.VacsResult, response, other.wireContext())))
	require.Empty(t, results)
	// Human call controls and results survive unrelated master elections.
	require.NoError(t, handlers.Handle(t.Context(), own, authorityMessage(t, es.VacsResult, response, own.wireContext())))
	require.Len(t, results, 1)
	require.Equal(t, "browser-request", results[0].RequestID)
	require.Equal(t, uint32(2), results[0].State.Waiting)
	require.Error(t, handlers.Handle(t.Context(), own, authorityMessage(t, es.VacsResult, response, own.wireContext())), "duplicate replies are not delivered")
}

func TestVacsRelayRejectsExpiredDisconnectedAndUnsupportedPlugins(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "own")
	own := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "member")
	binding := &commandBinding{hub: hub, client: own, session: 1, cid: "123"}
	deliver := func(fe.VacsResultEvent) { t.Fatal("unexpected response") }
	require.ErrorContains(t, binding.SendVacsRequest(es.VacsRequestEvent{RequestId: "a", Action: "status"}, deliver), "Update")
	own.vacsRelay.Store(true)
	require.NoError(t, binding.SendVacsRequest(es.VacsRequestEvent{RequestId: "b", Action: "status"}, deliver))
	bytes, err := (<-root.send).Marshal()
	require.NoError(t, err)
	var wire es.Envelope
	require.NoError(t, proto.Unmarshal(bytes, &wire))
	id := wire.GetVacsRequest().RequestId
	entry := own.executions["vacs:"+id]
	entry.deadline = time.Now().Add(-time.Second)
	own.executions["vacs:"+id] = entry
	payload, err := proto.Marshal(&es.VacsResultEvent{RequestId: id})
	require.NoError(t, err)
	require.False(t, own.validateExecutionEcho("vacs_result", payload, es.CommandOrigin_COMMAND_ORIGIN_UNSPECIFIED, ""))
	require.Error(t, handleVacsResult(t.Context(), own, Message{Type: es.VacsResult, Message: payload}))
	own.observer = true
	require.Error(t, binding.SendVacsRequest(es.VacsRequestEvent{RequestId: "c", Action: "status"}, deliver))
	own.observer = false
	close(root.closed)
	require.Error(t, binding.SendVacsRequest(es.VacsRequestEvent{RequestId: "d", Action: "status"}, deliver))
	require.Empty(t, root.send, "a reconnect never replays call actions")
}
