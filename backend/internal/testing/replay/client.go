package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/testing/recorder"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// ReceivedMessage represents a message received from the server
type ReceivedMessage struct {
	MessageType int
	Data        []byte
	ReceivedAt  time.Time
	EventType   string // Parsed from the protobuf envelope
}

// Client simulates an EuroScope WebSocket client for replay
type Client struct {
	conn             *websocket.Conn
	config           Config
	done             chan struct{}
	receivedMessages []ReceivedMessage
	mu               sync.Mutex
	writes           sync.Mutex
	instance         string
	transportScope   *euroscopeEvents.MembershipContext
	membershipScope  *euroscopeEvents.MembershipContext
	membershipReady  bool
	lastControl      *euroscopeEvents.ControlEvent
	scopeChanged     chan struct{}
}

// NewClient creates a new replay client
func NewClient(config Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &Client{
		config:           config,
		done:             make(chan struct{}),
		receivedMessages: []ReceivedMessage{},
		instance:         uuid.NewString(),
		scopeChanged:     make(chan struct{}, 1),
	}, nil
}

// Connect establishes a WebSocket connection to the server
func (c *Client) Connect(ctx context.Context) error {
	if c.config.Verbose {
		slog.Info("Connecting to server", slog.String("url", c.config.ServerURL))
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.DialContext(ctx, c.config.ServerURL, http.Header{})
	if err != nil {
		return &ErrConnectionFailed{URL: c.config.ServerURL, Err: err}
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()

	c.conn = conn

	// Send authentication token immediately after connection
	if err := c.SendProtobuf(&euroscopeEvents.TokenEvent{Token: "__TEST_TOKEN__", InstanceId: c.instance}, euroscopeEvents.Authentication); err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	// The physical transport is established before any operational membership.
	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		conn.Close()
		return err
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return fmt.Errorf("transport handshake: %w", err)
	}
	var envelope euroscopeEvents.Envelope
	if err := proto.Unmarshal(data, &envelope); err != nil {
		conn.Close()
		return err
	}
	if envelope.GetControl() == nil || envelope.Context == nil || envelope.Context.InstanceId != c.instance || envelope.Context.TransportGeneration == 0 || envelope.Context.MembershipId != "" {
		conn.Close()
		return fmt.Errorf("invalid transport handshake")
	}
	c.transportScope = proto.Clone(envelope.Context).(*euroscopeEvents.MembershipContext)
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		conn.Close()
		return err
	}

	if c.config.Verbose {
		slog.Info("Connected and authenticated successfully")
	}

	return nil
}

// SendEvent sends a single event to the server
func (c *Client) SendEvent(event recorder.RecordedEvent) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	if c.config.Verbose {
		slog.Debug("Sending event",
			slog.Int("index", event.Index),
			slog.String("type", event.Type),
			slog.Int64("timestamp_ms", event.TimestampMs))
	}

	var data []byte
	if err := json.Unmarshal(event.Payload, &data); err != nil {
		return fmt.Errorf("recorded event is not a protobuf envelope: %w", err)
	}
	var envelope euroscopeEvents.Envelope
	if err := proto.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("invalid recorded envelope: %w", err)
	}
	// Historical recordings contain their original establishment messages and
	// source incarnations. Translate those fixtures, never replay credentials or
	// ask the runtime to accept the removed single-session protocol.
	if envelope.GetToken() != nil {
		return nil
	}
	if login := envelope.GetLogin(); login != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return c.JoinMembership(ctx, login)
	}
	return c.sendEnvelope(&envelope)
}

// JoinMembership supplies physical evidence and waits for the server's scoped
// role before replaying a snapshot. It uses no client-selected session IDs.
func (c *Client) JoinMembership(ctx context.Context, login *euroscopeEvents.LoginEvent) error {
	environment := strings.ToUpper(strings.TrimSpace(login.Connection))
	if environment == "" {
		environment = "LIVE"
	}
	if environment == "PLAYBACK" {
		environment = "PLAYBACK_" + c.instance
	}
	control := &euroscopeEvents.ControlEvent{Environment: environment, Callsign: login.Callsign, PrimaryFrequency: login.Position, Observer: login.Observer, LocalIp: login.LocalIp, RelevantAirport: login.Airport, Range: login.Range}
	c.mu.Lock()
	if !proto.Equal(c.lastControl, control) {
		c.membershipScope = nil
		c.membershipReady = false
	}
	c.lastControl = proto.Clone(control).(*euroscopeEvents.ControlEvent)
	c.mu.Unlock()
	if err := c.SendProtobuf(control, euroscopeEvents.Control); err != nil {
		return err
	}
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for {
		c.mu.Lock()
		ready := c.membershipReady && c.membershipScope != nil && c.membershipScope.Airport == login.Airport && c.membershipScope.Environment == environment
		c.mu.Unlock()
		if ready {
			return nil
		}
		select {
		case <-c.scopeChanged:
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("membership %s/%s did not receive a role", login.Airport, environment)
		}
	}
}

// SendProtobuf wraps and sends one binary protobuf event.
func (c *Client) SendProtobuf(message proto.Message, eventType euroscopeEvents.EventType) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := euroscopeEvents.MarshalEnvelope(message, eventType)
	if err != nil {
		return fmt.Errorf("failed to marshal protobuf message: %w", err)
	}

	var envelope euroscopeEvents.Envelope
	if err := proto.Unmarshal(data, &envelope); err != nil {
		return err
	}
	return c.sendEnvelope(&envelope)
}

func (c *Client) sendEnvelope(envelope *euroscopeEvents.Envelope) error {
	c.mu.Lock()
	if envelope.GetToken() == nil {
		scope := c.membershipScope
		if envelope.GetControl() != nil {
			scope = c.transportScope
		}
		if scope == nil {
			c.mu.Unlock()
			return fmt.Errorf("server-issued membership context is not ready")
		}
		if recorded := envelope.Context; recorded != nil && recorded.MembershipId != "" && recorded.Airport != "" && recorded.Airport != scope.Airport {
			c.mu.Unlock()
			return fmt.Errorf("recorded event belongs to another airport")
		}
		envelope.Context = proto.Clone(scope).(*euroscopeEvents.MembershipContext)
	}
	c.mu.Unlock()
	data, err := proto.Marshal(envelope)
	if err != nil {
		return err
	}
	c.writes.Lock()
	defer c.writes.Unlock()
	if err := c.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

// Close closes the WebSocket connection
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}

	if c.config.Verbose {
		slog.Info("Closing connection")
	}

	// Send close message
	c.writes.Lock()
	err := c.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	c.writes.Unlock()
	if err != nil {
		slog.Warn("Failed to send close message", slog.Any("error", err))
	}

	// Close connection
	if closeErr := c.conn.Close(); closeErr != nil {
		return closeErr
	}

	close(c.done)
	return err
}

// ReadMessages reads messages from the server in the background
func (c *Client) ReadMessages(ctx context.Context, handler func(messageType int, data []byte)) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.done:
				return
			default:
				messageType, data, err := c.conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						slog.Warn("Unexpected WebSocket close", slog.Any("error", err))
					}
					return
				}

				// Capture the message
				msg := ReceivedMessage{
					MessageType: messageType,
					Data:        data,
					ReceivedAt:  time.Now(),
				}

				if eventType, _, err := euroscopeEvents.UnmarshalEnvelope(data); err == nil {
					msg.EventType = euroscopeEvents.EventName(eventType)
				}

				c.mu.Lock()
				var envelope euroscopeEvents.Envelope
				if proto.Unmarshal(data, &envelope) == nil && envelope.Context != nil && envelope.Context.InstanceId == c.instance && envelope.Context.MembershipId != "" {
					if envelope.GetMembershipRemoved() != nil {
						if c.membershipScope != nil && c.membershipScope.MembershipId == envelope.Context.MembershipId {
							c.membershipScope = nil
							c.membershipReady = false
						}
					} else {
						if c.membershipScope == nil || c.membershipScope.MembershipId != envelope.Context.MembershipId {
							c.membershipReady = false
						}
						c.membershipScope = proto.Clone(envelope.Context).(*euroscopeEvents.MembershipContext)
						if envelope.GetSessionInfo() != nil {
							c.membershipReady = true
						}
					}
					select {
					case c.scopeChanged <- struct{}{}:
					default:
					}
				}
				c.receivedMessages = append(c.receivedMessages, msg)
				c.mu.Unlock()

				if handler != nil {
					handler(messageType, data)
				}
			}
		}
	}()
}

// GetReceivedMessages returns all messages received from the server
func (c *Client) GetReceivedMessages() []ReceivedMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ReceivedMessage{}, c.receivedMessages...)
}
