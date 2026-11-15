package euroscope

import (
	"context"
	"time"

	"FlightStrips/internal/shared"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"

	"github.com/google/uuid"
)

func (b *commandBinding) SendVacsRequest(event es.VacsRequestEvent, deliver func(fe.VacsResultEvent)) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if !b.client.vacsRelay.Load() {
		return shared.RejectAction("Update the EuroScope plugin to use VACS controls")
	}
	// A server-owned ID prevents another browser/tab from claiming a response.
	browserID := event.RequestId
	event.RequestId = uuid.NewString()
	key := "vacs:" + event.RequestId
	now := time.Now()
	b.client.executionMu.Lock()
	for id, request := range b.client.executions {
		if now.After(request.deadline) {
			delete(b.client.executions, id)
		}
	}
	if len(b.client.executions) >= maxPendingExecutions {
		b.client.executionMu.Unlock()
		return shared.RejectAction("Too many pending plugin requests")
	}
	if b.client.executions == nil {
		b.client.executions = map[string]commandExecution{}
	}
	b.client.executions[key] = commandExecution{response: es.VacsResult, deadline: now.Add(10 * time.Second), deliverVacs: func(result fe.VacsResultEvent) {
		result.RequestID = browserID
		deliver(result)
	}}
	b.client.executionMu.Unlock()
	if err := b.enqueue(event); err != nil {
		b.client.executionMu.Lock()
		delete(b.client.executions, key)
		b.client.executionMu.Unlock()
		return err
	}
	return nil
}

func handleVacsResult(_ context.Context, client *Client, message Message) error {
	var event es.VacsResultEvent
	if err := message.ProtoUnmarshal(&event); err != nil {
		return err
	}
	key := "vacs:" + event.RequestId
	client.executionMu.Lock()
	request, ok := client.executions[key]
	if !ok || request.response != es.VacsResult || time.Now().After(request.deadline) {
		client.executionMu.Unlock()
		return shared.RejectAction("VACS result does not match membership request")
	}
	delete(client.executions, key)
	client.executionMu.Unlock()
	if request.deliverVacs != nil {
		request.deliverVacs(fe.VacsResultEvent{State: event.State, Error: event.Error})
	}
	return nil
}
