package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	es "FlightStrips/pkg/events/euroscope"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestFixtureControllerUsesIssuedTransportAndMembership(t *testing.T) {
	result := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			result <- err
			return
		}
		defer conn.Close()
		read := func() (*es.Envelope, error) {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return nil, err
			}
			var envelope es.Envelope
			err = proto.Unmarshal(data, &envelope)
			return &envelope, err
		}
		send := func(scope *es.MembershipContext, event es.OutgoingMessage) error {
			data, err := es.NewScopedMessage(scope, event).Marshal()
			if err != nil {
				return err
			}
			return conn.WriteMessage(websocket.BinaryMessage, data)
		}
		token, err := read()
		if err != nil {
			result <- err
			return
		}
		if token.GetToken() == nil || token.GetToken().InstanceId == "" {
			result <- fmt.Errorf("missing plugin instance ID")
			return
		}
		transport := &es.MembershipContext{InstanceId: token.GetToken().InstanceId, TransportGeneration: 123}
		if err = send(transport, es.ControlEvent{}); err != nil {
			result <- err
			return
		}
		control, err := read()
		if err != nil {
			result <- err
			return
		}
		if !proto.Equal(transport, control.Context) || control.GetControl() == nil || control.GetControl().RelevantAirport != "EKCH" {
			result <- fmt.Errorf("invalid physical control evidence")
			return
		}
		member := &es.MembershipContext{InstanceId: transport.InstanceId, TransportGeneration: 123, MembershipId: "issued-member", SessionId: 42, Airport: "EKCH", Environment: "LIVE", Generation: 9, MasterEpoch: 17}
		if err = send(member, es.SessionInfoEvent{Role: es.SessionInfoMaster}); err != nil {
			result <- err
			return
		}
		snapshot, err := read()
		if err != nil {
			result <- err
			return
		}
		if snapshot.GetSync() == nil || !proto.Equal(member, snapshot.Context) {
			result <- fmt.Errorf("snapshot did not retain assigned membership and master epoch")
			return
		}
		result <- nil
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	conn, err := connectFixtureController(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), "fixture-token", &es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Position: "118.105", Callsign: "EKCH_A_TWR"}, &es.SyncEvent{}, nil)
	require.NoError(t, err)
	defer conn.Close()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestFixtureControllerRefreshesTransportControlAfterMembershipAdmission(t *testing.T) {
	result := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			result <- err
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(13 * time.Second))
		read := func() (*es.Envelope, error) {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return nil, err
			}
			var e es.Envelope
			err = proto.Unmarshal(data, &e)
			return &e, err
		}
		send := func(scope *es.MembershipContext, event es.OutgoingMessage) error {
			data, err := es.NewScopedMessage(scope, event).Marshal()
			if err != nil {
				return err
			}
			return conn.WriteMessage(websocket.BinaryMessage, data)
		}
		token, err := read()
		if err != nil {
			result <- err
			return
		}
		transport := &es.MembershipContext{InstanceId: token.GetToken().InstanceId, TransportGeneration: 123}
		if err = send(transport, es.ControlEvent{}); err != nil {
			result <- err
			return
		}
		first, err := read()
		if err != nil {
			result <- err
			return
		}
		if first.GetControl() == nil || !proto.Equal(transport, first.Context) {
			result <- fmt.Errorf("initial control is not transport scoped")
			return
		}
		member := &es.MembershipContext{InstanceId: transport.InstanceId, TransportGeneration: 123, MembershipId: "issued-member", SessionId: 42, Airport: "EKCH", Environment: "LIVE", Generation: 9, MasterEpoch: 17}
		if err = send(member, es.SessionInfoEvent{Role: es.SessionInfoSlave}); err != nil {
			result <- err
			return
		}
		heartbeat, err := read()
		if err != nil {
			result <- err
			return
		}
		if !proto.Equal(transport, heartbeat.Context) || !proto.Equal(first.GetControl(), heartbeat.GetControl()) {
			result <- fmt.Errorf("control refresh changed physical evidence or used membership context: %v", heartbeat)
			return
		}
		result <- nil
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	conn, err := connectFixtureController(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), "fixture-token", &es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Position: "118.105", Callsign: "EKCH_A_TWR"}, nil, nil)
	require.NoError(t, err)
	defer conn.Close()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
