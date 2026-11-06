package shared

import (
	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/proto"
)

type Message[TType comparable] struct {
	Board       *events.BoardContext
	Type        TType
	Message     []byte
	Scope       *es.MembershipContext
	Origin      es.CommandOrigin
	ExecutionID string
}

func (m *Message[TType]) JsonUnmarshal(v interface{}) error {
	return json.Unmarshal(m.Message, v)
}

func (m *Message[TType]) ProtoUnmarshal(v proto.Message) error {
	return proto.Unmarshal(m.Message, v)
}

type boardDispatchKey struct{}

type Handler[TType comparable, T any] func(ctx context.Context, client T, message Message[TType]) error

type MessageHandlers[TType comparable, TClient any] struct {
	controllerActions map[TType]bool
	handlers          map[TType]Handler[TType, TClient]
}

func NewMessageHandlers[TType comparable, TClient any]() MessageHandlers[TType, TClient] {
	return MessageHandlers[TType, TClient]{
		handlers:          make(map[TType]Handler[TType, TClient]),
		controllerActions: make(map[TType]bool),
	}
}

func (handlers MessageHandlers[TType, TClient]) Add(eventType TType, handler Handler[TType, TClient]) {
	handlers.handlers[eventType] = handler
}

// AddAction records trusted controller intent at transport registration, keeping
// observations distinct without a dispatcher-maintained command-name switch.
func (handlers MessageHandlers[TType, TClient]) AddAction(eventType TType, handler Handler[TType, TClient]) {
	handlers.Add(eventType, handler)
	handlers.controllerActions[eventType] = true
}

func (handlers MessageHandlers[TType, TClient]) Remove(eventType TType) {
	delete(handlers.handlers, eventType)
	delete(handlers.controllerActions, eventType)
}

func (handlers MessageHandlers[TType, TClient]) Handle(ctx context.Context, client TClient, message Message[TType]) error {
	if source, ok := any(client).(interface {
		ApplyScopedMessage(context.Context, *es.MembershipContext, string, []byte, es.CommandOrigin, string, func(context.Context) error) error
	}); ok && message.Scope != nil {
		scope := message.Scope
		message.Scope = nil
		return source.ApplyScopedMessage(ctx, scope, fmt.Sprint(message.Type), message.Message, message.Origin, message.ExecutionID, func(ctx context.Context) error { return handlers.Handle(ctx, client, message) })
	}
	if source, ok := any(client).(interface {
		ApplyBoardMessage(context.Context, *events.BoardContext, string, func(context.Context) error) error
	}); ok {
		if _, applied := ctx.Value(boardDispatchKey{}).(bool); !applied {
			return source.ApplyBoardMessage(ctx, message.Board, fmt.Sprint(message.Type), func(ctx context.Context) error {
				return handlers.Handle(context.WithValue(ctx, boardDispatchKey{}, true), client, message)
			})
		}
	}
	if handler, ok := handlers.handlers[message.Type]; ok {
		if handlers.controllerActions[message.Type] {
			if source, ok := any(client).(interface {
				PrepareControllerActionContext(context.Context) (context.Context, error)
			}); ok {
				var err error
				ctx, err = source.PrepareControllerActionContext(ctx)
				if err != nil {
					return err
				}
			}
		}
		if source, ok := any(client).(interface {
			PrepareMessageContext(context.Context, string) (context.Context, error)
		}); ok {
			var err error
			ctx, err = source.PrepareMessageContext(ctx, fmt.Sprint(message.Type))
			if err != nil {
				return err
			}
		}
		return handler(ctx, client, message)
	}

	return fmt.Errorf("no handler for event type: %v", message.Type)
}
