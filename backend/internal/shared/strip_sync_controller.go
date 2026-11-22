package shared

import (
	"FlightStrips/internal/models"
	"context"
	"strings"
)

type stripSyncControllerKey struct{}
type stripSyncWriteBasisKey struct{}
type stripSyncLifecycleRestartKey struct{}

// WithStripSyncController carries the authenticated sender, not the tracking
// controller claimed by an incoming strip snapshot.
func WithStripSyncController(ctx context.Context, callsign string) context.Context {
	return context.WithValue(ctx, stripSyncControllerKey{}, strings.TrimSpace(callsign))
}

func StripSyncController(ctx context.Context) (string, bool) {
	callsign, present := ctx.Value(stripSyncControllerKey{}).(string)
	return callsign, present
}

// WithStripSyncWriteBasis identifies the persisted strip snapshot used to
// prepare a full-sync write so persistence can preserve intervening clearances.
func WithStripSyncWriteBasis(ctx context.Context, strip *models.Strip) context.Context {
	return context.WithValue(ctx, stripSyncWriteBasisKey{}, strip)
}

func StripSyncWriteBasis(ctx context.Context) *models.Strip {
	strip, _ := ctx.Value(stripSyncWriteBasisKey{}).(*models.Strip)
	return strip
}

func WithStripSyncLifecycleRestart(ctx context.Context, restart bool) context.Context {
	return context.WithValue(ctx, stripSyncLifecycleRestartKey{}, restart)
}

func StripSyncLifecycleRestart(ctx context.Context) bool {
	restart, _ := ctx.Value(stripSyncLifecycleRestartKey{}).(bool)
	return restart
}

func IsTrackingController(sender, trackingController string) bool {
	sender = strings.TrimSpace(sender)
	return sender != "" && strings.EqualFold(sender, strings.TrimSpace(trackingController))
}
