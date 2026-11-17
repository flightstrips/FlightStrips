package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"FlightStrips/internal/metrics"
	"github.com/google/uuid"
	gorilla "github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/attribute"
)

// Physical connection diagnostics are deliberately separate from mutable airport
// membership and never participate in command authorization.
type connectionDiagnostics struct {
	id, ray, peer, reportedIP string
	started                   time.Time
	lastPong                  atomic.Int64
	mu                        sync.Mutex
	clientStage               string
}

var physicalConnections sync.Map // *gorilla.Conn -> *connectionDiagnostics

func newConnectionDiagnostics(r *http.Request) *connectionDiagnostics {
	peer, _, _ := net.SplitHostPort(r.RemoteAddr)
	d := &connectionDiagnostics{id: uuid.NewString(), started: time.Now(), peer: peer,
		ray: boundedDiagnostic(r.Header.Get("CF-Ray")), reportedIP: boundedDiagnostic(r.Header.Get("CF-Connecting-IP"))}
	d.lastPong.Store(d.started.UnixNano())
	return d
}

func boundedDiagnostic(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return -1
		}
		return r
	}, value)
	if len(value) > 128 {
		return value[:128]
	}
	return value
}

func diagnostics(conn *gorilla.Conn) *connectionDiagnostics {
	if d, ok := physicalConnections.Load(conn); ok {
		return d.(*connectionDiagnostics)
	}
	return nil
}

func ConnectionLogAttrs(conn *gorilla.Conn) []any {
	if d := diagnostics(conn); d != nil {
		return append(d.identityLogAttrs(), "connection_age_ms", time.Since(d.started).Milliseconds(),
			"last_pong_age_ms", time.Since(time.Unix(0, d.lastPong.Load())).Milliseconds())
	}
	return nil
}

func (d *connectionDiagnostics) identityLogAttrs() []any {
	return []any{"connection_id", d.id, "cf_ray", d.ray, "peer_ip", d.peer, "reported_client_ip", d.reportedIP}
}

// ConnectionIdentityLogAttrs can be retained by a logical membership after the
// physical reader unregisters. It deliberately omits point-in-time ages.
func ConnectionIdentityLogAttrs(conn *gorilla.Conn) []any {
	if d := diagnostics(conn); d != nil {
		return d.identityLogAttrs()
	}
	return nil
}

func connectionSpanAttributes(conn *gorilla.Conn) []attribute.KeyValue {
	if d := diagnostics(conn); d != nil {
		return []attribute.KeyValue{attribute.String("connection.id", d.id), attribute.String("cf.ray", d.ray)}
	}
	return nil
}

type connectionHealth struct {
	Type             string `json:"type"`
	Protocol         int    `json:"protocol"`
	ConnectionID     string `json:"connection_id"`
	CFRay            string `json:"cf_ray,omitempty"`
	Sequence         string `json:"sequence,omitempty"`
	WaitingForPlugin bool   `json:"waiting_for_plugin"`
}

func (h connectionHealth) Marshal() ([]byte, error) { return json.Marshal(h) }

func healthResponse(client Client, sequence string) connectionHealth {
	h := connectionHealth{Type: "connection_health", Protocol: 1, Sequence: sequence, WaitingForPlugin: client.GetSession() < 0}
	if d := diagnostics(client.GetConnection()); d != nil {
		h.ConnectionID, h.CFRay = d.id, d.ray
	}
	return h
}

func handleConnectionHealth(client Client, payload []byte) (bool, error) {
	if !client.IsAuthenticated() {
		return false, errors.New("connection health authentication expired")
	}
	var request struct {
		Sequence        string `json:"sequence"`
		Stage           string `json:"stage"`
		PreviousFailure string `json:"previous_failure"`
		RequestSnapshot bool   `json:"request_snapshot"`
	}
	if len(payload) > 2048 {
		return false, errors.New("connection health payload too large")
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		return false, err
	}
	if len(request.Sequence) == 0 || len(request.Sequence) > 64 {
		return false, errors.New("invalid connection health sequence")
	}
	stage := boundedRecoveryValue(request.Stage, "authenticating", "synchronizing", "ready", "waiting_for_plugin")
	if d := diagnostics(client.GetConnection()); d != nil {
		d.mu.Lock()
		changed := d.clientStage != stage
		d.clientStage = stage
		d.mu.Unlock()
		if changed {
			attrs := append(ConnectionLogAttrs(client.GetConnection()), "cid", client.GetCid(), "source", client.GetSource(), "stage", stage,
				"previous_failure", boundedRecoveryValue(request.PreviousFailure, "connect_timeout", "snapshot_timeout", "heartbeat_timeout", "socket_closed", "setup_failed"))
			slog.Info("Websocket client recovery stage", attrs...)
			metrics.RecordConnectionEvent(context.Background(), client.GetSource(), "client_stage_"+stage)
		}
	}
	if !client.Enqueue(healthResponse(client, request.Sequence)) {
		return false, errors.New("connection health response queue unavailable")
	}
	return request.RequestSnapshot, nil
}

func boundedRecoveryValue(value string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return "other"
}
