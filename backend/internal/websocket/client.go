package websocket

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/constants"
	"FlightStrips/pkg/events"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Client interface {
	Close() error

	GetCid() string
	GetAirport() string
	GetPosition() string
	GetConnection() *websocket.Conn

	IsAuthenticated() bool
	SetUser(user shared.AuthenticatedUser)

	HandlePong() error

	GetSendChannel() chan events.OutgoingMessage
}

// ReadPump pumps messages from the WebSocket connection to the hub.
func ReadPump[TType comparable, TClient Client, THub Hub[TType, TClient]](hub THub, client TClient) {
	slog.Debug("ReadPump started", slog.String("cid", client.GetCid()))
	defer func() {
		hub.Unregister(client)
		client.GetConnection().Close()
	}()

	err := client.GetConnection().SetReadDeadline(time.Now().Add(constants.PongWait))
	if err != nil {
		slog.Info("Failed to set read deadline", slog.Any("error", err))
		return
	}
	client.GetConnection().SetPongHandler(func(string) error {
		err := client.GetConnection().SetReadDeadline(time.Now().Add(constants.PongWait))
		if err != nil {
			slog.Info("Failed to set read deadline in pong handler", slog.Any("error", err))
			return err
		}
		return client.HandlePong()
	})

	for {
		_, message, err := client.GetConnection().ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Info("Unexpected websocket close", slog.Any("error", err))
			}
			break
		}
		parsedMessage, err := parseMessage[TType](message)
		if err != nil {
			slog.Info("Failed to parse message", slog.Any("error", err))
			continue
		}

		// Create trace span for message handling
		tracer := otel.Tracer("websocket")
		ctx, span := tracer.Start(context.Background(), "websocket.message",
			trace.WithAttributes(
				attribute.String("message.type", fmt.Sprintf("%v", parsedMessage.Type)),
				attribute.String("client.cid", client.GetCid()),
				attribute.String("client.position", client.GetPosition()),
			),
		)

		handlers := hub.GetMessageHandlers()
		err = handlers.Handle(ctx, client, parsedMessage)

		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
			slog.ErrorContext(ctx, "Failed to handle message", slog.Any("error", err), slog.String("message", string(message)))
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}
}

type internalMessage[TType comparable] struct {
	Type TType `json:"type"`
}

func parseMessage[TType comparable](message []byte) (shared.Message[TType], error) {
	var msg internalMessage[TType]
	err := json.Unmarshal(message, &msg)
	if err != nil {
		return shared.Message[TType]{}, err
	}
	return shared.Message[TType]{
		Type:    msg.Type,
		Message: message,
	}, nil
}

// WritePump pumps messages from the hub to the WebSocket connection.
func WritePump[TClient Client](client TClient) {
	slog.Debug("WritePump started", slog.String("cid", client.GetCid()))
	ticker := time.NewTicker(constants.PingPeriod)
	defer func() {
		ticker.Stop()
		client.GetConnection().Close()
	}()

	for {
		select {
		case message, ok := <-client.GetSendChannel():
			err := client.GetConnection().SetWriteDeadline(time.Now().Add(constants.WriteWait))
			if err != nil {
				slog.Info("Failed to set write deadline", slog.Any("error", err))
				return
			}
			if !ok {
				// The hub closed the channel.
				err := client.GetConnection().WriteMessage(websocket.CloseMessage, []byte{})
				if err != nil {
					slog.Info("Failed to close connection", slog.Any("error", err))
				}
				return
			}

			bytes, err := message.Marshal()
			if err != nil {
				slog.Error("Failed to marshal message", slog.Any("error", err))
				continue
			}

			if err := client.GetConnection().WriteMessage(websocket.TextMessage, bytes); err != nil {
				return
			}
		case <-ticker.C:
			if err := client.GetConnection().SetWriteDeadline(time.Now().Add(constants.WriteWait)); err != nil {
				return
			}
			if err := client.GetConnection().WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
