package shared

import "context"

type StripService interface {
	MoveToBay(ctx context.Context, session int32, callsign string, bay string, sendNotification bool) error
	MoveStripBetween(ctx context.Context, session int32, callsign string, before *string, bay string) error
	ClearStrip(ctx context.Context, session int32, callsign string, cid string) error
	UnclearStrip(ctx context.Context, session int32, callsign string, cid string) error
	AutoAssumeForClearedStrip(ctx context.Context, session int32, callsign string, stripVersion int32) error
}
