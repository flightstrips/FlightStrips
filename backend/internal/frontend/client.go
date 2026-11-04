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
	frequencyRequests sync.Map
	conn              *gorilla.Conn
	session           int32
	sessionName       string
	send              chan events.OutgoingMessage
	closeOnce         sync.Once
	closed            chan struct{}
	hub               *Hub
	userMu            sync.RWMutex
	user              shared.AuthenticatedUser

	position       string
	callsign       string
	airport        string
	version        string
	readOnly       bool
	initialPending bool // owned by the hub loop; retry until the snapshot is queued

	// The hub owns identity transitions. Socket readers and writers use a
	// snapshot so association/disconnect cannot expose a partial session identity.
	identityMu sync.RWMutex
}

type clientIdentity struct {
	session                                  int32
	sessionName, position, airport, callsign string
	readOnly                                 bool
}

type messageIdentityKey struct{}
type messageClientIdentity struct {
	client   *Client
	identity clientIdentity
}

// Nested action handlers retain the identity captured before command authority
// resolution. Direct use-case tests without transport context use current state.
func (c *Client) messageIdentity(ctx context.Context) clientIdentity {
	if captured, ok := ctx.Value(messageIdentityKey{}).(messageClientIdentity); ok && captured.client == c {
		return captured.identity
	}
	return c.identitySnapshot()
}

func (c *Client) identitySnapshot() clientIdentity {
	if c == nil {
		return clientIdentity{}
	}
	c.identityMu.RLock()
	defer c.identityMu.RUnlock()
	return clientIdentity{session: c.session, sessionName: c.sessionName, position: c.position, airport: c.airport, callsign: c.callsign, readOnly: c.readOnly}
}

// setIdentity publishes an association or disconnect as one identity transition.
func (c *Client) setIdentity(session int32, sessionName, position, airport, callsign string, readOnly bool) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	c.sessionName = sessionName
	c.session = session
	c.position = position
	c.readOnly = readOnly
	c.airport = airport
	c.callsign = callsign
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

	identity := c.identitySnapshot()
	metrics.RecordSlowConsumerDisconnect(context.Background(), identity.sessionName, identity.airport, c.GetSource())
	slog.Warn("Disconnecting slow websocket client",
		slog.String("source", c.GetSource()),
		slog.String("cid", c.GetCid()),
		slog.Int("session", int(identity.session)),
		slog.String("callsign", identity.callsign),
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
	c.userMu.RLock()
	defer c.userMu.RUnlock()
	return c.user.GetCid()
}

func (c *Client) GetCallsign() string {
	return c.identitySnapshot().callsign
}

func (c *Client) GetAirport() string {
	return c.identitySnapshot().airport
}

func (c *Client) GetPosition() string {
	return c.identitySnapshot().position
}

func (c *Client) GetSession() int32 {
	return c.identitySnapshot().session
}

func (c *Client) GetSessionName() string {
	return c.identitySnapshot().sessionName
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
	c.userMu.RLock()
	defer c.userMu.RUnlock()
	return c.user.IsValid()
}

func (c *Client) SetUser(user shared.AuthenticatedUser) {
	c.userMu.Lock()
	defer c.userMu.Unlock()
	c.user = user
}

func (c *Client) SetReadOnly(readOnly bool) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	c.readOnly = readOnly
}

func (c *Client) CanHandleMessage(messageType string) error {
	if !c.identitySnapshot().readOnly || messageType == "token" || strings.HasPrefix(messageType, "aman.") {
		return nil
	}

	return shared.RejectAction("observer clients are read-only")
}

// HandlePong handles pong messages from the client
func (c *Client) HandlePong() error {
	identity := c.identitySnapshot()
	if identity.session == WaitingForEuroscopeConnectionSessionId {
		return nil
	}

	// Update the last seen timestamp in the database
	controllerRepo := c.hub.server.GetControllerRepository()
	now := time.Now().UTC()
	count, err := controllerRepo.SetFrontendSeen(context.Background(), c.GetCid(), identity.session, &now)

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

func (c *Client) PrepareMessageContext(ctx context.Context, message string) (context.Context, error) {
	identity := c.messageIdentity(ctx)
	if captured, ok := ctx.Value(messageIdentityKey{}).(messageClientIdentity); ok {
		if captured.client != c {
			return ctx, shared.RejectAction("cannot replace initiating frontend identity")
		}
	} else {
		ctx = context.WithValue(ctx, messageIdentityKey{}, messageClientIdentity{client: c, identity: identity})
	}
	if c == nil || message == "token" || (identity.readOnly && strings.HasPrefix(message, "aman.")) {
		return ctx, nil
	}
	if c.hub == nil || c.hub.server == nil {
		return ctx, shared.RejectAction("own plugin is unavailable")
	}
	if identity.readOnly {
		return ctx, shared.RejectAction("observer clients are read-only")
	}
	return shared.EnsureHumanCommand(ctx, c.hub.server.GetEuroscopeHub(), identity.session, c.GetCid())
}
