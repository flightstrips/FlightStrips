package repository

import (
	"FlightStrips/internal/models"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// StripRepository defines the interface for strip data access
type StripRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, strip *models.Strip) error
	GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error)
	List(ctx context.Context, session int32) ([]*models.Strip, error)
	Update(ctx context.Context, strip *models.Strip) (int64, error)
	Delete(ctx context.Context, session int32, callsign string) error

	// Specialized queries
	ListByOrigin(ctx context.Context, session int32, origin string) ([]*models.Strip, error)
	GetBay(ctx context.Context, session int32, callsign string) (string, error)

	// Sequence management
	UpdateSequence(ctx context.Context, session int32, callsign string, sequence int32) (int64, error)
	UpdateBayAndSequence(ctx context.Context, session int32, callsign string, bay string, sequence int32) (int64, error)
	UpdateSequenceBulk(ctx context.Context, session int32, callsigns []string, sequences []int32) error
	RecalculateSequences(ctx context.Context, session int32, bay string, spacing int32) error
	ListSequences(ctx context.Context, session int32, bay string) ([]*models.StripSequence, error)
	GetSequence(ctx context.Context, session int32, callsign string, bay string) (int32, error)
	GetMaxSequenceInBay(ctx context.Context, session int32, bay string) (int32, error)
	GetMinSequenceInBay(ctx context.Context, session int32, bay string) (int32, error)
	GetNextSequence(ctx context.Context, session int32, bay string, sequence int32) (int32, error)
	GetPrevSequence(ctx context.Context, session int32, bay string, sequence int32, excludeCallsign string) (int32, error)

	// Field updates
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
	UpdateMarked(ctx context.Context, session int32, callsign string, marked bool, version *int32) (int64, error)
	UpdateRegistration(ctx context.Context, session int32, callsign string, registration string) error
	UpdateTrackingController(ctx context.Context, session int32, callsign string, trackingController string) (int64, error)
	UpdateRunwayClearance(ctx context.Context, session int32, callsign string) (int64, error)
	UpdateRunwayConfirmation(ctx context.Context, session int32, callsign string) (int64, error)
	ResetRunwayClearance(ctx context.Context, session int32, callsign string) (int64, error)

	// Owner management
	SetOwner(ctx context.Context, session int32, callsign string, owner *string, version int32) (int64, error)
	SetNextOwners(ctx context.Context, session int32, callsign string, nextOwners []string) error
	SetPreviousOwners(ctx context.Context, session int32, callsign string, previousOwners []string) error
	SetNextAndPreviousOwners(ctx context.Context, session int32, callsign string, nextOwners []string, previousOwners []string) error

	// CDM data
	GetCdmData(ctx context.Context, session int32) ([]*models.CdmDataRow, error)
	GetCdmDataForCallsign(ctx context.Context, session int32, callsign string) (*models.CdmData, error)
	SetCdmData(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error)

	// Release point
	UpdateReleasePoint(ctx context.Context, session int32, callsign string, releasePoint *string) (int64, error)

	// Unexpected change tracking
	AppendUnexpectedChangeField(ctx context.Context, session int32, callsign string, fieldName string) error
	RemoveUnexpectedChangeField(ctx context.Context, session int32, callsign string, fieldName string) error

	// Controller-modified field tracking
	AppendControllerModifiedField(ctx context.Context, session int32, callsign string, fieldName string) error

	// Manual FPL creation
	UpdateIFRManualFPLFields(ctx context.Context, session int32, callsign string, destination string, sid *string, assignedSquawk *string, eobt *string, aircraftType *string, requestedAltitude *int32, route *string, stand *string, runway *string) (int64, error)
	UpdateVFRManualFPLFields(ctx context.Context, session int32, callsign string, aircraftType *string, personsOnBoard *int32, assignedSquawk string, fplType *string, language *string, remarks *string, bay string) (int64, error)
	SetHasFP(ctx context.Context, session int32, callsign string, hasFP bool) error

	// PDC methods
	SetPdcRequested(ctx context.Context, session int32, callsign string, pdcState string, pdcRequestedAt *time.Time, pdcRequestRemarks *string) error
	SetPdcMessageSent(ctx context.Context, session int32, callsign string, pdcState string, pdcMessageSequence *int32, pdcMessageSent *time.Time) error
	UpdatePdcStatus(ctx context.Context, session int32, callsign string, pdcState string) error
}

// ControllerRepository defines the interface for controller data access
type ControllerRepository interface {
	Create(ctx context.Context, controller *models.Controller) error
	Get(ctx context.Context, callsign string, session int32) (*models.Controller, error)
	GetByCid(ctx context.Context, cid string) (*models.Controller, error)
	GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Controller, error)
	GetByPosition(ctx context.Context, session int32, position string) ([]*models.Controller, error)
	List(ctx context.Context, session int32) ([]*models.Controller, error)
	ListBySession(ctx context.Context, session int32) ([]*models.Controller, error)
	Delete(ctx context.Context, session int32, callsign string) error

	SetPosition(ctx context.Context, session int32, callsign string, position string) (int64, error)
	SetCid(ctx context.Context, session int32, callsign string, cid *string) (int64, error)
	SetLayout(ctx context.Context, session int32, position string, layout *string) (int64, error)
	SetEuroscopeSeen(ctx context.Context, cid string, session int32, lastSeen *time.Time) (int64, error)
	SetFrontendSeen(ctx context.Context, cid string, session int32, lastSeen *time.Time) (int64, error)
}

// SessionRepository defines the interface for session data access
type SessionRepository interface {
	Create(ctx context.Context, session *models.Session) (int32, error)
	Get(ctx context.Context, name string, airport string) (*models.Session, error)
	GetByID(ctx context.Context, id int32) (*models.Session, error)
	GetByNameAndAirport(ctx context.Context, name string, airport string) (*models.Session, error)
	GetByNames(ctx context.Context, name string) ([]*models.Session, error)
	GetExpiredSessions(ctx context.Context, expiredBefore *time.Time) ([]*models.Session, error)
	List(ctx context.Context) ([]*models.Session, error)
	Delete(ctx context.Context, id int32) (int64, error)

	UpdateActiveRunways(ctx context.Context, id int32, activeRunways pkgModels.ActiveRunways) error
	UpdateSessionSids(ctx context.Context, id int32, sids pkgModels.AvailableSids) error
	GetSessionSids(ctx context.Context, id int32) (pkgModels.AvailableSids, error)
	UpdateCdmMaster(ctx context.Context, id int32, master bool) error
	IncrementPdcSequence(ctx context.Context, id int32) (int32, error)
	IncrementPdcMessageSequence(ctx context.Context, id int32) (int32, error)
}

// CoordinationRepository defines the interface for coordination data access
type CoordinationRepository interface {
	Create(ctx context.Context, coordination *models.Coordination) error
	GetByID(ctx context.Context, id int32) (*models.Coordination, error)
	GetByStripID(ctx context.Context, session int32, stripID int32) (*models.Coordination, error)
	GetByStripCallsign(ctx context.Context, session int32, callsign string) (*models.Coordination, error)
	ListBySession(ctx context.Context, session int32) ([]*models.Coordination, error)
	ListByStrip(ctx context.Context, session int32, stripID int32) ([]*models.Coordination, error)
	Delete(ctx context.Context, id int32) error
}

// SectorOwnerRepository defines the interface for sector owner data access
type SectorOwnerRepository interface {
	CreateBulk(ctx context.Context, owner []*models.SectorOwner) error
	GetByID(ctx context.Context, id int32) (*models.SectorOwner, error)
	ListBySession(ctx context.Context, session int32) ([]*models.SectorOwner, error)
	Delete(ctx context.Context, id int32) error
	DeleteAllBySession(ctx context.Context, session int32) error
	RemoveBySession(ctx context.Context, session int32) error
	WithTx(tx pgx.Tx) SectorOwnerRepository
}

// TacticalStripRepository defines the interface for tactical strip data access
type TacticalStripRepository interface {
	Create(ctx context.Context, sessionID int32, stripType, bay, label string, aircraft *string, producedBy string, sequence int32) (*models.TacticalStrip, error)
	ListBySession(ctx context.Context, sessionID int32) ([]*models.TacticalStrip, error)
	Delete(ctx context.Context, id int64, sessionID int32) error
	Confirm(ctx context.Context, id int64, sessionID int32, confirmedBy string) (*models.TacticalStrip, error)
	StartTimer(ctx context.Context, id int64, sessionID int32) (*models.TacticalStrip, error)
	UpdateBayAndSequence(ctx context.Context, id int64, sessionID int32, bay string, sequence int32) (*models.TacticalStrip, error)
	UpdateSequence(ctx context.Context, id int64, sessionID int32, sequence int32) (*models.TacticalStrip, error)
	GetSequenceByID(ctx context.Context, id int64, sessionID int32) (int32, error)
	ListBaySequences(ctx context.Context, sessionID int32, bay string) ([]*models.TacticalStripSequence, error)
	GetMaxSequenceInBayUnified(ctx context.Context, session int32, bay string) (int32, error)
	GetNextSequenceUnified(ctx context.Context, session int32, bay string, prev int32) (int32, error)
	GetPrevSequenceUnified(ctx context.Context, session int32, bay string, seq int32, excludeCallsign string) (int32, error)
}
