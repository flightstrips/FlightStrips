package testutil

import (
	"FlightStrips/internal/models"
	"context"
	"time"
)

// MockControllerRepository is a configurable mock for repository.ControllerRepository.
type MockControllerRepository struct {
	CreateFn           func(ctx context.Context, controller *models.Controller) error
	GetFn              func(ctx context.Context, callsign string, session int32) (*models.Controller, error)
	GetByCidFn         func(ctx context.Context, cid string) (*models.Controller, error)
	GetByCallsignFn    func(ctx context.Context, session int32, callsign string) (*models.Controller, error)
	GetByPositionFn    func(ctx context.Context, session int32, position string) ([]*models.Controller, error)
	ListFn             func(ctx context.Context, session int32) ([]*models.Controller, error)
	ListBySessionFn    func(ctx context.Context, session int32) ([]*models.Controller, error)
	DeleteFn           func(ctx context.Context, session int32, callsign string) error
	SetPositionFn      func(ctx context.Context, session int32, callsign string, position string) (int64, error)
	SetCidFn           func(ctx context.Context, session int32, callsign string, cid *string) (int64, error)
	SetObserverFn      func(ctx context.Context, session int32, callsign string, observer bool) (int64, error)
	SetLayoutFn        func(ctx context.Context, session int32, position string, layout *string) (int64, error)
	SetEuroscopeSeenFn func(ctx context.Context, cid string, session int32, lastSeen *time.Time) (int64, error)
	SetFrontendSeenFn  func(ctx context.Context, cid string, session int32, lastSeen *time.Time) (int64, error)
}

func (m *MockControllerRepository) Create(ctx context.Context, controller *models.Controller) error {
	if m.CreateFn == nil {
		panic("unexpected call to MockControllerRepository.Create")
	}
	return m.CreateFn(ctx, controller)
}

func (m *MockControllerRepository) Get(ctx context.Context, callsign string, session int32) (*models.Controller, error) {
	if m.GetFn == nil {
		panic("unexpected call to MockControllerRepository.Get")
	}
	return m.GetFn(ctx, callsign, session)
}

func (m *MockControllerRepository) GetByCid(ctx context.Context, cid string) (*models.Controller, error) {
	if m.GetByCidFn == nil {
		panic("unexpected call to MockControllerRepository.GetByCid")
	}
	return m.GetByCidFn(ctx, cid)
}

func (m *MockControllerRepository) GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Controller, error) {
	if m.GetByCallsignFn == nil {
		panic("unexpected call to MockControllerRepository.GetByCallsign")
	}
	return m.GetByCallsignFn(ctx, session, callsign)
}

func (m *MockControllerRepository) GetByPosition(ctx context.Context, session int32, position string) ([]*models.Controller, error) {
	if m.GetByPositionFn == nil {
		panic("unexpected call to MockControllerRepository.GetByPosition")
	}
	return m.GetByPositionFn(ctx, session, position)
}

func (m *MockControllerRepository) List(ctx context.Context, session int32) ([]*models.Controller, error) {
	if m.ListFn == nil {
		panic("unexpected call to MockControllerRepository.List")
	}
	return m.ListFn(ctx, session)
}

func (m *MockControllerRepository) ListBySession(ctx context.Context, session int32) ([]*models.Controller, error) {
	if m.ListBySessionFn == nil {
		panic("unexpected call to MockControllerRepository.ListBySession")
	}
	return m.ListBySessionFn(ctx, session)
}

func (m *MockControllerRepository) Delete(ctx context.Context, session int32, callsign string) error {
	if m.DeleteFn == nil {
		panic("unexpected call to MockControllerRepository.Delete")
	}
	return m.DeleteFn(ctx, session, callsign)
}

func (m *MockControllerRepository) SetPosition(ctx context.Context, session int32, callsign string, position string) (int64, error) {
	if m.SetPositionFn == nil {
		panic("unexpected call to MockControllerRepository.SetPosition")
	}
	return m.SetPositionFn(ctx, session, callsign, position)
}

func (m *MockControllerRepository) SetCid(ctx context.Context, session int32, callsign string, cid *string) (int64, error) {
	if m.SetCidFn == nil {
		panic("unexpected call to MockControllerRepository.SetCid")
	}
	return m.SetCidFn(ctx, session, callsign, cid)
}

func (m *MockControllerRepository) SetObserver(ctx context.Context, session int32, callsign string, observer bool) (int64, error) {
	if m.SetObserverFn == nil {
		panic("unexpected call to MockControllerRepository.SetObserver")
	}
	return m.SetObserverFn(ctx, session, callsign, observer)
}

func (m *MockControllerRepository) SetLayout(ctx context.Context, session int32, position string, layout *string) (int64, error) {
	if m.SetLayoutFn == nil {
		panic("unexpected call to MockControllerRepository.SetLayout")
	}
	return m.SetLayoutFn(ctx, session, position, layout)
}

func (m *MockControllerRepository) SetEuroscopeSeen(ctx context.Context, cid string, session int32, lastSeen *time.Time) (int64, error) {
	if m.SetEuroscopeSeenFn == nil {
		panic("unexpected call to MockControllerRepository.SetEuroscopeSeen")
	}
	return m.SetEuroscopeSeenFn(ctx, cid, session, lastSeen)
}

func (m *MockControllerRepository) SetFrontendSeen(ctx context.Context, cid string, session int32, lastSeen *time.Time) (int64, error) {
	if m.SetFrontendSeenFn == nil {
		panic("unexpected call to MockControllerRepository.SetFrontendSeen")
	}
	return m.SetFrontendSeenFn(ctx, cid, session, lastSeen)
}
