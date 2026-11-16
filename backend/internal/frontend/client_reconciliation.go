package frontend

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
)

// reconcile queues the snapshot before any events delivered while it was being
// read. Build runs without holding deliveryMu, so hub broadcasts keep flowing.
func (c *Client) reconcile(build func() (events.OutgoingMessage, error)) error {
	c.deliveryMu.Lock()
	select {
	case <-c.closed:
		c.deliveryMu.Unlock()
		return shared.RejectAction("board subscription is closed")
	default:
	}
	if c.reconciliationBuffer != nil {
		c.deliveryMu.Unlock()
		return shared.RejectAction("reconciliation already in progress")
	}
	c.reconciliationBuffer = make([]events.OutgoingMessage, 0)
	c.deliveryMu.Unlock()

	snapshot, err := build()

	c.deliveryMu.Lock()
	defer c.deliveryMu.Unlock()
	pending := c.reconciliationBuffer
	c.reconciliationBuffer = nil
	if err == nil && !c.enqueue(snapshot) {
		return shared.RejectAction("unable to queue reconciliation state")
	}
	// Even a failed snapshot must release concurrent events. The caller reports
	// the read/authority error afterwards, keeping client-side mutations fenced.
	for _, message := range pending {
		if !c.enqueue(message) {
			return shared.RejectAction("unable to queue reconciliation updates")
		}
	}
	return err
}
