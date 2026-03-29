package shared

import (
	"FlightStrips/internal/repository"
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ServerInjectable interface {
	GetServer() Server
	SetServer(server Server)
}

type Session struct {
	Id      int32
	Name    string
	Airport string
}

// PdcIssueClearanceParams configures IssueClearance (CPDLC and/or web delivery).
type PdcIssueClearanceParams struct {
	Callsign     string
	Remarks      string
	CID          string
	SessionID    int32
	Atis         string // empty defaults to "A" in implementation
	SkipCPDLC    bool   // when true, do not send Hoppie CPDLC (e.g. web-only pilot)
	WebRequestID *int64 // when set with SkipCPDLC, persist clearance text to this row
}

type PdcService interface {
	IssueClearance(ctx context.Context, p PdcIssueClearanceParams) error
	ManualStateChange(ctx context.Context, callsign string, sessionID int32, newState string) error
	RevertToVoice(ctx context.Context, callsign string, sessionID int32, cid string) error
}

// Alias for repository type used in handlers
type StripRepository = repository.StripRepository

type Server interface {
	GetDatabasePool() *pgxpool.Pool
	GetEuroscopeHub() EuroscopeHub
	GetFrontendHub() FrontendHub
	GetOrCreateSession(airport string, name string) (Session, error)
	GetCdmService() CdmService
	GetPdcService() PdcService

	// Repository access
	GetStripRepository() repository.StripRepository
	GetControllerRepository() repository.ControllerRepository
	GetSessionRepository() repository.SessionRepository
	GetSectorOwnerRepository() repository.SectorOwnerRepository
	GetCoordinationRepository() repository.CoordinationRepository
	GetTacticalStripRepository() repository.TacticalStripRepository

	// TODO move to another service
	UpdateSectors(sessionId int32) ([]SectorChange, error)
	UpdateRouteForStrip(callsign string, sessionId int32, sendUpdate bool) error
	UpdateRoutesForSession(sessionId int32, sendUpdate bool) error
	UpdateLayouts(sessionId int32) error
}

type ConnectedUser struct {
	Cid      string
	Session  int32
	Position string
	Callsign string
	Airport  string
}
