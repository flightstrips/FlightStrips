package euroscope

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"
	"context"
	"time"
)

// Resolve a specific connected controller, never the session master. Returning
// a sender pins the selected connection so a reconnect cannot replay the text.
func (hub *Hub) FrequencyMessageSender(session int32, cid, position string) func(euroscope.SendFrequencyMessageEvent) bool {
	var selected *Client
	for _, client := range hub.clientsSnapshot() {
		identity := client.identitySnapshot()
		if client.session != session || client.GetCid() != cid || identity.position != position || identity.observer || !client.frequencyMessages.Load() || client.isClosed() {
			continue
		}
		if selected != nil {
			return nil
		}
		selected = client
	}
	if selected == nil {
		return nil
	}
	binding := &commandBinding{hub: hub, client: selected, session: session, cid: cid}
	return binding.FrequencyMessageSender(position, func(event frontend.FrequencyMessageResultEvent) {
		if hub.server != nil && hub.server.GetFrontendHub() != nil {
			hub.server.GetFrontendHub().Send(session, cid, event)
		}
	})

}

func handleFrequencyMessageResult(ctx context.Context, client *Client, message Message) error {
	var event euroscope.FrequencyMessageResultEvent
	if err := message.ProtoUnmarshal(&event); err != nil {
		return err
	}
	client.executionMu.Lock()
	request, ok := client.executions["frequency:"+event.RequestId]
	if !ok || request.response != euroscope.FrequencyMessageResult || request.callsign != event.Callsign || time.Now().After(request.deadline) {
		client.executionMu.Unlock()
		return shared.RejectAction("frequency result does not match membership request")
	}
	delete(client.executions, "frequency:"+event.RequestId)
	client.executionMu.Unlock()
	if request.deliver != nil {
		request.deliver(frontend.FrequencyMessageResultEvent{RequestID: event.RequestId, Callsign: event.Callsign, Submitted: event.Submitted, Error: event.Error, AppliedActions: event.AppliedActions})
	}

	return nil
}

// The frontend captures this method on its already authenticated own binding.
// Request IDs are specific typed business results, never a CID-wide sender.
func (b *commandBinding) FrequencyMessageSender(position string, deliver func(frontend.FrequencyMessageResultEvent)) func(euroscope.SendFrequencyMessageEvent) bool {
	current := b.client.identitySnapshot()
	if current.position != position || current.observer || !b.client.frequencyMessages.Load() || b.Validate() != nil {
		return nil
	}
	return func(event euroscope.SendFrequencyMessageEvent) bool {
		current := b.client.identitySnapshot()
		if current.position != position || current.observer || !b.client.frequencyMessages.Load() || event.RequestId == "" || event.Callsign == "" || b.Validate() != nil {
			return false
		}
		now := time.Now()
		key := "frequency:" + event.RequestId
		b.client.executionMu.Lock()
		for id, request := range b.client.executions {
			if now.After(request.deadline) {
				delete(b.client.executions, id)
			}
		}
		if _, exists := b.client.executions[key]; exists || len(b.client.executions) >= maxPendingExecutions {
			b.client.executionMu.Unlock()
			return false
		}
		if b.client.executions == nil {
			b.client.executions = map[string]commandExecution{}
		}
		b.client.executions[key] = commandExecution{callsign: event.Callsign, response: euroscope.FrequencyMessageResult, deadline: now.Add(executionLifetime), deliver: deliver}
		b.client.executionMu.Unlock()
		if b.enqueue(event) != nil {
			b.client.executionMu.Lock()
			delete(b.client.executions, key)
			b.client.executionMu.Unlock()
			return false
		}
		return true
	}
}
