package euroscope

import "FlightStrips/pkg/events"

func startQueuedTestClient(client *Client) *Client {
	if client.send == nil {
		client.send = make(chan events.OutgoingMessage, 1)
	}
	client.closed = make(chan struct{})
	return client
}
