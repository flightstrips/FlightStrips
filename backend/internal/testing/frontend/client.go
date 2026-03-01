package frontend

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ReceivedMessage represents a message received from the server
type ReceivedMessage struct {
	MessageType int
	Data        []byte
	ReceivedAt  time.Time
	EventType   string // Parsed from JSON
}

// Client simulates a frontend WebSocket client for testing
type Client struct {
	conn             *websocket.Conn
	config           Config
	done             chan struct{}
	verbose          bool
	receivedMessages []ReceivedMessage
	mu               sync.Mutex
}

// Config contains configuration for the frontend client
type Config struct {
	ServerURL string
	Token     string
	Verbose   bool
}

// NewClient creates a new frontend client simulator
func NewClient(config Config) *Client {
	return &Client{
		config:           config,
		done:             make(chan struct{}),
		verbose:          config.Verbose,
		receivedMessages: []ReceivedMessage{},
	}
}

// Connect establishes a WebSocket connection to the server
func (c *Client) Connect(ctx context.Context) error {
	if c.verbose {
		slog.Info("Frontend client connecting", slog.String("url", c.config.ServerURL))
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.DialContext(ctx, c.config.ServerURL, http.Header{})
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()

	c.conn = conn

	// Send authentication token
	authEvent := map[string]string{
		"type":  "token",
		"token": c.config.Token,
	}
	if err := c.SendRawMessage(authEvent); err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	if c.verbose {
		slog.Info("Frontend client connected and authenticated")
	}

	return nil
}

// Close closes the WebSocket connection
func (c *Client) Close() error {
	if c.conn != nil {
		close(c.done)
		return c.conn.Close()
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

	if c.verbose {
		slog.Debug("Frontend sending message", slog.String("data", string(data)))
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// UpdateStrip sends a strip update event
func (c *Client) UpdateStrip(callsign string, version int32, updates map[string]interface{}) error {
	event := map[string]interface{}{
		"type":     "update_strip_data",
		"callsign": callsign,
	}

	// Add all strip fields using pointers for optional values
	for field, value := range updates {
		switch field {
		case "version":
			// Skip version field, not part of the event
			continue
		case "runway":
			if v, ok := value.(string); ok {
				event["runway"] = v
			}
		case "sid":
			if v, ok := value.(string); ok {
				event["sid"] = v
			}
		case "route":
			if v, ok := value.(string); ok {
				event["route"] = v
			}
		case "altitude", "cleared_altitude":
			if v, ok := value.(float64); ok {
				event["altitude"] = int32(v)
			} else if v, ok := value.(int32); ok {
				event["altitude"] = v
			}
		case "heading":
			if v, ok := value.(float64); ok {
				event["heading"] = int32(v)
			} else if v, ok := value.(int32); ok {
				event["heading"] = v
			}
		case "stand":
			if v, ok := value.(string); ok {
				event["stand"] = v
			}
		case "eobt":
			if v, ok := value.(string); ok {
				event["eobt"] = v
			}
		}
	}

	if c.verbose {
		slog.Info("Frontend updating strip",
			slog.String("callsign", callsign),
			slog.Int("version", int(version)))
		
		// Log the actual event being sent
		if jsonData, err := json.Marshal(event); err == nil {
			slog.Info("Frontend sending JSON", slog.String("json", string(jsonData)))
		}
	}

	return c.SendRawMessage(event)
}

// UpdateStripField updates a single field on a strip
func (c *Client) UpdateStripField(callsign string, version int32, field string, value interface{}) error {
	updates := map[string]interface{}{
		field: value,
	}

	if c.verbose {
		slog.Info("Frontend updating strip field",
			slog.String("callsign", callsign),
			slog.String("field", field),
			slog.Any("value", value))
	}

	return c.UpdateStrip(callsign, version, updates)
}

// ReadMessages starts reading messages from the server
func (c *Client) ReadMessages(ctx context.Context, handler func(int, []byte)) {
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
					if c.verbose {
						slog.Debug("Frontend client read error", slog.Any("error", err))
					}
					return
				}

				// Capture the message
				msg := ReceivedMessage{
					MessageType: messageType,
					Data:        data,
					ReceivedAt:  time.Now(),
				}

				// Parse event type from JSON
				var eventData map[string]interface{}
				if err := json.Unmarshal(data, &eventData); err == nil {
					if eventType, ok := eventData["type"].(string); ok {
						msg.EventType = eventType
						if c.verbose {
							slog.Debug("Frontend received message", slog.String("type", eventType))
						}
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
