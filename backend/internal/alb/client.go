package alb

import (
	"FlightStrips/pkg/constants"
	"encoding/json"
	"log/slog"
	"time"

	gorilla "github.com/gorilla/websocket"
)

type Client struct {
	hub      *Hub
	conn     *gorilla.Conn
	send     chan []byte
	callsign string
}

// WritePump pumps messages from the send channel to the WebSocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(constants.PingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(constants.WriteWait)); err != nil {
				return
			}
			if !ok {
				_ = c.conn.WriteMessage(gorilla.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(gorilla.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(constants.WriteWait)); err != nil {
				return
			}
			if err := c.conn.WriteMessage(gorilla.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ReadPump pumps messages from the WebSocket connection and dispatches them by type.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	_ = c.conn.SetReadDeadline(time.Now().Add(constants.PongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(constants.PongWait))
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if gorilla.IsUnexpectedCloseError(err, gorilla.CloseGoingAway, gorilla.CloseAbnormalClosure, gorilla.CloseNoStatusReceived) {
				slog.Info("ALB unexpected websocket close", slog.Any("error", err))
			}
			break
		}

		var base struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(message, &base); err != nil {
			slog.Info("ALB failed to parse message type", slog.Any("error", err))
			continue
		}

		switch base.Type {
		case "login":
			handleLogin(c, message)
		case "query":
			handleQuery(c, message)
		case "a2a":
			handleA2A(c, c.hub, message)
		default:
			slog.Debug("ALB unknown message type", slog.String("type", base.Type))
		}
	}
}
