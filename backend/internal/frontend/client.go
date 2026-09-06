package frontend

import (
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
)

const (
	WaitingForEuroscopeConnectionSessionId int32 = -1
	WaitingForEuroscopeConnectionPosition        = ""
	WaitingForEuroscopeConnectionAirport         = ""
	WaitingForEuroscopeConnectionCallsign        = ""
)

// Client represents a frontend gorilla client
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
	readOnly bool

	// identityMu guards sessionName, airport and callsign. The hub goroutine
	// rewrites them when a client is associated with a session or disconnected
	// from one, while a client's own read goroutine can read them through
	// disconnectSlowConsumer. Reads made on the hub goroutine itself do not need
	// the lock, because they cannot race with the hub's own writes.
	identityMu sync.RWMutex
}

// setIdentity replaces the session identity fields under the lock that
// disconnectSlowConsumer reads them with.
func (c *Client) setIdentity(sessionName, airport, callsign string) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	c.sessionName = sessionName
	c.airport = airport
	c.callsign = callsign
}

// identity reads the session identity fields from a goroutine other than the
// hub's. A string is two words, so an unsynchronised read can otherwise observe
// a new pointer with a stale length.
func (c *Client) identity() (sessionName, airport, callsign string) {
	c.identityMu.RLock()
	defer c.identityMu.RUnlock()
	return c.sessionName, c.airport, c.callsign
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

	sessionName, airport, callsign := c.identity()
	metrics.RecordSlowConsumerDisconnect(context.Background(), sessionName, airport, c.GetSource())
	slog.Warn("Disconnecting slow websocket client",
		slog.String("source", c.GetSource()),
		slog.String("cid", c.GetCid()),
		slog.Int("session", int(c.session)),
		slog.String("callsign", callsign),
		slog.Int("queue_capacity", cap(c.send)),
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
	return "frontend"
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

func (c *Client) SetReadOnly(readOnly bool) {
	c.readOnly = readOnly
}

func (c *Client) CanHandleMessage(messageType string) error {
	if !c.readOnly || messageType == "token" || strings.HasPrefix(messageType, "aman.") {
		return nil
	}

	return errors.New("observer clients are read-only")
}

// HandlePong handles pong messages from the client
func (c *Client) HandlePong() error {
	if c.session == WaitingForEuroscopeConnectionSessionId {
		return nil
	}

	// Update the last seen timestamp in the database
	controllerRepo := c.hub.server.GetControllerRepository()
	now := time.Now().UTC()
	count, err := controllerRepo.SetFrontendSeen(context.Background(), c.GetCid(), c.session, &now)

	if count != 1 {
		//return &ControllerNotFoundError{}
		return errors.New("failed to update last seen timestamp")
	}
	return err
}

// RecordMessage is a no-op for frontend clients (recording is EuroScope-only)
func (c *Client) RecordMessage(rawMessage []byte) {
	// Frontend messages are not recorded
}
