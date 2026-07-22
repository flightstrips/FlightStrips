package services

import (
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/standdiagnostics"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StandAllocationCommand is the caller's explicit allocation intent. The
// later lifecycle and handler tasks decide when to issue each command; this
// service owns the transaction that applies it.
type StandAllocationCommand string

const (
	AutomaticStandAllocation   StandAllocationCommand = "AUTOMATIC_ALLOCATION"
	AutomaticStandReallocation StandAllocationCommand = "AUTOMATIC_REALLOCATION"
	CompatibleManualStand      StandAllocationCommand = "MANUAL_ASSIGNMENT"
	IncompatibleManualOverride StandAllocationCommand = "MANUAL_OVERRIDE"
	observedStandAllocation    StandAllocationCommand = "OBSERVED_STAND"
)

var (
	ErrNoAvailableStand              = errors.New("no compatible stand is available")
	ErrNoCompatibleStand             = errors.New("no stand is compatible with the flight")
	ErrIncompatibleManualAssignment  = errors.New("manual stand is not compatible or available")
	ErrAllocationRetriesExhausted    = errors.New("stand allocation retries exhausted")
	ErrUnknownManualOverrideStand    = errors.New("manual override stand is not configured")
	ErrAutomaticAllocationSuppressed = errors.New("automatic stand allocation suppressed after repeated incompatibility")
	errAllocationVersionConflict     = errors.New("stand assignment version conflict")
)

const automaticNoCompatibleFailureThreshold = 3

// StandAllocationRequest contains facts already resolved by the SAT data
// layer. It intentionally excludes lifecycle and controller authorization
// policy, which belong to later tasks.
type StandAllocationRequest struct {
	SessionID       int32
	Callsign        string
	Airport         string
	Direction       sat.AssignmentDirection
	Stage           string
	FlightFacts     sat.FlightCompatibilityFacts
	AssignmentFacts sat.AssignmentFlightFacts
	ETA             *time.Time
	ETASource       *string
	ExpiresAt       *time.Time
	// DepartureTOBT is used to decide whether a future arrival booking prevents
	// a departure from taking its stand.
	DepartureTOBT  *time.Time
	VatsimCID      *int64
	VatsimRevision *int64

	Stand          string
	ConflictReason string

	DisplaceStage string
	// DisplaceArrivalStages extends DisplaceStage for callers that may displace
	// more than one non-final arrival stage. It is deliberately arrival-only:
	// an observed departure must never displace another departure or a confirmed
	// inbound stand booking.
	DisplaceArrivalStages []string
}

// StandAllocationResult is complete only after the transaction commits. The
// eventual event/validation layer can use its decision data without rerunning
// compatibility or selection.
type StandAllocationResult struct {
	Command             StandAllocationCommand
	Assignment          models.StandAssignment
	Removed             bool
	RemovedAssignments  []models.StandAssignment
	Selection           *sat.StandSelection
	MatchedVariant      *sat.StandCompatibilityMatch
	Compatibility       sat.StandCompatibilityEvaluation
	ConflictReason      string
	Attempts            int
	AvailableCandidates []string
}

type StandAllocationPublisher func(context.Context, StandAllocationResult) error

// StandAllocationService is the sole owner of the allocation transaction. It
// builds on the SAT persistence repositories rather than bypassing their
// versioning and transaction-bound access.
type StandAllocationService struct {
	pool        *pgxpool.Pool
	strips      repository.StripRepository
	assignments repository.StandAssignmentRepository
	stands      *sat.StandCapabilityRegistry
	policy      *sat.AirlineAssignmentConfig
	random      func() float64
	publish     StandAllocationPublisher
	attempts    int
	now         func() time.Time
	failures    *standdiagnostics.AllocationFailureLog

	automaticFailureMu            sync.Mutex
	automaticNoCompatibleFailures map[string]automaticNoCompatibleFailure
}

type automaticNoCompatibleFailure struct {
	fingerprint string
	failures    int
}

type StandAllocationOption func(*StandAllocationService)

func WithStandAllocationRandom(random func() float64) StandAllocationOption {
	return func(service *StandAllocationService) { service.random = random }
}

func WithStandAllocationPublisher(publisher StandAllocationPublisher) StandAllocationOption {
	return func(service *StandAllocationService) { service.publish = publisher }
}

func WithStandAllocationFailureLog(failures *standdiagnostics.AllocationFailureLog) StandAllocationOption {
	return func(service *StandAllocationService) { service.failures = failures }
}

func (s *StandAllocationService) SetPublisher(publisher StandAllocationPublisher) {
	s.publish = publisher
}

func (s *StandAllocationService) PublishAssignment(ctx context.Context, assignment models.StandAssignment) error {
	s.publishCommitted(ctx, StandAllocationResult{Assignment: assignment})
	return nil
}

func (s *StandAllocationService) publishCommitted(ctx context.Context, result StandAllocationResult) {
	if s.publish == nil {
		return
	}
	if err := s.publish(ctx, result); err != nil {
		slog.ErrorContext(ctx, "Failed to publish committed stand allocation",
			slog.Int("session", int(result.Assignment.SessionID)),
			slog.String("callsign", result.Assignment.Callsign),
			slog.Any("error", err))
	}
}

// ReleaseAssignment clears the operational strip stand and removes the SAT
// assignment in one transaction. Publishing happens only after commit so
// connected clients never observe a removal that was rolled back.
func (s *StandAllocationService) ReleaseAssignment(ctx context.Context, assignment *models.StandAssignment) error {
	if assignment == nil || assignment.SessionID <= 0 || strings.TrimSpace(assignment.Callsign) == "" {
		return errors.New("stand assignment release requires an assignment")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT id FROM sessions WHERE id = $1 FOR UPDATE", assignment.SessionID); err != nil {
		return err
	}

	txStrips := s.strips.WithTx(tx)
	txAssignments := s.assignments.WithTx(tx)
	strip, err := txStrips.LockByCallsign(ctx, assignment.SessionID, assignment.Callsign)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	currentAssignments, err := txAssignments.LockAssignments(ctx, assignment.SessionID, assignment.Callsign)
	if err != nil {
		return err
	}
	var current *models.StandAssignment
	for _, candidate := range currentAssignments {
		if candidate != nil && strings.EqualFold(candidate.Callsign, assignment.Callsign) {
			current = candidate
			break
		}
	}
	if current == nil {
		return nil
	}
	if current.ID != assignment.ID || current.Version != assignment.Version {
		return errAllocationVersionConflict
	}

	if strip != nil && strip.Stand != nil && strings.EqualFold(strings.TrimSpace(*strip.Stand), strings.TrimSpace(current.Stand)) {
		updated, err := txStrips.UpdateStand(ctx, assignment.SessionID, assignment.Callsign, nil, nil)
		if err != nil {
			return err
		}
		if updated != 1 {
			return errAllocationVersionConflict
		}
	}
	deleted, err := txAssignments.DeleteAssignment(ctx, assignment.SessionID, current.ID, current.Version)
	if err != nil {
		return err
	}
	if deleted != 1 {
		return errAllocationVersionConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.publishCommitted(ctx, StandAllocationResult{Assignment: *current, Removed: true})
	return nil
}

// WithStandAllocationAttempts bounds retries for serialization, uniqueness,
// and optimistic-version conflicts. The default is three attempts.
func WithStandAllocationAttempts(attempts int) StandAllocationOption {
	return func(service *StandAllocationService) {
		if attempts > 0 {
			service.attempts = attempts
		}
	}
}

// WithStandAllocationClock injects the clock used for assignment timestamps.
// It lets lifecycle and tests drive expiry from a deterministic time source.
func WithStandAllocationClock(now func() time.Time) StandAllocationOption {
	return func(service *StandAllocationService) {
		if now != nil {
			service.now = now
		}
	}
}

func NewStandAllocationService(pool *pgxpool.Pool, strips repository.StripRepository, assignments repository.StandAssignmentRepository, stands *sat.StandCapabilityRegistry, policy *sat.AirlineAssignmentConfig, options ...StandAllocationOption) (*StandAllocationService, error) {
	if pool == nil || strips == nil || assignments == nil || stands == nil || policy == nil {
		return nil, errors.New("stand allocation requires database, repositories, capabilities, and policy")
	}
	service := &StandAllocationService{
		pool: pool, strips: strips, assignments: assignments, stands: stands,
		policy: policy, random: rand.Float64, attempts: 3, now: time.Now,
		failures: standdiagnostics.NewAllocationFailureLog(100),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	if service.random == nil {
		return nil, errors.New("stand allocation random source is nil")
	}
	return service, nil
}

func (s *StandAllocationService) Allocate(ctx context.Context, request StandAllocationRequest) (*StandAllocationResult, error) {
	return s.allocate(ctx, AutomaticStandAllocation, request)
}

func (s *StandAllocationService) Reallocate(ctx context.Context, request StandAllocationRequest) (*StandAllocationResult, error) {
	return s.allocate(ctx, AutomaticStandReallocation, request)
}

func (s *StandAllocationService) AssignManually(ctx context.Context, request StandAllocationRequest) (*StandAllocationResult, error) {
	return s.allocate(ctx, CompatibleManualStand, request)
}

// assignObservedStand adopts the stand where EuroScope saw an aircraft when it
// is operationally free. The aircraft is already physically present, so stand
// capability preferences must not trigger a relocation by themselves. An
// unavailable result is expected during wrong-stand recovery, so it must not
// be retained as a controller-facing allocation failure.
func (s *StandAllocationService) assignObservedStand(ctx context.Context, request StandAllocationRequest) (*StandAllocationResult, error) {
	// A departure observed on a stand is physically there. Its presence takes
	// precedence over a provisional inbound booking, but never over the final
	// CONFIRMED arrival stage.
	request.DisplaceArrivalStages = []string{StageEstimated, StageAssigned}
	return s.allocateWithFailureLogging(ctx, observedStandAllocation, request, false)
}

func (s *StandAllocationService) OverrideManually(ctx context.Context, request StandAllocationRequest) (*StandAllocationResult, error) {
	return s.allocate(ctx, IncompatibleManualOverride, request)
}

// CreateManualBlock applies the same session lock and occupancy graph used by
// stand allocation before persisting a controller-created block.
func (s *StandAllocationService) CreateManualBlock(ctx context.Context, airport string, block *models.StandBlock) error {
	if block == nil || block.SessionID <= 0 {
		return errors.New("manual stand block requires a session")
	}
	block.Stand = standName(block.Stand)
	physical, known := s.stands.Lookup(airport, block.Stand)
	if !known {
		return fmt.Errorf("%w: %s", ErrUnknownManualOverrideStand, block.Stand)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT id FROM sessions WHERE id = $1 FOR UPDATE", block.SessionID); err != nil {
		return err
	}
	store := s.assignments.WithTx(tx)
	assignments, err := store.LockAssignments(ctx, block.SessionID, "")
	if err != nil {
		return err
	}
	blocks, err := store.LockActiveManualBlocks(ctx, block.SessionID)
	if err != nil {
		return err
	}
	for _, existing := range assignments {
		if existing == nil || expired(existing.ExpiresAt, s.now()) {
			continue
		}
		if standName(existing.Stand) == block.Stand || blocksEachOther(physical.Blocks, s.assignedBlocks(airport, existing), block.Stand, existing.Stand) {
			return fmt.Errorf("%w: %s is reserved or adjacency-blocked by %s", ErrIncompatibleManualAssignment, block.Stand, existing.Callsign)
		}
	}
	for _, existing := range blocks {
		if existing != nil && standName(existing.Stand) == block.Stand {
			return fmt.Errorf("%w: %s is already blocked", ErrIncompatibleManualAssignment, block.Stand)
		}
	}
	if err := store.CreateBlock(ctx, block); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *StandAllocationService) DeleteManualBlock(ctx context.Context, session int32, id int64, version int32) (int64, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT id FROM sessions WHERE id = $1 FOR UPDATE", session); err != nil {
		return 0, err
	}
	count, err := s.assignments.WithTx(tx).DeleteBlock(ctx, session, id, version)
	if err != nil || count != 1 {
		return count, err
	}
	return count, tx.Commit(ctx)
}

func (s *StandAllocationService) allocate(ctx context.Context, command StandAllocationCommand, request StandAllocationRequest) (*StandAllocationResult, error) {
	return s.allocateWithFailureLogging(ctx, command, request, true)
}

func (s *StandAllocationService) allocateWithFailureLogging(ctx context.Context, command StandAllocationCommand, request StandAllocationRequest, recordFailures bool) (*StandAllocationResult, error) {
	if err := validateStandAllocationRequest(command, &request); err != nil {
		if recordFailures {
			s.recordAllocationFailure(command, request, "invalid_request", err, 0)
		}
		return nil, err
	}
	if isAutomaticStandAllocation(command) && s.automaticAllocationSuppressed(request) {
		return nil, ErrAutomaticAllocationSuppressed
	}
	tried := map[string]struct{}{}
	for attempt := 1; attempt <= s.attempts; attempt++ {
		result, selected, err := s.allocateOnce(ctx, command, request, tried, attempt)
		if err == nil {
			s.clearAutomaticNoCompatibleFailure(request)
			tier := 0
			rule := ""
			category := "manual"
			if result.Selection != nil {
				tier, rule = result.Selection.Tier, result.Selection.RuleID
				category = "airline_rule"
				if result.Selection.FallbackUsed {
					category = "fallback"
				}
			}
			metrics.RecordSATAssignment(ctx, result.Assignment.Stage, result.Assignment.Source, category, tier)
			metrics.RecordSATOutcome(ctx, "assigned", string(request.Direction))
			if command == IncompatibleManualOverride {
				metrics.RecordSATOutcome(ctx, "override", string(request.Direction))
			}
			if result.ConflictReason != "" {
				metrics.RecordSATConflict(ctx, "operational")
			}
			slog.InfoContext(ctx, "SAT stand allocation committed",
				slog.String("callsign", request.Callsign), slog.Int("session", int(request.SessionID)),
				slog.String("command", string(command)), slog.String("stand", result.Assignment.Stand),
				slog.String("stage", result.Assignment.Stage), slog.String("source", result.Assignment.Source),
				slog.String("rule_id", rule), slog.Int("tier", tier), slog.Int("attempt", attempt))
			s.publishCommitted(ctx, *result)
			return result, nil
		}
		if selected != "" {
			tried[selected] = struct{}{}
		}
		if !retryableStandAllocationError(err) {
			if isAutomaticStandAllocation(command) {
				if errors.Is(err, ErrNoCompatibleStand) {
					s.noteAutomaticNoCompatibleFailure(request)
				} else {
					s.clearAutomaticNoCompatibleFailure(request)
				}
			}
			outcome := standAllocationFailureOutcome(err)
			metrics.RecordSATOutcome(ctx, outcome, string(request.Direction))
			slog.WarnContext(ctx, "SAT stand allocation rejected", slog.String("callsign", request.Callsign), slog.String("command", string(command)), slog.String("outcome", outcome), slog.Any("error", err))
			if recordFailures {
				s.recordAllocationFailure(command, request, outcome, err, attempt)
			}
			return nil, err
		}
		metrics.RecordSATConflict(ctx, "database_contention")
		slog.WarnContext(ctx, "SAT allocation contention; retrying", slog.String("callsign", request.Callsign), slog.Int("attempt", attempt), slog.Any("error", err))
	}
	metrics.RecordSATOutcome(ctx, "database_contention", string(request.Direction))
	err := fmt.Errorf("%w after %d attempts", ErrAllocationRetriesExhausted, s.attempts)
	if recordFailures {
		s.recordAllocationFailure(command, request, "database_contention", err, s.attempts)
	}
	return nil, err
}

func isAutomaticStandAllocation(command StandAllocationCommand) bool {
	return command == AutomaticStandAllocation || command == AutomaticStandReallocation
}

func (s *StandAllocationService) automaticAllocationSuppressed(request StandAllocationRequest) bool {
	key, fingerprint := automaticFailureKeyAndFingerprint(request)
	s.automaticFailureMu.Lock()
	defer s.automaticFailureMu.Unlock()
	failure, ok := s.automaticNoCompatibleFailures[key]
	return ok && failure.fingerprint == fingerprint && failure.failures >= automaticNoCompatibleFailureThreshold
}

func (s *StandAllocationService) noteAutomaticNoCompatibleFailure(request StandAllocationRequest) {
	key, fingerprint := automaticFailureKeyAndFingerprint(request)
	s.automaticFailureMu.Lock()
	defer s.automaticFailureMu.Unlock()
	if s.automaticNoCompatibleFailures == nil {
		s.automaticNoCompatibleFailures = make(map[string]automaticNoCompatibleFailure)
	}
	failure := s.automaticNoCompatibleFailures[key]
	if failure.fingerprint != fingerprint {
		failure = automaticNoCompatibleFailure{fingerprint: fingerprint}
	}
	failure.failures++
	s.automaticNoCompatibleFailures[key] = failure
}

func (s *StandAllocationService) clearAutomaticNoCompatibleFailure(request StandAllocationRequest) {
	key, _ := automaticFailureKeyAndFingerprint(request)
	s.automaticFailureMu.Lock()
	defer s.automaticFailureMu.Unlock()
	delete(s.automaticNoCompatibleFailures, key)
}

func automaticFailureKeyAndFingerprint(request StandAllocationRequest) (string, string) {
	facts := request.FlightFacts
	assignmentFacts := request.AssignmentFacts
	key := fmt.Sprintf("%d:%s", request.SessionID, strings.ToUpper(strings.TrimSpace(request.Callsign)))
	fingerprint := fmt.Sprintf("%s|%s|%s|%s|%t|%s|%.6f|%.6f|%.6f|%.3f|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		strings.ToUpper(strings.TrimSpace(request.Airport)), request.Direction,
		facts.Origin, facts.Destination, facts.AircraftKnown, facts.Aircraft.Type,
		facts.Aircraft.WingspanMetres, facts.Aircraft.LengthMetres, facts.Aircraft.HeightMetres,
		facts.Aircraft.MTOWKilograms, facts.Aircraft.UseCode, facts.EngineType, facts.WTC,
		facts.BorderEndpoint, facts.BorderStatus, assignmentFacts.AircraftType,
		assignmentFacts.AircraftUse, assignmentFacts.BorderStatus, assignmentFacts.Direction,
		assignmentFacts.Special)
	return key, fingerprint
}

func suppressAutomaticAllocationError(err error) error {
	if errors.Is(err, ErrAutomaticAllocationSuppressed) {
		return nil
	}
	return err
}

func standAllocationFailureOutcome(err error) string {
	switch {
	case errors.Is(err, ErrNoCompatibleStand):
		return "no_compatible_stand"
	case errors.Is(err, ErrNoAvailableStand):
		return "no_available_stand"
	case errors.Is(err, ErrIncompatibleManualAssignment):
		return "manual_stand_unavailable"
	case errors.Is(err, ErrUnknownManualOverrideStand):
		return "unknown_stand"
	default:
		return "error"
	}
}

func (s *StandAllocationService) recordAllocationFailure(command StandAllocationCommand, request StandAllocationRequest, outcome string, err error, attempts int) {
	if s.failures == nil {
		return
	}
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	s.failures.Record(standdiagnostics.AllocationFailure{
		OccurredAt:     now().UTC(),
		SessionID:      request.SessionID,
		Airport:        request.Airport,
		Callsign:       request.Callsign,
		Command:        string(command),
		Outcome:        outcome,
		Reason:         reason,
		Direction:      string(request.Direction),
		Stage:          request.Stage,
		AttemptedStand: request.Stand,
		AircraftType:   request.AssignmentFacts.AircraftType,
		EngineType:     string(request.FlightFacts.EngineType),
		WTC:            request.FlightFacts.WTC,
		BorderStatus:   string(request.FlightFacts.BorderStatus),
		Attempts:       attempts,
	})
}

func validateStandAllocationRequest(command StandAllocationCommand, request *StandAllocationRequest) error {
	request.Callsign = strings.ToUpper(strings.TrimSpace(request.Callsign))
	request.Airport = strings.ToUpper(strings.TrimSpace(request.Airport))
	request.Stand = standName(request.Stand)
	if request.SessionID <= 0 || request.Callsign == "" || request.Airport == "" {
		return errors.New("stand allocation requires session, callsign, and airport")
	}
	if request.Direction != sat.AssignmentDirectionArrival && request.Direction != sat.AssignmentDirectionDeparture {
		return fmt.Errorf("invalid stand allocation direction %q", request.Direction)
	}
	if request.Stage == "" {
		request.Stage = "ASSIGNED"
	}
	if command == CompatibleManualStand || command == IncompatibleManualOverride || command == observedStandAllocation {
		if request.Stand == "" {
			return errors.New("manual stand allocation requires a stand")
		}
	}
	if command == IncompatibleManualOverride && strings.TrimSpace(request.ConflictReason) == "" {
		return errors.New("manual override requires a conflict reason")
	}
	request.AssignmentFacts.Callsign = request.Callsign
	request.AssignmentFacts.Direction = request.Direction
	return nil
}

func (s *StandAllocationService) allocateOnce(ctx context.Context, command StandAllocationCommand, request StandAllocationRequest, tried map[string]struct{}, attempt int) (*StandAllocationResult, string, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT id FROM sessions WHERE id = $1 FOR UPDATE", request.SessionID); err != nil {
		return nil, "", err
	}

	txStrips := s.strips.WithTx(tx)
	txAssignments := s.assignments.WithTx(tx)
	strip, err := txStrips.LockByCallsign(ctx, request.SessionID, request.Callsign)
	if err != nil {
		return nil, "", fmt.Errorf("load allocation strip: %w", err)
	}
	assignments, err := txAssignments.LockAssignments(ctx, request.SessionID, request.Callsign)
	if err != nil {
		return nil, "", err
	}
	blocks, err := txAssignments.LockActiveManualBlocks(ctx, request.SessionID)
	if err != nil {
		return nil, "", err
	}

	evaluation := s.stands.EvaluateCompatibility(request.Airport, request.FlightFacts)
	if command == CompatibleManualStand || command == IncompatibleManualOverride {
		evaluation = s.stands.EvaluateManualCompatibility(request.Airport, request.FlightFacts)
	}
	selected, selection, match, available, conflict, err := s.selectStand(command, request, evaluation, assignments, blocks, tried)
	if err != nil {
		return nil, selected, err
	}
	var removedAssignments []models.StandAssignment
	if request.displacesArrivalStage() && selected != "" {
		removedAssignments, err = s.displaceAssignments(ctx, txStrips, txAssignments, request, selected, assignments)
		if err != nil {
			return nil, selected, err
		}
	}
	request.Stand = selected
	assignment, err := s.persistStandAllocation(ctx, txAssignments, command, request, assignments, selection, match, conflict)
	if err != nil {
		return nil, selected, err
	}
	if strip.Stand == nil || *strip.Stand != selected {
		updated, err := txStrips.UpdateStand(ctx, request.SessionID, request.Callsign, &selected, nil)
		if err != nil {
			return nil, selected, err
		}
		if updated != 1 {
			return nil, selected, errAllocationVersionConflict
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, selected, err
	}
	return &StandAllocationResult{
		Command: command, Assignment: *assignment, Selection: selection, MatchedVariant: match,
		Compatibility: evaluation, ConflictReason: conflict, Attempts: attempt, AvailableCandidates: available,
		RemovedAssignments: removedAssignments,
	}, selected, nil
}

// StandAvailable reports whether the named stand is currently compatible with
// the request's flight facts and free of occupancy or manual blocks. It runs
// the same locking read as an allocation so the lifecycle can decide whether to
// renew an existing reservation in place or reallocate. The transaction is
// read-only and rolls back without persisting any state.
func (s *StandAllocationService) StandAvailable(ctx context.Context, request StandAllocationRequest, stand string) (bool, error) {
	if err := validateStandAllocationRequest(AutomaticStandAllocation, &request); err != nil {
		return false, err
	}
	target := standName(stand)
	if target == "" {
		return false, errors.New("stand availability requires a stand")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT id FROM sessions WHERE id = $1 FOR UPDATE", request.SessionID); err != nil {
		return false, err
	}
	txAssignments := s.assignments.WithTx(tx)
	assignments, err := txAssignments.LockAssignments(ctx, request.SessionID, request.Callsign)
	if err != nil {
		return false, err
	}
	blocks, err := txAssignments.LockActiveManualBlocks(ctx, request.SessionID)
	if err != nil {
		return false, err
	}
	evaluation := s.stands.EvaluateCompatibility(request.Airport, request.FlightFacts)
	matches := make(map[string]sat.StandCompatibilityMatch, len(evaluation.Matches))
	for _, match := range evaluation.Matches {
		matches[standName(match.Stand.Name)] = match
	}
	if _, compatible := matches[target]; !compatible {
		return false, nil
	}
	availability := s.availability(request, assignments, blocks, matches)
	return len(availability[target]) == 0, nil
}

func (s *StandAllocationService) selectStand(command StandAllocationCommand, request StandAllocationRequest, evaluation sat.StandCompatibilityEvaluation, assignments []*models.StandAssignment, blocks []*models.StandBlock, tried map[string]struct{}) (string, *sat.StandSelection, *sat.StandCompatibilityMatch, []string, string, error) {
	matches := make(map[string]sat.StandCompatibilityMatch, len(evaluation.Matches))
	for _, match := range evaluation.Matches {
		matches[standName(match.Stand.Name)] = match
	}
	if command == observedStandAllocation {
		target := standName(request.Stand)
		match, compatible := matches[target]
		if !compatible {
			stand, known := s.stands.Lookup(request.Airport, target)
			if !known {
				return target, nil, nil, nil, "", fmt.Errorf("%w: %s", ErrUnknownManualOverrideStand, target)
			}
			if len(stand.Variants) == 0 {
				return target, nil, nil, nil, "", fmt.Errorf("%w: %s", ErrIncompatibleManualAssignment, target)
			}
			match = sat.StandCompatibilityMatch{
				Stand:  stand,
				Blocks: slices.Clone(stand.Blocks),
			}
			matches[target] = match
		}
		availability := s.availability(request, assignments, blocks, matches)
		if len(availability[target]) > 0 {
			return target, nil, nil, nil, "", fmt.Errorf("%w: %s", ErrIncompatibleManualAssignment, target)
		}
		return target, nil, &match, []string{target}, "", nil
	}
	if len(matches) == 0 && command != IncompatibleManualOverride {
		return "", nil, nil, nil, "", ErrNoCompatibleStand
	}
	availability := s.availability(request, assignments, blocks, matches)
	if command == IncompatibleManualOverride {
		stand, known := s.stands.Lookup(request.Airport, request.Stand)
		if !known {
			return "", nil, nil, nil, "", fmt.Errorf("%w: %s", ErrUnknownManualOverrideStand, request.Stand)
		}
		match, compatible := matches[request.Stand]
		reasons := append([]string{request.ConflictReason}, availability[request.Stand]...)
		if !compatible {
			reasons = append(reasons, compatibilityReason(request.Stand, evaluation.Rejections))
			if len(stand.Variants) > 0 {
				match = sat.StandCompatibilityMatch{Stand: stand, Variant: stand.Variants[0], Blocks: slices.Clone(stand.Variants[0].Blocks)}
			}
		}
		return request.Stand, nil, &match, nil, joinAllocationReasons(reasons), nil
	}
	if command == CompatibleManualStand {
		match, compatible := matches[request.Stand]
		if !compatible || len(availability[request.Stand]) > 0 {
			return request.Stand, nil, nil, nil, "", fmt.Errorf("%w: %s", ErrIncompatibleManualAssignment, request.Stand)
		}
		return request.Stand, nil, &match, []string{request.Stand}, "", nil
	}

	available := make([]string, 0, len(matches))
	for stand := range matches {
		if len(availability[stand]) == 0 {
			if _, retrying := tried[stand]; !retrying {
				available = append(available, stand)
			}
		}
	}
	slices.Sort(available)
	selection, err := s.policy.SelectStand(request.AssignmentFacts, available, s.random)
	if err != nil {
		return "", nil, nil, available, "", err
	}
	if selection == nil {
		return "", nil, nil, available, "", ErrNoAvailableStand
	}
	match := matches[selection.Stand]
	return selection.Stand, selection, &match, available, "", nil
}

func (s *StandAllocationService) availability(request StandAllocationRequest, assignments []*models.StandAssignment, blocks []*models.StandBlock, matches map[string]sat.StandCompatibilityMatch) map[string][]string {
	now := s.now()
	result := map[string][]string{}
	for candidate, match := range matches {
		for _, assignment := range assignments {
			if assignment == nil || strings.EqualFold(assignment.Callsign, request.Callsign) || expired(assignment.ExpiresAt, now) {
				continue
			}
			futureArrivalBlocks := false
			// Arrival assignments are planning information until their ETA. A
			// flight that has not landed must not physically occupy its planned
			// stand or prevent a departure from using it.
			if assignment.Direction == string(sat.AssignmentDirectionArrival) && assignment.ETA != nil && assignment.ETA.After(now) {
				futureArrivalBlocks = futureArrivalBlocksRequest(*assignment.ETA, request)
				if !futureArrivalBlocks {
					continue
				}
			}
			if candidate == standName(assignment.Stand) {
				if request.displacesAssignment(assignment) && (!futureArrivalBlocks || sameArrivalETA(request.ETA, assignment.ETA)) {
					continue
				}
				result[candidate] = append(result[candidate], "reserved by "+assignment.Callsign)
				continue
			}
			if blocksEachOther(match.Blocks, s.assignedBlocks(request.Airport, assignment), candidate, assignment.Stand) {
				result[candidate] = append(result[candidate], "blocked by allocated neighbor "+assignment.Stand)
			}
		}
		for _, block := range blocks {
			if block == nil {
				continue
			}
			blockedStand := standName(block.Stand)
			directlyBlocked := candidate == blockedStand
			adjacencyBlocked := blocksEachOther(s.configuredStandBlocks(request.Airport, candidate), s.configuredStandBlocks(request.Airport, blockedStand), candidate, blockedStand)
			if !directlyBlocked && !adjacencyBlocked {
				continue
			}
			reason := "manually blocked"
			if adjacencyBlocked && !directlyBlocked {
				reason = "blocked by manual block " + blockedStand
			}
			if block.Reason != nil && strings.TrimSpace(*block.Reason) != "" {
				reason += ": " + strings.TrimSpace(*block.Reason)
			}
			result[candidate] = append(result[candidate], reason)
		}
	}
	return result
}

func sameArrivalETA(left, right *time.Time) bool {
	return left != nil && right != nil && left.Equal(*right)
}

// futureArrivalBlocksRequest treats a future arrival as a reservation from its
// ETA. A departure may use the stand only when its TOBT plus the stand-release
// buffer is before that ETA; a later-arriving inbound cannot take a stand that
// is already reserved first.
func futureArrivalBlocksRequest(arrivalETA time.Time, request StandAllocationRequest) bool {
	switch request.Direction {
	case sat.AssignmentDirectionDeparture:
		return request.DepartureTOBT != nil && request.DepartureTOBT.Add(defaultDepartureBlockExtension).After(arrivalETA)
	case sat.AssignmentDirectionArrival:
		return request.ETA == nil || !request.ETA.Before(arrivalETA)
	default:
		return false
	}
}

func (s *StandAllocationService) configuredStandBlocks(airport, standName string) []string {
	stand, found := s.stands.Lookup(airport, standName)
	if !found {
		return nil
	}
	return stand.Blocks
}

func (s *StandAllocationService) assignedBlocks(airport string, assignment *models.StandAssignment) []string {
	stand, found := s.stands.Lookup(airport, assignment.Stand)
	if !found {
		return nil
	}
	if assignment.MatchedVariant != nil {
		for _, variant := range stand.Variants {
			if allocationVariantKey(airport, stand.Name, variant.Line) == *assignment.MatchedVariant {
				return slices.Clone(variant.Blocks)
			}
		}
	}
	return slices.Clone(stand.Blocks)
}

func (s *StandAllocationService) persistStandAllocation(ctx context.Context, store repository.StandAssignmentRepository, command StandAllocationCommand, request StandAllocationRequest, current []*models.StandAssignment, selection *sat.StandSelection, match *sat.StandCompatibilityMatch, conflict string) (*models.StandAssignment, error) {
	var existing *models.StandAssignment
	for _, assignment := range current {
		if assignment != nil && strings.EqualFold(assignment.Callsign, request.Callsign) {
			existing = assignment
			break
		}
	}
	next := &models.StandAssignment{SessionID: request.SessionID, Callsign: request.Callsign}
	if existing != nil {
		*next = *existing
	}
	now := s.now().UTC()
	next.Stand, next.Direction, next.Stage = request.Stand, string(request.Direction), request.Stage
	next.Source, next.Manual = allocationSource(command)
	next.RuleID, next.Tier, next.MatchedVariant = allocationSelectionMetadata(request, selection, match)
	next.ConflictReason = nil
	if conflict != "" {
		next.ConflictReason = &conflict
	}
	next.ETA, next.ETASource, next.AssignedAt, next.ExpiresAt = request.ETA, request.ETASource, &now, request.ExpiresAt
	next.Acknowledged, next.AcknowledgedAt, next.AcknowledgedBy = false, nil, nil
	next.VatsimCID, next.VatsimRevision = request.VatsimCID, request.VatsimRevision
	if existing == nil {
		if err := store.CreateAssignment(ctx, next); err != nil {
			return nil, err
		}
		return next, nil
	}
	updated, err := store.UpdateAssignment(ctx, next)
	if err != nil {
		return nil, err
	}
	if updated != 1 {
		return nil, errAllocationVersionConflict
	}
	next.Version++
	return next, nil
}

func allocationSelectionMetadata(request StandAllocationRequest, selection *sat.StandSelection, match *sat.StandCompatibilityMatch) (*string, *int32, *string) {
	var ruleID, variant *string
	var tier *int32
	if selection != nil {
		ruleID = stringPointer(selection.RuleID)
		value := int32(selection.Tier)
		tier = &value
	}
	if match != nil && match.Variant.Line > 0 {
		value := allocationVariantKey(request.Airport, request.Stand, match.Variant.Line)
		variant = &value
	}
	return ruleID, tier, variant
}

func allocationSource(command StandAllocationCommand) (string, bool) {
	switch command {
	case CompatibleManualStand:
		return "MANUAL", true
	case IncompatibleManualOverride:
		return "MANUAL_OVERRIDE", true
	default:
		return "AUTOMATIC", false
	}
}

func expired(at *time.Time, now time.Time) bool { return at != nil && !at.After(now) }

func blocksEachOther(candidateBlocks, assignedBlocks []string, candidate, assigned string) bool {
	return containsStand(candidateBlocks, assigned) || containsStand(assignedBlocks, candidate)
}

func containsStand(blocks []string, wanted string) bool {
	for _, stand := range blocks {
		if standName(stand) == standName(wanted) {
			return true
		}
	}
	return false
}

func compatibilityReason(stand string, rejections []sat.StandCompatibilityRejection) string {
	for _, rejection := range rejections {
		if standName(rejection.Stand) == standName(stand) {
			return fmt.Sprintf("incompatible %s: expected %s, got %s", rejection.Capability, rejection.Expected, rejection.Actual)
		}
	}
	return "no compatible stand variant"
}

func joinAllocationReasons(reasons []string) string {
	seen := map[string]struct{}{}
	var result []string
	for _, reason := range reasons {
		reason = strings.TrimSpace(reason)
		if reason != "" {
			if _, exists := seen[reason]; !exists {
				seen[reason] = struct{}{}
				result = append(result, reason)
			}
		}
	}
	return strings.Join(result, "; ")
}

func (s *StandAllocationService) displaceAssignments(ctx context.Context, strips repository.StripRepository, assignments repository.StandAssignmentRepository, request StandAllocationRequest, selected string, current []*models.StandAssignment) ([]models.StandAssignment, error) {
	if !request.displacesArrivalStage() || selected == "" {
		return nil, nil
	}
	removed := []models.StandAssignment{}
	for _, assignment := range current {
		if assignment == nil || strings.EqualFold(assignment.Callsign, request.Callsign) {
			continue
		}
		if !request.displacesAssignment(assignment) {
			continue
		}
		if standName(assignment.Stand) != standName(selected) {
			continue
		}
		if _, err := strips.UpdateStand(ctx, request.SessionID, assignment.Callsign, nil, nil); err != nil {
			return nil, err
		}
		deleted, err := assignments.DeleteAssignment(ctx, request.SessionID, assignment.ID, assignment.Version)
		if err != nil {
			return nil, err
		}
		if deleted != 1 {
			return nil, errAllocationVersionConflict
		}
		removed = append(removed, *assignment)
	}
	return removed, nil
}

func (request StandAllocationRequest) displacesArrivalStage() bool {
	return request.DisplaceStage != "" || len(request.DisplaceArrivalStages) > 0
}

func (request StandAllocationRequest) displacesAssignment(assignment *models.StandAssignment) bool {
	if assignment == nil || assignment.Direction != string(sat.AssignmentDirectionArrival) {
		return false
	}
	if assignment.Stage == request.DisplaceStage && request.DisplaceStage != "" {
		return true
	}
	return slices.Contains(request.DisplaceArrivalStages, assignment.Stage)
}

func standName(value string) string      { return strings.ToUpper(strings.TrimSpace(value)) }
func stringPointer(value string) *string { return &value }
func allocationVariantKey(airport, stand string, line int) string {
	return fmt.Sprintf("%s:%s:%d", standName(airport), standName(stand), line)
}

func retryableStandAllocationError(err error) bool {
	if errors.Is(err, errAllocationVersionConflict) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01" || pgErr.Code == "23505")
}
