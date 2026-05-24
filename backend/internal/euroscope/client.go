package euroscope

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
)

type Client struct {
	conn        *gorilla.Conn
	session     int32
	sessionName string
	send        chan events.OutgoingMessage
	closeOnce   sync.Once
	closed      chan struct{}
	hub         *Hub
	user        shared.AuthenticatedUser

	position string
	callsign string
	airport  string
	version  string
	observer bool
	localIP  string
}

func (c *Client) GetSendChannel() chan events.OutgoingMessage {
	return c.send
}

func (c *Client) Enqueue(message events.OutgoingMessage) bool {
	select {
	case <-c.closed:
		return false
	case c.send <- message:
		return true
	default:
		c.disconnectSlowConsumer()
		return false
	}
}

func (c *Client) disconnectSlowConsumer() {
	shouldUnregister := false
	c.closeOnce.Do(func() {
		close(c.closed)
		shouldUnregister = true
	})
	if !shouldUnregister {
		return
	}

	slog.Warn("Disconnecting slow websocket client",
		slog.String("source", c.GetSource()),
		slog.String("cid", c.GetCid()),
		slog.Int("session", int(c.session)),
	)

	if c.hub != nil {
		go c.hub.Unregister(c)
		return
	}

	_ = c.Close()
}

func (c *Client) Close() error {
	if c.closed != nil {
		c.closeOnce.Do(func() {
			close(c.closed)
		})
	}
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) GetCid() string {
	return c.user.GetCid()
}

func (c *Client) GetCallsign() string {
	return c.callsign
}

func (c *Client) GetAirport() string {
	return c.airport
}

func (c *Client) GetPosition() string {
	return c.position
}

func (c *Client) GetSession() int32 {
	return c.session
}

func (c *Client) GetSessionName() string {
	return c.sessionName
}

func (c *Client) GetSource() string {
	return "euroscope"
}

func (c *Client) GetVersion() string {
	return c.version
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

func (c *Client) CanHandleMessage(messageType string) error {
	if !c.observer || messageType == "token" || messageType == "login" || messageType == "runway" {
		return nil
	}

	return errors.New("observer Euroscope clients cannot publish operational data")
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
