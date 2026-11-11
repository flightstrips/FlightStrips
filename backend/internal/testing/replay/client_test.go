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
	updates := []*es.LoginEvent{
		{Airport: "EKCH", Connection: "LIVE", Position: "119.500", Callsign: "EKCH_A_TWR"},
		{Airport: "EKCH", Connection: "LIVE", Position: "119.500", Callsign: "EKCH_A_TWR", Range: 200},
		{Airport: "EKCH", Connection: "LIVE", Position: "119.500", Callsign: "EKCH_A_TWR", Range: 200, LocalIp: "192.0.2.1"},
	}
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
		// In-place updates do not trigger another SessionInfo when the role is
		// unchanged. The client must retain readiness and keep sending evidence.
		for _, update := range updates {
			event, err := read()
			if err != nil {
				peerErrors <- err
				return
			}
			control := event.GetControl()
			if control == nil || control.PrimaryFrequency != update.Position || control.Range != update.Range || control.LocalIp != update.LocalIp || !proto.Equal(transport, event.Context) {
				peerErrors <- fmt.Errorf("in-place update lost control evidence or transport scope")
				return
			}
			if update == updates[0] {
				if err := send(member, control, es.Control); err != nil {
					peerErrors <- err
					return
				}
			}
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
		if event.ExecutionId != "" || event.CommandOrigin != es.CommandOrigin_COMMAND_ORIGIN_UNSPECIFIED {
			peerErrors <- fmt.Errorf("historical SDK result retained stale execution authority")
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
		if kind == es.GroundState {
			envelope.ExecutionId = "historical-command"
			envelope.CommandOrigin = es.CommandOrigin_COMMAND_ORIGIN_HUMAN
		}
		data, err = proto.Marshal(&envelope)
		require.NoError(t, err)
		payload, err := json.Marshal(data)
		require.NoError(t, err)
		return recorder.RecordedEvent{Type: es.EventName(kind), Payload: payload}
	}
	require.NoError(t, client.SendEvent(recorded(&es.TokenEvent{Token: "historical-secret"}, es.Authentication, nil)))
	require.NoError(t, client.SendEvent(recorded(&es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Position: "118.105", Callsign: "EKCH_A_TWR"}, es.Login, nil)))
	for _, update := range updates {
		updateCtx, updateCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		err := client.JoinMembership(updateCtx, update)
		updateCancel()
		require.NoError(t, err, "in-place update must not wait for another role announcement")
	}
	require.NoError(t, client.SendEvent(recorded(&es.GroundStateEvent{Callsign: "SAS123", GroundState: es.GroundStateTaxi}, es.GroundState, &es.MembershipContext{InstanceId: "old-process", TransportGeneration: 1, MembershipId: "old-member", SessionId: 2, Airport: "EKCH", Environment: "LIVE", Generation: 3, MasterEpoch: 4})))
	require.Eventually(t, func() bool { return receivedKind(client, "master_info") }, time.Second, time.Millisecond)
	require.NoError(t, client.SendProtobuf(&es.AssignedSquawkEvent{Callsign: "SAS123", Squawk: "1234"}, es.AssignedSquawk))
	require.Eventually(t, func() bool { return receivedKind(client, "membership_removed") }, time.Second, time.Millisecond)
	require.Error(t, client.SendProtobuf(&es.GroundStateEvent{Callsign: "SAS123"}, es.GroundState))
	require.NoError(t, <-peerErrors)
}

func TestJoinMembershipWaitsForNewRoleAfterIdentityOrAssociationChange(t *testing.T) {
	controls := make(chan *es.Envelope, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var event es.Envelope
			if proto.Unmarshal(data, &event) == nil {
				controls <- &event
			}
			// Deliberately withhold a replacement SessionInfo.
		}
	}))
	defer server.Close()
	config := DefaultConfig()
	config.SessionFile = "identity-fixture"
	client, err := NewClient(config)
	require.NoError(t, err)
	client.conn, _, err = websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer client.Close()
	client.transportScope = &es.MembershipContext{InstanceId: client.instance, TransportGeneration: 1}
	for _, name := range []string{"callsign", "environment", "observer", "airport", "offline frequency", "missing frequency"} {
		t.Run(name, func(t *testing.T) {
			client.lastControl = &es.ControlEvent{Environment: "LIVE", Callsign: "EKCH_A_TWR", PrimaryFrequency: "118.105", RelevantAirport: "EKCH"}
			client.membershipScope = &es.MembershipContext{Airport: "EKCH", Environment: "LIVE", MembershipId: "old-member"}
			client.membershipReady = true
			login := &es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Callsign: "EKCH_A_TWR", Position: "118.105"}
			switch name {
			case "callsign":
				login.Callsign = "EKCH_B_TWR"
			case "environment":
				login.Connection = "SWEATBOX"
			case "observer":
				login.Observer = true
			case "airport":
				login.Airport = "EKBI"
			case "offline frequency":
				login.Position = "199.998"
			case "missing frequency":
				login.Position = ""
			}
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			require.ErrorIs(t, client.JoinMembership(ctx, login), context.DeadlineExceeded)
			require.False(t, client.membershipReady)
			require.Nil(t, client.membershipScope)
			select {
			case event := <-controls:
				require.NotNil(t, event.GetControl())
				require.True(t, proto.Equal(client.transportScope, event.Context))
			case <-time.After(time.Second):
				t.Fatal("replacement control was not sent")
			}
		})
	}
}

func TestFrequencyUpdateWaitsForAppliedControlContext(t *testing.T) {
	for _, changedRole := range []bool{false, true} {
		t.Run(fmt.Sprintf("role_changed=%t", changedRole), func(t *testing.T) {
			releaseAck := make(chan struct{})
			defer close(releaseAck)
			received := make(chan *es.Envelope, 1)
			peerErrors := make(chan error, 1)
			epoch := uint64(7)
			if changedRole {
				epoch++
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					peerErrors <- err
					return
				}
				defer conn.Close()
				read := func() (*es.Envelope, error) {
					_, data, err := conn.ReadMessage()
					if err != nil {
						return nil, err
					}
					var event es.Envelope
					err = proto.Unmarshal(data, &event)
					return &event, err
				}
				control, err := read()
				if err != nil {
					peerErrors <- err
					return
				}
				scope := proto.Clone(control.Context).(*es.MembershipContext)
				scope.MembershipId, scope.Airport, scope.Environment, scope.MasterEpoch = "member", "EKCH", "LIVE", epoch
				send := func(message proto.Message, kind es.EventType) error {
					data, err := es.MarshalEnvelope(message, kind)
					if err != nil {
						return err
					}
					var event es.Envelope
					if err := proto.Unmarshal(data, &event); err != nil {
						return err
					}
					event.Context = scope
					data, err = proto.Marshal(&event)
					if err != nil {
						return err
					}
					return conn.WriteMessage(websocket.BinaryMessage, data)
				}
				// An earlier control echo must not acknowledge the new frequency.
				stale := proto.Clone(control.GetControl()).(*es.ControlEvent)
				stale.PrimaryFrequency = "119.500"
				if err := send(stale, es.Control); err != nil {
					peerErrors <- err
					return
				}
				if changedRole {
					if err := send(&es.SessionInfoEvent{Role: es.SessionInfoMaster}, es.SessionInfo); err != nil {
						peerErrors <- err
						return
					}
				}
				<-releaseAck
				if err := send(control.GetControl(), es.Control); err != nil {
					peerErrors <- err
					return
				}
				event, err := read()
				if err != nil {
					peerErrors <- err
					return
				}
				received <- event
			}))
			defer server.Close()
			config := DefaultConfig()
			config.SessionFile = "frequency-election"
			client, err := NewClient(config)
			require.NoError(t, err)
			client.conn, _, err = websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer client.Close()
			client.transportScope = &es.MembershipContext{InstanceId: client.instance, TransportGeneration: 1}
			client.membershipScope = &es.MembershipContext{InstanceId: client.instance, TransportGeneration: 1, MembershipId: "member", Airport: "EKCH", Environment: "LIVE", MasterEpoch: 7}
			client.membershipReady = true
			client.lastControl = &es.ControlEvent{Environment: "LIVE", Callsign: "EKCH_A_TWR", PrimaryFrequency: "119.500", RelevantAirport: "EKCH"}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			client.ReadMessages(ctx, nil)
			joined := make(chan error, 1)
			go func() {
				joined <- client.JoinMembership(ctx, &es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Callsign: "EKCH_A_TWR", Position: "131.040"})
			}()
			messages := 1
			if changedRole {
				messages++
			}
			require.Eventually(t, func() bool { return len(client.GetReceivedMessages()) == messages }, time.Second, time.Millisecond)
			select {
			case err := <-joined:
				t.Fatalf("frequency update completed before its acknowledgement: %v", err)
			default:
			}
			require.ErrorContains(t, client.SendProtobuf(&es.GroundStateEvent{Callsign: "SAS123"}, es.GroundState), "not been acknowledged")
			releaseAck <- struct{}{}
			require.NoError(t, <-joined)
			require.NoError(t, client.SendProtobuf(&es.GroundStateEvent{Callsign: "SAS123", GroundState: es.GroundStateTaxi}, es.GroundState))
			select {
			case event := <-received:
				require.NotNil(t, event.GetGroundState())
				require.Equal(t, epoch, event.Context.MasterEpoch)
			case err := <-peerErrors:
				t.Fatal(err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}

func receivedKind(client *Client, kind string) bool {
	for _, event := range client.GetReceivedMessages() {
		if event.EventType == kind {
			return true
		}
	}
	return false
}
