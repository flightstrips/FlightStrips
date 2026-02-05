package websocket

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"log/slog"
	"net/http"

	gorilla "github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type ConnectionUpgrader[TType comparable, TClient Client] struct {
	authenticationService shared.AuthenticationService
	hub                   Hub[TType, TClient]
	upgrader              gorilla.Upgrader
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

	// authentication
	var authenticationEvent events.AuthenticationEvent
	err = conn.ReadJSON(&authenticationEvent)
	if err != nil {
		span.RecordError(err)
		slog.Debug("Failed to read authentication event", slog.Any("error", err))
		return
	}

	user, err := u.authenticationService.Validate(authenticationEvent.Token)
	if err != nil {
		span.RecordError(err)
		slog.Debug("Failed to validate authentication token", slog.Any("error", err))
		return
	}

	span.SetAttributes(
		attribute.String("user.cid", user.GetCid()),
	)

	client, err := u.hub.HandleNewConnection(conn, user)
	if err != nil {
		span.RecordError(err)
		slog.Warn("Failed to handle new connection", slog.Any("error", err))
		return
	}

	go WritePump(client)
	go ReadPump(u.hub, client)
}
