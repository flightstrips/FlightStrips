package testutil

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"context"

	"github.com/jackc/pgx/v5"
)

// MockSectorOwnerRepository is a configurable mock for repository.SectorOwnerRepository.
type MockSectorOwnerRepository struct {
	CreateBulkFn         func(ctx context.Context, owner []*models.SectorOwner) error
	GetByIDFn            func(ctx context.Context, id int32) (*models.SectorOwner, error)
	ListBySessionFn      func(ctx context.Context, session int32) ([]*models.SectorOwner, error)
	DeleteFn             func(ctx context.Context, id int32) error
	DeleteAllBySessionFn func(ctx context.Context, session int32) error
	RemoveBySessionFn    func(ctx context.Context, session int32) error
	WithTxFn             func(tx pgx.Tx) repository.SectorOwnerRepository
}

func (m *MockSectorOwnerRepository) CreateBulk(ctx context.Context, owner []*models.SectorOwner) error {
	if m.CreateBulkFn == nil {
		panic("unexpected call to MockSectorOwnerRepository.CreateBulk")
	}
	return m.CreateBulkFn(ctx, owner)
}

func (m *MockSectorOwnerRepository) GetByID(ctx context.Context, id int32) (*models.SectorOwner, error) {
	if m.GetByIDFn == nil {
		panic("unexpected call to MockSectorOwnerRepository.GetByID")
	}
	return m.GetByIDFn(ctx, id)
}

func (m *MockSectorOwnerRepository) ListBySession(ctx context.Context, session int32) ([]*models.SectorOwner, error) {
	if m.ListBySessionFn == nil {
		panic("unexpected call to MockSectorOwnerRepository.ListBySession")
	}
	return m.ListBySessionFn(ctx, session)
}

func (m *MockSectorOwnerRepository) Delete(ctx context.Context, id int32) error {
	if m.DeleteFn == nil {
		panic("unexpected call to MockSectorOwnerRepository.Delete")
	}
	return m.DeleteFn(ctx, id)
}

func (m *MockSectorOwnerRepository) DeleteAllBySession(ctx context.Context, session int32) error {
	if m.DeleteAllBySessionFn == nil {
		panic("unexpected call to MockSectorOwnerRepository.DeleteAllBySession")
	}
	return m.DeleteAllBySessionFn(ctx, session)
}

func (m *MockSectorOwnerRepository) RemoveBySession(ctx context.Context, session int32) error {
	if m.RemoveBySessionFn == nil {
		panic("unexpected call to MockSectorOwnerRepository.RemoveBySession")
	}
	return m.RemoveBySessionFn(ctx, session)
}

func (m *MockSectorOwnerRepository) WithTx(tx pgx.Tx) repository.SectorOwnerRepository {
	if m.WithTxFn == nil {
		panic("unexpected call to MockSectorOwnerRepository.WithTx")
	}
	return m.WithTxFn(tx)
}
