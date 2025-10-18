package shared

import (
	"encoding/json"
	"fmt"
)

type Message[TType comparable] struct {
	Type    TType
	Message []byte
}

func (m *Message[TType]) JsonUnmarshal(v interface{}) error {
	return json.Unmarshal(m.Message, v)
}

type Handler[TType comparable, T any] func(client T, message Message[TType]) error

type MessageHandlers[TType comparable, TClient any] struct {
	handlers map[TType]Handler[TType, TClient]
}

func NewMessageHandlers[TType comparable, TClient any]() MessageHandlers[TType, TClient] {
	return MessageHandlers[TType, TClient]{
		handlers: make(map[TType]Handler[TType, TClient]),
	}
}

func (handlers MessageHandlers[TType, TClient]) Add(eventType TType, handler Handler[TType, TClient]) {
	handlers.handlers[eventType] = handler
}

func (handlers MessageHandlers[TType, TClient]) Remove(eventType TType) {
	delete(handlers.handlers, eventType)
}

func (handlers MessageHandlers[TType, TClient]) Handle(client TClient, message Message[TType]) error {
	if handler, ok := handlers.handlers[message.Type]; ok {
		return handler(client, message)
	}

	return fmt.Errorf("no handler for event type: %s", message.Type)
}
