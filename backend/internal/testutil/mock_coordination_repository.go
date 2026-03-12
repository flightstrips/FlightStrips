package testutil

import (
	"FlightStrips/internal/models"
	"context"
)

// MockCoordinationRepository is a configurable mock for repository.CoordinationRepository.
type MockCoordinationRepository struct {
	CreateFn           func(ctx context.Context, coordination *models.Coordination) error
	GetByIDFn          func(ctx context.Context, id int32) (*models.Coordination, error)
	GetByStripIDFn     func(ctx context.Context, session int32, stripID int32) (*models.Coordination, error)
	GetByStripCallsignFn func(ctx context.Context, session int32, callsign string) (*models.Coordination, error)
	ListBySessionFn    func(ctx context.Context, session int32) ([]*models.Coordination, error)
	ListByStripFn      func(ctx context.Context, session int32, stripID int32) ([]*models.Coordination, error)
	DeleteFn           func(ctx context.Context, id int32) error
}

func (m *MockCoordinationRepository) Create(ctx context.Context, coordination *models.Coordination) error {
	if m.CreateFn == nil {
		panic("unexpected call to MockCoordinationRepository.Create")
	}
	return m.CreateFn(ctx, coordination)
}

func (m *MockCoordinationRepository) GetByID(ctx context.Context, id int32) (*models.Coordination, error) {
	if m.GetByIDFn == nil {
		panic("unexpected call to MockCoordinationRepository.GetByID")
	}
	return m.GetByIDFn(ctx, id)
}

func (m *MockCoordinationRepository) GetByStripID(ctx context.Context, session int32, stripID int32) (*models.Coordination, error) {
	if m.GetByStripIDFn == nil {
		panic("unexpected call to MockCoordinationRepository.GetByStripID")
	}
	return m.GetByStripIDFn(ctx, session, stripID)
}

func (m *MockCoordinationRepository) GetByStripCallsign(ctx context.Context, session int32, callsign string) (*models.Coordination, error) {
	if m.GetByStripCallsignFn == nil {
		panic("unexpected call to MockCoordinationRepository.GetByStripCallsign")
	}
	return m.GetByStripCallsignFn(ctx, session, callsign)
}

func (m *MockCoordinationRepository) ListBySession(ctx context.Context, session int32) ([]*models.Coordination, error) {
	if m.ListBySessionFn == nil {
		panic("unexpected call to MockCoordinationRepository.ListBySession")
	}
	return m.ListBySessionFn(ctx, session)
}

func (m *MockCoordinationRepository) ListByStrip(ctx context.Context, session int32, stripID int32) ([]*models.Coordination, error) {
	if m.ListByStripFn == nil {
		panic("unexpected call to MockCoordinationRepository.ListByStrip")
	}
	return m.ListByStripFn(ctx, session, stripID)
}

func (m *MockCoordinationRepository) Delete(ctx context.Context, id int32) error {
	if m.DeleteFn == nil {
		panic("unexpected call to MockCoordinationRepository.Delete")
	}
	return m.DeleteFn(ctx, id)
}
