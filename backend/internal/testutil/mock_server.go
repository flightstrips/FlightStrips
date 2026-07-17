package testutil

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MockServer is a configurable mock for shared.Server.
type MockServer struct {
	FrontendHubVal       shared.FrontendHub
	EuroscopeHubVal      shared.EuroscopeHub
	CdmServiceVal        shared.CdmService
	CoordRepoVal         repository.CoordinationRepository
	ControllerRepoVal    repository.ControllerRepository
	SectorRepoVal        repository.SectorOwnerRepository
	SessionRepoVal       repository.SessionRepository
	StripRepoVal         repository.StripRepository
	TacticalStripRepoVal repository.TacticalStripRepository

	GetOrCreateSessionFn                func(airport string, name string) (shared.Session, error)
	UpdateSectorsFn                     func(sessionId int32) ([]shared.SectorChange, error)
	RecalculateSessionFn                func(sessionId int32, sendUpdate bool) ([]shared.SectorChange, error)
	RecalculateSessionContextFn         func(ctx context.Context, sessionId int32, sendUpdate bool) ([]shared.SectorChange, error)
	UpdateRouteForStripFn               func(callsign string, sessionId int32, sendUpdate bool) error
	UpdateRouteForStripCtxFn            func(ctx context.Context, callsign string, sessionId int32, sendUpdate bool) error
	UpdateRoutesForSessionFn            func(sessionId int32, sendUpdate bool) error
	UpdateLayoutsFn                     func(sessionId int32) error
	ComputeNextDisplayForStripContextFn func(ctx context.Context, strip *internalModels.Strip, sessionId int32) (*internalModels.NextDisplay, error)
}

func (m *MockServer) GetDatabasePool() *pgxpool.Pool { return nil }

func (m *MockServer) GetEuroscopeHub() shared.EuroscopeHub { return m.EuroscopeHubVal }

func (m *MockServer) GetFrontendHub() shared.FrontendHub { return m.FrontendHubVal }

func (m *MockServer) GetOrCreateSession(airport string, name string) (shared.Session, error) {
	if m.GetOrCreateSessionFn != nil {
		return m.GetOrCreateSessionFn(airport, name)
	}
	return shared.Session{}, nil
}

func (m *MockServer) GetCdmService() shared.CdmService { return m.CdmServiceVal }

func (m *MockServer) GetStripRepository() repository.StripRepository { return m.StripRepoVal }

func (m *MockServer) GetControllerRepository() repository.ControllerRepository {
	return m.ControllerRepoVal
}

func (m *MockServer) GetSessionRepository() repository.SessionRepository { return m.SessionRepoVal }

func (m *MockServer) GetSectorOwnerRepository() repository.SectorOwnerRepository {
	return m.SectorRepoVal
}

func (m *MockServer) GetCoordinationRepository() repository.CoordinationRepository {
	return m.CoordRepoVal
}

func (m *MockServer) GetTacticalStripRepository() repository.TacticalStripRepository {
	return m.TacticalStripRepoVal
}

func (m *MockServer) GetStandAssignmentRepository() repository.StandAssignmentRepository { return nil }

func (m *MockServer) UpdateSectors(sessionId int32) ([]shared.SectorChange, error) {
	if m.UpdateSectorsFn != nil {
		return m.UpdateSectorsFn(sessionId)
	}
	return nil, nil
}

func (m *MockServer) RecalculateSession(sessionId int32, sendUpdate bool) ([]shared.SectorChange, error) {
	if m.RecalculateSessionFn != nil {
		return m.RecalculateSessionFn(sessionId, sendUpdate)
	}
	return m.RecalculateSessionContext(context.Background(), sessionId, sendUpdate)
}

func (m *MockServer) RecalculateSessionContext(ctx context.Context, sessionId int32, sendUpdate bool) ([]shared.SectorChange, error) {
	if m.RecalculateSessionContextFn != nil {
		return m.RecalculateSessionContextFn(ctx, sessionId, sendUpdate)
	}

	changes, err := m.UpdateSectors(sessionId)
	if err != nil {
		return nil, err
	}
	if err := m.UpdateLayouts(sessionId); err != nil {
		return nil, err
	}
	if err := m.UpdateRoutesForSession(sessionId, sendUpdate); err != nil {
		return nil, err
	}
	return changes, nil
}

func (m *MockServer) UpdateRouteForStrip(callsign string, sessionId int32, sendUpdate bool) error {
	if m.UpdateRouteForStripFn != nil {
		return m.UpdateRouteForStripFn(callsign, sessionId, sendUpdate)
	}
	return nil
}

func (m *MockServer) UpdateRouteForStripContext(ctx context.Context, callsign string, sessionId int32, sendUpdate bool) error {
	if m.UpdateRouteForStripCtxFn != nil {
		return m.UpdateRouteForStripCtxFn(ctx, callsign, sessionId, sendUpdate)
	}
	return m.UpdateRouteForStrip(callsign, sessionId, sendUpdate)
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

func (m *MockServer) ComputeNextDisplayForStripContext(ctx context.Context, strip *internalModels.Strip, sessionId int32) (*internalModels.NextDisplay, error) {
	if m.ComputeNextDisplayForStripContextFn != nil {
		return m.ComputeNextDisplayForStripContextFn(ctx, strip, sessionId)
	}
	return nil, nil
}
