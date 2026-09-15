package shared

import (
	"context"
	"strings"
)

type stripSyncControllerKey struct{}

// WithStripSyncController carries the authenticated sender, not the tracking
// controller claimed by an incoming strip snapshot.
func WithStripSyncController(ctx context.Context, callsign string) context.Context {
	return context.WithValue(ctx, stripSyncControllerKey{}, strings.TrimSpace(callsign))
}

func StripSyncController(ctx context.Context) (string, bool) {
	callsign, present := ctx.Value(stripSyncControllerKey{}).(string)
	return callsign, present
}

func IsTrackingController(sender, trackingController string) bool {
	sender = strings.TrimSpace(sender)
	return sender != "" && strings.EqualFold(sender, strings.TrimSpace(trackingController))
}
