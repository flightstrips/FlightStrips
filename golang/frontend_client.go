package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
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
	// Read the initial connection message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read initial connection message: %w", err)
	}

	// Handle the initial connection
	_, airport, position, err := server.handleInitialConnectionEvent(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to handle initial connection: %w", err)
	}

	// Return the initial connection response
	err = server.returnInitialConnectionResponseEvent(conn, airport)
	if err != nil {
		return nil, fmt.Errorf("failed to return initial connection response: %w", err)
	}

	// Create and return the client
	client := FrontendClient{
		BaseWebsocketClient{
			server:   server,
			send:     make(chan []byte, 100),
			conn:     conn,
			session:  1, // TODO
			position: position,
			airport:  airport,
			user:     nil, // TODO
		},
	}

	return &client, nil
}

// FrontendEventsHandler handles the Frontend events endpoint
func (s *Server) FrontendEventsHandler(w http.ResponseWriter, r *http.Request) {
	handleWebsocketConnection(s, w, r, FrontendClientInitializer, s.FrontendHub)
}
