package services

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/models"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strings"
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
	coordRepo       repository.CoordinationRepository
	controllerRepo  repository.ControllerRepository
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

func (s *StripService) SetCoordinationRepo(coordRepo repository.CoordinationRepository) {
	s.coordRepo = coordRepo
}

func (s *StripService) SetControllerRepo(controllerRepo repository.ControllerRepository) {
	s.controllerRepo = controllerRepo
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
// BAY_HIDDEN after a 240-second delay, provided the strip is still in BAY_STAND
// when the timer fires.
func (s *StripService) scheduleStandAutoHide(session int32, callsign string) {
	go func() {
		time.Sleep(240 * time.Second)

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

	// For strips in the AIRBORNE bay, also send an ES handover so the owning
	// EuroScope client initiates the transfer to the target controller.
	if strip.Bay == shared.BAY_AIRBORNE && s.euroscopeHub != nil {
		controllers, err := server.GetControllerRepository().ListBySession(ctx, session)
		if err == nil {
			var ownerCid *string
			var targetCallsign string
			for _, c := range controllers {
				if c.Position == from && c.Cid != nil && *c.Cid != "" {
					ownerCid = c.Cid
				}
				if c.Position == to {
					targetCallsign = c.Callsign
				}
			}
			if ownerCid != nil && targetCallsign != "" {
				s.euroscopeHub.SendCoordinationHandover(session, *ownerCid, callsign, targetCallsign)
			}
		}
	}

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

	slog.Debug("Resolving airborne controller",
		slog.String("sid", *strip.Sid),
		slog.Any("priority_list", controllerPriority),
	)

	for _, callsign := range controllerPriority {
		position, err := config.GetPositionByName(callsign)
		if err != nil {
			continue
		}
		for _, controller := range controllers {
			if controller.Position == position.Frequency && controller.Cid != nil && *controller.Cid != "" {
				slog.Debug("Matched airborne controller",
					slog.String("sid", *strip.Sid),
					slog.String("matched_callsign", callsign),
					slog.String("controller_position", controller.Position),
				)
				return controller, nil
			}
		}
	}

	slog.Debug("No online controller found for AIRBORNE transfer", slog.String("sid", *strip.Sid))
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
	s.frontendHub.SendBayEvent(session, callsign, bay, sequence)
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
	s.frontendHub.SendBulkBayEvent(session, bay, entries)
}

// ClearStrip moves strip to cleared bay and notifies EuroScope to set cleared flag
func (s *StripService) ClearStrip(ctx context.Context, session int32, callsign string, cid string) error {
	if err := s.MoveToBay(ctx, session, callsign, shared.BAY_CLEARED, true); err != nil {
		return fmt.Errorf("failed to move strip to cleared bay: %w", err)
	}

	if _, err := s.stripRepo.UpdateClearedFlag(ctx, session, callsign, true, shared.BAY_CLEARED, nil); err != nil {
		slog.ErrorContext(ctx, "ClearStrip: failed to update cleared flag", slog.Any("error", err))
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

	if _, err := s.stripRepo.UpdateClearedFlag(ctx, session, callsign, false, shared.BAY_NOT_CLEARED, nil); err != nil {
		slog.ErrorContext(ctx, "UnclearStrip: failed to update cleared flag", slog.Any("error", err))
	}

	if s.euroscopeHub != nil {
		s.euroscopeHub.SendClearedFlag(session, cid, callsign, false)
	}

	return nil
}

// AutoAssumeForClearedStrip finds the SQ (or fallback DEL) sector owner for the
// session and assigns them as the strip owner. It sends an owners update broadcast
// to all frontend clients. If no SQ/DEL owner is found, the strip is left unowned.
func (s *StripService) AutoAssumeForClearedStrip(ctx context.Context, session int32, callsign string) error {
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

	count, err := s.stripRepo.SetOwner(ctx, session, callsign, &sqPosition, strip.Version)
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

// UpdateAssignedSquawk updates the assigned squawk for a strip and notifies the frontend.
func (s *StripService) UpdateAssignedSquawk(ctx context.Context, session int32, callsign string, squawk string) error {
	count, err := s.stripRepo.UpdateAssignedSquawk(ctx, session, callsign, &squawk, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "AssignedSquawk"))
	} else {
		s.frontendHub.SendAssignedSquawkEvent(session, callsign, squawk)
	}
	return nil
}

// UpdateSquawk updates the current squawk for a strip and notifies the frontend.
func (s *StripService) UpdateSquawk(ctx context.Context, session int32, callsign string, squawk string) error {
	count, err := s.stripRepo.UpdateSquawk(ctx, session, callsign, &squawk, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "Squawk"))
	} else {
		s.frontendHub.SendSquawkEvent(session, callsign, squawk)
	}
	return nil
}

// UpdateRequestedAltitude updates the requested altitude for a strip and notifies the frontend.
func (s *StripService) UpdateRequestedAltitude(ctx context.Context, session int32, callsign string, altitude int32) error {
	count, err := s.stripRepo.UpdateRequestedAltitude(ctx, session, callsign, &altitude, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "RequestedAltitude"))
	} else {
		s.frontendHub.SendRequestedAltitudeEvent(session, callsign, altitude)
	}
	return nil
}

// UpdateClearedAltitude updates the cleared altitude for a strip and notifies the frontend.
func (s *StripService) UpdateClearedAltitude(ctx context.Context, session int32, callsign string, altitude int32) error {
	count, err := s.stripRepo.UpdateClearedAltitude(ctx, session, callsign, &altitude, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "ClearedAltitude"))
	} else {
		s.frontendHub.SendClearedAltitudeEvent(session, callsign, altitude)
	}
	return nil
}

// UpdateCommunicationType updates the communication type for a strip and notifies the frontend.
func (s *StripService) UpdateCommunicationType(ctx context.Context, session int32, callsign string, commType string) error {
	count, err := s.stripRepo.UpdateCommunicationType(ctx, session, callsign, &commType, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "CommunicationType"))
		return nil
	}
	s.frontendHub.SendCommunicationTypeEvent(session, callsign, commType)
	return nil
}

// UpdateHeading updates the heading for a strip and notifies the frontend.
func (s *StripService) UpdateHeading(ctx context.Context, session int32, callsign string, heading int32) error {
	count, err := s.stripRepo.UpdateHeading(ctx, session, callsign, &heading, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "SetHeading"))
		return nil
	}
	s.frontendHub.SendSetHeadingEvent(session, callsign, heading)
	return nil
}

// DeleteStrip removes a strip from the database and notifies the frontend.
func (s *StripService) DeleteStrip(ctx context.Context, session int32, callsign string) error {
	err := s.stripRepo.Delete(ctx, session, callsign)
	s.frontendHub.SendAircraftDisconnect(session, callsign)
	return err
}

// UpdateGroundState updates the ground state for a strip, recomputes the bay, and moves
// the strip if the bay changed.
func (s *StripService) UpdateGroundState(ctx context.Context, session int32, callsign string, groundState string, airport string) error {
	existingStrip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "GroundState"))
			return nil
		}
		return err
	}

	if existingStrip.State != nil && *existingStrip.State == groundState {
		return nil
	}

	dbStrip := database.Strip{
		Origin:      existingStrip.Origin,
		Destination: existingStrip.Destination,
		Cleared:     existingStrip.Cleared,
		Bay:         existingStrip.Bay,
	}
	bay := shared.GetDepartureBayFromGroundState(groundState, dbStrip)

	_, err = s.stripRepo.UpdateGroundState(ctx, session, callsign, &groundState, bay, nil)
	if err != nil {
		return err
	}

	if existingStrip.Bay != bay {
		return s.MoveToBay(context.Background(), session, callsign, bay, true)
	}

	return nil
}

// UpdateClearedFlag updates the cleared flag for a strip, recomputes the bay,
// triggers auto-assumption if cleared, and moves the strip if the bay changed.
func (s *StripService) UpdateClearedFlag(ctx context.Context, session int32, callsign string, cleared bool) error {
	existingStrip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "FlightStripOnline"))
			return nil
		}
		return err
	}

	if existingStrip.Cleared == cleared {
		return nil
	}

	bay := existingStrip.Bay
	if bay == shared.BAY_NOT_CLEARED || bay == shared.BAY_UNKNOWN {
		bay = shared.BAY_CLEARED
	}
	if bay == "" {
		bay = shared.BAY_HIDDEN
	}

	_, err = s.stripRepo.UpdateClearedFlag(ctx, session, callsign, cleared, bay, nil)
	if err != nil {
		return err
	}

	if cleared {
		// Skip auto-assume if PDC clearance is pending pilot WILCO (strip is in CLEARED pdc state)
		if existingStrip.PdcState != "CLEARED" {
			if err := s.AutoAssumeForClearedStrip(ctx, session, callsign); err != nil {
				slog.Error("Failed to auto-assume cleared strip from EuroScope", slog.Any("error", err))
			}
		}
	}

	if existingStrip.Bay != bay {
		return s.MoveToBay(ctx, session, callsign, bay, true)
	}

	return nil
}

// UpdateStand updates the stand for a strip, notifies the frontend, and triggers route recalculation.
func (s *StripService) UpdateStand(ctx context.Context, session int32, callsign string, stand string) error {
	count, err := s.stripRepo.UpdateStand(ctx, session, callsign, &stand, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "Stand"))
		return nil
	}
	s.frontendHub.SendStandEvent(session, callsign, stand)

	server := s.frontendHub.GetServer()
	if server != nil {
		if err := server.UpdateRouteForStrip(callsign, session, true); err != nil {
			slog.Error("Error updating route after stand assignment", slog.String("callsign", callsign), slog.Any("error", err))
		}
	}
	return nil
}

// UpdateAircraftPosition updates the aircraft position and moves the strip to a new bay if needed.
func (s *StripService) UpdateAircraftPosition(ctx context.Context, session int32, callsign string, lat, lon float64, altitude int32, airport string) error {
	existingStrip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "FlightStripOffline"))
			return nil
		}
		return err
	}

	dbStrip := database.Strip{
		Origin:      existingStrip.Origin,
		Destination: existingStrip.Destination,
		Cleared:     existingStrip.Cleared,
		Bay:         existingStrip.Bay,
		State:       existingStrip.State,
	}
	bay := shared.GetDepartureBayFromPosition(lat, lon, int64(altitude), dbStrip, config.GetAirborneAltitudeAGL(), airport)

	existingState := "<nil>"
	if existingStrip.State != nil {
		existingState = *existingStrip.State
	}
	slog.Debug("UpdateAircraftPosition",
		slog.String("callsign", callsign),
		slog.String("current_bay", existingStrip.Bay),
		slog.String("current_state", existingState),
		slog.Int("altitude", int(altitude)),
		slog.Int64("airborne_threshold_agl", config.GetAirborneAltitudeAGL()),
		slog.String("computed_bay", bay),
	)

	_, err = s.stripRepo.UpdateAircraftPosition(ctx, session, callsign, &lat, &lon, &altitude, bay, nil)
	if err != nil {
		return err
	}

	if existingStrip.Bay != bay {
		slog.Debug("UpdateAircraftPosition: bay changed, moving strip",
			slog.String("callsign", callsign),
			slog.String("from_bay", existingStrip.Bay),
			slog.String("to_bay", bay),
		)
		if err := s.MoveToBay(context.Background(), session, callsign, bay, true); err != nil {
			return err
		}
		if existingStrip.Bay == shared.BAY_DEPART && bay == shared.BAY_AIRBORNE {
			return s.AutoTransferAirborneStrip(ctx, session, callsign)
		}
	}

	return nil
}

// HandleTrackingControllerChanged processes a tracking controller change event,
// potentially accepting a coordination and moving the strip if it becomes airborne.
func (s *StripService) HandleTrackingControllerChanged(ctx context.Context, session int32, callsign string, trackingController string) error {
	if s.controllerRepo == nil {
		return errors.New("controller repository not configured")
	}
	if s.coordRepo == nil {
		return errors.New("coordination repository not configured")
	}

	if _, err := s.stripRepo.UpdateTrackingController(ctx, session, callsign, trackingController); err != nil {
		return err
	}

	// Only act on assumption (non-empty tracking controller).
	if trackingController == "" {
		return nil
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	// Resolve the assuming controller's position.
	assumingController, err := s.controllerRepo.GetByCallsign(ctx, session, trackingController)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	assumingPosition := ""
	if err == nil {
		assumingPosition = assumingController.Position
	}

	// Check for a pending coordination on this strip.
	coordination, err := s.coordRepo.GetByStripID(ctx, session, strip.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	hasCoordination := err == nil

	if hasCoordination && assumingPosition == coordination.FromPosition {
		// The FROM controller assumed the tag to initiate the handover to the TO controller.
		// Don't move or accept yet — wait for the TO controller to assume.
		return nil
	}

	// Accepting: either the TO controller assumed, or there is no coordination.
	if hasCoordination && assumingPosition != "" {
		if err := s.AcceptCoordination(ctx, session, callsign, assumingPosition); err != nil {
			slog.Error("Failed to accept coordination on tracking controller change", slog.String("callsign", callsign), slog.Any("error", err))
		}
	}

	if strip.Bay != shared.BAY_AIRBORNE {
		return nil
	}

	count, err := s.stripRepo.UpdateBay(ctx, session, callsign, shared.BAY_HIDDEN, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to move airborne strip to hidden after tracking controller assumption")
	}

	return s.MoveToBay(ctx, session, callsign, shared.BAY_HIDDEN, true)
}

// HandleCoordinationReceived processes an EuroScope coordination-received event,
// moving the strip from ARR_HIDDEN to FINAL and creating an arrival coordination record.
func (s *StripService) HandleCoordinationReceived(ctx context.Context, session int32, callsign string, controllerCallsign string) error {
	if s.controllerRepo == nil {
		return errors.New("controller repository not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("Strip not found for coordination_received event", slog.String("callsign", callsign))
			return nil
		}
		return err
	}

	if strip.Bay != shared.BAY_ARR_HIDDEN {
		slog.Debug("coordination_received on strip not in ARR_HIDDEN, ignoring", slog.String("callsign", callsign), slog.String("bay", strip.Bay))
		return nil
	}

	controller, err := s.controllerRepo.GetByCallsign(ctx, session, controllerCallsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("Controller not found for coordination_received event", slog.String("controller_callsign", controllerCallsign))
			return nil
		}
		return err
	}

	slog.Debug("Received coordination received event", slog.String("callsign", callsign), slog.String("from_controller", controllerCallsign))

	if _, err := s.stripRepo.UpdateBay(ctx, session, callsign, shared.BAY_FINAL, nil); err != nil {
		return err
	}

	if err := s.MoveToBay(ctx, session, callsign, shared.BAY_FINAL, true); err != nil {
		return err
	}

	fromPosition := ""
	if strip.Owner != nil {
		fromPosition = *strip.Owner
	}

	return s.CreateEsArrivalCoordination(ctx, session, callsign, fromPosition, controller.Position, controller.Cid)
}

// AssumeStripCoordination handles the full frontend assume flow:
// accepts a pending coordination, or directly assumes an unowned strip.
func (s *StripService) AssumeStripCoordination(ctx context.Context, session int32, callsign string, position string) error {
	if s.coordRepo == nil {
		return errors.New("coordination repository not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	// Check for a pending coordination first.
	coordination, err := s.coordRepo.GetByStripID(ctx, session, strip.ID)
	if err == nil {
		// Coordination exists — validate it targets this position.
		if coordination.ToPosition != position {
			// Special case: strip is unowned and this position is next in line.
			// The coordination is stale (e.g. from an ES arrival push to a different position).
			// Delete it and allow direct assumption.
			if (strip.Owner == nil || *strip.Owner == "") && slices.Contains(strip.NextOwners, position) {
				_ = s.coordRepo.Delete(ctx, coordination.ID)

				nextOwners := strip.NextOwners
				index := slices.Index(nextOwners, position)
				if index >= 0 {
					nextOwners = nextOwners[index+1:]
				}

				if err := s.stripRepo.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, strip.PreviousOwners); err != nil {
					return err
				}

				count, err := s.stripRepo.SetOwner(ctx, session, callsign, &position, strip.Version)
				if err != nil {
					return err
				}
				if count != 1 {
					return errors.New("failed to set strip owner")
				}
				s.frontendHub.SendCoordinationAssume(session, callsign, position)
				s.frontendHub.SendOwnersUpdate(session, callsign, position, nextOwners, strip.PreviousOwners)
				return nil
			}
			return errors.New("cannot assume strip which is not transferred to you")
		}

		// Capture ES handover fields before AcceptCoordination deletes the record.
		esHandoverCid := coordination.EsHandoverCid
		fromEs := coordination.FromEs

		if err := s.AcceptCoordination(ctx, session, callsign, position); err != nil {
			return err
		}

		// If this coordination originated from an ES push, signal the ES client to assume+drop.
		if fromEs && esHandoverCid != nil && *esHandoverCid != "" && s.euroscopeHub != nil {
			s.euroscopeHub.SendAssumeAndDrop(session, *esHandoverCid, callsign)
		}

		return nil
	}

	// Strip is not owned by anyone and has no pending coordination — assume directly.
	if strip.Owner == nil || *strip.Owner == "" {
		nextOwners := strip.NextOwners
		index := slices.Index(nextOwners, position)
		if index >= 0 {
			nextOwners = nextOwners[index+1:]
		}

		if err := s.stripRepo.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, strip.PreviousOwners); err != nil {
			return err
		}

		count, err := s.stripRepo.SetOwner(ctx, session, callsign, &position, strip.Version)
		if err != nil {
			return err
		}
		if count != 1 {
			return errors.New("failed to set strip owner")
		}
		s.frontendHub.SendCoordinationAssume(session, callsign, position)
		s.frontendHub.SendOwnersUpdate(session, callsign, position, nextOwners, strip.PreviousOwners)
		return nil
	}

	return errors.New("cannot assume strip which is not transferred to you")
}

// ForceAssumeStrip forcibly takes ownership of a strip, overriding any existing owner.
// Unlike AssumeStripCoordination it does not check NextOwners — any controller
// may force-assume any strip regardless of current ownership.
//
// After assuming:
//   - The route is recalculated starting from the new owner.
//   - The displaced owner is removed from previous controllers.
//   - Any controllers that appear in the recalculated next owners are removed from previous controllers.
func (s *StripService) ForceAssumeStrip(ctx context.Context, session int32, callsign string, position string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	// Delete any stale coordination so it does not block future operations.
	if s.coordRepo != nil {
		if coord, err := s.coordRepo.GetByStripID(ctx, session, strip.ID); err == nil {
			_ = s.coordRepo.Delete(ctx, coord.ID)
		}
	}

	// Trim NextOwners: remove the new owner and all positions before it.
	nextOwners := strip.NextOwners
	index := slices.Index(nextOwners, position)
	isExpectedHandoff := index >= 0
	if isExpectedHandoff {
		nextOwners = nextOwners[index+1:]
	}

	// Build the new previous owners list.
	// Filter out the displaced owner first (avoids duplicates), then — for an
	// expected handoff — append them so they appear as a past controller.
	previousOwners := make([]string, 0, len(strip.PreviousOwners)+1)
	for _, p := range strip.PreviousOwners {
		if strip.Owner == nil || *strip.Owner != p {
			previousOwners = append(previousOwners, p)
		}
	}
	if isExpectedHandoff && strip.Owner != nil && *strip.Owner != "" {
		previousOwners = append(previousOwners, *strip.Owner)
	}

	if err := s.stripRepo.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, previousOwners); err != nil {
		return err
	}

	count, err := s.stripRepo.SetOwner(ctx, session, callsign, &position, strip.Version)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to set strip owner")
	}

	// Recalculate the route starting from the new owner. This updates NextOwners in the
	// DB. We pass sendUpdate=false because we send the owners update ourselves below with
	// the fully cleaned previous owners.
	if s.frontendHub != nil {
		if server := s.frontendHub.GetServer(); server != nil {
			_ = server.UpdateRouteForStrip(callsign, session, false)

			// Re-read to pick up the recalculated NextOwners.
			if updated, err := s.stripRepo.GetByCallsign(ctx, session, callsign); err == nil {
				nextOwners = updated.NextOwners
			}
		}
	}

	// Any controller that now appears in NextOwners must be removed from PreviousOwners
	// — they are upcoming controllers, not past ones.
	cleanedPrevious := make([]string, 0, len(previousOwners))
	for _, p := range previousOwners {
		if !slices.Contains(nextOwners, p) {
			cleanedPrevious = append(cleanedPrevious, p)
		}
	}
	if !slices.Equal(cleanedPrevious, previousOwners) {
		if err := s.stripRepo.SetPreviousOwners(ctx, session, callsign, cleanedPrevious); err != nil {
			return err
		}
		previousOwners = cleanedPrevious
	}

	s.frontendHub.SendCoordinationAssume(session, callsign, position)
	s.frontendHub.SendOwnersUpdate(session, callsign, position, nextOwners, previousOwners)
	return nil
}

// RejectCoordination rejects a pending coordination transfer.
func (s *StripService) RejectCoordination(ctx context.Context, session int32, callsign string, position string) error {
	if s.coordRepo == nil {
		return errors.New("coordination repository not configured")
	}

	coordination, err := s.coordRepo.GetByStripCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if coordination.ToPosition != position {
		return errors.New("cannot reject strip which is not transferred to you")
	}

	if err = s.coordRepo.Delete(ctx, coordination.ID); err != nil {
		return err
	}
	s.frontendHub.SendCoordinationReject(session, callsign, position)
	return nil
}

// CreateTagRequest creates a tag-request coordination, allowing a non-owner to request the strip tag from the current owner.
func (s *StripService) CreateTagRequest(ctx context.Context, session int32, callsign string, requesterPosition string) error {
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

	if strip.Owner == nil || *strip.Owner == "" {
		return errors.New("cannot request tag of an unowned strip")
	}

	if *strip.Owner == requesterPosition {
		return errors.New("cannot request tag of a strip you already own")
	}

	coordRepo := server.GetCoordinationRepository()

	// Check no active coordination already exists
	if _, err := coordRepo.GetByStripCallsign(ctx, session, callsign); err == nil {
		return errors.New("an active coordination already exists for this strip")
	}

	coord := &internalModels.Coordination{
		Session:      session,
		StripID:      strip.ID,
		FromPosition: *strip.Owner,
		ToPosition:   requesterPosition,
		IsTagRequest: true,
	}

	if err := coordRepo.Create(ctx, coord); err != nil {
		return err
	}

	s.frontendHub.SendCoordinationTagRequest(session, callsign, *strip.Owner, requesterPosition)
	return nil
}

// AcceptTagRequest allows the strip owner to accept a pending tag request, transferring ownership to the requester.
func (s *StripService) AcceptTagRequest(ctx context.Context, session int32, callsign string, ownerPosition string) error {
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

	if strip.Owner == nil || *strip.Owner != ownerPosition {
		return errors.New("only the strip owner can accept a tag request")
	}

	coordRepo := server.GetCoordinationRepository()
	coordination, err := coordRepo.GetByStripCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if !coordination.IsTagRequest {
		return errors.New("no pending tag request for this strip")
	}

	requesterPosition := coordination.ToPosition

	if err := coordRepo.Delete(ctx, coordination.ID); err != nil {
		return err
	}

	// Trim next owners: remove the requester and all positions before it.
	nextOwners := strip.NextOwners
	index := slices.Index(nextOwners, requesterPosition)
	isExpectedHandoff := index >= 0
	if isExpectedHandoff {
		nextOwners = nextOwners[index+1:]
	}

	// Build the new previous owners list.
	// Filter out the displaced owner first (avoids duplicates), then — for an
	// expected handoff — append them so they appear as a past controller.
	previousOwners := make([]string, 0, len(strip.PreviousOwners)+1)
	for _, p := range strip.PreviousOwners {
		if p != ownerPosition {
			previousOwners = append(previousOwners, p)
		}
	}
	if isExpectedHandoff {
		previousOwners = append(previousOwners, ownerPosition)
	}

	if err := s.stripRepo.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, previousOwners); err != nil {
		return err
	}

	count, err := s.stripRepo.SetOwner(ctx, session, callsign, &requesterPosition, strip.Version)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to set strip owner after accepting tag request")
	}

	// Recalculate route from the new owner (same as ForceAssumeStrip).
	if server := s.frontendHub.GetServer(); server != nil {
		_ = server.UpdateRouteForStrip(callsign, session, false)

		if updated, err := s.stripRepo.GetByCallsign(ctx, session, callsign); err == nil {
			nextOwners = updated.NextOwners
		}
	}

	// Remove from previous owners anyone who now appears in next owners.
	cleanedPrevious := make([]string, 0, len(previousOwners))
	for _, p := range previousOwners {
		if !slices.Contains(nextOwners, p) {
			cleanedPrevious = append(cleanedPrevious, p)
		}
	}
	if !slices.Equal(cleanedPrevious, previousOwners) {
		if err := s.stripRepo.SetPreviousOwners(ctx, session, callsign, cleanedPrevious); err != nil {
			return err
		}
		previousOwners = cleanedPrevious
	}

	s.frontendHub.SendCoordinationAssume(session, callsign, requesterPosition)
	s.frontendHub.SendOwnersUpdate(session, callsign, requesterPosition, nextOwners, previousOwners)
	return nil
}

// CancelCoordinationTransfer cancels a pending coordination transfer initiated by the caller.
func (s *StripService) CancelCoordinationTransfer(ctx context.Context, session int32, callsign string, position string) error {
	if s.coordRepo == nil {
		return errors.New("coordination repository not configured")
	}

	coordination, err := s.coordRepo.GetByStripCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if coordination.FromPosition != position {
		return errors.New("cannot cancel a transfer that you did not initiate")
	}

	if err := s.coordRepo.Delete(ctx, coordination.ID); err != nil {
		return err
	}
	s.frontendHub.SendCoordinationReject(session, callsign, position)
	return nil
}

// FreeStrip releases ownership of a strip and notifies all frontend clients.
func (s *StripService) FreeStrip(ctx context.Context, session int32, callsign string, position string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if strip.Owner == nil || *strip.Owner != position {
		return errors.New("cannot free strip which is not owned by you")
	}

	previousOwners := append(strip.PreviousOwners, position)

	if err := s.stripRepo.SetPreviousOwners(ctx, session, callsign, previousOwners); err != nil {
		return err
	}

	count, err := s.stripRepo.SetOwner(ctx, session, callsign, nil, strip.Version)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to set strip owner")
	}

	s.frontendHub.SendCoordinationFree(session, callsign)
	s.frontendHub.SendOwnersUpdate(session, callsign, "", strip.NextOwners, previousOwners)
	return nil
}

// UpdateClearedFlagForMove handles the frontend "move to cleared/not-cleared bay" action.
// It updates the cleared flag and bay in the DB, triggers auto-assumption, and notifies EuroScope.
func (s *StripService) UpdateClearedFlagForMove(ctx context.Context, session int32, callsign string, isCleared bool, bay string, cid string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	count, err := s.stripRepo.UpdateClearedFlag(ctx, session, callsign, isCleared, bay, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to update strip cleared flag")
	}

	// Only trigger side-effects when the cleared flag actually changed value.
	if strip.Cleared != isCleared {
		if isCleared {
			if err := s.AutoAssumeForClearedStrip(ctx, session, callsign); err != nil {
				slog.Error("Failed to auto-assume cleared strip", slog.Any("error", err))
			}
		}
		if s.euroscopeHub != nil {
			s.euroscopeHub.SendClearedFlag(session, cid, callsign, isCleared)
		}
	}
	return nil
}

// UpdateGroundStateForMove handles the frontend "move to general bay" action.
// It computes the new ground state, updates the DB, and notifies EuroScope.
func (s *StripService) UpdateGroundStateForMove(ctx context.Context, session int32, callsign string, bay string, cid string, airport string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	state := strip.State
	if strip.Origin == airport {
		groundState := shared.GetGroundState(bay)
		if groundState != euroscope.GroundStateUnknown {
			state = &groundState
		}
	}

	count, err := s.stripRepo.UpdateGroundState(ctx, session, callsign, state, bay, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to update strip bay/ground state")
	}

	if state != strip.State && state != nil && s.euroscopeHub != nil {
		s.euroscopeHub.SendGroundState(session, cid, callsign, *state)
	}

	// If the strip is moved backward from rwy-dep to a non-airborne bay, reset runway_cleared.
	if strip.Bay == shared.BAY_DEPART && bay != shared.BAY_DEPART && bay != shared.BAY_AIRBORNE && strip.RunwayCleared {
		if _, err := s.stripRepo.ResetRunwayClearance(ctx, session, callsign); err != nil {
			return err
		}
		s.frontendHub.SendStripUpdate(session, callsign)
	}

	return nil
}

// UpdateReleasePoint updates the release point for a strip and broadcasts to all frontend clients.
func (s *StripService) UpdateReleasePoint(ctx context.Context, session int32, callsign string, releasePoint string) error {
	affected, err := s.stripRepo.UpdateReleasePoint(ctx, session, callsign, &releasePoint)
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("failed to update release point")
	}
	s.frontendHub.Broadcast(session, frontend.ReleasePointEvent{
		Callsign:     callsign,
		ReleasePoint: releasePoint,
	})
	return nil
}

// ApplyReleasePoint updates the release point with ownership enforcement and broadcasts.
// Non-owners may overwrite an existing value (marks the cell yellow for both controllers).
// Non-owners setting a value on a strip that has none are rejected.
func (s *StripService) ApplyReleasePoint(ctx context.Context, session int32, callsign string, releasePoint string, clientPosition string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	isOwner := strip.Owner == nil || *strip.Owner == "" || *strip.Owner == clientPosition
	unexpectedChange := false

	if !isOwner {
		hasExisting := strip.ReleasePoint != nil && *strip.ReleasePoint != ""
		if hasExisting && *strip.ReleasePoint != releasePoint {
			// Non-owner overwriting existing value — allow, mark as unexpected change.
			if err := s.stripRepo.AppendUnexpectedChangeField(ctx, session, callsign, "release_point"); err != nil {
				return err
			}
			unexpectedChange = true
		} else if !hasExisting {
			// Non-owner setting a value that didn't exist — reject.
			return errors.New("cannot modify holding point on unowned strip")
		}
		// Non-owner setting same value as existing — silently allow.
	}

	if err := s.UpdateReleasePoint(ctx, session, callsign, releasePoint); err != nil {
		return err
	}

	if unexpectedChange {
		s.frontendHub.SendStripUpdate(session, callsign)
	}
	return nil
}

// UpdateMarked updates the marked flag for a strip and broadcasts to all frontend clients.
func (s *StripService) UpdateMarked(ctx context.Context, session int32, callsign string, marked bool) error {
	affected, err := s.stripRepo.UpdateMarked(ctx, session, callsign, marked, nil)
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("failed to update marked flag")
	}
	s.frontendHub.Broadcast(session, frontend.MarkedEvent{
		Callsign: callsign,
		Marked:   marked,
	})
	return nil
}

// RunwayClearance marks a strip as runway-cleared, moving it from TAXI_LWR to DEPART (if applicable),
// then broadcasts the full updated strip to all clients in the session.
// For departures from this airport in TAXI_LWR or DEPART bay, the ground state is set to 'DEPA'
// and sent to EuroScope.
func (s *StripService) RunwayClearance(ctx context.Context, session int32, callsign string, cid string, airport string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	affected, err := s.stripRepo.UpdateRunwayClearance(ctx, session, callsign)
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("failed to update runway clearance")
	}

	// For departures moving to or already at rwy-dep, set state to DEPA and notify ES.
	isAtOrMovingToDepart := strip.Bay == shared.BAY_DEPART || strip.Bay == shared.BAY_TAXI_LWR
	if isAtOrMovingToDepart && strip.Origin == airport {
		state := euroscope.GroundStateDepart
		if _, err := s.stripRepo.UpdateGroundState(ctx, session, callsign, &state, shared.BAY_DEPART, nil); err != nil {
			return err
		}
		if s.euroscopeHub != nil {
			s.euroscopeHub.SendGroundState(session, cid, callsign, state)
		}
	}

	s.frontendHub.SendStripUpdate(session, callsign)
	return nil
}

// PropagateRunwayChange updates the runway on strips that had an auto-assigned runway
// matching the old active runways.
func (s *StripService) PropagateRunwayChange(ctx context.Context, session int32, airport string, oldRunways models.ActiveRunways, newRunways models.ActiveRunways) error {
	strips, err := s.stripRepo.List(ctx, session)
	if err != nil {
		return err
	}

	for _, strip := range strips {
		if strip.Runway == nil || *strip.Runway == "" {
			continue
		}
		currentRunway := *strip.Runway
		isArrival := strip.Destination == airport

		var oldList []string
		var newList []string
		if isArrival {
			oldList = oldRunways.ArrivalRunways
			newList = newRunways.ArrivalRunways
		} else {
			oldList = oldRunways.DepartureRunways
			newList = newRunways.DepartureRunways
		}

		if !slices.Contains(oldList, currentRunway) {
			continue
		}
		if len(newList) == 0 {
			continue
		}

		newRunway := newList[0]
		if newRunway == currentRunway {
			continue
		}

		if _, err := s.stripRepo.UpdateRunway(ctx, session, strip.Callsign, &newRunway, nil); err != nil {
			slog.Error("Failed to update auto-assigned runway on strip",
				slog.String("callsign", strip.Callsign),
				slog.String("old_runway", currentRunway),
				slog.String("new_runway", newRunway),
				slog.Any("error", err))
		}
	}
	return nil
}

// SyncStrip creates or updates a strip from an EuroScope sync/strip-update event.
// The strip parameter must be of type euroscope.Strip.
func (s *StripService) SyncStrip(ctx context.Context, session int32, strip interface{}, airport string) error {
	esStrip, ok := strip.(euroscope.Strip)
	if !ok {
		return fmt.Errorf("SyncStrip: unexpected strip type %T", strip)
	}
	return s.syncEuroscopeStrip(ctx, session, esStrip, airport)
}

func (s *StripService) syncEuroscopeStrip(ctx context.Context, session int32, strip euroscope.Strip, airport string) error {
	server := s.frontendHub.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	// Fetch the session so we can read ActiveRunways for runway auto-assignment.
	sessionObj, err := server.GetSessionRepository().GetByID(ctx, session)
	if err != nil {
		return err
	}

	existingStrip, err := s.stripRepo.GetByCallsign(ctx, session, strip.Callsign)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	var bay string

	if errors.Is(err, pgx.ErrNoRows) {
		// Strip doesn't exist, so insert
		bay = shared.GetDepartureBay(strip, nil, config.GetAirborneAltitudeAGL(), airport)

		isArrival := strip.Destination == airport
		runwayForStrip := strip.Runway
		if runwayForStrip == "" {
			runwayForStrip = autoAssignRunway(isArrival, sessionObj.ActiveRunways)
		}

		newStrip := &internalModels.Strip{
			Callsign:           strip.Callsign,
			Session:            session,
			Origin:             strip.Origin,
			Destination:        strip.Destination,
			Alternative:        &strip.Alternate,
			Route:              &strip.Route,
			Remarks:            &strip.Remarks,
			Runway:             &runwayForStrip,
			Squawk:             &strip.Squawk,
			AssignedSquawk:     &strip.AssignedSquawk,
			Sid:                &strip.Sid,
			Cleared:            strip.Cleared,
			State:              &strip.GroundState,
			ClearedAltitude:    &strip.ClearedAltitude,
			RequestedAltitude:  &strip.RequestedAltitude,
			Heading:            &strip.Heading,
			AircraftType:       &strip.AircraftType,
			AircraftCategory:   &strip.AircraftCategory,
			PositionLatitude:   &strip.Position.Lat,
			PositionLongitude:  &strip.Position.Lon,
			PositionAltitude:   &strip.Position.Altitude,
			Stand:              &strip.Stand,
			Capabilities:       &strip.Capabilities,
			CommunicationType:  &strip.CommunicationType,
			Tobt:               &strip.Eobt,
			Bay:                bay,
			Eobt:               &strip.Eobt,
			TrackingController: strip.TrackingController,
			EngineType:         strip.EngineType,
		}
		reg := ParseRegistration(strip.Callsign, strip.Remarks)
		newStrip.Registration = &reg
		if err = s.stripRepo.Create(ctx, newStrip); err != nil {
			return err
		}
		if strip.HasFP {
			if err = s.stripRepo.SetHasFP(ctx, session, strip.Callsign, true); err != nil {
				slog.Warn("Failed to set has_fp on new strip", slog.String("callsign", strip.Callsign), slog.Any("error", err))
			}
		}
		slog.Debug("Inserted strip", slog.String("callsign", strip.Callsign))
	} else {
		// Strip exists, update it
		dbExistingStrip := database.Strip{
			Origin:      existingStrip.Origin,
			Destination: existingStrip.Destination,
			Cleared:     existingStrip.Cleared,
			Bay:         existingStrip.Bay,
			State:       existingStrip.State,
			Stand:       existingStrip.Stand,
		}
		bay = shared.GetDepartureBay(strip, &dbExistingStrip, config.GetAirborneAltitudeAGL(), airport)

		stand := existingStrip.Stand
		if strip.Stand != "" {
			stand = &strip.Stand
		}

		runway := existingStrip.Runway
		if strip.Runway != "" {
			runway = &strip.Runway
		} else if runway == nil || *runway == "" {
			isArrivalUpdate := strip.Destination == airport
			if assigned := autoAssignRunway(isArrivalUpdate, sessionObj.ActiveRunways); assigned != "" {
				runway = &assigned
			}
		}

		updateStrip := &internalModels.Strip{
			Callsign:           strip.Callsign,
			Session:            session,
			Origin:             strip.Origin,
			Destination:        strip.Destination,
			Alternative:        &strip.Alternate,
			Route:              &strip.Route,
			Remarks:            &strip.Remarks,
			AssignedSquawk:     &strip.AssignedSquawk,
			Squawk:             &strip.Squawk,
			Sid:                &strip.Sid,
			ClearedAltitude:    &strip.ClearedAltitude,
			Heading:            &strip.Heading,
			AircraftType:       &strip.AircraftType,
			Runway:             runway,
			RequestedAltitude:  &strip.RequestedAltitude,
			Capabilities:       &strip.Capabilities,
			CommunicationType:  &strip.CommunicationType,
			AircraftCategory:   &strip.AircraftCategory,
			Stand:              stand,
			Cleared:            strip.Cleared,
			State:              &strip.GroundState,
			PositionLatitude:   &strip.Position.Lat,
			PositionLongitude:  &strip.Position.Lon,
			PositionAltitude:   &strip.Position.Altitude,
			Bay:                bay,
			Tobt: func() *string {
				if strip.Eobt != "" {
					return &strip.Eobt
				}
				return existingStrip.Tobt
			}(),
			Eobt: func() *string {
				if strip.Eobt != "" {
					return &strip.Eobt
				}
				return existingStrip.Eobt
			}(),
			Registration:       existingStrip.Registration,
			Owner:              existingStrip.Owner,
			TrackingController: strip.TrackingController,
			EngineType:         strip.EngineType,
		}
		if _, err = s.stripRepo.Update(ctx, updateStrip); err != nil {
			return err
		}
		if strip.HasFP != existingStrip.HasFP {
			if err = s.stripRepo.SetHasFP(ctx, session, strip.Callsign, strip.HasFP); err != nil {
				slog.Warn("Failed to update has_fp on strip", slog.String("callsign", strip.Callsign), slog.Any("error", err))
			}
		}
		slog.Debug("Updated strip", slog.String("callsign", strip.Callsign))

		// Mark unexpected changes: stand is always unexpected when overwriting a non-empty value.
		// Runway is unexpected only for apron bays (not CLX/DEL/TWR).
		if strip.Stand != "" && existingStrip.Stand != nil && *existingStrip.Stand != "" && *existingStrip.Stand != strip.Stand {
			if err := s.stripRepo.AppendUnexpectedChangeField(ctx, session, strip.Callsign, "stand"); err != nil {
				slog.Warn("Failed to mark stand as unexpected change", slog.String("callsign", strip.Callsign), slog.Any("error", err))
			}
		}
		if strip.Runway != "" && existingStrip.Runway != nil && *existingStrip.Runway != "" && *existingStrip.Runway != strip.Runway && isApronBay(bay) {
			if err := s.stripRepo.AppendUnexpectedChangeField(ctx, session, strip.Callsign, "runway"); err != nil {
				slog.Warn("Failed to mark runway as unexpected change", slog.String("callsign", strip.Callsign), slog.Any("error", err))
			}
		}

		if existingStrip.Registration == nil || remarksContainsRegService(strip.Remarks) {
			newReg := ParseRegistration(strip.Callsign, strip.Remarks)
			if err := s.stripRepo.UpdateRegistration(ctx, session, strip.Callsign, newReg); err != nil {
				slog.Error("Failed to update registration from remarks", slog.Any("error", err))
			}
		}
	}

	if err := server.UpdateRouteForStrip(strip.Callsign, session, false); err != nil {
		slog.Error("Error updating route for strip", slog.String("callsign", strip.Callsign), slog.Any("error", err))
	}

	if err := s.MoveToBay(ctx, session, strip.Callsign, bay, false); err != nil {
		slog.Error("Error moving bay for strip", slog.String("callsign", strip.Callsign), slog.Any("error", err))
	}

	s.frontendHub.SendStripUpdate(session, strip.Callsign)

	return nil
}

var remarksRegReService = regexp.MustCompile(`\bREG/([A-Z0-9-]+)`)

func remarksContainsRegService(remarks string) bool {
	return remarksRegReService.MatchString(strings.ToUpper(remarks))
}

// isApronBay returns true if the bay is managed by the apron controller
// (i.e., not CLX/DEL bays and not the TWR departure lineup bay).
// Runway unexpected-change marking is only applied for apron bays.
func isApronBay(bay string) bool {
	switch bay {
	case shared.BAY_PUSH, shared.BAY_TAXI, shared.BAY_TAXI_LWR, shared.BAY_TAXI_TWR,
		shared.BAY_TWY_ARR, shared.BAY_STAND:
		return true
	default:
		return false
	}
}

// autoAssignRunway returns the first active runway for the strip's direction,
// or "" if no active runways are configured.
func autoAssignRunway(isArrival bool, activeRunways models.ActiveRunways) string {
	if isArrival {
		if len(activeRunways.ArrivalRunways) > 0 {
			return activeRunways.ArrivalRunways[0]
		}
	} else {
		if len(activeRunways.DepartureRunways) > 0 {
			return activeRunways.DepartureRunways[0]
		}
	}
	return ""
}
