package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

const (
	WaitingForEuroscopeConnectionSessionId int32 = -1
	WaitingForEuroscopeConnectionPosition = ""
	WaitingForEuroscopeConnectionAirport = ""
)

// FrontendClient represents a frontend websocket client
type FrontendClient struct {
	BaseWebsocketClient
}

// HandlePong handles pong messages from the client
func (c *FrontendClient) HandlePong() error {
	// Update the last seen timestamp in the database
	/*
		server := c.hub.server
		db := data.New(server.DBPool)
		params := data.SetControllerFrontendSeenParams{
			Cid:              c.user.CID,
			LastSeenFrontend: pgtype.Timestamp{Valid: true, Time: time.Now().UTC()},
		}
		_, err := db.SetControllerFrontendSeen(context.Background(), params)
		return err
	*/

	return nil
}

// HandleMessage handles incoming messages from the client
func (c *FrontendClient) HandleMessage(message []byte) error {
	var event Event
	err := json.Unmarshal(message, &event)
	if err != nil {
		return fmt.Errorf("error unmarshalling event: %w", err)
	}

	// Handle the event based on its type
	_, err = c.server.frontEndEventHandler(c, event)
	return err
}

// FrontendClientInitializer creates a new Frontend client
func FrontendClientInitializer(server *Server, conn *websocket.Conn) (*FrontendClient, error) {
	// Read the authentication message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read authentication message: %w", err)
	}

	// Authenticate the user
	user, err := server.eventhandlerAuthentication(msg)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Create and return the client
	client := FrontendClient{
		BaseWebsocketClient: BaseWebsocketClient{
			server:   server,
			send:     make(chan []byte, 100),
			conn:     conn,
			session:  WaitingForEuroscopeConnectionSessionId,
			position: WaitingForEuroscopeConnectionPosition,
			airport:  WaitingForEuroscopeConnectionAirport,
			user:     user,
		},
	}

	return &client, nil
}

// FrontendEventsHandler handles the Frontend events endpoint
func (s *Server) FrontendEventsHandler(w http.ResponseWriter, r *http.Request) {
	handleWebsocketConnection(s, w, r, FrontendClientInitializer, s.FrontendHub)
}
