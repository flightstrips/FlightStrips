package shared

import "context"

type CdmService interface {
	HandleReadyRequest(ctx context.Context, session int32, callsign string) error
	RequestBetterTobt(ctx context.Context, session int32, callsign string) error
}
