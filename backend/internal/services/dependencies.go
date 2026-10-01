package services

import (
	"context"
)

type RouteRecalculator interface {
	UpdateRouteForStrip(callsign string, sessionID int32, sendUpdate bool) error
	UpdateRouteForStripContext(ctx context.Context, callsign string, sessionID int32, sendUpdate bool) error
}
