package euroscope

import (
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"
	"context"
)

// Resolve a specific connected controller, never the session master. Returning
// a sender pins the selected connection so a reconnect cannot replay the text.
func (hub *Hub) FrequencyMessageSender(session int32, cid, position string) func(euroscope.SendFrequencyMessageEvent) bool {
	hub.clientsMu.RLock()
	defer hub.clientsMu.RUnlock()
	for client := range hub.clients {
		select {
		case <-client.closed:
			continue
		default:
		}
		identity := client.identitySnapshot()
		if client.session == session && client.GetCid() == cid && identity.position == position && !identity.observer && client.frequencyMessages.Load() {
			return func(event euroscope.SendFrequencyMessageEvent) bool {
				select {
				case <-client.closed:
					return false
				default:
				}
				current := client.identitySnapshot()
				return current.position == position && !current.observer && client.frequencyMessages.Load() && client.Enqueue(event)
			}
		}
	}
	return nil
}

func handleFrequencyMessageResult(ctx context.Context, client *Client, message Message) error {
	var event euroscope.FrequencyMessageResultEvent
	if err := message.ProtoUnmarshal(&event); err != nil {
		return err
	}
	client.hub.server.GetFrontendHub().Send(client.session, client.GetCid(), frontend.FrequencyMessageResultEvent{
		RequestID: event.RequestId, Callsign: event.Callsign, Submitted: event.Submitted,
		Error: event.Error, AppliedActions: event.AppliedActions,
	})
	return nil
}
