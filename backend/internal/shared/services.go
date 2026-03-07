package shared

import (
	"FlightStrips/pkg/events/frontend"
	"context"
)

type StripService interface {
	MoveToBay(ctx context.Context, session int32, callsign string, bay string, sendNotification bool) error
	MoveStripBetween(ctx context.Context, session int32, callsign string, insertAfter *frontend.StripRef, bay string) error
	MoveTacticalStripBetween(ctx context.Context, session int32, id int64, insertAfter *frontend.StripRef, bay string) error
	ClearStrip(ctx context.Context, session int32, callsign string, cid string) error
	UnclearStrip(ctx context.Context, session int32, callsign string, cid string) error
	AutoAssumeForClearedStrip(ctx context.Context, session int32, callsign string, stripVersion int32) error
}
