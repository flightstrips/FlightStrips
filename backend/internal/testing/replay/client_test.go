package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/testing/recorder"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestRecordedPeerUsesIssuedContextAndStopsAfterMembershipRemoval(t *testing.T) {
	peerErrors := make(chan error, 1)
	stop := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			peerErrors <- err
			return
		}
		defer conn.Close()
		read := func() (*es.Envelope, error) {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				return nil, err
			}
			if kind != websocket.BinaryMessage {
				return nil, fmt.Errorf("non-binary fixture event")
			}
			var envelope es.Envelope
			err = proto.Unmarshal(data, &envelope)
			return &envelope, err
		}
		send := func(scope *es.MembershipContext, message proto.Message, kind es.EventType) error {
			data, err := es.MarshalEnvelope(message, kind)
			if err != nil {
				return err
			}
			var envelope es.Envelope
			if err = proto.Unmarshal(data, &envelope); err != nil {
				return err
			}
			envelope.Context = scope
			data, err = proto.Marshal(&envelope)
			if err != nil {
				return err
			}
			return conn.WriteMessage(websocket.BinaryMessage, data)
		}
		token, err := read()
		if err != nil {
			peerErrors <- err
			return
		}
		if token.GetToken() == nil || token.GetToken().InstanceId == "" {
			peerErrors <- fmt.Errorf("missing instance identity")
			return
		}
		transport := &es.MembershipContext{InstanceId: token.GetToken().InstanceId, TransportGeneration: 123}
		if err = send(transport, &es.ControlEvent{}, es.Control); err != nil {
			peerErrors <- err
			return
		}
		control, err := read()
		if err != nil {
			peerErrors <- err
			return
		}
		if control.GetControl() == nil || !proto.Equal(transport, control.Context) {
			peerErrors <- fmt.Errorf("control did not use transport context")
			return
		}
		member := &es.MembershipContext{InstanceId: transport.InstanceId, TransportGeneration: 123, MembershipId: "issued-member", SessionId: 17, Airport: "EKCH", Environment: "LIVE", Generation: 42, MasterEpoch: 7}
		if err = send(member, &es.SessionInfoEvent{Role: es.SessionInfoMaster}, es.SessionInfo); err != nil {
			peerErrors <- err
			return
		}
		event, err := read()
		if err != nil {
			peerErrors <- err
			return
		}
		if event.GetGroundState() == nil || !proto.Equal(member, event.Context) {
			peerErrors <- fmt.Errorf("historical event did not use issued membership context")
			return
		}
		member = proto.Clone(member).(*es.MembershipContext)
		member.MasterEpoch = 9
		if err = send(member, &es.MasterInfoEvent{MasterCallsign: "EKCH_A_TWR"}, es.MasterInfo); err != nil {
			peerErrors <- err
			return
		}
		event, err = read()
		if err != nil {
			peerErrors <- err
			return
		}
		if event.GetAssignedSquawk() == nil || !proto.Equal(member, event.Context) {
			peerErrors <- fmt.Errorf("event did not capture current master epoch")
			return
		}
		if err = send(member, &es.MembershipRemovedEvent{Reason: "test"}, es.MembershipRemoved); err != nil {
			peerErrors <- err
			return
		}
		peerErrors <- nil
		<-stop
	}))
	defer server.Close()
	defer close(stop)
	config := DefaultConfig()
	config.SessionFile = "historical-fixture"
	config.ServerURL = "ws" + strings.TrimPrefix(server.URL, "http")
	client, err := NewClient(config)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.Connect(ctx))
	defer client.Close()
	client.ReadMessages(ctx, nil)
	require.Error(t, client.SendProtobuf(&es.GroundStateEvent{Callsign: "SAS123"}, es.GroundState), "no operational frame before membership")
	recorded := func(message proto.Message, kind es.EventType, scope *es.MembershipContext) recorder.RecordedEvent {
		data, err := es.MarshalEnvelope(message, kind)
		require.NoError(t, err)
		var envelope es.Envelope
		require.NoError(t, proto.Unmarshal(data, &envelope))
		envelope.Context = scope
		data, err = proto.Marshal(&envelope)
		require.NoError(t, err)
		payload, err := json.Marshal(data)
		require.NoError(t, err)
		return recorder.RecordedEvent{Type: es.EventName(kind), Payload: payload}
	}
	require.NoError(t, client.SendEvent(recorded(&es.TokenEvent{Token: "historical-secret"}, es.Authentication, nil)))
	require.NoError(t, client.SendEvent(recorded(&es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Position: "118.105", Callsign: "EKCH_A_TWR"}, es.Login, nil)))
	require.NoError(t, client.SendEvent(recorded(&es.GroundStateEvent{Callsign: "SAS123", GroundState: es.GroundStateTaxi}, es.GroundState, &es.MembershipContext{InstanceId: "old-process", TransportGeneration: 1, MembershipId: "old-member", SessionId: 2, Airport: "EKCH", Environment: "LIVE", Generation: 3, MasterEpoch: 4})))
	require.Eventually(t, func() bool { return receivedKind(client, "master_info") }, time.Second, time.Millisecond)
	require.NoError(t, client.SendProtobuf(&es.AssignedSquawkEvent{Callsign: "SAS123", Squawk: "1234"}, es.AssignedSquawk))
	require.Eventually(t, func() bool { return receivedKind(client, "membership_removed") }, time.Second, time.Millisecond)
	require.Error(t, client.SendProtobuf(&es.GroundStateEvent{Callsign: "SAS123"}, es.GroundState))
	require.NoError(t, <-peerErrors)
}

func receivedKind(client *Client, kind string) bool {
	for _, event := range client.GetReceivedMessages() {
		if event.EventType == kind {
			return true
		}
	}
	return false
}
