package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"FlightStrips/internal/testing/recorder"

	"github.com/gorilla/websocket"
)

// ReceivedMessage represents a message received from the server
type ReceivedMessage struct {
	MessageType int
	Data        []byte
	ReceivedAt  time.Time
	EventType   string // Parsed from JSON
}

// Client simulates an EuroScope WebSocket client for replay
type Client struct {
	conn             *websocket.Conn
	config           Config
	done             chan struct{}
	receivedMessages []ReceivedMessage
	mu               sync.Mutex
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
	authEvent := map[string]string{
		"type":  "token",
		"token": "__TEST_TOKEN__",
	}
	if err := c.SendRawMessage(authEvent); err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to authenticate: %w", err)
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

	// Send the raw payload
	if err := c.conn.WriteMessage(websocket.TextMessage, event.Payload); err != nil {
		return fmt.Errorf("failed to send event: %w", err)
	}

	return nil
}

// SendRawMessage sends a raw JSON message to the server
func (c *Client) SendRawMessage(message interface{}) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
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
	err := c.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
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

				// Try to parse event type from JSON
				var eventData map[string]interface{}
				if err := json.Unmarshal(data, &eventData); err == nil {
					if eventType, ok := eventData["type"].(string); ok {
						msg.EventType = eventType
					}
				}

				c.mu.Lock()
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
