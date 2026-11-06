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
	return connectFixtureControllerProtocol(ctx, endpoint, token, login, snapshot, onEvent, instance, nil, nil)
}

// Optional protocol callbacks let a suite replace the external SDK boundary
// while retaining production authentication, election, dispatch and echo checks.
func connectFixtureControllerProtocol(ctx context.Context, endpoint, token string, login *es.LoginEvent, snapshot *es.SyncEvent, onEvent func(es.EventType, []byte, []byte), instance string, execute func(*es.Envelope) *es.Envelope, expose func(func(proto.Message, es.EventType) error)) (*websocket.Conn, error) {
	conn, response, err := websocket.DefaultDialer.DialContext(ctx, endpoint, nil)
	if response != nil {
		response.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	var writes sync.Mutex
	var scope *es.MembershipContext
	var transportScope *es.MembershipContext
	var scopeMu sync.RWMutex
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
			scopeMu.RLock()
			selected := scope
			if kind == es.Control {
				selected = transportScope
			}
			if selected == nil {
				scopeMu.RUnlock()
				return fmt.Errorf("server-issued context is not ready")
			}
			envelope.Context = proto.Clone(selected).(*es.MembershipContext)
			scopeMu.RUnlock()
			data, err = proto.Marshal(&envelope)
			if err != nil {
				return err
			}
		}
		writes.Lock()
		defer writes.Unlock()
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}
	if err := send(&es.TokenEvent{Token: token, InstanceId: instance, FrequencyMessages: execute != nil}, es.Authentication); err != nil {
		conn.Close()
		return nil, err
	}
	environment := strings.ToUpper(strings.TrimSpace(login.Connection))
	if environment == "" {
		environment = "LIVE"
	}
	if environment == "PLAYBACK" {
		environment = "PLAYBACK_" + instance
	}
	control := &es.ControlEvent{Environment: environment, Callsign: login.Callsign, PrimaryFrequency: login.Position, Observer: login.Observer, LocalIp: login.LocalIp, RelevantAirport: login.Airport, Range: login.Range, TunedFrequenciesKnown: execute != nil, EligibleAirports: []string{login.Airport}}
	readDone := make(chan struct{})
	ready := make(chan error, 1)
	go func() {
		defer close(readDone)
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
				scopeMu.Lock()
				scope = proto.Clone(envelope.Context).(*es.MembershipContext)
				if envelope.Context.MembershipId == "" {
					transportScope = &es.MembershipContext{InstanceId: envelope.Context.InstanceId, TransportGeneration: envelope.Context.TransportGeneration}
				}
				scopeMu.Unlock()
				if kind == es.Control && scope.MembershipId == "" {
					if err := send(control, es.Control); err != nil {
						ready <- err
						return
					}
					continue
				}
			}
			if err == nil && execute != nil {
				if reply := execute(&envelope); reply != nil {
					data, marshalErr := proto.Marshal(reply)
					if marshalErr == nil {
						writes.Lock()
						marshalErr = conn.WriteMessage(websocket.BinaryMessage, data)
						writes.Unlock()
					}
					if marshalErr != nil {
						return
					}
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
		if expose != nil {
			expose(send)
		}
		// Match the native plugin's ten-second control refresh. Coverage evidence
		// expires without it even while an admitted membership remains connected.
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-readDone:
					return
				case <-ticker.C:
					if err := send(control, es.Control); err != nil {
						conn.Close()
						return
					}
				}
			}
		}()
		return conn, nil
	case <-time.After(15 * time.Second):
		conn.Close()
		return nil, fmt.Errorf("controller %s did not acknowledge role", login.Callsign)
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	}
}
