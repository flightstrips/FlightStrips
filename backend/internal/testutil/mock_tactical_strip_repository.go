package testutil

import (
	"FlightStrips/internal/models"
	"context"
)

type MockTacticalStripRepository struct {
	CreateFn                 func(ctx context.Context, sessionID int32, stripType, bay, label string, aircraft *string, producedBy string, sequence int32) (*models.TacticalStrip, error)
	ListBySessionFn          func(ctx context.Context, sessionID int32) ([]*models.TacticalStrip, error)
	DeleteFn                 func(ctx context.Context, id int64, sessionID int32) error
	ConfirmFn                func(ctx context.Context, id int64, sessionID int32, confirmedBy string) (*models.TacticalStrip, error)
	StartTimerFn             func(ctx context.Context, id int64, sessionID int32) (*models.TacticalStrip, error)
	UpdateBayAndSequenceFn   func(ctx context.Context, id int64, sessionID int32, bay string, sequence int32) (*models.TacticalStrip, error)
	UpdateSequenceFn         func(ctx context.Context, id int64, sessionID int32, sequence int32) (*models.TacticalStrip, error)
	GetSequenceByIDFn        func(ctx context.Context, id int64, sessionID int32) (int32, error)
	ListBaySequencesFn       func(ctx context.Context, sessionID int32, bay string) ([]*models.TacticalStripSequence, error)
	GetMaxSequenceInBayFn    func(ctx context.Context, session int32, bay string) (int32, error)
	GetNextSequenceUnifiedFn func(ctx context.Context, session int32, bay string, prev int32) (int32, error)
	GetPrevSequenceUnifiedFn func(ctx context.Context, session int32, bay string, seq int32, excludeCallsign string) (int32, error)
}

func (m *MockTacticalStripRepository) Create(ctx context.Context, sessionID int32, stripType, bay, label string, aircraft *string, producedBy string, sequence int32) (*models.TacticalStrip, error) {
	if m.CreateFn == nil {
		panic("unexpected call to MockTacticalStripRepository.Create")
	}
	return m.CreateFn(ctx, sessionID, stripType, bay, label, aircraft, producedBy, sequence)
}

func (m *MockTacticalStripRepository) ListBySession(ctx context.Context, sessionID int32) ([]*models.TacticalStrip, error) {
	if m.ListBySessionFn == nil {
		panic("unexpected call to MockTacticalStripRepository.ListBySession")
	}
	return m.ListBySessionFn(ctx, sessionID)
}

func (m *MockTacticalStripRepository) Delete(ctx context.Context, id int64, sessionID int32) error {
	if m.DeleteFn == nil {
		panic("unexpected call to MockTacticalStripRepository.Delete")
	}
	return m.DeleteFn(ctx, id, sessionID)
}

func (m *MockTacticalStripRepository) Confirm(ctx context.Context, id int64, sessionID int32, confirmedBy string) (*models.TacticalStrip, error) {
	if m.ConfirmFn == nil {
		panic("unexpected call to MockTacticalStripRepository.Confirm")
	}
	return m.ConfirmFn(ctx, id, sessionID, confirmedBy)
}

func (m *MockTacticalStripRepository) StartTimer(ctx context.Context, id int64, sessionID int32) (*models.TacticalStrip, error) {
	if m.StartTimerFn == nil {
		panic("unexpected call to MockTacticalStripRepository.StartTimer")
	}
	return m.StartTimerFn(ctx, id, sessionID)
}

func (m *MockTacticalStripRepository) UpdateBayAndSequence(ctx context.Context, id int64, sessionID int32, bay string, sequence int32) (*models.TacticalStrip, error) {
	if m.UpdateBayAndSequenceFn == nil {
		panic("unexpected call to MockTacticalStripRepository.UpdateBayAndSequence")
	}
	return m.UpdateBayAndSequenceFn(ctx, id, sessionID, bay, sequence)
}

func (m *MockTacticalStripRepository) UpdateSequence(ctx context.Context, id int64, sessionID int32, sequence int32) (*models.TacticalStrip, error) {
	if m.UpdateSequenceFn == nil {
		panic("unexpected call to MockTacticalStripRepository.UpdateSequence")
	}
	return m.UpdateSequenceFn(ctx, id, sessionID, sequence)
}

func (m *MockTacticalStripRepository) GetSequenceByID(ctx context.Context, id int64, sessionID int32) (int32, error) {
	if m.GetSequenceByIDFn == nil {
		panic("unexpected call to MockTacticalStripRepository.GetSequenceByID")
	}
	return m.GetSequenceByIDFn(ctx, id, sessionID)
}

func (m *MockTacticalStripRepository) ListBaySequences(ctx context.Context, sessionID int32, bay string) ([]*models.TacticalStripSequence, error) {
	if m.ListBaySequencesFn == nil {
		panic("unexpected call to MockTacticalStripRepository.ListBaySequences")
	}
	return m.ListBaySequencesFn(ctx, sessionID, bay)
}

func (m *MockTacticalStripRepository) GetMaxSequenceInBayUnified(ctx context.Context, session int32, bay string) (int32, error) {
	if m.GetMaxSequenceInBayFn == nil {
		panic("unexpected call to MockTacticalStripRepository.GetMaxSequenceInBayUnified")
	}
	return m.GetMaxSequenceInBayFn(ctx, session, bay)
}

func (m *MockTacticalStripRepository) GetNextSequenceUnified(ctx context.Context, session int32, bay string, prev int32) (int32, error) {
	if m.GetNextSequenceUnifiedFn == nil {
		panic("unexpected call to MockTacticalStripRepository.GetNextSequenceUnified")
	}
	return m.GetNextSequenceUnifiedFn(ctx, session, bay, prev)
}

func (m *MockTacticalStripRepository) GetPrevSequenceUnified(ctx context.Context, session int32, bay string, seq int32, excludeCallsign string) (int32, error) {
	if m.GetPrevSequenceUnifiedFn == nil {
		panic("unexpected call to MockTacticalStripRepository.GetPrevSequenceUnified")
	}
	return m.GetPrevSequenceUnifiedFn(ctx, session, bay, seq, excludeCallsign)
}
