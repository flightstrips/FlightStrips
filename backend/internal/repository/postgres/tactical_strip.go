package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type tacticalStripRepository struct {
	queries *database.Queries
}

func NewTacticalStripRepository(db *pgxpool.Pool) *tacticalStripRepository {
	return &tacticalStripRepository{
		queries: database.New(db),
	}
}

func tacticalStripToModel(db database.TacticalStrip) *models.TacticalStrip {
	m := &models.TacticalStrip{
		ID:          db.ID,
		SessionID:   db.SessionID,
		Type:        db.Type,
		Bay:         db.Bay,
		Label:       db.Label,
		Aircraft:    db.Aircraft,
		ProducedBy:  db.ProducedBy,
		Sequence:    db.Sequence,
		Confirmed:   db.Confirmed,
		ConfirmedBy: db.ConfirmedBy,
	}
	if db.TimerStart.Valid {
		t := db.TimerStart.Time
		m.TimerStart = &t
	}
	if db.CreatedAt.Valid {
		m.CreatedAt = db.CreatedAt.Time
	}
	return m
}

func (r *tacticalStripRepository) Create(ctx context.Context, sessionID int32, stripType, bay, label string, aircraft *string, producedBy string, sequence int32) (*models.TacticalStrip, error) {
	result, err := r.queries.CreateTacticalStrip(ctx, database.CreateTacticalStripParams{
		SessionID:  sessionID,
		Type:       stripType,
		Bay:        bay,
		Label:      label,
		Aircraft:   aircraft,
		ProducedBy: producedBy,
		Sequence:   sequence,
	})
	if err != nil {
		return nil, err
	}
	return tacticalStripToModel(result), nil
}

func (r *tacticalStripRepository) ListBySession(ctx context.Context, sessionID int32) ([]*models.TacticalStrip, error) {
	rows, err := r.queries.ListTacticalStripsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	result := make([]*models.TacticalStrip, len(rows))
	for i, row := range rows {
		result[i] = tacticalStripToModel(row)
	}
	return result, nil
}

func (r *tacticalStripRepository) Delete(ctx context.Context, id int64, sessionID int32) error {
	return r.queries.DeleteTacticalStrip(ctx, database.DeleteTacticalStripParams{
		ID:        id,
		SessionID: sessionID,
	})
}

func (r *tacticalStripRepository) Confirm(ctx context.Context, id int64, sessionID int32, confirmedBy string) (*models.TacticalStrip, error) {
	result, err := r.queries.ConfirmTacticalStrip(ctx, database.ConfirmTacticalStripParams{
		ID:          id,
		SessionID:   sessionID,
		ConfirmedBy: &confirmedBy,
	})
	if err != nil {
		return nil, err
	}
	return tacticalStripToModel(result), nil
}

func (r *tacticalStripRepository) StartTimer(ctx context.Context, id int64, sessionID int32) (*models.TacticalStrip, error) {
	result, err := r.queries.StartTacticalStripTimer(ctx, database.StartTacticalStripTimerParams{
		ID:        id,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, err
	}
	return tacticalStripToModel(result), nil
}

func (r *tacticalStripRepository) UpdateSequence(ctx context.Context, id int64, sessionID int32, sequence int32) (*models.TacticalStrip, error) {
	result, err := r.queries.UpdateTacticalStripSequence(ctx, database.UpdateTacticalStripSequenceParams{
		ID:        id,
		SessionID: sessionID,
		Sequence:  sequence,
	})
	if err != nil {
		return nil, err
	}
	return tacticalStripToModel(result), nil
}

func (r *tacticalStripRepository) UpdateBayAndSequence(ctx context.Context, id int64, sessionID int32, bay string, sequence int32) (*models.TacticalStrip, error) {
	result, err := r.queries.UpdateTacticalStripBayAndSequence(ctx, database.UpdateTacticalStripBayAndSequenceParams{
		ID:        id,
		SessionID: sessionID,
		Bay:       bay,
		Sequence:  sequence,
	})
	if err != nil {
		return nil, err
	}
	return tacticalStripToModel(result), nil
}

func (r *tacticalStripRepository) GetSequenceByID(ctx context.Context, id int64, sessionID int32) (int32, error) {
	return r.queries.GetTacticalStripSequenceByID(ctx, database.GetTacticalStripSequenceByIDParams{
		ID:        id,
		SessionID: sessionID,
	})
}

func (r *tacticalStripRepository) ListBaySequences(ctx context.Context, sessionID int32, bay string) ([]*models.TacticalStripSequence, error) {
	rows, err := r.queries.ListTacticalStripBaySequences(ctx, database.ListTacticalStripBaySequencesParams{
		SessionID: sessionID,
		Bay:       bay,
	})
	if err != nil {
		return nil, err
	}
	result := make([]*models.TacticalStripSequence, len(rows))
	for i, row := range rows {
		result[i] = &models.TacticalStripSequence{
			ID:       row.ID,
			Sequence: row.Sequence,
		}
	}
	return result, nil
}

func (r *tacticalStripRepository) GetMaxSequenceInBayUnified(ctx context.Context, session int32, bay string) (int32, error) {
	return r.queries.GetMaxSequenceInBayUnified(ctx, database.GetMaxSequenceInBayUnifiedParams{
		Session: session,
		Bay:     bay,
	})
}

func (r *tacticalStripRepository) GetNextSequenceUnified(ctx context.Context, session int32, bay string, prev int32) (int32, error) {
	seq, err := r.queries.GetNextSequenceUnified(ctx, database.GetNextSequenceUnifiedParams{
		Session: session,
		Bay:     bay,
		Prev:    prev,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, pgx.ErrNoRows
		}
		return 0, err
	}
	return seq, nil
}

func (r *tacticalStripRepository) GetPrevSequenceUnified(ctx context.Context, session int32, bay string, seq int32, excludeCallsign string) (int32, error) {
	prev, err := r.queries.GetPrevSequenceUnified(ctx, database.GetPrevSequenceUnifiedParams{
		Session:         session,
		Bay:             bay,
		Seq:             seq,
		ExcludeCallsign: excludeCallsign,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, pgx.ErrNoRows
		}
		return 0, err
	}
	return prev, nil
}

// Compile-time check: ensure pgtype is imported (used in tacticalStripToModel).
var _ = pgtype.Timestamptz{}
