package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"
)

// EuroscopeClient represents a euroscope websocket client
type EuroscopeClient struct {
	BaseWebsocketClient
}

// EuroscopeClientInitializer creates a new Euroscope client
func EuroscopeClientInitializer(server *Server, conn *websocket.Conn) (*EuroscopeClient, error) {
	// Read the authentication message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read authentication message: %w", err)
	}

	// Authenticate the user
	user, err := server.euroscopeeventhandlerAuthentication(msg)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Read the login message
	_, msg, err = conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read login message: %w", err)
	}

	// Handle the login
	event, sessionID, err := server.euroscopeeventhandlerLogin(msg)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	// Create and return the client
	client := EuroscopeClient{BaseWebsocketClient{
		server:  server,
		send:    make(chan []byte, 100),
		conn:    conn,
		session: sessionID,
		user:    user,

		position: event.Position,
		airport:  event.Airport,
	}}

	// TODO move!
	client.send <- []byte("{\"type\": \"session_info\", \"role\": \"master\"}")

	return &client, nil
}

// EuroscopeEventsHandler handles the Euroscope events endpoint
func (s *Server) EuroscopeEventsHandler(w http.ResponseWriter, r *http.Request) {
	handleWebsocketConnection(s, w, r, EuroscopeClientInitializer, s.EuroscopeHub)
}

// HandlePong handles pong messages from the client
func (c *EuroscopeClient) HandlePong() error {
	// Update the last seen timestamp in the database
	db := data.New(c.server.DBPool)
	params := data.SetControllerEuroscopeSeenParams{
		Callsign:          c.user.cid,
		Session:           c.session,
		LastSeenEuroscope: pgtype.Timestamp{Valid: true, Time: time.Now().UTC()},
	}
	_, err := db.SetControllerEuroscopeSeen(context.Background(), params)
	return err
}

// HandleMessage handles incoming messages from the client
func (c *EuroscopeClient) HandleMessage(message []byte) error {
	var event EuroscopeEvent
	err := json.Unmarshal(message, &event)
	if err != nil {
		return fmt.Errorf("error unmarshalling event: %w", err)
	}

	// Handle the event based on its type
	return c.server.euroscopeEventsHandler(c, event, message)
}
