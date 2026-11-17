package websocket

import (
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/shared"
	"log/slog"
	"net/http"
	"time"

	gorilla "github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type ConnectionUpgrader[TType comparable, TClient Client] struct {
	authenticationService shared.AuthenticationService
	hub                   Hub[TType, TClient]
	upgrader              gorilla.Upgrader
	authenticationTimeout time.Duration
}

func NewConnectionUpgrader[TType comparable, TClient Client](hub Hub[TType, TClient], authenticationService shared.AuthenticationService) *ConnectionUpgrader[TType, TClient] {
	return &ConnectionUpgrader[TType, TClient]{
		hub:                   hub,
		authenticationTimeout: 15 * time.Second,
		authenticationService: authenticationService,
		upgrader: gorilla.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // TODO: Implement proper origin checking
			},
		},
	}

}

func (u ConnectionUpgrader[TType, TClient]) Upgrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tracer := otel.Tracer("websocket")
	ctx, span := tracer.Start(ctx, "websocket.upgrade")
	defer span.End()
	diagnostic := newConnectionDiagnostics(r)
	span.SetAttributes(attribute.String("connection.id", diagnostic.id), attribute.String("cf.ray", diagnostic.ray))
	slog.InfoContext(ctx, "Websocket connection opening", "connection_id", diagnostic.id, "cf_ray", diagnostic.ray,
		"peer_ip", diagnostic.peer, "reported_client_ip", diagnostic.reportedIP, "path", r.URL.Path)

	conn, err := u.upgrader.Upgrade(w, r, http.Header{"X-Flightstrips-Connection-Id": []string{diagnostic.id}})
	if err != nil {
		span.RecordError(err)
		slog.Debug("Failed to upgrade connection", "connection_id", diagnostic.id, "cf_ray", diagnostic.ray, "error", err)
		return
	}
	physicalConnections.Store(conn, diagnostic)
	// Upgrade hijacks the HTTP connection. Until both pumps take ownership,
	// every failure must close it so the peer can retry instead of waiting on
	// an open socket that will never receive an initial snapshot.
	handedOff := false
	defer func() {
		if !handedOff {
			physicalConnections.Delete(conn)
			_ = conn.Close()
		}
	}()

	// authentication
	if err := conn.SetReadDeadline(time.Now().Add(u.authenticationTimeout)); err != nil {
		return
	}
	frameType, authenticationMessage, err := conn.ReadMessage()
	if err != nil {
		span.RecordError(err)
		slog.Debug("Failed to read authentication event", append(ConnectionLogAttrs(conn), slog.Any("error", err))...)
		metrics.RecordConnectionEvent(ctx, "other", "authentication_failed")
		return
	}
	authenticationEvent, err := u.hub.DecodeAuthentication(frameType, authenticationMessage)
	if err != nil {
		span.RecordError(err)
		slog.Debug("Failed to decode authentication event", append(ConnectionLogAttrs(conn), "error", err)...)
		metrics.RecordConnectionEvent(ctx, "other", "authentication_failed")
		return
	}

	user, err := u.authenticationService.Validate(authenticationEvent.Token)
	if err != nil {
		span.RecordError(err)
		slog.Debug("Failed to validate authentication token", append(ConnectionLogAttrs(conn), "error", err)...)
		metrics.RecordConnectionEvent(ctx, "other", "authentication_failed")
		return
	}

	span.SetAttributes(
		attribute.String("user.cid", user.GetCid()),
	)

	client, err := u.hub.HandleNewConnection(conn, user, authenticationEvent)
	if err != nil {
		// Admission uses the same typed rejection policy as message handling.
		// Expected rejections are useful diagnostics, not server failures.
		level := min(messageErrorLogLevel(err), slog.LevelWarn)
		message := "Websocket connection rejected"
		if level >= slog.LevelWarn {
			span.RecordError(err)
			message = "Failed to handle new connection"
		}
		slog.Log(ctx, level, message, append(ConnectionLogAttrs(conn), "cid", user.GetCid(), slog.Any("error", err))...)
		metrics.RecordConnectionEvent(ctx, "other", "admission_failed")
		return
	}

	handedOff = true
	slog.InfoContext(ctx, "Websocket connection admitted", append(ConnectionLogAttrs(conn), "cid", client.GetCid(), "source", client.GetSource())...)
	metrics.RecordConnectionEvent(ctx, client.GetSource(), "opened")
	if client.GetSource() == "frontend" {
		client.Enqueue(healthResponse(client, ""))
	}
	go WritePump(client)
	go ReadPump(u.hub, client)
}
