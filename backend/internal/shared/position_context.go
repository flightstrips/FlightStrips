package shared

import (
	"FlightStrips/internal/models"
	"context"
	"time"
)

type receiptTimeKey struct{}

func WithReceiptTime(ctx context.Context, at time.Time) context.Context {
	return context.WithValue(ctx, receiptTimeKey{}, at)
}

func ReceiptTime(ctx context.Context, fallback func() time.Time) time.Time {
	if at, ok := ctx.Value(receiptTimeKey{}).(time.Time); ok {
		return at
	}
	return fallback()
}

// Assignment snapshots are message scoped; transaction-bound repositories must
// always bypass them when validating a mutation.
func CachePositionAssignment(ctx context.Context, session int32, callsign string, assignment *models.StandAssignment) {
	if state := GetWebsocketMessageState(ctx); state != nil {
		state.AssignmentSession = session
		state.AssignmentCallsign = callsign
		state.Assignment = assignment
		state.AssignmentLoaded = true
	}
}

func InvalidatePositionAssignment(ctx context.Context) {
	if state := GetWebsocketMessageState(ctx); state != nil {
		state.AssignmentLoaded = false
	}
}

// PublishStripUpdate preserves attribution for context-aware production hubs.
func PublishStripUpdate(ctx context.Context, publisher interface{ SendStripUpdate(int32, string) }, session int32, callsign string) {
	if contextual, ok := publisher.(interface {
		SendStripUpdateContext(context.Context, int32, string)
	}); ok {
		contextual.SendStripUpdateContext(ctx, session, callsign)
	} else {
		publisher.SendStripUpdate(session, callsign)
	}
}
