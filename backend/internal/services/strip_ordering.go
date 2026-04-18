package services

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/frontend"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/jackc/pgx/v5"
)

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
	_, err := s.stripRepo.UpdateBayAndSequence(ctx, session, callsign, bay, sequence)
	if err != nil {
		return fmt.Errorf("failed to update strip sequence: %w", err)
	}

	if sendNotification {
		slog.DebugContext(ctx, "Strip moved to bay", slog.String("callsign", callsign), slog.String("bay", bay), slog.Int("sequence", int(sequence)))
		// Send update notification
		s.sendStripUpdate(session, callsign, sequence, bay)
	}
	return nil
}

func (s *StripService) MoveToBay(ctx context.Context, session int32, callsign string, bay string, sendNotification bool) error {
	var maxInBay int32
	var err error
	if s.tacticalRepo != nil {
		maxInBay, err = s.tacticalRepo.GetMaxSequenceInBayUnified(ctx, session, bay)
	} else {
		maxInBay, err = s.stripRepo.GetMaxSequenceInBay(ctx, session, bay)
	}
	if err != nil {
		return fmt.Errorf("failed to get max sequence in bay: %w", err)
	}

	order, _ := s.calculateOrderBetween(maxInBay, nil)
	if err := s.updateStripSequence(ctx, session, callsign, order, bay, sendNotification); err != nil {
		return err
	}

	if err := s.reevaluateStripValidationPrecedence(ctx, session, callsign, sendNotification, true); err != nil {
		return err
	}

	if bay == shared.BAY_STAND {
		s.scheduleStandAutoHide(session, callsign)
	}

	return nil
}

// resolveRefSequence returns the current sequence of any strip type.
// Returns 0 (and nil error) when ref is nil, meaning "insert at top of bay".
func (s *StripService) resolveRefSequence(ctx context.Context, session int32, bay string, ref *frontend.StripRef) (int32, error) {
	if ref == nil {
		return 0, nil
	}
	switch ref.Kind {
	case "flight":
		if ref.Callsign == nil {
			return 0, fmt.Errorf("flight strip ref missing callsign")
		}
		return s.stripRepo.GetSequence(ctx, session, *ref.Callsign, bay)
	case "tactical":
		if ref.ID == nil {
			return 0, fmt.Errorf("tactical strip ref missing id")
		}
		if s.tacticalRepo == nil {
			return 0, fmt.Errorf("tactical strip repository not configured")
		}
		return s.tacticalRepo.GetSequenceByID(ctx, *ref.ID, session)
	default:
		return 0, fmt.Errorf("unknown strip ref kind: %s", ref.Kind)
	}
}

// MoveStripBetween moves a flight strip so it appears immediately after insertAfter.
// insertAfter = nil → move to top of bay (no predecessor).
// insertAfter = X   → move immediately after X (X is the predecessor).
func (s *StripService) MoveStripBetween(ctx context.Context, session int32, callsign string, insertAfter *frontend.StripRef, bay string) error {
	var prev int32
	if insertAfter != nil {
		seq, err := s.resolveRefSequence(ctx, session, bay, insertAfter)
		if err != nil {
			return fmt.Errorf("failed to resolve ref sequence: %w", err)
		}
		prev = seq
	}

	// next = smallest sequence > prev across all strip types
	var next *int32
	if s.tacticalRepo != nil {
		nextSeq, err := s.tacticalRepo.GetNextSequenceUnified(ctx, session, bay, prev)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to get next sequence: %w", err)
		} else if err == nil {
			next = &nextSeq
		}
	} else {
		nextSeq, err := s.stripRepo.GetNextSequence(ctx, session, bay, prev)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to get next sequence: %w", err)
		} else if err == nil {
			next = &nextSeq
		}
	}

	slog.DebugContext(ctx, "Moving strip", slog.String("callsign", callsign), slog.String("bay", bay), slog.Int("prev", int(prev)), slog.Any("next", next))

	newOrder, needsRecalc := s.calculateOrderBetween(prev, next)
	if needsRecalc {
		if err := s.updateStripSequence(ctx, session, callsign, newOrder, bay, false); err != nil {
			return err
		}
		return s.recalculateAllStripSequences(ctx, session, bay)
	}
	return s.updateStripSequence(ctx, session, callsign, newOrder, bay, true)
}

// MoveTacticalStripBetween moves a tactical strip so it appears immediately after insertAfter.
// insertAfter = nil → move to top of bay.
// insertAfter = X   → move immediately after X.
func (s *StripService) MoveTacticalStripBetween(ctx context.Context, session int32, id int64, insertAfter *frontend.StripRef, bay string) error {
	if s.tacticalRepo == nil {
		return fmt.Errorf("tactical strip repository not configured")
	}

	var prev int32
	if insertAfter != nil {
		seq, err := s.resolveRefSequence(ctx, session, bay, insertAfter)
		if err != nil {
			return fmt.Errorf("failed to resolve ref sequence: %w", err)
		}
		prev = seq
	}

	var next *int32
	nextSeq, err := s.tacticalRepo.GetNextSequenceUnified(ctx, session, bay, prev)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("failed to get next sequence: %w", err)
	} else if err == nil {
		next = &nextSeq
	}

	slog.DebugContext(ctx, "Moving tactical strip", slog.Int64("id", id), slog.String("bay", bay), slog.Int("prev", int(prev)), slog.Any("next", next))

	newOrder, needsRecalc := s.calculateOrderBetween(prev, next)
	if needsRecalc {
		_, err := s.tacticalRepo.UpdateBayAndSequence(ctx, id, session, bay, newOrder)
		if err != nil {
			return fmt.Errorf("failed to update tactical strip sequence: %w", err)
		}
		return s.recalculateAllStripSequences(ctx, session, bay)
	}

	_, err = s.tacticalRepo.UpdateBayAndSequence(ctx, id, session, bay, newOrder)
	if err != nil {
		return fmt.Errorf("failed to update tactical strip sequence: %w", err)
	}
	s.publisher.SendTacticalStripMoved(session, id, bay, newOrder)
	return nil
}

// recalculateAllStripSequences recalculates sequences for all strips (both flight and tactical)
// in a bay, spacing them InitialOrderSpacing apart based on their current order.
func (s *StripService) recalculateAllStripSequences(ctx context.Context, session int32, bay string) error {
	if s.tacticalRepo == nil {
		// Fallback: single-table recalculation
		return s.recalculateFlightStripsOnly(ctx, session, bay)
	}

	// Fetch sequences for both tables
	flightSeqs, err := s.stripRepo.ListSequences(ctx, session, bay)
	if err != nil {
		return fmt.Errorf("failed to list flight strip sequences: %w", err)
	}
	tacticalSeqs, err := s.tacticalRepo.ListBaySequences(ctx, session, bay)
	if err != nil {
		return fmt.Errorf("failed to list tactical strip sequences: %w", err)
	}

	// Build a unified sorted list
	type entry struct {
		isFlght   bool
		callsign  string
		tactialID int64
		sequence  int32
	}
	entries := make([]entry, 0, len(flightSeqs)+len(tacticalSeqs))
	for _, s := range flightSeqs {
		if s.Sequence != nil {
			entries = append(entries, entry{isFlght: true, callsign: s.Callsign, sequence: *s.Sequence})
		}
	}
	for _, t := range tacticalSeqs {
		entries = append(entries, entry{isFlght: false, tactialID: t.ID, sequence: t.Sequence})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].sequence < entries[j].sequence
	})

	// Assign new sequences
	newFlightCallsigns := make([]string, 0)
	newFlightSeqs := make([]int32, 0)

	for i, e := range entries {
		newSeq := int32((i + 1) * InitialOrderSpacing)
		if e.isFlght {
			newFlightCallsigns = append(newFlightCallsigns, e.callsign)
			newFlightSeqs = append(newFlightSeqs, newSeq)
		} else {
			_, err := s.tacticalRepo.UpdateSequence(ctx, e.tactialID, session, newSeq)
			if err != nil {
				return fmt.Errorf("failed to update tactical strip sequence during recalc: %w", err)
			}
			s.publisher.SendTacticalStripMoved(session, e.tactialID, bay, newSeq)
		}
	}

	if len(newFlightCallsigns) > 0 {
		if err := s.stripRepo.UpdateSequenceBulk(ctx, session, newFlightCallsigns, newFlightSeqs); err != nil {
			return fmt.Errorf("failed to bulk update flight strip sequences during recalc: %w", err)
		}
		s.sendBulkSequenceUpdate(session, newFlightCallsigns, newFlightSeqs, bay)
	}

	return nil
}

func (s *StripService) recalculateFlightStripsOnly(ctx context.Context, session int32, bay string) error {
	err := s.stripRepo.RecalculateSequences(ctx, session, bay, InitialOrderSpacing)
	if err != nil {
		return fmt.Errorf("failed to recalculate strip sequences: %w", err)
	}

	sequences, err := s.stripRepo.ListSequences(ctx, session, bay)
	if err != nil {
		return fmt.Errorf("failed to list strip sequences: %w", err)
	}

	callsigns := make([]string, 0, len(sequences))
	seqs := make([]int32, 0, len(sequences))
	for _, seq := range sequences {
		if seq.Sequence != nil {
			callsigns = append(callsigns, seq.Callsign)
			seqs = append(seqs, *seq.Sequence)
		}
	}

	s.sendBulkSequenceUpdate(session, callsigns, seqs, bay)
	return nil
}

func (s *StripService) sendStripUpdate(session int32, callsign string, sequence int32, bay string) {
	s.publisher.SendBayEvent(session, callsign, bay, sequence)
}

func (s *StripService) sendBulkSequenceUpdate(session int32, callsigns []string, sequences []int32, bay string) {
	if len(callsigns) != len(sequences) {
		return
	}

	// Send a single atomic bulk event so all frontends apply all sequence changes
	// in one setState call, preventing transient ordering inconsistencies.
	entries := make([]frontend.BulkBayEntry, len(callsigns))
	for i, callsign := range callsigns {
		entries[i] = frontend.BulkBayEntry{Callsign: callsign, Sequence: sequences[i]}
	}
	s.publisher.SendBulkBayEvent(session, bay, entries)
}
