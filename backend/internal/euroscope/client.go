package euroscope

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"context"
	"errors"
	"time"

	gorilla "github.com/gorilla/websocket"
)

type Client struct {
	conn    *gorilla.Conn
	session int32
	send    chan events.OutgoingMessage
	hub     *Hub
	user    shared.AuthenticatedUser

	position string
	callsign string
	airport  string
}

func (c *Client) GetSendChannel() chan events.OutgoingMessage {
	return c.send
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) GetCid() string {
	return c.user.GetCid()
}

func (c *Client) GetAirport() string {
	return c.airport
}

func (c *Client) GetPosition() string {
	return c.position
}

func (c *Client) GetConnection() *gorilla.Conn {
	return c.conn
}

func (c *Client) IsAuthenticated() bool {
	return c.user.IsValid()
}

func (c *Client) SetUser(user shared.AuthenticatedUser) {
	c.user = user
}

// HandlePong handles pong messages from the client
func (c *Client) HandlePong() error {
	// Update the last seen timestamp in the database
	controllerRepo := c.hub.server.GetControllerRepository()
	now := time.Now().UTC()
	count, err := controllerRepo.SetEuroscopeSeen(context.Background(), c.GetCid(), c.session, &now)

	if count != 1 {
		return errors.New("failed to update last seen timestamp")
	}
	return err
}

// RecordMessage records an incoming message if recording is enabled
func (c *Client) RecordMessage(rawMessage []byte) {
	c.hub.recordMessage(c.session, rawMessage)
}
