package euroscope

import (
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/shared"
	es "FlightStrips/pkg/events/euroscope"
	front "FlightStrips/pkg/events/frontend"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const maxPendingExecutions = 256
const executionLifetime = 30 * time.Second
const squawkExecutionLifetime = 2 * time.Minute

type commandExecution struct {
	callsign string
	response es.EventType
	origin   es.CommandOrigin
	epoch    uint64
	deadline time.Time
	deliver  func(front.FrequencyMessageResultEvent)
	expected []byte
}

type executionResultKey struct{}

func executionResponse(kind es.EventType) es.EventType {
	switch kind {
	case es.GenerateSquawk:
		return es.AssignedSquawk
	case es.Hold, es.AssignedSquawk, es.RequestedAltitude, es.ClearedAltitude, es.GroundState, es.ClearedFlag, es.SetHeading, es.Stand, es.CommunicationType:
		return kind
	default:
		return es.EventType_EVENT_UNKNOWN
	}
}

// Extract the callsign from the existing typed message contract, never from a
// client-provided routing key. Only the small response allowlist above uses it.
func eventCallsign(kind es.EventType, payload []byte) string {
	descriptor := (&es.Envelope{}).ProtoReflect().Descriptor().Oneofs().ByName("event").Fields().ByNumber(protoreflect.FieldNumber(kind))
	if descriptor == nil {
		return ""
	}
	message := dynamicpb.NewMessage(descriptor.Message())
	if proto.Unmarshal(payload, message) != nil {
		return ""
	}
	field := message.Descriptor().Fields().ByName("callsign")
	if field == nil || field.Kind() != protoreflect.StringKind {
		return ""
	}
	return message.Get(field).String()
}

func (c *Client) registerExecution(event es.OutgoingMessage, origin es.CommandOrigin, epoch uint64) (string, error) {
	response := executionResponse(event.GetType())
	if response == es.EventType_EVENT_UNKNOWN {
		return "", nil
	}
	wire, err := event.Marshal()
	if err != nil {
		return "", err
	}
	kind, payload, err := es.UnmarshalEnvelope(wire)
	if err != nil {
		return "", err
	}
	callsign := eventCallsign(kind, payload)
	if callsign == "" {
		return "", shared.RejectAction("SDK command has no aircraft identity")
	}
	now := time.Now()
	c.executionMu.Lock()
	defer c.executionMu.Unlock()
	for id, request := range c.executions {
		if now.After(request.deadline) {
			delete(c.executions, id)
		}
	}
	if len(c.executions) >= maxPendingExecutions {
		return "", shared.RejectAction("too many outstanding SDK commands")
	}
	if c.executions == nil {
		c.executions = make(map[string]commandExecution)
	}
	id := fmt.Sprintf("%x", newGeneration())
	lifetime := executionLifetime
	if kind == es.GenerateSquawk {
		lifetime = squawkExecutionLifetime
	}
	request := commandExecution{callsign: callsign, response: response, origin: origin, epoch: epoch, deadline: now.Add(lifetime)}
	if response == kind {
		request.expected = payload
	}
	c.executions[id] = request
	return id, nil
}

func (c *Client) validateExecutionEcho(kind string, payload []byte, origin es.CommandOrigin, id string) bool {
	if kind == "frequency_message_result" {
		var event es.FrequencyMessageResultEvent
		if proto.Unmarshal(payload, &event) != nil {
			return false
		}
		c.executionMu.Lock()
		defer c.executionMu.Unlock()
		request, ok := c.executions["frequency:"+event.RequestId]
		return ok && request.response == es.FrequencyMessageResult && request.callsign == event.Callsign && time.Now().Before(request.deadline)
	}
	if id == "" || (origin != es.CommandOrigin_COMMAND_ORIGIN_HUMAN && origin != es.CommandOrigin_COMMAND_ORIGIN_AUTOMATIC) {
		return false
	}
	value, ok := es.EventType_value["EVENT_"+strings.ToUpper(kind)]
	if !ok {
		return false
	}
	eventType := es.EventType(value)
	c.executionMu.Lock()
	defer c.executionMu.Unlock()
	request, ok := c.executions[id]
	if !ok || request.origin != origin || request.response != eventType || request.callsign != eventCallsign(eventType, payload) || time.Now().After(request.deadline) {
		return false
	}
	if request.expected != nil && !sameEventPayload(eventType, request.expected, payload) {
		return false
	}
	if origin == es.CommandOrigin_COMMAND_ORIGIN_AUTOMATIC && request.epoch != c.hub.election(c.session).epoch.Load() {
		return false
	}
	delete(c.executions, id)
	return true
}

func sameEventPayload(kind es.EventType, expected, observed []byte) bool {
	descriptor := (&es.Envelope{}).ProtoReflect().Descriptor().Oneofs().ByName("event").Fields().ByNumber(protoreflect.FieldNumber(kind))
	if descriptor == nil {
		return false
	}
	left, right := dynamicpb.NewMessage(descriptor.Message()), dynamicpb.NewMessage(descriptor.Message())
	return proto.Unmarshal(expected, left) == nil && proto.Unmarshal(observed, right) == nil && proto.Equal(left, right)
}
