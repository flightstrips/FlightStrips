package services

import (
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/frontend"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// InitialOrderSpacing is the gap between strips when initially created or after recalculation
	InitialOrderSpacing = 1000
	// MinOrderGap is the minimum gap before recalculation is needed
	MinOrderGap = 5
)

type StripService struct {
	stripRepo       repository.StripRepository
	tacticalRepo    repository.TacticalStripRepository
	sectorOwnerRepo repository.SectorOwnerRepository
	frontendHub     shared.FrontendHub
	euroscopeHub    shared.EuroscopeHub
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

func (s *StripService) SetSectorOwnerRepo(sectorOwnerRepo repository.SectorOwnerRepository) {
	s.sectorOwnerRepo = sectorOwnerRepo
}

func (s *StripService) SetTacticalStripRepo(tacticalRepo repository.TacticalStripRepository) {
	s.tacticalRepo = tacticalRepo
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
	_, err := s.stripRepo.UpdateBayAndSequence(ctx, session, callsign, bay, sequence)
	if err != nil {
		return fmt.Errorf("failed to update strip sequence: %w", err)
	}

	if sendNotification {
		slog.Debug("Strip moved to bay", slog.String("callsign", callsign), slog.String("bay", bay), slog.Int("sequence", int(sequence)))
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

	if bay == shared.BAY_STAND {
		s.scheduleStandAutoHide(session, callsign)
	}

	return nil
}

// scheduleStandAutoHide starts a background goroutine that moves the strip to
// BAY_HIDDEN after a 15-second delay, provided the strip is still in BAY_STAND
// when the timer fires. This implements the "brief stand visibility" behaviour
// described in GitHub issue #33.
func (s *StripService) scheduleStandAutoHide(session int32, callsign string) {
	go func() {
		time.Sleep(15 * time.Second)

		ctx := context.Background()

		strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
		if err != nil {
			// Strip was deleted while we were waiting — nothing to do.
			slog.Debug("Auto-hide from STAND: strip not found, skipping",
				slog.String("callsign", callsign),
				slog.Int("session", int(session)))
			return
		}

		if strip.Bay != shared.BAY_STAND {
			// Strip was moved to a different bay before the timer fired — do not override.
			slog.Debug("Auto-hide from STAND: strip already moved, skipping",
				slog.String("callsign", callsign),
				slog.String("current_bay", strip.Bay))
			return
		}

		slog.Info("Auto-hiding arrival strip from STAND bay after 15 s",
			slog.String("callsign", callsign),
			slog.Int("session", int(session)))

		if err := s.MoveToBay(ctx, session, callsign, shared.BAY_HIDDEN, true); err != nil {
			slog.Error("Auto-hide from STAND: failed to move strip to HIDDEN",
				slog.String("callsign", callsign),
				slog.Int("session", int(session)),
				slog.Any("error", err))
		}
	}()
}

func (s *StripService) CreateCoordinationTransfer(ctx context.Context, session int32, callsign string, from string, to string) error {
	if s.frontendHub == nil {
		return errors.New("frontend hub not configured")
	}

	server := s.frontendHub.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	coord := &internalModels.Coordination{
		Session:      session,
		StripID:      strip.ID,
		FromPosition: from,
		ToPosition:   to,
	}

	if err := server.GetCoordinationRepository().Create(ctx, coord); err != nil {
		return err
	}

	s.frontendHub.SendCoordinationTransfer(session, callsign, from, to)
	return nil
}

func (s *StripService) CreateEsArrivalCoordination(ctx context.Context, session int32, callsign string, from string, to string, esHandoverCid *string) error {
	if s.frontendHub == nil {
		return errors.New("frontend hub not configured")
	}

	server := s.frontendHub.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	coord := &internalModels.Coordination{
		Session:       session,
		StripID:       strip.ID,
		FromPosition:  from,
		ToPosition:    to,
		FromEs:        true,
		EsHandoverCid: esHandoverCid,
	}

	if err := server.GetCoordinationRepository().Create(ctx, coord); err != nil {
		return err
	}

	s.frontendHub.SendCoordinationTransfer(session, callsign, from, to)
	return nil
}

// AcceptCoordination accepts a pending coordination for a strip by the given assumingPosition.
// It deletes the coordination, updates the next/previous owners, sets the strip owner, and
// sends frontend notifications. Returns nil without error if no coordination exists.
func (s *StripService) AcceptCoordination(ctx context.Context, session int32, callsign string, assumingPosition string) error {
	if s.frontendHub == nil {
		return errors.New("frontend hub not configured")
	}
	server := s.frontendHub.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	coordination, err := server.GetCoordinationRepository().GetByStripID(ctx, session, strip.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := server.GetCoordinationRepository().Delete(ctx, coordination.ID); err != nil {
		return err
	}

	nextOwners := strip.NextOwners
	index := slices.Index(nextOwners, assumingPosition)
	if index >= 0 {
		nextOwners = nextOwners[index+1:]
	}

	previousOwners := append(strip.PreviousOwners, coordination.FromPosition)

	if err := s.stripRepo.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, previousOwners); err != nil {
		return err
	}

	count, err := s.stripRepo.SetOwner(ctx, session, callsign, &assumingPosition, strip.Version)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to set strip owner after accepting coordination")
	}

	s.frontendHub.SendCoordinationAssume(session, callsign, assumingPosition)
	s.frontendHub.SendOwnersUpdate(session, callsign, assumingPosition, nextOwners, previousOwners)
	return nil
}

func (s *StripService) AutoTransferAirborneStrip(ctx context.Context, session int32, callsign string) error {
	slog.Debug("Attempting automatic AIRBORNE transfer", slog.String("callsign", callsign), slog.Int("session", int(session)))
	if s.frontendHub == nil {
		return errors.New("frontend hub not configured")
	}

	server := s.frontendHub.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if strip.Bay != shared.BAY_AIRBORNE {
		slog.Debug("Skipping automatic AIRBORNE transfer because strip is not in AIRBORNE bay", slog.String("callsign", callsign))
		return nil
	}

	if strip.Owner == nil || *strip.Owner == "" {
		slog.Debug("Skipping automatic AIRBORNE transfer without strip owner", slog.String("callsign", callsign))
		return nil
	}

	// fetch controllers once
	controllers, err := server.GetControllerRepository().ListBySession(ctx, session)
	if err != nil {
		return err
	}

	targetController, err := s.resolveAirborneController(strip, controllers)
	if err != nil {
		return err
	}
	if targetController == nil {
		slog.Debug("No suitable target controller found for automatic AIRBORNE transfer", slog.String("callsign", callsign))
		return nil
	}

	var ownerController *internalModels.Controller
	for _, c := range controllers {
		if c.Position == *strip.Owner {
			ownerController = c
			break
		}
	}

	if ownerController == nil || ownerController.Cid == nil || *ownerController.Cid == "" {
		slog.Debug("Owner controller CID not available; skipping euroscope handover send", slog.String("callsign", callsign))
		return nil
	}

	if ownerController.Position == targetController.Position {
		slog.Debug("Skipping automatic AIRBORNE transfer because target controller is the same as strip owner", slog.String("callsign", callsign))
		return nil
	}

	coordination, err := server.GetCoordinationRepository().GetByStripID(ctx, session, strip.ID)
	if err == nil {
		slog.Debug("Skipping automatic AIRBORNE transfer because coordination already exists",
			slog.String("callsign", callsign),
			slog.String("existing_to", coordination.ToPosition))
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	if err := s.CreateCoordinationTransfer(ctx, session, callsign, *strip.Owner, targetController.Position); err != nil {
		slog.Error("Failed to create coordination transfer for automatic AIRBORNE transfer", slog.String("callsign", callsign), slog.String("from", *strip.Owner), slog.String("to", targetController.Position), slog.Any("error", err))
		return err
	}

	if s.euroscopeHub != nil {
		s.euroscopeHub.SendCoordinationHandover(session, *ownerController.Cid, callsign, targetController.Callsign)
	}

	return nil
}

func (s *StripService) resolveAirborneController(strip *internalModels.Strip, controllers []*internalModels.Controller) (*internalModels.Controller, error) {
	if strip.Sid == nil || *strip.Sid == "" {
		return nil, nil
	}

	controllerPriority, err := config.GetAirborneControllerPriority(*strip.Sid)
	if err != nil {
		if errors.Is(err, config.ErrUnknownAirborneRoute) {
			slog.Debug("Unknown SID for AIRBORNE transfer; no controller priority defined", slog.String("sid", *strip.Sid))
			return nil, nil
		}
		return nil, err
	}

	for _, callsign := range controllerPriority {
		position, err := config.GetPositionByName(callsign)
		if err != nil {
			continue
		}
		for _, controller := range controllers {
			if controller.Position == position.Frequency && controller.Cid != nil && *controller.Cid != "" {
				return controller, nil
			}
		}
	}

	return nil, nil
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

	slog.Debug("Moving strip", slog.String("callsign", callsign), slog.String("bay", bay), slog.Int("prev", int(prev)), slog.Any("next", next))

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

	slog.Debug("Moving tactical strip", slog.Int64("id", id), slog.String("bay", bay), slog.Int("prev", int(prev)), slog.Any("next", next))

	newOrder, needsRecalc := s.calculateOrderBetween(prev, next)
	if needsRecalc {
		_, err := s.tacticalRepo.UpdateSequence(ctx, id, session, newOrder)
		if err != nil {
			return fmt.Errorf("failed to update tactical strip sequence: %w", err)
		}
		return s.recalculateAllStripSequences(ctx, session, bay)
	}

	_, err = s.tacticalRepo.UpdateSequence(ctx, id, session, newOrder)
	if err != nil {
		return fmt.Errorf("failed to update tactical strip sequence: %w", err)
	}
	s.frontendHub.SendTacticalStripMoved(session, id, bay, newOrder)
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
			s.frontendHub.SendTacticalStripMoved(session, e.tactialID, bay, newSeq)
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

	callsigns := make([]string, len(sequences))
	seqs := make([]int32, len(sequences))
	for i, seq := range sequences {
		if seq.Sequence != nil {
			callsigns[i] = seq.Callsign
			seqs[i] = *seq.Sequence
		}
	}

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

// AutoAssumeForClearedStrip finds the SQ (or fallback DEL) sector owner for the
// session and assigns them as the strip owner. It sends an owners update broadcast
// to all frontend clients. If no SQ/DEL owner is found, the strip is left unowned.
func (s *StripService) AutoAssumeForClearedStrip(ctx context.Context, session int32, callsign string, stripVersion int32) error {
	if s.sectorOwnerRepo == nil {
		return nil
	}

	owners, err := s.sectorOwnerRepo.ListBySession(ctx, session)
	if err != nil {
		return err
	}

	sqPosition := ""
	for _, owner := range owners {
		if slices.Contains(owner.Sector, "SQ") {
			sqPosition = owner.Position
			break
		}
	}
	if sqPosition == "" {
		for _, owner := range owners {
			if slices.Contains(owner.Sector, "DEL") {
				sqPosition = owner.Position
				break
			}
		}
	}

	if sqPosition == "" {
		slog.Debug("No SQ/DEL owner found for auto-assume", slog.String("callsign", callsign))
		return nil
	}

	slog.Debug("Auto-assuming cleared strip", slog.String("callsign", callsign), slog.String("position", sqPosition))

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	count, err := s.stripRepo.SetOwner(ctx, session, callsign, &sqPosition, stripVersion)
	if err != nil {
		return err
	}

	if count == 1 {
		s.frontendHub.SendOwnersUpdate(session, callsign, sqPosition, strip.NextOwners, strip.PreviousOwners)
	}

	return nil
}

// AutoAssumeForControllerOnline finds all cleared, unowned strips in the session whose
// next owner matches controllerPosition and assigns that controller as the strip owner.
// This is called when a controller comes online so they automatically inherit strips
// that were already cleared and waiting for them.
func (s *StripService) AutoAssumeForControllerOnline(ctx context.Context, session int32, controllerPosition string) error {
	strips, err := s.stripRepo.List(ctx, session)
	if err != nil {
		return err
	}

	for _, strip := range strips {
		if !strip.Cleared {
			continue
		}
		if strip.Owner != nil {
			continue
		}
		if len(strip.NextOwners) == 0 || strip.NextOwners[0] != controllerPosition {
			continue
		}

		count, err := s.stripRepo.SetOwner(ctx, session, strip.Callsign, &controllerPosition, strip.Version)
		if err != nil {
			slog.Error("AutoAssumeForControllerOnline: SetOwner failed",
				slog.String("callsign", strip.Callsign),
				slog.Any("error", err))
			continue
		}

		if count == 1 {
			slog.Debug("Auto-assumed strip on controller online",
				slog.String("callsign", strip.Callsign),
				slog.String("position", controllerPosition))
			s.frontendHub.SendOwnersUpdate(session, strip.Callsign, controllerPosition, strip.NextOwners, strip.PreviousOwners)
		}
	}

	return nil
}
