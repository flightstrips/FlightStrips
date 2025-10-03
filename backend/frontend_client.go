package main

import (
	"FlightStrips/database"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	WaitingForEuroscopeConnectionSessionId int32 = -1
	WaitingForEuroscopeConnectionPosition        = ""
	WaitingForEuroscopeConnectionAirport         = ""
	WaitingForEuroscopeConnectionCallsign        = ""
)

// FrontendClient represents a frontend websocket client
type FrontendClient struct {
	BaseWebsocketClient
}

// HandlePong handles pong messages from the client
func (c *FrontendClient) HandlePong() error {
	if c.session == WaitingForEuroscopeConnectionSessionId {
		return nil
	}

	// Update the last seen timestamp in the database
	server := c.server
	db := database.New(server.DBPool)
	params := database.SetControllerFrontendSeenParams{
		Cid:              c.user.cid,
		Session:          c.session,
		LastSeenFrontend: pgtype.Timestamp{Valid: true, Time: time.Now().UTC()},
	}
	count, err := db.SetControllerFrontendSeen(context.Background(), params)

	if count != 1 {
		return &ControllerNotFoundError{}
	}
	return err
}

// HandleMessage handles incoming messages from the client
func (c *FrontendClient) HandleMessage(message []byte) error {
	var event Event
	err := json.Unmarshal(message, &event)
	if err != nil {
		return fmt.Errorf("error unmarshalling event: %w", err)
	}

	// Handle the event based on its type
	handler, ok := c.server.FrontendEventHandlers.Handlers[event.Type]
	if !ok {
		return fmt.Errorf("no handler for event type: %s", event.Type)
	}

	return handler(c, message)
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

	db := database.New(server.DBPool)
	controller, err := db.GetControllerByCid(context.Background(), user.cid)

	var session int32
	var position, airport, callsign string
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}

		session = WaitingForEuroscopeConnectionSessionId
		position = WaitingForEuroscopeConnectionPosition
		airport = WaitingForEuroscopeConnectionAirport
		callsign = WaitingForEuroscopeConnectionCallsign
	} else {
		dbSession, err := db.GetSessionById(context.Background(), controller.Session)

		if err != nil {
			// this should not really happen due to the foreign key constraint on the controller table
			return nil, err
		}

		session = dbSession.ID
		position = controller.Position
		airport = dbSession.Airport
		callsign = controller.Callsign
	}

	// Create and return the client
	client := FrontendClient{
		BaseWebsocketClient: BaseWebsocketClient{
			server:   server,
			send:     make(chan []byte, 100),
			conn:     conn,
			callsign: callsign,
			session:  session,
			position: position,
			airport:  airport,
			user:     user,
		},
	}

	return &client, nil
}

// FrontendEventsHandler handles the Frontend events endpoint
func (s *Server) FrontendEventsHandler(w http.ResponseWriter, r *http.Request) {
	handleWebsocketConnection(s, w, r, FrontendClientInitializer, s.FrontendHub)
}

func SendFrontendEvent[T FrontendSendEvent](client *FrontendClient, event T) {
	json, err := json.Marshal(event)
	if err != nil {
		log.Println("Failed to marshal event: ", err)
		return
	}

	client.Send(json)
}
