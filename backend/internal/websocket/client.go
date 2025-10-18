package websocket

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/constants"
	"FlightStrips/pkg/events"
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type Client interface {
	Close() error

	GetCid() string
	GetAirport() string
	GetPosition() string
	GetConnection() *websocket.Conn

	IsAuthenticated() bool
	SetUser(user shared.AuthenticatedUser)

	HandlePong() error

	GetSendChannel() chan events.OutgoingMessage
}

// ReadPump pumps messages from the WebSocket connection to the hub.
func ReadPump[TType comparable, TClient Client, THub Hub[TType, TClient]](hub THub, client TClient) {
	log.Println("ReadPump")
	defer func() {
		hub.Unregister(client)
		client.GetConnection().Close()
	}()

	err := client.GetConnection().SetReadDeadline(time.Now().Add(constants.PongWait))
	if err != nil {
		log.Println("Failed to set read deadline:", err)
		return
	}
	client.GetConnection().SetPongHandler(func(string) error {
		err := client.GetConnection().SetReadDeadline(time.Now().Add(constants.PongWait))
		if err != nil {
			log.Println("Failed to set read deadline:", err)
			return err
		}
		return client.HandlePong()
	})

	for {
		_, message, err := client.GetConnection().ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		parsedMessage, err := parseMessage[TType](message)
		if err != nil {
			log.Println("Failed to parse message", err)
			continue
		}

		handlers := hub.GetMessageHandlers()
		err = handlers.Handle(client, parsedMessage)

		if err != nil {
			log.Println("Failed to handle message", err)
		}
	}
}

type internalMessage[TType comparable] struct {
	Type TType `json:"type"`
}

func parseMessage[TType comparable](message []byte) (shared.Message[TType], error) {
	var msg internalMessage[TType]
	err := json.Unmarshal(message, &msg)
	if err != nil {
		return shared.Message[TType]{}, err
	}
	return shared.Message[TType]{
		Type:    msg.Type,
		Message: message,
	}, nil
}

// WritePump pumps messages from the hub to the WebSocket connection.
func WritePump[TClient Client](client TClient) {
	log.Println("WritePump")
	ticker := time.NewTicker(constants.PingPeriod)
	defer func() {
		ticker.Stop()
		client.GetConnection().Close()
	}()

	for {
		select {
		case message, ok := <-client.GetSendChannel():
			err := client.GetConnection().SetWriteDeadline(time.Now().Add(constants.WriteWait))
			if err != nil {
				log.Println("Failed to set write deadline:", err)
				return
			}
			if !ok {
				// The hub closed the channel.
				err := client.GetConnection().WriteMessage(websocket.CloseMessage, []byte{})
				if err != nil {
					log.Println("Failed to close connection:", err)
				}
				return
			}

			bytes, err := message.Marshal()
			if err != nil {
				log.Println("Failed to marshal message:", err)
				continue
			}

			if err := client.GetConnection().WriteMessage(websocket.TextMessage, bytes); err != nil {
				return
			}
		case <-ticker.C:
			if err := client.GetConnection().SetWriteDeadline(time.Now().Add(constants.WriteWait)); err != nil {
				return
			}
			if err := client.GetConnection().WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
