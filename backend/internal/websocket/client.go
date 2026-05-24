package websocket

import (
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/constants"
	"FlightStrips/pkg/events"
	frontend "FlightStrips/pkg/events/frontend"
	"context"
	"encoding/json"
	"errors"
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
	GetCallsign() string
	GetAirport() string
	GetPosition() string
	GetSession() int32
	GetSessionName() string
	GetSource() string
	GetVersion() string
	GetConnection() *websocket.Conn

	IsAuthenticated() bool
	SetUser(user shared.AuthenticatedUser)
	CanHandleMessage(messageType string) error

	HandlePong() error

	GetSendChannel() chan events.OutgoingMessage
	Enqueue(message events.OutgoingMessage) bool

	// RecordMessage optionally records a message for replay
	RecordMessage(rawMessage []byte)
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
		slog.Warn("Failed to set read deadline", slog.Any("error", err))
		return
	}
	client.GetConnection().SetPongHandler(func(string) error {
		err := client.GetConnection().SetReadDeadline(time.Now().Add(constants.PongWait))
		if err != nil {
			slog.Warn("Failed to set read deadline in pong handler", slog.Any("error", err))
			return err
		}
		return client.HandlePong()
	})

	for {
		_, message, err := client.GetConnection().ReadMessage()
		if err != nil {
			logReadError(client, err)
			break
		}

		// Record the message if recording is enabled
		client.RecordMessage(message)

		parsedMessage, err := parseMessage[TType](message)
		if err != nil {
			slog.Warn("Failed to parse message", slog.Any("error", err))
			continue
		}

		msgType := fmt.Sprintf("%v", parsedMessage.Type)
		metrics.MessageReceived(context.Background(), client.GetSessionName(), client.GetAirport(), client.GetSource(), msgType, client.GetVersion())

		tracer := otel.Tracer("websocket")
		ctx, span := tracer.Start(context.Background(), msgType,
			trace.WithAttributes(
				attribute.String("message.type", msgType),
				attribute.String("client.cid", client.GetCid()),
				attribute.String("client.position", client.GetPosition()),
				attribute.Int("session", int(client.GetSession())),
			),
		)
		if shouldTrackMessageDBOperations(client.GetSource(), msgType) {
			ctx = shared.WithWebsocketMessageState(ctx, &shared.WebsocketMessageState{MessageType: msgType})
		}

		handlers := hub.GetMessageHandlers()
		start := time.Now()
		err = client.CanHandleMessage(msgType)
		if err == nil {
			err = handlers.Handle(ctx, client, parsedMessage)
		}
		if state := shared.GetWebsocketMessageState(ctx); state != nil {
			metrics.MessageDBOperations(ctx, client.GetSessionName(), client.GetAirport(), client.GetSource(), msgType, client.GetVersion(), state.DBOperations)
		}
		metrics.MessageHandled(ctx, client.GetSessionName(), client.GetAirport(), client.GetSource(), msgType, client.GetVersion(), time.Since(start), err == nil)

		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
			slog.ErrorContext(ctx, "Failed to handle message", slog.Any("error", err), slog.String("message", string(message)))
			client.Enqueue(frontend.ActionRejectedEvent{
				Action: fmt.Sprintf("%v", parsedMessage.Type),
				Reason: err.Error(),
			})
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}
}

func logReadError(client Client, err error) {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		attrs := []any{
			slog.String("source", client.GetSource()),
			slog.String("cid", client.GetCid()),
			slog.Int("session", int(client.GetSession())),
			slog.Int("close_code", closeErr.Code),
		}
		if sessionName := client.GetSessionName(); sessionName != "" {
			attrs = append(attrs, slog.String("session_name", sessionName))
		}
		if airport := client.GetAirport(); airport != "" {
			attrs = append(attrs, slog.String("airport", airport))
		}
		if callsign := client.GetCallsign(); callsign != "" {
			attrs = append(attrs, slog.String("callsign", callsign))
		}
		if position := client.GetPosition(); position != "" {
			attrs = append(attrs, slog.String("position", position))
		}
		if closeErr.Text != "" {
			attrs = append(attrs, slog.String("reason", closeErr.Text))
		}

		if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
			attrs = append(attrs, slog.Any("error", err))
			slog.Warn("Unexpected websocket close", attrs...)
			return
		}

		slog.Info("Websocket connection closed", attrs...)
		return
	}

	if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
		slog.Warn("Unexpected websocket close",
			slog.String("source", client.GetSource()),
			slog.String("cid", client.GetCid()),
			slog.Int("session", int(client.GetSession())),
			slog.Any("error", err),
		)
	}
}

func shouldTrackMessageDBOperations(source string, msgType string) bool {
	if source != "euroscope" {
		return false
	}

	switch msgType {
	case "strip_update", "squawk", "assigned_squawk", "tracking_controller_changed":
		return true
	default:
		return false
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
	tokenTicker := time.NewTicker(constants.TokenCheckPeriod)
	defer func() {
		ticker.Stop()
		tokenTicker.Stop()
		client.GetConnection().Close()
	}()

	for {
		select {
		case message, ok := <-client.GetSendChannel():
			err := client.GetConnection().SetWriteDeadline(time.Now().Add(constants.WriteWait))
			if err != nil {
				slog.Warn("Failed to set write deadline", slog.Any("error", err))
				return
			}
			if !ok {
				// The hub closed the channel.
				err := client.GetConnection().WriteMessage(websocket.CloseMessage, []byte{})
				if err != nil {
					slog.Warn("Failed to close connection", slog.Any("error", err))
				}
				return
			}

			bytes, err := message.Marshal()
			if err != nil {
				slog.Error("Failed to marshal message", slog.Any("error", err))
				continue
			}

			var typeHolder struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(bytes, &typeHolder)
			metrics.MessageSent(context.Background(), client.GetSessionName(), client.GetAirport(), client.GetSource(), typeHolder.Type, client.GetVersion())

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
		case <-tokenTicker.C:
			if !client.IsAuthenticated() {
				slog.Info("Token expired, disconnecting client", slog.String("cid", client.GetCid()))
				_ = client.GetConnection().SetWriteDeadline(time.Now().Add(constants.WriteWait))
				_ = client.GetConnection().WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "token expired"))
				return
			}
		}
	}
}
