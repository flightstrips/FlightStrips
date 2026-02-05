package websocket

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"log/slog"
	"net/http"

	gorilla "github.com/gorilla/websocket"
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
	conn, err := u.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Debug("Failed to upgrade connection", slog.Any("error", err))
		return
	}

	// authentication
	var authenticationEvent events.AuthenticationEvent
	err = conn.ReadJSON(&authenticationEvent)
	if err != nil {
		slog.Debug("Failed to read authentication event", slog.Any("error", err))
		return
	}

	user, err := u.authenticationService.Validate(authenticationEvent.Token)
	if err != nil {
		slog.Debug("Failed to validate authentication token", slog.Any("error", err))
		return
	}

	client, err := u.hub.HandleNewConnection(conn, user)
	if err != nil {
		slog.Warn("Failed to handle new connection", slog.Any("error", err))
		return
	}

	go WritePump(client)
	go ReadPump(u.hub, client)
}
