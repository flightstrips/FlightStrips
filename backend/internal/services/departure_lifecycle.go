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
	departureClockRolloverThreshold = 12 * time.Hour
	wrongStandAwaitingPrefix        = "WRONG_STAND_AWAITING_MESSAGE: observed "
	wrongStandPendingPrefix         = "WRONG_STAND_PENDING: observed "
	observedDeparturePriorConflict  = " | prior conflict: "
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
	allowPrefiles  bool
	messenger      wrongStandMessenger
	routeRecalc    RouteRecalculator
	standPublisher observedStandPublisher
	warningMu      sync.Mutex
	warnings       map[string]string
}

type wrongStandMessenger interface {
	SendPrivateMessageFromDelivery(session int32, callsign, message string) bool
}

type observedStandPublisher interface {
	SendStandEvent(session int32, callsign string, stand string)
}

func (s *DepartureLifecycleService) SetWrongStandMessenger(messenger wrongStandMessenger) {
	s.messenger = messenger
}

// SetRouteRecalculator lets the observed-stand recovery use the aircraft's
// physical stand for its ground route even while a protected booking remains.
func (s *DepartureLifecycleService) SetRouteRecalculator(routeRecalc RouteRecalculator) {
	s.routeRecalc = routeRecalc
}

func (s *DepartureLifecycleService) SetStandPublisher(publisher observedStandPublisher) {
	s.standPublisher = publisher
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

// WithDeparturePrefileAssignments enables advance stand reservations for
// offline VATSIM prefiles. The production default is false.
func WithDeparturePrefileAssignments(enabled bool) DepartureLifecycleOption {
	return func(s *DepartureLifecycleService) { s.allowPrefiles = enabled }
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
	if !s.allowPrefiles {
		return s.releaseOfflinePrefileReservation(ctx, session, strip.Callsign)
	}
	if err := s.cancelWrongStandEpisode(ctx, session, strip.Callsign); err != nil {
		return err
	}
	return s.ensureReservation(ctx, session, strip, flight)
}

func (s *DepartureLifecycleService) releaseOfflinePrefileReservation(ctx context.Context, session int32, callsign string) error {
	existing, err := s.assignments.GetAssignment(ctx, session, callsign)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	if existing.Manual || existing.Stage != StageReserved || existing.ObservedStand != nil {
		return nil
	}
	return s.allocations.ReleaseAssignment(ctx, existing)
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
// after its live position resolves to a stand. A free observed spawn stand
// replaces the reservation atomically. Because the aircraft is already
// physically present, capability mismatches alone do not force relocation.
// An occupied or blocked observed stand leaves the reservation intact and
// records the mismatch for the warning/deadline workflow.
func (s *DepartureLifecycleService) activateObservedBlock(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo) (bool, error) {
	observed, found := s.stands.StandAtPosition(strings.TrimSpace(strip.Origin), flight.Latitude, flight.Longitude)
	existing, err := s.assignments.GetAssignment(ctx, session, strip.Callsign)
	if err != nil && !isNotFound(err) {
		return false, err
	}
	if !found {
		s.clearUnassignedStandWarning(session, strip.Callsign)
		slog.Debug("online departure position does not resolve to a configured stand",
			slog.String("callsign", strip.Callsign),
			slog.Float64("latitude", flight.Latitude),
			slog.Float64("longitude", flight.Longitude))
		if existing != nil && existing.Stage == StageDepartureBlock && validDeparturePosition(flight.Latitude, flight.Longitude) {
			// The aircraft has positively moved away from every configured stand.
			// Its previous stand is physically free now; do not retain a nil-expiry
			// departure block until the EuroScope strip eventually disappears.
			return false, s.allocations.ReleaseAssignment(ctx, existing)
		}
		if err := s.cancelWrongStandEpisode(ctx, session, strip.Callsign); err != nil {
			return false, err
		}
		return false, nil
	}

	if existing != nil && strings.EqualFold(existing.Stand, observed.Name) {
		if err := s.activateBlock(ctx, session, strip, flight); err != nil {
			return true, err
		}
		return true, s.reconcileConfirmedArrivalConflict(ctx, session, strip, flight, observed.Name)
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

	projectedRelease := s.computeBlockExpiry(strip)
	expiry := projectedRelease
	request := s.buildRequest(session, strip, flight, StageDepartureBlock, expiry)
	request.Stand = observed.Name
	observedRequest := request
	observedRequest.ExpiresAt = nil
	observedStand := observed.Name
	observedRequest.ObservedStand = &observedStand
	if result, err := s.allocations.assignObservedStand(ctx, observedRequest); err == nil {
		s.useObservedStandForRoute(ctx, session, strip.Callsign, observed.Name)
		if result.StandChanged && s.standPublisher != nil {
			s.standPublisher.SendStandEvent(session, strip.Callsign, observed.Name)
		}
		s.clearUnassignedStandWarning(session, strip.Callsign)
		return true, nil
	} else if existing == nil {
		automaticRequest := s.buildRequest(session, strip, flight, StageDepartureBlock, expiry)
		result, allocationErr := s.allocations.Allocate(ctx, automaticRequest)
		if allocationErr != nil {
			if errors.Is(allocationErr, ErrAutomaticAllocationSuppressed) {
				s.useObservedStandForRoute(ctx, session, strip.Callsign, observed.Name)
				return false, nil
			}
			s.useObservedStandForRoute(ctx, session, strip.Callsign, observed.Name)
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
	// Keep the allocation reservation intact, but route from the physical stand
	// that EuroScope/VATSIM observed. A confirmed inbound booking may prevent
	// this departure from claiming that stand, not from using its real location.
	s.useObservedStandForRoute(ctx, session, strip.Callsign, observed.Name)

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
	// A live aircraft on the wrong stand must not lose or replace its persisted
	// destination assignment while it waits to relocate.
	updated.ExpiresAt = nil
	updated.ProjectedReleaseAt = expiry
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

func (s *DepartureLifecycleService) reconcileConfirmedArrivalConflict(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo, stand string) error {
	request := s.buildRequest(session, strip, flight, StageDepartureBlock, nil)
	conflict, err := s.allocations.ConfirmedArrivalConflictAtStand(ctx, request, stand)
	if err != nil {
		return err
	}
	assignment, err := s.assignments.GetAssignment(ctx, session, strip.Callsign)
	if err != nil {
		return err
	}
	currentManaged := assignment.ConflictReason != nil && strings.HasPrefix(*assignment.ConflictReason, observedDepartureConflictPrefix)
	if conflict == currentManaged {
		return nil
	}
	updated := *assignment
	if conflict {
		updated.ConflictReason = observedDepartureConflictReason(assignment.ConflictReason)
	} else {
		updated.ConflictReason = priorObservedDepartureConflict(assignment.ConflictReason)
	}
	updated.Acknowledged = false
	updated.AcknowledgedAt = nil
	updated.AcknowledgedBy = nil
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected != 1 {
		return errAllocationVersionConflict
	}
	updated.Version++
	return s.allocations.PublishAssignment(ctx, updated)
}

func validDeparturePosition(latitude, longitude float64) bool {
	return latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180 &&
		(latitude != 0 || longitude != 0)
}

func (s *DepartureLifecycleService) useObservedStandForRoute(ctx context.Context, session int32, callsign, stand string) {
	stand = standName(stand)
	if stand == "" {
		return
	}
	updated, err := s.strips.UpdateStand(ctx, session, callsign, &stand, nil)
	if err != nil {
		slog.WarnContext(ctx, "failed to persist observed stand for departure route",
			slog.String("callsign", callsign), slog.String("stand", stand), slog.Any("error", err))
		return
	}
	if updated != 0 && s.standPublisher != nil {
		s.standPublisher.SendStandEvent(session, callsign, stand)
	}
	if s.routeRecalc == nil {
		return
	}
	if err := s.routeRecalc.UpdateRouteForStripContext(ctx, callsign, session, true); err != nil {
		slog.WarnContext(ctx, "failed to recalculate departure route from observed stand",
			slog.String("callsign", callsign), slog.String("stand", stand), slog.Any("error", err))
	}
}

func isWrongStandConflictReason(reason string) bool {
	return strings.HasPrefix(reason, wrongStandPendingPrefix) || strings.HasPrefix(reason, wrongStandAwaitingPrefix)
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
	updated.ExpiresAt = nil
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("activate wrong stand warning version conflict for %s", assignment.Callsign)
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
	applyVatsimIdentity(&updated, strip, flight.CID, flight.Revision)
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
	projectedRelease := s.computeBlockExpiry(strip)
	expiry := projectedRelease
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
	observedStandRecorded := existing.ObservedStand != nil && strings.EqualFold(strings.TrimSpace(*existing.ObservedStand), strings.TrimSpace(existing.Stand))
	if existing.Stage == StageDepartureBlock {
		if expiry == nil && existing.ExpiresAt != nil && !onAssignedStand {
			return nil
		}
		if sameExpiry(existing.ExpiresAt, expiry) && sameExpiry(existing.ProjectedReleaseAt, projectedRelease) && (!onAssignedStand || observedStandRecorded) {
			return nil
		}
	}
	updated := *existing
	updated.Stage = StageDepartureBlock
	updated.ExpiresAt = expiry
	updated.ProjectedReleaseAt = projectedRelease
	if onAssignedStand {
		observedStand := existing.Stand
		updated.ObservedStand = &observedStand
	}
	if updated.ConflictReason != nil && strings.HasPrefix(*updated.ConflictReason, wrongStandPendingPrefix) {
		updated.ConflictReason = nil
	}
	updated.AssignedAt = &now
	applyVatsimIdentity(&updated, strip, flight.CID, flight.Revision)
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
	reloaded.ProjectedReleaseAt = projectedRelease
	if onAssignedStand {
		observedStand := reloaded.Stand
		reloaded.ObservedStand = &observedStand
	}
	reloaded.AssignedAt = &now
	applyVatsimIdentity(reloaded, strip, flight.CID, flight.Revision)
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
// engine facts. A departure that has been observed on its assigned stand keeps
// that stand: it is already physically there, so later fact corrections must
// not trigger an automatic relocation. Reservations without a live observed
// position can still be reallocated when their facts change.
func (s *DepartureLifecycleService) revalidateFacts(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo) error {
	existing, err := s.assignments.GetAssignment(ctx, session, strip.Callsign)
	if err != nil && !isNotFound(err) {
		return err
	}
	if existing == nil {
		return nil
	}
	if s.flightIsAtAssignedStand(strip, flight, existing.Stand) {
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
		DepartureTOBT:   departureTobtTime(strip, s.now()),
		DepartureTSAT:   departureTsatTime(strip, s.now()),
		DepartureReady:  departureExpectedToVacate(strip),
		VatsimCID:       existing.VatsimCID,
		VatsimRevision:  existing.VatsimRevision,
	}
	_, err = s.allocations.Reallocate(ctx, request)
	return err
}

func (s *DepartureLifecycleService) flightIsAtAssignedStand(strip *models.Strip, flight vatsim.DepartureFlightInfo, stand string) bool {
	airport := strings.TrimSpace(flight.Origin)
	if strip != nil && strings.TrimSpace(strip.Origin) != "" {
		airport = strings.TrimSpace(strip.Origin)
	}
	observed, found := s.stands.StandAtPosition(airport, flight.Latitude, flight.Longitude)
	return found && strings.EqualFold(observed.Name, stand)
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
			if err := retrySerializableOperation(func() error {
				return s.releaseIfDue(ctx, session.ID, assignment, now)
			}); err != nil {
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
	if assignment.ConflictReason != nil &&
		(strings.HasPrefix(*assignment.ConflictReason, wrongStandPendingPrefix) ||
			strings.HasPrefix(*assignment.ConflictReason, wrongStandAwaitingPrefix)) {
		// Clear deadlines written by older versions too. A restart must retain
		// the original assignment rather than adopting the observed stand.
		return s.clearDepartureBlockExpiry(ctx, assignment)
	}
	if assignment.ExpiresAt == nil || assignment.ExpiresAt.After(now) {
		return nil
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
		DepartureTOBT:   departureTobtTime(strip, s.now()),
		DepartureTSAT:   departureTsatTime(strip, s.now()),
		DepartureReady:  departureExpectedToVacate(strip),
	}
	request.VatsimCID, request.VatsimRevision = resolvedVatsimIdentity(strip, flight.CID, flight.Revision)
	return request
}

func departureExpectedToVacate(strip *models.Strip) bool {
	if strip == nil {
		return false
	}
	state := ""
	if strip.State != nil {
		state = strings.TrimSpace(*strip.State)
	}
	return strip.StartReq || strings.EqualFold(strings.TrimSpace(strip.Bay), "PUSH") || strings.EqualFold(state, "PUSH")
}

// resolvedVatsimIdentity prefers the current feed record, then falls back to
// VATSIM identity already reconciled onto the EuroScope strip. This keeps
// controller- and EuroScope-triggered assignments linked to the pilot when the
// allocation does not originate directly from a VATSIM callback.
func resolvedVatsimIdentity(strip *models.Strip, cid string, revision int64) (*int64, *int64) {
	if parsed := parseCID(cid); parsed != nil {
		return parsed, &revision
	}
	if strip == nil {
		return nil, nil
	}
	parsed := parseCID(valueString(strip.VatsimCID))
	if parsed == nil {
		return nil, nil
	}
	return parsed, strip.VatsimRevision
}

func applyVatsimIdentity(assignment *models.StandAssignment, strip *models.Strip, cid string, revision int64) {
	if assignment == nil {
		return
	}
	resolvedCID, resolvedRevision := resolvedVatsimIdentity(strip, cid, revision)
	if resolvedCID != nil {
		assignment.VatsimCID = resolvedCID
	}
	if resolvedRevision != nil {
		assignment.VatsimRevision = resolvedRevision
	}
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
	release := departureTobtTime(strip, now)
	if tsat := departureTsatTime(strip, now); tsat != nil && (release == nil || tsat.After(*release)) {
		release = tsat
	}
	if release == nil {
		return nil
	}
	extension := s.blockExtension
	if departureExpectedToVacate(strip) {
		extension = 0
	}
	expiry := release.Add(extension)
	return &expiry
}

func priorObservedDepartureConflict(reason *string) *string {
	if reason == nil || !strings.HasPrefix(*reason, observedDepartureConflictPrefix) {
		return reason
	}
	_, prior, found := strings.Cut(*reason, observedDeparturePriorConflict)
	if !found || strings.TrimSpace(prior) == "" {
		return nil
	}
	prior = strings.TrimSpace(prior)
	return &prior
}

func observedDepartureConflictReason(existing *string) *string {
	reason := observedDepartureConflictPrefix + " projected release overlaps arrival ETA"
	if existing != nil && !strings.HasPrefix(*existing, observedDepartureConflictPrefix) && strings.TrimSpace(*existing) != "" {
		reason += observedDeparturePriorConflict + strings.TrimSpace(*existing)
	}
	return &reason
}

func departureTobtTime(strip *models.Strip, now time.Time) *time.Time {
	if strip == nil {
		return nil
	}
	if calculation := strip.CdmData.EffectiveCalculation(); calculation != nil && calculation.InvalidReason != nil &&
		strings.TrimSpace(*calculation.InvalidReason) == models.CdmInvalidReasonStaleTobt {
		return nil
	}
	tobt, ok := parseDepartureClockUTC(stripClockValue(strip.EffectiveTobt()), now)
	if !ok {
		return nil
	}
	return &tobt
}

func departureTsatTime(strip *models.Strip, now time.Time) *time.Time {
	if strip == nil {
		return nil
	}
	if calculation := strip.CdmData.EffectiveCalculation(); calculation != nil && calculation.InvalidReason != nil &&
		strings.TrimSpace(*calculation.InvalidReason) == models.CdmInvalidReasonStaleTsat {
		return nil
	}
	tsat, ok := parseDepartureClockUTC(stripClockValue(strip.EffectiveTsat()), now)
	if !ok {
		return nil
	}
	return &tsat
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
