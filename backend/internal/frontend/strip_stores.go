package frontend

import (
	internalModels "FlightStrips/internal/models"
	"context"
)

// FrontendStripUpdateStore is the strip persistence surface needed by frontend field updates.
type FrontendStripUpdateStore interface {
	GetByCallsign(ctx context.Context, session int32, callsign string) (*internalModels.Strip, error)
	AppendControllerModifiedField(ctx context.Context, session int32, callsign string, fieldName string) error
	UpdateRunway(ctx context.Context, session int32, callsign string, runway *string, version *int32) (int64, error)
}

// SnapshotStripStore is the strip persistence surface needed to build initial snapshots.
type SnapshotStripStore interface {
	List(ctx context.Context, session int32) ([]*internalModels.Strip, error)
}
