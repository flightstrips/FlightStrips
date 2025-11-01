package services

import (
	"context"
	"fmt"

	"FlightStrips/internal/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// InitialOrderSpacing is the gap between strips when initially created or after recalculation
	InitialOrderSpacing = 1000
	// MinOrderGap is the minimum gap before recalculation is needed
	MinOrderGap = 2
)

type StripService struct {
	queries *database.Queries
}

func NewStripService(dbPool *pgxpool.Pool) *StripService {
	return &StripService{
		queries: database.New(dbPool),
	}
}

// calculateOrderBetween calculates the order value for a strip being inserted between two existing strips.
// prevOrder is the order of the strip before the insertion point (use 0 if inserting at the beginning).
// nextOrder is the order of the strip after the insertion point (use nil if inserting at the end).
// Returns the new order value and a boolean indicating if recalculation of all orders is needed.
func (s *StripService) calculateOrderBetween(prevOrder int32, nextOrder *int32) (int32, bool) {
	// If inserting at the end
	if nextOrder == nil {
		return prevOrder + InitialOrderSpacing, false
	}

	// Calculate the midpoint between the two strips
	gap := *nextOrder - prevOrder

	// Check if we need to recalculate due to insufficient gap
	if gap <= MinOrderGap {
		return 0, true
	}

	newOrder := prevOrder + (gap / 2)
	return newOrder, false
}

// needsRecalculation checks if the gap between two order values is too small
// and requires recalculation of all strip orders.
func (s *StripService) needsRecalculation(prevOrder, nextOrder int32) bool {
	gap := nextOrder - prevOrder
	return gap <= MinOrderGap
}

// UpdateStripSequence updates the sequence of a single strip in the database.
func (s *StripService) UpdateStripSequence(ctx context.Context, session int32, callsign string, sequence int32) error {
	_, err := s.queries.UpdateStripSequence(ctx, database.UpdateStripSequenceParams{
		Session:  session,
		Callsign: callsign,
		Sequence: sequence,
	})
	if err != nil {
		return fmt.Errorf("failed to update strip sequence: %w", err)
	}

	// Send update notification
	sendStripUpdate(session, callsign, sequence)
	return nil
}

func (s *StripService) MoveToBay(ctx context.Context, session int32, callsign string, bay string) error {
	maxInBay, err := s.queries.GetMaxSequenceInBay(ctx, database.GetMaxSequenceInBayParams{
		Session: session,
		Bay:     bay,
	})

	if err != nil {
		return fmt.Errorf("failed to get max sequence in bay: %w", err)
	}

	order, _ := s.calculateOrderBetween(maxInBay, nil)
	return s.UpdateStripSequence(ctx, session, callsign, order)
}

// MoveStripBetween moves a strip between two other strips, calculating the appropriate sequence value.
// If recalculation is needed (gap too small), it will recalculate all strips in the session.
func (s *StripService) MoveStripBetween(ctx context.Context, session int32, callsign string, prevCallsign string, nextCallsign *string) error {
	prevOrder, err := s.queries.GetSequence(ctx, database.GetSequenceParams{
		Session:  session,
		Callsign: prevCallsign,
	})
	if err != nil {
		return fmt.Errorf("failed to get previous strip sequence: %w", err)
	}

	if !prevOrder.Valid {
		return fmt.Errorf("previous strip sequence not found or invalid")
	}

	var nextOrder *int32
	if nextCallsign != nil {
		dbNextOrder, err := s.queries.GetSequence(ctx, database.GetSequenceParams{
			Session:  session,
			Callsign: *nextCallsign,
		})
		if err != nil {
			return fmt.Errorf("failed to get next strip sequence: %w", err)
		}

		if dbNextOrder.Valid {
			nextOrder = &dbNextOrder.Int32
		}
	}

	newOrder, needsRecalc := s.calculateOrderBetween(prevOrder.Int32, nextOrder)

	if needsRecalc {
		// Gap is too small, need to recalculate all sequences
		return s.recalculateAllStripSequences(ctx, session)
	}

	return s.UpdateStripSequence(ctx, session, callsign, newOrder)
}

// recalculateAllStripSequences recalculates sequences for all strips in a session,
// spacing them InitialOrderSpacing apart based on their current order.
func (s *StripService) recalculateAllStripSequences(ctx context.Context, session int32) error {
	// Recalculate with proper spacing
	err := s.queries.RecalculateStripSequences(ctx, database.RecalculateStripSequencesParams{
		Session: session,
		Spacing: InitialOrderSpacing,
	})
	if err != nil {
		return fmt.Errorf("failed to recalculate strip sequences: %w", err)
	}

	// Get updated sequences
	sequences, err := s.queries.ListStripSequences(ctx, session)
	if err != nil {
		return fmt.Errorf("failed to list strip sequences: %w", err)
	}

	// Prepare data for bulk update notification
	callsigns := make([]string, len(sequences))
	seqs := make([]int32, len(sequences))
	for i, seq := range sequences {
		if seq.Sequence.Valid {
			callsigns[i] = seq.Callsign
			seqs[i] = seq.Sequence.Int32
		}
	}

	// Send bulk update notification
	sendBulkSequenceUpdate(session, callsigns, seqs)
	return nil
}

// GetStripSequences retrieves all strip callsigns and their sequences for a given session.
func (s *StripService) GetStripSequences(ctx context.Context, session int32) ([]database.ListStripSequencesRow, error) {
	sequences, err := s.queries.ListStripSequences(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("failed to get strip sequences: %w", err)
	}
	return sequences, nil
}

// UpdateStripSequenceBulk updates multiple strip sequences in a single transaction.
func (s *StripService) UpdateStripSequenceBulk(ctx context.Context, session int32, callsigns []string, sequences []int32) error {
	if len(callsigns) != len(sequences) {
		return fmt.Errorf("callsigns and sequences length mismatch")
	}

	err := s.queries.UpdateStripSequenceBulk(ctx, database.UpdateStripSequenceBulkParams{
		Callsigns: callsigns,
		Sequences: sequences,
		Session:   session,
	})
	if err != nil {
		return fmt.Errorf("failed to bulk update strip sequences: %w", err)
	}

	// Send bulk update notification
	sendBulkSequenceUpdate(session, callsigns, sequences)
	return nil
}

func sendStripUpdate(session int32, callsign string, sequence int32) {
	// DO NOT TOUCH
}

func sendBulkSequenceUpdate(session int32, callsigns []string, sequences []int32) {
	// DO NOT TOUCH
}
