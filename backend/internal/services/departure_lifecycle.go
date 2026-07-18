package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// Departure reservation stages persisted on StandAssignment.Stage. The
// lifecycle owns these values; the allocation service only persists them.
const (
	StageReserved       = "RESERVED"
	StageDepartureBlock = "DEPARTURE_BLOCK"
)

const (
	defaultDepartureHoldDuration    = 15 * time.Minute
	defaultDepartureBlockExtension  = 10 * time.Minute
	defaultDepartureSweepInterval   = 30 * time.Second
	defaultWrongStandDeadline       = 5 * time.Minute
	departureClockRolloverThreshold = 12 * time.Hour
	wrongStandAwaitingPrefix        = "WRONG_STAND_AWAITING_MESSAGE: observed "
	wrongStandPendingPrefix         = "WRONG_STAND_PENDING: observed "
	wrongStandForcedPrefix          = "WRONG_STAND_FORCED: observed "
)

// DepartureLifecycleService owns the timing rules that turn a prefiled
// departure into a reserved stand and then a departure block. It delegates the
// atomic stand selection to StandAllocationService and reconstructs every
// deadline from persisted timestamps, so a backend restart needs no in-memory
// state to resume sweeps.
type DepartureLifecycleService struct {
	allocations    *StandAllocationService
	assignments    repository.StandAssignmentRepository
	strips         repository.StripRepository
	sessions       lifecycleSessionLister
	stands         *sat.StandCapabilityRegistry
	aircraft       *sat.AircraftRegistry
	engines        *sat.AircraftEngineRegistry
	borders        *sat.AirportCountryRegistry
	now            func() time.Time
	hold           time.Duration
	blockExtension time.Duration
	sweepInterval  time.Duration
	messenger      wrongStandMessenger
	warningMu      sync.Mutex
	warnings       map[string]string
}

type wrongStandMessenger interface {
	SendPrivateMessageFromDelivery(session int32, callsign, message string) bool
}

func (s *DepartureLifecycleService) SetWrongStandMessenger(messenger wrongStandMessenger) {
	s.messenger = messenger
}

// CancelDeparture cancels transient wrong-stand state when a departure
// disappears from the live feed.
func (s *DepartureLifecycleService) CancelDeparture(ctx context.Context, session int32, callsign string) error {
	s.clearUnassignedStandWarning(session, callsign)
	return s.cancelWrongStandEpisode(ctx, session, callsign)
}

type lifecycleSessionLister interface {
	List(context.Context) ([]*models.Session, error)
}

type DepartureLifecycleOption func(*DepartureLifecycleService)

func WithDepartureLifecycleClock(now func() time.Time) DepartureLifecycleOption {
	return func(s *DepartureLifecycleService) {
		if now != nil {
			s.now = now
		}
	}
}

func WithDepartureHoldDuration(duration time.Duration) DepartureLifecycleOption {
	return func(s *DepartureLifecycleService) {
		if duration > 0 {
			s.hold = duration
		}
	}
}

func WithDepartureBlockExtension(duration time.Duration) DepartureLifecycleOption {
	return func(s *DepartureLifecycleService) {
		if duration > 0 {
			s.blockExtension = duration
		}
	}
}

func WithDepartureSweepInterval(duration time.Duration) DepartureLifecycleOption {
	return func(s *DepartureLifecycleService) {
		if duration > 0 {
			s.sweepInterval = duration
		}
	}
}

func NewDepartureLifecycleService(
	allocations *StandAllocationService,
	assignments repository.StandAssignmentRepository,
	strips repository.StripRepository,
	sessions lifecycleSessionLister,
	stands *sat.StandCapabilityRegistry,
	aircraft *sat.AircraftRegistry,
	engines *sat.AircraftEngineRegistry,
	borders *sat.AirportCountryRegistry,
	options ...DepartureLifecycleOption,
) (*DepartureLifecycleService, error) {
	if allocations == nil || assignments == nil || strips == nil || sessions == nil || stands == nil {
		return nil, errors.New("departure lifecycle requires allocation service, repositories, session store, and stand registry")
	}
	service := &DepartureLifecycleService{
		allocations:    allocations,
		assignments:    assignments,
		strips:         strips,
		sessions:       sessions,
		stands:         stands,
		aircraft:       aircraft,
		engines:        engines,
		borders:        borders,
		now:            time.Now,
		hold:           defaultDepartureHoldDuration,
		blockExtension: defaultDepartureBlockExtension,
		sweepInterval:  defaultDepartureSweepInterval,
		warnings:       make(map[string]string),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service, nil
}

// ProcessDeparture is the reconciler entry point. It dispatches to the
// reservation path while the aircraft is offline and to the block path once it
// is online. Both paths are idempotent: repeated feed polls with no change in
// revision or timing leave the persisted assignment untouched.
func (s *DepartureLifecycleService) ProcessDeparture(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo) (err error) {
	defer func() { err = suppressAutomaticAllocationError(err) }()
	if strip == nil || strings.TrimSpace(strip.Callsign) == "" {
		return nil
	}
	if flight.Online {
		activated, err := s.activateObservedBlock(ctx, session, strip, flight)
		if err != nil {
			return err
		}
		if !activated {
			return nil
		}
		return s.revalidateFacts(ctx, session, strip, flight)
	}
	if err := s.cancelWrongStandEpisode(ctx, session, strip.Callsign); err != nil {
		return err
	}
	return s.ensureReservation(ctx, session, strip, flight)
}

// ObserveDeparturePosition applies the live stand-detection path to an
// EuroScope aircraft that has no VATSIM feed record.
func (s *DepartureLifecycleService) ObserveDeparturePosition(ctx context.Context, session int32, strip *models.Strip, latitude, longitude float64) error {
	if strip == nil || strings.TrimSpace(strip.Callsign) == "" {
		return nil
	}
	activated, err := s.activateObservedBlock(ctx, session, strip, vatsim.DepartureFlightInfo{
		Callsign: strip.Callsign, Online: true, Origin: strip.Origin,
		Destination: strip.Destination, AircraftType: valueString(strip.AircraftType),
		Latitude: latitude, Longitude: longitude,
	})
	if err != nil || !activated {
		return err
	}
	return s.revalidateFacts(ctx, session, strip, vatsim.DepartureFlightInfo{
		Callsign: strip.Callsign, Online: true, Origin: strip.Origin,
		Destination: strip.Destination, AircraftType: valueString(strip.AircraftType),
		Latitude: latitude, Longitude: longitude,
	})
}

// activateObservedBlock only converts an online aircraft to a departure block
// after its live position resolves to a stand. A compatible, available spawn
// stand replaces the reservation atomically. An unavailable or incompatible
// observed stand leaves the reservation intact and records the mismatch for
// task 19's future warning/deadline workflow.
func (s *DepartureLifecycleService) activateObservedBlock(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo) (bool, error) {
	observed, found := s.stands.StandAtPosition(strings.TrimSpace(strip.Origin), flight.Latitude, flight.Longitude)
	if !found {
		s.clearUnassignedStandWarning(session, strip.Callsign)
		slog.Warn("online departure is not inside a configured stand radius",
			slog.String("callsign", strip.Callsign),
			slog.Float64("latitude", flight.Latitude),
			slog.Float64("longitude", flight.Longitude))
		if err := s.cancelWrongStandEpisode(ctx, session, strip.Callsign); err != nil {
			return false, err
		}
		return false, nil
	}

	existing, err := s.assignments.GetAssignment(ctx, session, strip.Callsign)
	if err != nil && !isNotFound(err) {
		return false, err
	}
	if existing != nil && strings.EqualFold(existing.Stand, observed.Name) {
		return true, s.activateBlock(ctx, session, strip, flight)
	}

	pendingReason := wrongStandPendingPrefix + observed.Name
	awaitingReason := wrongStandAwaitingPrefix + observed.Name
	if existing != nil && existing.ConflictReason != nil {
		switch *existing.ConflictReason {
		case pendingReason:
			return false, nil
		case awaitingReason:
			return false, s.deliverWrongStandWarning(ctx, existing)
		}
	}

	expiry := s.computeBlockExpiry(strip)
	request := s.buildRequest(session, strip, flight, StageDepartureBlock, expiry)
	request.Stand = observed.Name
	observedRequest := request
	observedRequest.ExpiresAt = nil
	if _, err := s.allocations.assignObservedStand(ctx, observedRequest); err == nil {
		s.clearUnassignedStandWarning(session, strip.Callsign)
		return true, nil
	} else if existing == nil {
		automaticRequest := s.buildRequest(session, strip, flight, StageDepartureBlock, expiry)
		result, allocationErr := s.allocations.Allocate(ctx, automaticRequest)
		if allocationErr != nil {
			if errors.Is(allocationErr, ErrAutomaticAllocationSuppressed) {
				return false, nil
			}
			s.deliverUnassignedOccupiedStandWarning(session, strip.Callsign, observed.Name)
			slog.Warn("observed departure stand is occupied and no alternative stand could be assigned",
				slog.String("callsign", strip.Callsign),
				slog.String("observedStand", observed.Name),
				slog.Any("observed_stand_error", err),
				slog.Any("alternative_allocation_error", allocationErr))
			return false, nil
		}
		s.clearUnassignedStandWarning(session, strip.Callsign)
		existing = &result.Assignment
	}

	if existing.ConflictReason != nil {
		switch *existing.ConflictReason {
		case pendingReason:
			return false, nil
		case awaitingReason:
			return false, s.deliverWrongStandWarning(ctx, existing)
		}
	}
	updated := *existing
	updated.ConflictReason = &awaitingReason
	updated.Acknowledged = false
	updated.AcknowledgedAt = nil
	updated.AcknowledgedBy = nil
	if affected, updateErr := s.assignments.UpdateAssignment(ctx, &updated); updateErr != nil {
		return false, updateErr
	} else if affected != 1 {
		return false, fmt.Errorf("record observed stand mismatch version conflict for %s", strip.Callsign)
	}
	updated.Version++
	if err := s.allocations.PublishAssignment(ctx, updated); err != nil {
		return false, fmt.Errorf("publish observed stand mismatch for %s: %w", strip.Callsign, err)
	}
	return false, s.deliverWrongStandWarning(ctx, &updated)
}

func (s *DepartureLifecycleService) deliverUnassignedOccupiedStandWarning(session int32, callsign, observedStand string) {
	if s.messenger == nil {
		return
	}
	key := fmt.Sprintf("%d:%s", session, strings.ToUpper(strings.TrimSpace(callsign)))

	s.warningMu.Lock()
	defer s.warningMu.Unlock()
	if warnedStand, ok := s.warnings[key]; ok && strings.EqualFold(warnedStand, observedStand) {
		return
	}
	if s.messenger.SendPrivateMessageFromDelivery(session, callsign,
		fmt.Sprintf("STAND ASSIGNMENT: STAND %s IS OCCUPIED. PLEASE RELOCATE", observedStand)) {
		s.warnings[key] = observedStand
	}
}

func (s *DepartureLifecycleService) clearUnassignedStandWarning(session int32, callsign string) {
	key := fmt.Sprintf("%d:%s", session, strings.ToUpper(strings.TrimSpace(callsign)))
	s.warningMu.Lock()
	delete(s.warnings, key)
	s.warningMu.Unlock()
}

func (s *DepartureLifecycleService) deliverWrongStandWarning(ctx context.Context, assignment *models.StandAssignment) error {
	if assignment == nil || assignment.ConflictReason == nil ||
		!strings.HasPrefix(*assignment.ConflictReason, wrongStandAwaitingPrefix) ||
		s.messenger == nil {
		return nil
	}
	if !s.messenger.SendPrivateMessageFromDelivery(assignment.SessionID, assignment.Callsign,
		fmt.Sprintf("STAND ASSIGNMENT: PLEASE RELOCATE TO YOUR ASSIGNED STAND %s", assignment.Stand)) {
		return nil
	}
	observed := strings.TrimSpace(strings.TrimPrefix(*assignment.ConflictReason, wrongStandAwaitingPrefix))
	updated := *assignment
	reason := wrongStandPendingPrefix + observed
	updated.ConflictReason = &reason
	deadline := s.now().Add(defaultWrongStandDeadline)
	updated.ExpiresAt = &deadline
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("activate wrong stand deadline version conflict for %s", assignment.Callsign)
	}
	updated.Version++
	return s.allocations.PublishAssignment(ctx, updated)
}

func (s *DepartureLifecycleService) cancelWrongStandEpisode(ctx context.Context, session int32, callsign string) error {
	existing, err := s.assignments.GetAssignment(ctx, session, callsign)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	if existing.ConflictReason == nil ||
		(!strings.HasPrefix(*existing.ConflictReason, wrongStandPendingPrefix) &&
			!strings.HasPrefix(*existing.ConflictReason, wrongStandAwaitingPrefix)) {
		return nil
	}
	updated := *existing
	updated.ConflictReason = nil
	expiry := s.now().Add(s.hold)
	updated.ExpiresAt = &expiry
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("cancel wrong stand episode version conflict for %s", callsign)
	}
	updated.Version++
	return s.allocations.PublishAssignment(ctx, updated)
}

// ensureReservation allocates a 15-minute hold for a new offline prefile, and
// renews or reallocates it when a later qualifying flight-plan revision arrives.
func (s *DepartureLifecycleService) ensureReservation(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo) error {
	existing, err := s.assignments.GetAssignment(ctx, session, strip.Callsign)
	if err != nil && !isNotFound(err) {
		return err
	}
	now := s.now()
	if existing == nil {
		expiry := now.Add(s.hold)
		request := s.buildRequest(session, strip, flight, StageReserved, &expiry)
		_, err := s.allocations.Allocate(ctx, request)
		return err
	}
	if existing.Stage != StageReserved {
		return nil
	}
	if existing.VatsimRevision != nil && *existing.VatsimRevision == flight.Revision {
		return nil
	}
	expiry := now.Add(s.hold)
	request := s.buildRequest(session, strip, flight, StageReserved, &expiry)
	available, err := s.allocations.StandAvailable(ctx, request, existing.Stand)
	if err != nil {
		return err
	}
	if available {
		return s.renewInPlace(ctx, strip, existing, flight, expiry, now)
	}
	_, err = s.allocations.Reallocate(ctx, request)
	return err
}

// renewInPlace extends the current reservation's hold without changing its
// stand. A version conflict from a concurrent allocation falls back to a full
// reallocation so the aircraft keeps a valid stand.
func (s *DepartureLifecycleService) renewInPlace(ctx context.Context, strip *models.Strip, existing *models.StandAssignment, flight vatsim.DepartureFlightInfo, expiry, now time.Time) error {
	updated := *existing
	updated.ExpiresAt = &expiry
	updated.AssignedAt = &now
	updated.Stage = StageReserved
	if parseCID(flight.CID) != nil {
		revision := flight.Revision
		updated.VatsimRevision = &revision
	}
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected == 1 {
		updated.Version++
		return s.allocations.PublishAssignment(ctx, updated)
	}
	request := s.buildRequest(existing.SessionID, strip, flight, StageReserved, &expiry)
	_, err = s.allocations.Reallocate(ctx, request)
	return err
}

// activateBlock converts an active reservation into a departure block when the
// aircraft comes online, and recomputes the TSAT/TOBT retention deadline on
// every subsequent poll. Aircraft without a prior reservation receive a fresh
// block so an online departure that skipped the prefile stage still gets a
// stand.
func (s *DepartureLifecycleService) activateBlock(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo) error {
	existing, err := s.assignments.GetAssignment(ctx, session, strip.Callsign)
	if err != nil && !isNotFound(err) {
		return err
	}
	expiry := s.computeBlockExpiry(strip)
	now := s.now()
	if existing == nil {
		request := s.buildRequest(session, strip, flight, StageDepartureBlock, expiry)
		_, err := s.allocations.Allocate(ctx, request)
		return err
	}
	if existing.Stage != StageReserved && existing.Stage != StageDepartureBlock {
		return nil
	}
	onAssignedStand := s.stripIsAtAssignedStand(strip, existing.Stand)
	if onAssignedStand {
		expiry = nil
	}
	if existing.Stage == StageDepartureBlock {
		if expiry == nil && existing.ExpiresAt != nil && !onAssignedStand {
			return nil
		}
		if sameExpiry(existing.ExpiresAt, expiry) {
			return nil
		}
	}
	updated := *existing
	updated.Stage = StageDepartureBlock
	updated.ExpiresAt = expiry
	if updated.ConflictReason != nil && strings.HasPrefix(*updated.ConflictReason, wrongStandPendingPrefix) {
		updated.ConflictReason = nil
	}
	updated.AssignedAt = &now
	if parseCID(flight.CID) != nil {
		revision := flight.Revision
		updated.VatsimRevision = &revision
	}
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected == 1 {
		if existing.Stage != StageDepartureBlock {
			slog.InfoContext(ctx, "SAT assignment stage changed", slog.String("callsign", existing.Callsign), slog.String("stand", existing.Stand), slog.String("from_stage", existing.Stage), slog.String("to_stage", StageDepartureBlock))
		}
		updated.Version++
		return s.allocations.PublishAssignment(ctx, updated)
	}
	reloaded, reloadErr := s.assignments.GetAssignment(ctx, session, strip.Callsign)
	if reloadErr != nil || reloaded == nil {
		return nil
	}
	if reloaded.Stage == StageDepartureBlock && expiry == nil && reloaded.ExpiresAt != nil {
		return nil
	}
	reloaded.Stage = StageDepartureBlock
	reloaded.ExpiresAt = expiry
	reloaded.AssignedAt = &now
	if parseCID(flight.CID) != nil {
		revision := flight.Revision
		reloaded.VatsimRevision = &revision
	}
	affected, err = s.assignments.UpdateAssignment(ctx, reloaded)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("activate departure block version conflict for %s", strip.Callsign)
	}
	reloaded.Version++
	return s.allocations.PublishAssignment(ctx, *reloaded)
}

// revalidateFacts re-runs compatibility against the strip's current aircraft and
// engine facts. EuroScope often supplies a live engine type or corrected
// aircraft type after the initial reservation; when the assigned stand no
// longer fits, a reallocation produces the conflict and selects a replacement.
func (s *DepartureLifecycleService) revalidateFacts(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo) error {
	existing, err := s.assignments.GetAssignment(ctx, session, strip.Callsign)
	if err != nil && !isNotFound(err) {
		return err
	}
	if existing == nil {
		return nil
	}
	facts, assignmentFacts := s.resolveFacts(strip, flight)
	if !facts.Complete() {
		return nil
	}
	evaluation := s.stands.EvaluateCompatibility(strings.ToUpper(strings.TrimSpace(strip.Origin)), facts)
	if standCompatible(evaluation, existing.Stand) {
		return nil
	}
	request := StandAllocationRequest{
		SessionID:       session,
		Callsign:        strip.Callsign,
		Airport:         strings.ToUpper(strings.TrimSpace(strip.Origin)),
		Direction:       sat.AssignmentDirectionDeparture,
		Stage:           existing.Stage,
		FlightFacts:     facts,
		AssignmentFacts: assignmentFacts,
		ExpiresAt:       existing.ExpiresAt,
		VatsimCID:       existing.VatsimCID,
		VatsimRevision:  existing.VatsimRevision,
	}
	_, err = s.allocations.Reallocate(ctx, request)
	return err
}

// ReleaseExpired releases expired offline reservations and completed departure
// blocks. A departure block is completed when its strip no longer exists (the
// aircraft has departed). The sweep is idempotent and reconstructs every
// deadline from persisted ExpiresAt timestamps, so it is safe to run after a
// restart.
func (s *DepartureLifecycleService) ReleaseExpired(ctx context.Context) error {
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	for _, session := range sessions {
		if session == nil {
			continue
		}
		assignments, err := s.assignments.ListAssignments(ctx, session.ID)
		if err != nil {
			slog.Warn("departure sweep cannot list session assignments",
				slog.Int("sessionID", int(session.ID)),
				slog.Any("error", err))
			continue
		}
		for _, assignment := range assignments {
			if assignment == nil {
				continue
			}
			if err := s.releaseIfDue(ctx, session.ID, assignment, now); err != nil {
				slog.Warn("departure sweep failed to release assignment",
					slog.String("callsign", assignment.Callsign),
					slog.Any("error", err))
			}
		}
	}
	return nil
}

func (s *DepartureLifecycleService) releaseIfDue(ctx context.Context, session int32, assignment *models.StandAssignment, now time.Time) error {
	strip, err := s.strips.GetByCallsign(ctx, session, assignment.Callsign)
	if err != nil && !isNotFound(err) {
		return err
	}
	if strip == nil {
		err := s.allocations.ReleaseAssignment(ctx, assignment)
		recordSATExpiry(ctx, assignment, "strip_removed", err)
		return err
	}
	if assignment.Stage == StageDepartureBlock && s.stripIsAtAssignedStand(strip, assignment.Stand) &&
		(assignment.ConflictReason == nil ||
			(!strings.HasPrefix(*assignment.ConflictReason, wrongStandPendingPrefix) &&
				!strings.HasPrefix(*assignment.ConflictReason, wrongStandAwaitingPrefix))) {
		return s.clearDepartureBlockExpiry(ctx, assignment)
	}
	if assignment.ExpiresAt == nil || assignment.ExpiresAt.After(now) {
		return nil
	}
	if assignment.ConflictReason != nil && strings.HasPrefix(*assignment.ConflictReason, wrongStandPendingPrefix) {
		return s.forceObservedStand(ctx, strip, assignment)
	}
	err = s.allocations.ReleaseAssignment(ctx, assignment)
	recordSATExpiry(ctx, assignment, "expired", err)
	return err
}

func (s *DepartureLifecycleService) clearDepartureBlockExpiry(ctx context.Context, assignment *models.StandAssignment) error {
	if assignment == nil || assignment.ExpiresAt == nil {
		return nil
	}
	updated := *assignment
	updated.ExpiresAt = nil
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("clear departure block expiry version conflict for %s", assignment.Callsign)
	}
	updated.Version++
	return s.allocations.PublishAssignment(ctx, updated)
}

func (s *DepartureLifecycleService) stripIsAtAssignedStand(strip *models.Strip, stand string) bool {
	if strip == nil {
		return false
	}
	if strip.PositionLatitude != nil && strip.PositionLongitude != nil {
		observed, found := s.stands.StandAtPosition(strings.TrimSpace(strip.Origin), *strip.PositionLatitude, *strip.PositionLongitude)
		return found && strings.EqualFold(observed.Name, stand)
	}
	return strip.Stand != nil && strings.EqualFold(strings.TrimSpace(*strip.Stand), strings.TrimSpace(stand))
}

func (s *DepartureLifecycleService) forceObservedStand(ctx context.Context, strip *models.Strip, assignment *models.StandAssignment) error {
	observed := strings.TrimSpace(strings.TrimPrefix(*assignment.ConflictReason, wrongStandPendingPrefix))
	if observed == "" {
		return nil
	}
	request := s.buildRequest(assignment.SessionID, strip, vatsim.DepartureFlightInfo{}, StageDepartureBlock, s.computeBlockExpiry(strip))
	request.Stand = observed
	request.ConflictReason = wrongStandForcedPrefix + observed + "; assigned " + assignment.Stand
	_, err := s.allocations.OverrideManually(ctx, request)
	return err
}

// StartSweep runs the expired-release loop until the context is cancelled. It is
// registered as a worker by the application composition root.
func (s *DepartureLifecycleService) StartSweep(ctx context.Context) {
	ticker := time.NewTicker(s.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ReleaseExpired(ctx); err != nil {
				slog.Warn("Departure lifecycle sweep failed", slog.Any("error", err))
			}
		}
	}
}

func (s *DepartureLifecycleService) buildRequest(session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo, stage string, expiresAt *time.Time) StandAllocationRequest {
	facts, assignmentFacts := s.resolveFacts(strip, flight)
	request := StandAllocationRequest{
		SessionID:       session,
		Callsign:        strip.Callsign,
		Airport:         strings.ToUpper(strings.TrimSpace(strip.Origin)),
		Direction:       sat.AssignmentDirectionDeparture,
		Stage:           stage,
		FlightFacts:     facts,
		AssignmentFacts: assignmentFacts,
		ExpiresAt:       expiresAt,
	}
	if cid := parseCID(flight.CID); cid != nil {
		revision := flight.Revision
		request.VatsimCID = cid
		request.VatsimRevision = &revision
	}
	return request
}

func (s *DepartureLifecycleService) resolveFacts(strip *models.Strip, flight vatsim.DepartureFlightInfo) (sat.FlightCompatibilityFacts, sat.AssignmentFlightFacts) {
	aircraftType := flight.AircraftType
	if strip != nil && strip.AircraftType != nil && strings.TrimSpace(*strip.AircraftType) != "" {
		aircraftType = *strip.AircraftType
	}
	origin, destination := "", ""
	if strip != nil {
		origin = strip.Origin
		destination = strip.Destination
	}
	input := sat.FlightCompatibilityInput{
		Direction:      sat.Departure,
		Origin:         origin,
		Destination:    destination,
		AircraftType:   aircraftType,
		LiveEngineType: engineTypeValue(strip),
	}
	facts := sat.ResolveFlightCompatibilityFacts(input, s.aircraft, s.engines, s.borders)
	assignmentFacts := sat.AssignmentFlightFacts{
		Callsign:     strip.Callsign,
		AircraftType: aircraftType,
		AircraftUse:  facts.Aircraft.UseCode,
		BorderStatus: facts.BorderStatus,
		Direction:    sat.AssignmentDirectionDeparture,
	}
	return facts, assignmentFacts
}

func (s *DepartureLifecycleService) computeBlockExpiry(strip *models.Strip) *time.Time {
	now := s.now()
	invalidReason := ""
	if calculation := strip.CdmData.EffectiveCalculation(); calculation != nil && calculation.InvalidReason != nil {
		invalidReason = strings.TrimSpace(*calculation.InvalidReason)
	}
	if invalidReason != models.CdmInvalidReasonStaleTsat {
		if expiry, ok := departureBlockExpiry(stripClockValue(strip.EffectiveTsat()), now, s.blockExtension); ok {
			return &expiry
		}
	}
	if invalidReason != models.CdmInvalidReasonStaleTobt {
		if expiry, ok := departureBlockExpiry(stripClockValue(strip.EffectiveTobt()), now, s.blockExtension); ok {
			return &expiry
		}
	}
	return nil
}

func departureBlockExpiry(value string, now time.Time, extension time.Duration) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	t, ok := parseDepartureClockUTC(value, now)
	if !ok {
		return time.Time{}, false
	}
	expiry := t.Add(extension)
	return expiry, expiry.After(now)
}

func standCompatible(evaluation sat.StandCompatibilityEvaluation, stand string) bool {
	target := standName(stand)
	for _, match := range evaluation.Matches {
		if standName(match.Stand.Name) == target {
			return true
		}
	}
	return false
}

func sameExpiry(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(*right)
}

func engineTypeValue(strip *models.Strip) string {
	if strip == nil {
		return ""
	}
	return strings.TrimSpace(strip.EngineType)
}

func stripClockValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func parseCID(cid string) *int64 {
	cid = strings.TrimSpace(cid)
	if cid == "" {
		return nil
	}
	value, err := strconv.ParseInt(cid, 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

// parseDepartureClockUTC parses a CDM HHMM or HHMMSS clock string into a UTC
// timestamp on the current day, rolling to the next day when the value is more
// than half a day in the past so TSAT/TOBT deadlines near midnight stay valid.
func parseDepartureClockUTC(value string, now time.Time) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if len(value) != 4 && len(value) != 6 {
		return time.Time{}, false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return time.Time{}, false
		}
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[2]-'0')*10 + int(value[3]-'0')
	second := 0
	if len(value) == 6 {
		second = int(value[4]-'0')*10 + int(value[5]-'0')
	}
	if hour > 23 || minute > 59 || second > 59 {
		return time.Time{}, false
	}
	candidate := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), hour, minute, second, 0, time.UTC)
	if now.UTC().Sub(candidate) > departureClockRolloverThreshold {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate, true
}

// isNotFound reports whether the error is a missing-row lookup result. The
// lifecycle treats a missing assignment as "no reservation yet" rather than a
// hard failure.
func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
