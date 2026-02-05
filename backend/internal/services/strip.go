package services

import (
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

const (
	// InitialOrderSpacing is the gap between strips when initially created or after recalculation
	InitialOrderSpacing = 1000
	// MinOrderGap is the minimum gap before recalculation is needed
	MinOrderGap = 5
)

type StripService struct {
	stripRepo    repository.StripRepository
	frontendHub  shared.FrontendHub
	euroscopeHub shared.EuroscopeHub
}

func NewStripService(stripRepo repository.StripRepository) *StripService {
	return &StripService{
		stripRepo: stripRepo,
	}
}

func (s *StripService) SetFrontendHub(frontendHub shared.FrontendHub) {
	s.frontendHub = frontendHub
}

func (s *StripService) SetEuroscopeHub(euroscopeHub shared.EuroscopeHub) {
	s.euroscopeHub = euroscopeHub
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

// updateStripSequence updates the sequence of a single strip in the database.
func (s *StripService) updateStripSequence(ctx context.Context, session int32, callsign string, sequence int32, bay string, sendNotification bool) error {
	_, err := s.stripRepo.UpdateSequence(ctx, session, callsign, sequence)
	if err != nil {
		return fmt.Errorf("failed to update strip sequence: %w", err)
	}

	if sendNotification {
		// Send update notification
		s.sendStripUpdate(session, callsign, sequence, bay)
	}
	return nil
}

func (s *StripService) MoveToBay(ctx context.Context, session int32, callsign string, bay string, sendNotification bool) error {
	maxInBay, err := s.stripRepo.GetMaxSequenceInBay(ctx, session, bay)
	if err != nil {
		return fmt.Errorf("failed to get max sequence in bay: %w", err)
	}

	order, _ := s.calculateOrderBetween(maxInBay, nil)
	return s.updateStripSequence(ctx, session, callsign, order, bay, sendNotification)
}

// MoveStripBetween moves a strip between two other strips, calculating the appropriate sequence value.
// If recalculation is needed (gap too small), it will recalculate all strips in the session.
func (s *StripService) MoveStripBetween(ctx context.Context, session int32, callsign string, before *string, bay string) error {
	var prev int32
	var next *int32

	if before == nil {
		slog.Debug("Moving strip to start of bay", slog.String("bay", bay), slog.Int("session", int(session)), slog.String("callsign", callsign))
		prev = 0
		nextOrder, err := s.stripRepo.GetMinSequenceInBay(ctx, session, bay)
		if err != nil {
			return fmt.Errorf("failed to get min sequence in bay: %w", err)
		}
		next = &nextOrder
	} else {
		slog.Debug("Moving strip between other strips", slog.String("before", *before), slog.String("bay", bay), slog.Int("session", int(session)), slog.String("callsign", callsign))
		var err error
		prev, err = s.stripRepo.GetSequence(ctx, session, *before, bay)
		if err != nil {
			return fmt.Errorf("failed to get sequence: %w", err)
		}
		nextOrder, err := s.stripRepo.GetNextSequence(ctx, session, bay, prev)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("failed to get next sequence: %w", err)
			}
		} else {
			next = &nextOrder
		}
	}

	newOrder, needsRecalc := s.calculateOrderBetween(prev, next)

	if needsRecalc {
		err := s.updateStripSequence(ctx, session, callsign, newOrder, bay, false)
		if err != nil {
			return err
		}
		// Gap is too small, need to recalculate all sequences
		return s.recalculateAllStripSequences(ctx, session, bay)
	}

	return s.updateStripSequence(ctx, session, callsign, newOrder, bay, true)
}

// recalculateAllStripSequences recalculates sequences for all strips in a session,
// spacing them InitialOrderSpacing apart based on their current order.
func (s *StripService) recalculateAllStripSequences(ctx context.Context, session int32, bay string) error {
	// Recalculate with proper spacing
	err := s.stripRepo.RecalculateSequences(ctx, session, bay, InitialOrderSpacing)
	if err != nil {
		return fmt.Errorf("failed to recalculate strip sequences: %w", err)
	}

	// Get updated sequences
	sequences, err := s.stripRepo.ListSequences(ctx, session, bay)
	if err != nil {
		return fmt.Errorf("failed to list strip sequences: %w", err)
	}

	// Prepare data for bulk update notification
	callsigns := make([]string, len(sequences))
	seqs := make([]int32, len(sequences))
	for i, seq := range sequences {
		if seq.Sequence != nil {
			callsigns[i] = seq.Callsign
			seqs[i] = *seq.Sequence
		}
	}

	// Send bulk update notification
	s.sendBulkSequenceUpdate(session, callsigns, seqs, bay)
	return nil
}

func (s *StripService) sendStripUpdate(session int32, callsign string, sequence int32, bay string) {
	s.frontendHub.SendBayEvent(session, callsign, bay, sequence)
}

func (s *StripService) sendBulkSequenceUpdate(session int32, callsigns []string, sequences []int32, bay string) {
	if len(callsigns) != len(sequences) {
		return
	}

	for i, callsign := range callsigns {
		seq := sequences[i]
		s.frontendHub.SendBayEvent(session, callsign, bay, seq)
	}
}

// ClearStrip moves strip to cleared bay and notifies EuroScope to set cleared flag
func (s *StripService) ClearStrip(ctx context.Context, session int32, callsign string, cid string) error {
	if err := s.MoveToBay(ctx, session, callsign, shared.BAY_CLEARED, true); err != nil {
		return fmt.Errorf("failed to move strip to cleared bay: %w", err)
	}

	if s.euroscopeHub != nil {
		s.euroscopeHub.SendClearedFlag(session, cid, callsign, true)
	}

	return nil
}

// UnclearStrip moves strip back to not-cleared bay and notifies EuroScope to clear the cleared flag
func (s *StripService) UnclearStrip(ctx context.Context, session int32, callsign string, cid string) error {
	if err := s.MoveToBay(ctx, session, callsign, shared.BAY_NOT_CLEARED, true); err != nil {
		return fmt.Errorf("failed to move strip to not-cleared bay: %w", err)
	}

	if s.euroscopeHub != nil {
		s.euroscopeHub.SendClearedFlag(session, cid, callsign, false)
	}

	return nil
}
