package websocket

import (
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
	requiredSubprotocol   string
}

// RequireSubprotocol enforces a wire protocol before the WebSocket upgrade.
func (u *ConnectionUpgrader[TType, TClient]) RequireSubprotocol(protocol string) {
	u.requiredSubprotocol = protocol
	u.upgrader.Subprotocols = []string{protocol}
}

func NewConnectionUpgrader[TType comparable, TClient Client](hub Hub[TType, TClient], authenticationService shared.AuthenticationService) *ConnectionUpgrader[TType, TClient] {
	return &ConnectionUpgrader[TType, TClient]{
		hub:                   hub,
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
	if u.requiredSubprotocol != "" && !gorilla.IsWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}
	if u.requiredSubprotocol != "" {
		found := false
		for _, offered := range gorilla.Subprotocols(r) {
			if offered == u.requiredSubprotocol {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "unsupported websocket subprotocol", http.StatusForbidden)
			return
		}
	}
	ctx := r.Context()
	tracer := otel.Tracer("websocket")
	ctx, span := tracer.Start(ctx, "websocket.upgrade")
	defer span.End()

	conn, err := u.upgrader.Upgrade(w, r, nil)
	if err != nil {
		span.RecordError(err)
		slog.Debug("Failed to upgrade connection", slog.Any("error", err))
		return
	}
	if u.requiredSubprotocol != "" {
		conn.SetReadLimit(4 << 20)
	}

	// authentication
	frameType, authenticationMessage, err := conn.ReadMessage()
	if err != nil {
		_ = conn.WriteControl(gorilla.CloseMessage, gorilla.FormatCloseMessage(gorilla.ClosePolicyViolation, "authentication failed"), time.Now().Add(time.Second))
		_ = conn.Close()
		span.RecordError(err)
		slog.Debug("Failed to read authentication event", slog.Any("error", err))
		return
	}
	authenticationEvent, err := u.hub.DecodeAuthentication(frameType, authenticationMessage)
	if err != nil {
		closeCode := gorilla.ClosePolicyViolation
		if u.requiredSubprotocol != "" && frameType != gorilla.BinaryMessage {
			closeCode = gorilla.CloseUnsupportedData
		}
		_ = conn.WriteControl(gorilla.CloseMessage, gorilla.FormatCloseMessage(closeCode, "authentication failed"), time.Now().Add(time.Second))
		_ = conn.Close()
		span.RecordError(err)
		slog.Debug("Failed to decode authentication event", slog.Any("error", err))
		return
	}

	user, err := u.authenticationService.Validate(authenticationEvent.Token)
	if err != nil {
		_ = conn.WriteControl(gorilla.CloseMessage, gorilla.FormatCloseMessage(gorilla.ClosePolicyViolation, "authentication failed"), time.Now().Add(time.Second))
		_ = conn.Close()
		span.RecordError(err)
		slog.Debug("Failed to validate authentication token", slog.Any("error", err))
		return
	}

	span.SetAttributes(
		attribute.String("user.cid", user.GetCid()),
	)

	client, err := u.hub.HandleNewConnection(conn, user, authenticationEvent)
	if err != nil {
		_ = conn.Close()
		span.RecordError(err)
		slog.Warn("Failed to handle new connection", slog.Any("error", err))
		return
	}

	go WritePump(client)
	go ReadPump(u.hub, client)
}
