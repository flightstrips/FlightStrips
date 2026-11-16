package frontend

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"FlightStrips/pkg/events/frontend"
	"context"
)

// Refresh server-owned operational data on the existing authorized subscription.
func handleActionReconciliation(ctx context.Context, client *Client, message Message) error {
	var request struct {
		RequestID string `json:"request_id"`
	}
	if err := message.JsonUnmarshal(&request); err != nil {
		return err
	}
	if request.RequestID == "" {
		return shared.RejectAction("refresh request identity required")
	}
	identity := client.messageIdentity(ctx)
	if err := shared.ValidateCommandAuthority(ctx, identity.session); err != nil {
		return err
	}
	return client.reconcile(func() (events.OutgoingMessage, error) {
		snapshot, _, err := client.hub.getSnapshotBuilder().Build(ctx, InitialSnapshotRequest{
			Board: client.selection.board, SessionID: identity.session,
			Position: identity.position, Airport: identity.airport, Callsign: identity.callsign,
			UserCID: client.GetCid(), ReadOnly: identity.readOnly,
		})
		if err != nil {
			return nil, err
		}
		if err := shared.ValidateCommandAuthority(ctx, identity.session); err != nil {
			return nil, err
		}
		return frontend.ActionReconciliationEvent{InitialEvent: snapshot, RequestID: request.RequestID}, nil
	})
}
