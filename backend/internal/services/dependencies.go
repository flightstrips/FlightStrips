package services

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"time"
)

type StripCdmService interface {
	shared.CdmService
	PrepareEuroscopeEobtSync(session int32, data *internalModels.CdmData, eobt string, now time.Time) (*internalModels.CdmData, string, bool)
}

type StripEuroscopeCommander interface {
	shared.EuroscopeStripCommander
	SendEobt(session int32, cid string, callsign string, eobt string)
}

type StripLifecycleStore interface {
	Create(ctx context.Context, strip *internalModels.Strip) error
	Update(ctx context.Context, strip *internalModels.Strip) (int64, error)
	Delete(ctx context.Context, session int32, callsign string) error
}

type StripReader interface {
	GetByCallsign(ctx context.Context, session int32, callsign string) (*internalModels.Strip, error)
	List(ctx context.Context, session int32) ([]*internalModels.Strip, error)
}

type StripOrderingStore interface {
	UpdateBayAndSequence(ctx context.Context, session int32, callsign string, bay string, sequence int32) (int64, error)
	UpdateSequenceBulk(ctx context.Context, session int32, callsigns []string, sequences []int32) error
	RecalculateSequences(ctx context.Context, session int32, bay string, spacing int32) error
	ListSequences(ctx context.Context, session int32, bay string) ([]*internalModels.StripSequence, error)
	GetSequence(ctx context.Context, session int32, callsign string, bay string) (int32, error)
	GetMaxSequenceInBay(ctx context.Context, session int32, bay string) (int32, error)
	GetNextSequence(ctx context.Context, session int32, bay string, sequence int32) (int32, error)
}

type StripFieldStore interface {
	UpdateSquawk(ctx context.Context, session int32, callsign string, squawk *string, version *int32) (int64, error)
	UpdateAssignedSquawk(ctx context.Context, session int32, callsign string, assignedSquawk *string, version *int32) (int64, error)
	UpdateClearedAltitude(ctx context.Context, session int32, callsign string, altitude *int32, version *int32) (int64, error)
	UpdateRequestedAltitude(ctx context.Context, session int32, callsign string, altitude *int32, version *int32) (int64, error)
	UpdateCommunicationType(ctx context.Context, session int32, callsign string, commType *string, version *int32) (int64, error)
	UpdateGroundState(ctx context.Context, session int32, callsign string, state *string, bay string, version *int32) (int64, error)
	UpdateClearedFlag(ctx context.Context, session int32, callsign string, cleared bool, bay string, version *int32) (int64, error)
	UpdateAircraftPosition(ctx context.Context, session int32, callsign string, lat *float64, lon *float64, alt *int32, bay string, version *int32) (int64, error)
	UpdateBay(ctx context.Context, session int32, callsign string, bay string, version *int32) (int64, error)
	UpdateHeading(ctx context.Context, session int32, callsign string, heading *int32, version *int32) (int64, error)
	UpdateStand(ctx context.Context, session int32, callsign string, stand *string, version *int32) (int64, error)
	UpdateRunway(ctx context.Context, session int32, callsign string, runway *string, version *int32) (int64, error)
	UpdateStartReq(ctx context.Context, session int32, callsign string, startReq bool, version *int32) (int64, error)
	UpdateMarked(ctx context.Context, session int32, callsign string, marked bool, version *int32) (int64, error)
	UpdateTrackingController(ctx context.Context, session int32, callsign string, trackingController string) (int64, error)
	UpdateRunwayClearance(ctx context.Context, session int32, callsign string) (int64, error)
	UpdateRunwayConfirmation(ctx context.Context, session int32, callsign string) (int64, error)
	ResetRunwayClearance(ctx context.Context, session int32, callsign string) (int64, error)
	UpdateReleasePoint(ctx context.Context, session int32, callsign string, releasePoint *string) (int64, error)
	AppendUnexpectedChangeField(ctx context.Context, session int32, callsign string, fieldName string) error
}

type StripOwnerStore interface {
	SetOwner(ctx context.Context, session int32, callsign string, owner *string, version int32) (int64, error)
	SetPreviousOwners(ctx context.Context, session int32, callsign string, previousOwners []string) error
	SetNextAndPreviousOwners(ctx context.Context, session int32, callsign string, nextOwners []string, previousOwners []string) error
}

type StripCdmStore interface {
	SetCdmData(ctx context.Context, session int32, callsign string, data *internalModels.CdmData) (int64, error)
}

type StripValidationStatusStore interface {
	SetValidationStatus(ctx context.Context, session int32, callsign string, status *internalModels.ValidationStatus) error
	AcknowledgeValidationStatus(ctx context.Context, session int32, callsign string, activationKey string) (int64, error)
	ClearValidationStatus(ctx context.Context, session int32, callsign string) error
}

type StripManualFplStore interface {
	UpdateIFRManualFPLFields(ctx context.Context, session int32, callsign string, destination string, sid *string, assignedSquawk *string, eobt *string, aircraftType *string, requestedAltitude *int32, route *string, stand *string, runway *string) (int64, error)
	UpdateVFRManualFPLFields(ctx context.Context, session int32, callsign string, aircraftType *string, personsOnBoard *int32, assignedSquawk string, fplType *string, language *string, remarks *string, bay string) (int64, error)
}

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
