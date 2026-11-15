package frontend

import (
	"encoding/json"
	"testing"

	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"

	"github.com/stretchr/testify/require"
)

type testVacsBinding struct {
	identityTestBinding
	sent    []es.VacsRequestEvent
	deliver func(fe.VacsResultEvent)
}

func (b *testVacsBinding) SendVacsRequest(req es.VacsRequestEvent, deliver func(fe.VacsResultEvent)) error {
	b.sent = append(b.sent, req)
	b.deliver = deliver
	return nil
}

func TestVacsUsesCapturedBindingAndDeliversOnlyToInitiatingBrowser(t *testing.T) {
	client := &Client{session: 1, airport: "AAAA", send: make(chan events.OutgoingMessage, 8), closed: make(chan struct{})}
	other := &Client{session: 1, airport: "AAAA", send: make(chan events.OutgoingMessage, 8), closed: make(chan struct{})}
	binding := &testVacsBinding{identityTestBinding: identityTestBinding{session: 1, cid: "123"}}
	ctx, err := shared.WithHumanCommand(t.Context(), binding)
	require.NoError(t, err)
	req := fe.VacsRequestEvent{RequestID: "browser", Action: "answer", CallID: "oldest"}
	require.NoError(t, handleVacsRequest(ctx, client, marshalFrontendMessage(t, fe.VacsRequest, req)))
	require.Len(t, binding.sent, 1)
	binding.deliver(fe.VacsResultEvent{RequestID: req.RequestID, State: &es.VacsState{Status: "incoming", Sequence: 1}})
	require.IsType(t, fe.VacsResultEvent{}, <-client.send)
	require.Empty(t, other.send)
	require.NoError(t, handleVacsRequest(ctx, client, marshalFrontendMessage(t, fe.VacsRequest, req)))
	require.Len(t, binding.sent, 1, "duplicate actions must not be resent")
	require.NotEmpty(t, (<-client.send).(fe.VacsResultEvent).Error)
	client.setIdentity(2, "LIVE", "", "BBBB", "", false)
	binding.deliver(fe.VacsResultEvent{RequestID: req.RequestID})
	require.Empty(t, client.send, "old replies cannot cross a controller context change")
}

func TestVacsRejectsInvalidOrUnboundRequests(t *testing.T) {
	binding := &testVacsBinding{identityTestBinding: identityTestBinding{session: 1, cid: "123"}}
	ctx, err := shared.WithHumanCommand(t.Context(), binding)
	require.NoError(t, err)
	for _, req := range []fe.VacsRequestEvent{
		{Action: "status"}, {RequestID: "x", Action: "dial"}, {RequestID: "x", Action: "answer"},
		{RequestID: "x", Action: "status", ClientID: "host"}, {RequestID: "x", Action: "app_quit"},
	} {
		client := &Client{send: make(chan events.OutgoingMessage, 1), closed: make(chan struct{})}
		require.NoError(t, handleVacsRequest(ctx, client, marshalFrontendMessage(t, fe.VacsRequest, req)))
		require.NotEmpty(t, (<-client.send).(fe.VacsResultEvent).Error)
	}
	client := &Client{send: make(chan events.OutgoingMessage, 1), closed: make(chan struct{})}
	require.NoError(t, handleVacsRequest(t.Context(), client, marshalFrontendMessage(t, fe.VacsRequest, fe.VacsRequestEvent{RequestID: "x", Action: "status"})))
	require.NotEmpty(t, (<-client.send).(fe.VacsResultEvent).Error)
	require.Empty(t, binding.sent)
}

func marshalFrontendMessage(t *testing.T, kind fe.EventType, event any) Message {
	t.Helper()
	data, err := json.Marshal(event)
	require.NoError(t, err)
	return Message{Type: kind, Message: data}
}
