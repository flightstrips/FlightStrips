package websocket

import (
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/constants"
	"FlightStrips/pkg/events"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	frontend "FlightStrips/pkg/events/frontend"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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
	var dispatcher *shared.PositionDispatcher
	if provider, ok := any(client).(interface {
		PositionDispatcher() *shared.PositionDispatcher
	}); ok {
		dispatcher = provider.PositionDispatcher()
	}
	defer func() {
		if dispatcher != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := dispatcher.Close(ctx); err != nil {
				slog.Warn("Position drain cancelled", "error", err)
			}
		}
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
		frameType, message, err := client.GetConnection().ReadMessage()
		if err != nil {
			logReadError(client, err)
			// An unexpected disconnect fences queued work immediately. Explicit
			// application shutdown uses the separate bounded drain path.
			draining := false
			if state, ok := any(client).(interface{ IsDraining() bool }); ok {
				draining = state.IsDraining()
			}
			if dispatcher != nil && !draining {
				_ = client.Close()
				dispatcher.Cancel()
			}
			break
		}
		if draining, ok := any(client).(interface{ IsDraining() bool }); ok && draining.IsDraining() {
			break
		}
		if isEuroscopeType[TType]() && frameType != websocket.BinaryMessage {
			slog.Warn("Rejected non-binary EuroScope websocket message")
			continue
		}

		receivedAt := time.Now()
		// Record the message if recording is enabled
		client.RecordMessage(message)

		parsedMessage, err := parseMessage[TType](message)
		if err != nil {
			slog.Warn("Failed to parse message", slog.Any("error", err))
			continue
		}

		msgType := messageTypeName(parsedMessage.Type)
		metrics.MessageReceived(context.Background(), client.GetSessionName(), client.GetAirport(), client.GetSource(), msgType, client.GetVersion(), len(message))

		var fence func(context.Context) context.Context
		if msgType == "aircraft_position_update" {
			if provider, ok := any(client).(interface {
				PositionFence() func(context.Context) context.Context
			}); ok {
				fence = provider.PositionFence()
			}
		}
		run := func(jobCtx context.Context) {
			if fence != nil {
				jobCtx = fence(jobCtx)
			}
			tracer := otel.Tracer("websocket")
			ctx, span := tracer.Start(shared.WithReceiptTime(jobCtx, receivedAt), msgType,
				trace.WithTimestamp(receivedAt),
				trace.WithAttributes(
					attribute.String("message.type", msgType),
					attribute.String("client.cid", client.GetCid()),
					attribute.String("client.position", client.GetPosition()),
					attribute.Int("session", int(client.GetSession())),
				),
			)
			if shouldTrackMessageDBOperations(client.GetSource(), msgType) {
				ctx = shared.WithWebsocketMessageState(ctx, &shared.WebsocketMessageState{
					MessageType: msgType, AutoCountDBOperations: true,
				})
			}
			// Count physical statements for every event without enabling mutable
			// message caches in handlers that have not audited cache invalidation.
			ctx, dbCounter := shared.WithDBOperationCounter(ctx)

			handlers := hub.GetMessageHandlers()
			start := time.Now()
			err := jobCtx.Err()
			if err == nil {
				err = client.CanHandleMessage(msgType)
			}
			if err == nil {
				err = handlers.Handle(ctx, client, parsedMessage)
			}
			operations := dbCounter.Finish()
			metrics.MessageDBOperations(ctx, client.GetSessionName(), client.GetAirport(), client.GetSource(), msgType, client.GetVersion(), operations)
			if state := shared.GetWebsocketMessageState(ctx); state != nil {
				state.DBOperations = operations
				metrics.MessageDBRetries(ctx, client.GetSessionName(), client.GetAirport(), client.GetSource(), msgType, client.GetVersion(), state.DBRetries)
			}
			metrics.MessageHandled(ctx, client.GetSessionName(), client.GetAirport(), client.GetSource(), msgType, client.GetVersion(), time.Since(start), err)

			if err != nil {
				span.SetStatus(codes.Error, err.Error())
				span.RecordError(err)
				if isEuroscopeType[TType]() {
					slog.ErrorContext(ctx, "Failed to handle protobuf message", slog.Any("error", err), slog.Int("message_bytes", len(message)))
				} else {
					slog.ErrorContext(ctx, "Failed to handle message", slog.Any("error", err), slog.String("message", string(message)))
					client.Enqueue(actionRejectedEvent(fmt.Sprintf("%v", parsedMessage.Type), parsedMessage.Message, err))
				}
			} else {
				span.SetStatus(codes.Ok, "")
			}
			span.SetAttributes(attribute.Float64("message.queue_ms", float64(start.Sub(receivedAt))/float64(time.Millisecond)), attribute.Float64("message.processing_ms", float64(time.Since(start))/float64(time.Millisecond)))
			metrics.MessageCompletion(ctx, client.GetSessionName(), client.GetAirport(), client.GetSource(), msgType, start.Sub(receivedAt), time.Since(receivedAt))
			span.End()
		}
		if dispatcher != nil && msgType == "aircraft_position_update" {
			var position euroscopeEvents.AircraftPositionUpdateEvent
			if decodeErr := parsedMessage.ProtoUnmarshal(&position); decodeErr == nil {
				if receiver, ok := any(client).(interface{ PositionReceived(string) }); ok {
					receiver.PositionReceived(position.Callsign)
				}
				key := fmt.Sprintf("%d/%s", client.GetSession(), strings.ToUpper(strings.TrimSpace(position.Callsign)))
				if submitErr := dispatcher.Submit(context.Background(), key, run); submitErr != nil {
					cancelled, cancel := context.WithCancel(context.Background())
					cancel()
					run(cancelled)
				}
				metrics.PositionQueueDepth(context.Background(), client.GetSessionName(), client.GetAirport(), dispatcher.Depth())
				continue
			}
		}
		// Only surveillance position reports belong to the bounded dispatcher.
		// Operational messages normally bypass its backlog, but an aircraft
		// disconnect must remain ordered after every position already accepted from
		// this socket. Otherwise an older queued position can cancel the newer
		// disconnect worker and leave the aircraft online indefinitely.
		if dispatcher != nil {
			if msgType == "aircraft_disconnect" {
				_ = dispatcher.RunBarrier(context.Background(), func() { run(context.Background()) })
				continue
			}
			// Ask a database batch that is still collecting to start now, but do
			// not wait for it.
			dispatcher.Flush()
		}
		run(context.Background())
	}
}

func actionRejectedEvent(action string, payload []byte, err error) frontend.ActionRejectedEvent {
	rejection := frontend.ActionRejectedEvent{
		Action: action,
		Reason: err.Error(),
	}
	if action != string(frontend.CoordinationForceAssumeRequestType) {
		return rejection
	}

	var request frontend.CoordinationForceAssumeRequestEvent
	if json.Unmarshal(payload, &request) == nil {
		rejection.RequestID = request.RequestID
	}
	return rejection
}

func logReadError(client Client, err error) {
	attrs := []any{
		slog.String("source", client.GetSource()),
		slog.String("cid", client.GetCid()),
		slog.Int("session", int(client.GetSession())),
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

	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		attrs = append(attrs, slog.Int("close_code", closeErr.Code))
		if closeErr.Text != "" {
			attrs = append(attrs, slog.String("reason", closeErr.Text))
		}

		if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
			attrs = append(attrs, slog.Any("error", err))
			slog.Warn("Websocket connection closed", attrs...)
			return
		}

		slog.Info("Websocket connection closed", attrs...)
		return
	}

	attrs = append(attrs, slog.Any("error", err))
	slog.Warn("Websocket connection closed", attrs...)
}

func shouldTrackMessageDBOperations(source string, msgType string) bool {
	if source != "euroscope" {
		return false
	}

	switch msgType {
	case "aircraft_position_update", "strip_update", "squawk", "assigned_squawk", "tracking_controller_changed":
		return true
	default:
		return false
	}
}

type internalMessage[TType comparable] struct {
	Type TType `json:"type"`
}

func parseMessage[TType comparable](message []byte) (shared.Message[TType], error) {
	if isEuroscopeType[TType]() {
		euroscopeType, payload, err := euroscopeEvents.UnmarshalEnvelope(message)
		if err != nil {
			return shared.Message[TType]{}, err
		}
		eventType, ok := any(euroscopeType).(TType)
		if !ok {
			return shared.Message[TType]{}, errors.New("invalid EuroScope event type")
		}
		return shared.Message[TType]{Type: eventType, Message: payload}, nil
	}

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

func isEuroscopeType[TType comparable]() bool {
	var zero TType
	_, ok := any(zero).(euroscopeEvents.EventType)
	return ok
}

func messageTypeName[TType comparable](messageType TType) string {
	if eventType, ok := any(messageType).(euroscopeEvents.EventType); ok {
		return strings.ToLower(strings.TrimPrefix(eventType.String(), "EVENT_"))
	}
	return fmt.Sprintf("%v", messageType)
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

			messageType := "unknown"
			frameType := websocket.TextMessage
			typed, isEuroscopeMessage := message.(euroscopeEvents.OutgoingMessage)
			if client.GetSource() == "euroscope" && !isEuroscopeMessage {
				slog.Error("Refusing to send a non-protobuf message to an EuroScope client")
				continue
			}
			if isEuroscopeMessage {
				messageType = messageTypeName(typed.GetType())
				frameType = websocket.BinaryMessage
			} else {
				var typeHolder struct {
					Type string `json:"type"`
				}
				_ = json.Unmarshal(bytes, &typeHolder)
				messageType = typeHolder.Type
			}

			if err := writeOutboundFrame(context.Background(), client.GetConnection(), client.GetSource(), messageType, frameType, bytes); err != nil {
				slog.Warn("Failed to write websocket message",
					slog.String("source", client.GetSource()),
					slog.String("cid", client.GetCid()),
					slog.String("message_type", messageType),
					slog.Any("error", err))
				return
			}
			metrics.MessageSent(context.Background(), client.GetSessionName(), client.GetAirport(), client.GetSource(), messageType, client.GetVersion())
		case <-ticker.C:
			if err := client.GetConnection().SetWriteDeadline(time.Now().Add(constants.WriteWait)); err != nil {
				return
			}
			if err := client.GetConnection().WriteMessage(websocket.PingMessage, nil); err != nil {
				slog.Warn("Failed to write websocket ping",
					slog.String("source", client.GetSource()),
					slog.String("cid", client.GetCid()),
					slog.Any("error", err))
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

type websocketMessageWriter interface {
	WriteMessage(messageType int, data []byte) error
}

func writeOutboundMessage(ctx context.Context, writer websocketMessageWriter, source, messageType string, payload []byte) error {
	return writeOutboundFrame(ctx, writer, source, messageType, websocket.TextMessage, payload)
}

func writeOutboundFrame(ctx context.Context, writer websocketMessageWriter, source, messageType string, frameType int, payload []byte) error {
	if err := writer.WriteMessage(frameType, payload); err != nil {
		return err
	}
	metrics.RecordOutboundPayload(ctx, source, messageType, len(payload))
	return nil
}
