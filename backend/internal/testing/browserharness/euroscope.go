package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	es "FlightStrips/pkg/events/euroscope"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// Connect a real protocol peer, wait for the server-assigned role and send a
// full sync only after master election. Continue draining the socket throughout
// the scenario so application publications exercise a live consumer.
func connectFixtureController(ctx context.Context, endpoint, token string, login *es.LoginEvent, snapshot *es.SyncEvent, onEvent func(es.EventType, []byte, []byte)) (*websocket.Conn, error) {
	conn, response, err := websocket.DefaultDialer.DialContext(ctx, endpoint, nil)
	if response != nil {
		response.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	var writes sync.Mutex
	send := func(message proto.Message, kind es.EventType) error {
		data, err := es.MarshalEnvelope(message, kind)
		if err != nil {
			return err
		}
		writes.Lock()
		defer writes.Unlock()
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}
	if err := send(&es.TokenEvent{Token: token}, es.Authentication); err != nil {
		conn.Close()
		return nil, err
	}
	if err := send(login, es.Login); err != nil {
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
			if err == nil && onEvent != nil {
				onEvent(kind, payload, data)
			}
			if err != nil || kind != es.SessionInfo {
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
