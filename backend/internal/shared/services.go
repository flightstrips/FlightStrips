package shared

import "context"

type StripService interface {
	MoveToBay(ctx context.Context, session int32, callsign string, bay string, sendNotification bool) error
	MoveStripBetween(ctx context.Context, session int32, callsign string, before *string, bay string) error
}
