package shared

import (
	"FlightStrips/pkg/events/euroscope"
	"context"
)

type CdmService interface {
	HandleReadyRequest(ctx context.Context, session int32, callsign string) error
	HandleLocalObservation(ctx context.Context, session int32, observation euroscope.CdmLocalDataEvent) error
	RequestBetterTobt(ctx context.Context, session int32, callsign string) error
}
