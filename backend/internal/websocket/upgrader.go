package websocket

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"log"
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
		log.Println("Failed to upgrade connection:", err)
		return
	}

	// authentication
	var authenticationEvent events.AuthenticationEvent
	err = conn.ReadJSON(&authenticationEvent)
	if err != nil {
		log.Println("Failed to handle new connection:", err)
		return
	}

	user, err := u.authenticationService.Validate(authenticationEvent.Token)
	if err != nil {
		log.Println("Failed to handle new connection:", err)
		return
	}

	client, err := u.hub.HandleNewConnection(conn, user)
	if err != nil {
		log.Println("Failed to handle new connection:", err)
		return
	}

	go WritePump(client)
	go ReadPump(u.hub, client)
}
