package pdc

import (
	"FlightStrips/internal/models"
	"context"
	"time"
)

// PdcStripStore is the strip persistence surface needed by the PDC workflow.
type PdcStripStore interface {
	GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error)
	List(ctx context.Context, session int32) ([]*models.Strip, error)
	Update(ctx context.Context, strip *models.Strip) (int64, error)
	SetPdcData(ctx context.Context, session int32, callsign string, data *models.PdcData) error
	SetPdcRequested(ctx context.Context, session int32, callsign string, pdcState string, pdcRequestedAt *time.Time, pdcRequestRemarks *string) error
	SetPdcMessageSent(ctx context.Context, session int32, callsign string, pdcState string, pdcMessageSequence *int32, pdcMessageSent *time.Time) error
	UpdatePdcStatus(ctx context.Context, session int32, callsign string, pdcState string) error
}
