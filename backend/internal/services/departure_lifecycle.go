package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
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
	if allocations == nil || assignments == nil || strips == nil || stands == nil {
		return nil, errors.New("departure lifecycle requires allocation service, repositories, and stand registry")
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
func (s *DepartureLifecycleService) ProcessDeparture(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo) error {
	if strip == nil || strings.TrimSpace(strip.Callsign) == "" {
		return nil
	}
	if flight.Online {
		if err := s.activateBlock(ctx, session, strip, flight); err != nil {
			return err
		}
		return s.revalidateFacts(ctx, session, strip, flight)
	}
	return s.ensureReservation(ctx, session, strip, flight)
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
	revision := flight.Revision
	updated.VatsimRevision = &revision
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
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
	if existing.Stage == StageDepartureBlock {
		if expiry == nil && existing.ExpiresAt != nil {
			return nil
		}
		if sameExpiry(existing.ExpiresAt, expiry) {
			return nil
		}
	}
	updated := *existing
	updated.Stage = StageDepartureBlock
	updated.ExpiresAt = expiry
	updated.AssignedAt = &now
	revision := flight.Revision
	updated.VatsimRevision = &revision
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
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
	reloaded.VatsimRevision = &revision
	_, err = s.assignments.UpdateAssignment(ctx, reloaded)
	return err
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
	if s.sessions == nil {
		return nil
	}
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
		_, err := s.assignments.DeleteAssignment(ctx, session, assignment.ID, assignment.Version)
		return err
	}
	if assignment.ExpiresAt == nil || assignment.ExpiresAt.After(now) {
		return nil
	}
	if _, err := s.strips.UpdateStand(ctx, session, assignment.Callsign, nil, nil); err != nil {
		return err
	}
	_, err = s.assignments.DeleteAssignment(ctx, session, assignment.ID, assignment.Version)
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
	revision := flight.Revision
	return StandAllocationRequest{
		SessionID:       session,
		Callsign:        strip.Callsign,
		Airport:         strings.ToUpper(strings.TrimSpace(strip.Origin)),
		Direction:       sat.AssignmentDirectionDeparture,
		Stage:           stage,
		FlightFacts:     facts,
		AssignmentFacts: assignmentFacts,
		ExpiresAt:       expiresAt,
		VatsimCID:       parseCID(flight.CID),
		VatsimRevision:  &revision,
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
	if value := stripClockValue(strip.EffectiveTsat()); value != "" {
		if t, ok := parseDepartureClockUTC(value, now); ok {
			expiry := t.Add(s.blockExtension)
			return &expiry
		}
	}
	if value := stripClockValue(strip.EffectiveTobt()); value != "" {
		if t, ok := parseDepartureClockUTC(value, now); ok {
			expiry := t.Add(s.blockExtension)
			return &expiry
		}
	}
	return nil
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
