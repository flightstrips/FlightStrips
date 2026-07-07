package ecfmp

import (
	"FlightStrips/internal/models"
	"context"
)

// StripStore is the strip persistence surface needed to apply ECFMP restrictions.
type StripStore interface {
	List(ctx context.Context, session int32) ([]*models.Strip, error)
	GetCdmData(ctx context.Context, session int32) ([]*models.CdmDataRow, error)
	SetCdmData(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error)
}
