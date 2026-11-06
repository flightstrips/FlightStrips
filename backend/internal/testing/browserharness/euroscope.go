package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	es "FlightStrips/pkg/events/euroscope"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// Connect a real protocol peer, wait for the server-assigned role and send a
// full sync only after master election. Continue draining the socket throughout
// the scenario so application publications exercise a live consumer.
func connectFixtureController(ctx context.Context, endpoint, token string, login *es.LoginEvent, snapshot *es.SyncEvent, onEvent func(es.EventType, []byte, []byte)) (*websocket.Conn, error) {
	return connectFixtureControllerInstance(ctx, endpoint, token, login, snapshot, onEvent, uuid.NewString())
}
func connectFixtureControllerInstance(ctx context.Context, endpoint, token string, login *es.LoginEvent, snapshot *es.SyncEvent, onEvent func(es.EventType, []byte, []byte), instance string) (*websocket.Conn, error) {
	conn, response, err := websocket.DefaultDialer.DialContext(ctx, endpoint, nil)
	if response != nil {
		response.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	var writes sync.Mutex
	var scope *es.MembershipContext
	send := func(message proto.Message, kind es.EventType) error {
		data, err := es.MarshalEnvelope(message, kind)
		if err != nil {
			return err
		}
		if kind != es.Authentication {
			var envelope es.Envelope
			if err := proto.Unmarshal(data, &envelope); err != nil {
				return err
			}
			if scope == nil {
				return fmt.Errorf("server-issued context is not ready")
			}
			envelope.Context = proto.Clone(scope).(*es.MembershipContext)
			data, err = proto.Marshal(&envelope)
			if err != nil {
				return err
			}
		}
		writes.Lock()
		defer writes.Unlock()
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}
	if err := send(&es.TokenEvent{Token: token, InstanceId: instance}, es.Authentication); err != nil {
		conn.Close()
		return nil, err
	}
	ready := make(chan error, 1)
	go func() {
		acknowledged := false
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				if !acknowledged {
					ready <- err
				}
				return
			}
			kind, payload, err := es.UnmarshalEnvelope(data)
			var envelope es.Envelope
			if err == nil {
				err = proto.Unmarshal(data, &envelope)
			}
			if err == nil && envelope.Context != nil && envelope.Context.InstanceId == instance {
				scope = proto.Clone(envelope.Context).(*es.MembershipContext)
				if kind == es.Control && scope.MembershipId == "" {
					environment := strings.ToUpper(strings.TrimSpace(login.Connection))
					if environment == "" {
						environment = "LIVE"
					}
					if environment == "PLAYBACK" {
						environment = "PLAYBACK_" + instance
					}
					if err := send(&es.ControlEvent{Environment: environment, Callsign: login.Callsign, PrimaryFrequency: login.Position, Observer: login.Observer, LocalIp: login.LocalIp, RelevantAirport: login.Airport, Range: login.Range}, es.Control); err != nil {
						ready <- err
						return
					}
					continue
				}
			}
			if err == nil && onEvent != nil {
				onEvent(kind, payload, data)
			}
			if err != nil || kind != es.SessionInfo || scope == nil || scope.MembershipId == "" || scope.Airport != login.Airport {
				continue
			}
			info := &es.SessionInfoEvent{}
			if proto.Unmarshal(payload, info) != nil {
				continue
			}
			if login.Observer && info.Role != es.SessionInfoObserver {
				continue
			}
			if snapshot != nil && info.Role != es.SessionInfoMaster {
				continue
			}
			if !acknowledged {
				if snapshot != nil {
					err = send(snapshot, es.Sync)
				}
				ready <- err
				acknowledged = true
			}
		}
	}()
	select {
	case err := <-ready:
		if err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	case <-time.After(15 * time.Second):
		conn.Close()
		return nil, fmt.Errorf("controller %s did not acknowledge role", login.Callsign)
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	}
}
