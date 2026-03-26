package testutil

import (
	"FlightStrips/internal/models"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"time"
)

// MockSessionRepository is a configurable mock for repository.SessionRepository.
type MockSessionRepository struct {
	GetByIDFn             func(ctx context.Context, id int32) (*models.Session, error)
	ListFn                func(ctx context.Context) ([]*models.Session, error)
	UpdateActiveRunwaysFn func(ctx context.Context, id int32, activeRunways pkgModels.ActiveRunways) error
	UpdateCdmMasterFn     func(ctx context.Context, id int32, master bool) error
}

func (m *MockSessionRepository) Create(ctx context.Context, session *models.Session) (int32, error) {
	panic("unexpected call to MockSessionRepository.Create")
}

func (m *MockSessionRepository) Get(ctx context.Context, name string, airport string) (*models.Session, error) {
	panic("unexpected call to MockSessionRepository.Get")
}

func (m *MockSessionRepository) GetByID(ctx context.Context, id int32) (*models.Session, error) {
	if m.GetByIDFn == nil {
		panic("unexpected call to MockSessionRepository.GetByID")
	}
	return m.GetByIDFn(ctx, id)
}

func (m *MockSessionRepository) GetByNameAndAirport(ctx context.Context, name string, airport string) (*models.Session, error) {
	panic("unexpected call to MockSessionRepository.GetByNameAndAirport")
}

func (m *MockSessionRepository) GetByNames(ctx context.Context, name string) ([]*models.Session, error) {
	panic("unexpected call to MockSessionRepository.GetByNames")
}

func (m *MockSessionRepository) GetExpiredSessions(ctx context.Context, expiredBefore *time.Time) ([]*models.Session, error) {
	panic("unexpected call to MockSessionRepository.GetExpiredSessions")
}

func (m *MockSessionRepository) List(ctx context.Context) ([]*models.Session, error) {
	if m.ListFn == nil {
		panic("unexpected call to MockSessionRepository.List")
	}
	return m.ListFn(ctx)
}

func (m *MockSessionRepository) Delete(ctx context.Context, id int32) (int64, error) {
	panic("unexpected call to MockSessionRepository.Delete")
}

func (m *MockSessionRepository) UpdateActiveRunways(ctx context.Context, id int32, activeRunways pkgModels.ActiveRunways) error {
	if m.UpdateActiveRunwaysFn != nil {
		return m.UpdateActiveRunwaysFn(ctx, id, activeRunways)
	}
	return nil
}

func (m *MockSessionRepository) UpdateCdmMaster(ctx context.Context, id int32, master bool) error {
	if m.UpdateCdmMasterFn != nil {
		return m.UpdateCdmMasterFn(ctx, id, master)
	}
	return nil
}

func (m *MockSessionRepository) UpdateSessionSids(ctx context.Context, id int32, sids pkgModels.AvailableSids) error {
	return nil
}

func (m *MockSessionRepository) GetSessionSids(ctx context.Context, id int32) (pkgModels.AvailableSids, error) {
	return pkgModels.AvailableSids{}, nil
}

func (m *MockSessionRepository) IncrementPdcSequence(ctx context.Context, id int32) (int32, error) {
	panic("unexpected call to MockSessionRepository.IncrementPdcSequence")
}

func (m *MockSessionRepository) IncrementPdcMessageSequence(ctx context.Context, id int32) (int32, error) {
	panic("unexpected call to MockSessionRepository.IncrementPdcMessageSequence")
}
