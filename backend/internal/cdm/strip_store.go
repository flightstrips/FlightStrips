package cdm

import (
	"FlightStrips/internal/models"
	"context"
)

// CdmStripStore is the strip persistence surface needed by CDM actions and sync.
type CdmStripStore interface {
	GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error)
	ListByOrigin(ctx context.Context, session int32, origin string) ([]*models.Strip, error)
	GetCdmData(ctx context.Context, session int32) ([]*models.CdmDataRow, error)
	GetCdmDataForCallsign(ctx context.Context, session int32, callsign string) (*models.CdmData, error)
	SetCdmData(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error)
}

// CdmSequenceStripStore is the strip persistence surface needed by CDM sequencing.
type CdmSequenceStripStore interface {
	ListByOrigin(ctx context.Context, session int32, origin string) ([]*models.Strip, error)
	SetCdmData(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error)
}
