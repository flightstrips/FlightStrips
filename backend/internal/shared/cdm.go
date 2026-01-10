package shared

import "context"

type CdmService interface {
	RequestBetterTobt(ctx context.Context, session int32, callsign string) error
}
