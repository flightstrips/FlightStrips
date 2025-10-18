package websocket

import (
	"FlightStrips/internal/shared"

	gorilla "github.com/gorilla/websocket"
)

type Hub[TType comparable, TClient Client] interface {
	Unregister(client TClient)
	GetMessageHandlers() shared.MessageHandlers[TType, TClient]

	HandleNewConnection(conn *gorilla.Conn, user shared.AuthenticatedUser) (TClient, error)
}
