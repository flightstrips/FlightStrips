package services

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
)

type RouteRecalculator interface {
	UpdateRouteForStrip(callsign string, sessionID int32, sendUpdate bool) error
	UpdateRouteForStripContext(ctx context.Context, callsign string, sessionID int32, sendUpdate bool) error
}

type StripRouteComputer interface {
	ComputeNextOwnersForStripContext(ctx context.Context, strip *internalModels.Strip, sessionID int32) ([]string, bool, error)
}

type SessionReader interface {
	GetByID(ctx context.Context, id int32) (*internalModels.Session, error)
}

type ControllerReader interface {
	GetByCid(ctx context.Context, cid string) (*internalModels.Controller, error)
	GetByCallsign(ctx context.Context, session int32, callsign string) (*internalModels.Controller, error)
	GetByPosition(ctx context.Context, session int32, position string) ([]*internalModels.Controller, error)
	ListBySession(ctx context.Context, session int32) ([]*internalModels.Controller, error)
}

type CoordinationStore interface {
	Create(ctx context.Context, coordination *internalModels.Coordination) error
	GetByStripID(ctx context.Context, session int32, stripID int32) (*internalModels.Coordination, error)
	GetByStripCallsign(ctx context.Context, session int32, callsign string) (*internalModels.Coordination, error)
	Delete(ctx context.Context, id int32) error
}

type FrontendNotifier interface {
	SendControllerOffline(session int32, callsign string, position string, identifier string)
}

type SessionRecalculator interface {
	RecalculateSessionContext(ctx context.Context, sessionID int32, sendUpdate bool) ([]shared.SectorChange, error)
}
