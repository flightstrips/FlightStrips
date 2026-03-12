package testutil

import (
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MockServer is a configurable mock for shared.Server.
type MockServer struct {
	FrontendHubVal    shared.FrontendHub
	CoordRepoVal      repository.CoordinationRepository
	ControllerRepoVal repository.ControllerRepository

	UpdateSectorsFn          func(sessionId int32) ([]shared.SectorChange, error)
	UpdateRouteForStripFn    func(callsign string, sessionId int32, sendUpdate bool) error
	UpdateRoutesForSessionFn func(sessionId int32, sendUpdate bool) error
	UpdateLayoutsFn          func(sessionId int32) error
}

func (m *MockServer) GetDatabasePool() *pgxpool.Pool { return nil }

func (m *MockServer) GetEuroscopeHub() shared.EuroscopeHub { return nil }

func (m *MockServer) GetFrontendHub() shared.FrontendHub { return m.FrontendHubVal }

func (m *MockServer) GetOrCreateSession(airport string, name string) (shared.Session, error) {
	return shared.Session{}, nil
}

func (m *MockServer) GetCdmService() shared.CdmService { return nil }

func (m *MockServer) GetPdcService() shared.PdcService { return nil }

func (m *MockServer) GetStripRepository() repository.StripRepository { return nil }

func (m *MockServer) GetControllerRepository() repository.ControllerRepository {
	return m.ControllerRepoVal
}

func (m *MockServer) GetSessionRepository() repository.SessionRepository { return nil }

func (m *MockServer) GetSectorOwnerRepository() repository.SectorOwnerRepository { return nil }

func (m *MockServer) GetCoordinationRepository() repository.CoordinationRepository {
	return m.CoordRepoVal
}

func (m *MockServer) GetTacticalStripRepository() repository.TacticalStripRepository { return nil }

func (m *MockServer) UpdateSectors(sessionId int32) ([]shared.SectorChange, error) {
	if m.UpdateSectorsFn != nil {
		return m.UpdateSectorsFn(sessionId)
	}
	return nil, nil
}

func (m *MockServer) UpdateRouteForStrip(callsign string, sessionId int32, sendUpdate bool) error {
	if m.UpdateRouteForStripFn != nil {
		return m.UpdateRouteForStripFn(callsign, sessionId, sendUpdate)
	}
	return nil
}

func (m *MockServer) UpdateRoutesForSession(sessionId int32, sendUpdate bool) error {
	if m.UpdateRoutesForSessionFn != nil {
		return m.UpdateRoutesForSessionFn(sessionId, sendUpdate)
	}
	return nil
}

func (m *MockServer) UpdateLayouts(sessionId int32) error {
	if m.UpdateLayoutsFn != nil {
		return m.UpdateLayoutsFn(sessionId)
	}
	return nil
}
