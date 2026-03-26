package shared

import (
	"context"
)

type CdmService interface {
	TriggerRecalculate(ctx context.Context, session int32, airport string)
	HandleReadyRequest(ctx context.Context, session int32, callsign string) error
	HandleTobtUpdate(ctx context.Context, session int32, callsign string, tobt string, sourcePosition string, sourceRole string) error
	HandleDeiceUpdate(ctx context.Context, session int32, callsign string, deiceType string) error
	HandleAsrtToggle(ctx context.Context, session int32, callsign string, asrt string) error
	HandleTsacUpdate(ctx context.Context, session int32, callsign string, tsac string) error
	HandleManualCtot(ctx context.Context, session int32, callsign string, ctot string) error
	HandleCtotRemove(ctx context.Context, session int32, callsign string) error
	HandleApproveReqTobt(ctx context.Context, session int32, callsign string, sourcePosition string, sourceRole string) error
	SyncAsatForGroundState(ctx context.Context, session int32, callsign string, groundState string) error
	RequestBetterTobt(ctx context.Context, session int32, callsign string) error
	SetSessionCdmMaster(ctx context.Context, sessionID int32, master bool) error
}
