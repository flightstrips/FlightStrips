package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/sat"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"strings"
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
)

var (
	ErrNoAvailableStand             = errors.New("no compatible stand is available")
	ErrIncompatibleManualAssignment = errors.New("manual stand is not compatible or available")
	ErrAllocationRetriesExhausted   = errors.New("stand allocation retries exhausted")
	ErrUnknownManualOverrideStand   = errors.New("manual override stand is not configured")
	errAllocationVersionConflict    = errors.New("stand assignment version conflict")
)

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
	VatsimCID       *int64
	VatsimRevision  *int64

	Stand          string
	ConflictReason string
}

// StandAllocationResult is complete only after the transaction commits. The
// eventual event/validation layer can use its decision data without rerunning
// compatibility or selection.
type StandAllocationResult struct {
	Command             StandAllocationCommand
	Assignment          models.StandAssignment
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
}

type StandAllocationOption func(*StandAllocationService)

func WithStandAllocationRandom(random func() float64) StandAllocationOption {
	return func(service *StandAllocationService) { service.random = random }
}

func WithStandAllocationPublisher(publisher StandAllocationPublisher) StandAllocationOption {
	return func(service *StandAllocationService) { service.publish = publisher }
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

func NewStandAllocationService(pool *pgxpool.Pool, strips repository.StripRepository, assignments repository.StandAssignmentRepository, stands *sat.StandCapabilityRegistry, policy *sat.AirlineAssignmentConfig, options ...StandAllocationOption) (*StandAllocationService, error) {
	if pool == nil || strips == nil || assignments == nil || stands == nil || policy == nil {
		return nil, errors.New("stand allocation requires database, repositories, capabilities, and policy")
	}
	service := &StandAllocationService{
		pool: pool, strips: strips, assignments: assignments, stands: stands,
		policy: policy, random: rand.Float64, attempts: 3,
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

func (s *StandAllocationService) OverrideManually(ctx context.Context, request StandAllocationRequest) (*StandAllocationResult, error) {
	return s.allocate(ctx, IncompatibleManualOverride, request)
}

func (s *StandAllocationService) allocate(ctx context.Context, command StandAllocationCommand, request StandAllocationRequest) (*StandAllocationResult, error) {
	if err := validateStandAllocationRequest(command, &request); err != nil {
		return nil, err
	}
	tried := map[string]struct{}{}
	for attempt := 1; attempt <= s.attempts; attempt++ {
		result, selected, err := s.allocateOnce(ctx, command, request, tried, attempt)
		if err == nil {
			if s.publish != nil {
				if err := s.publish(ctx, *result); err != nil {
					return result, fmt.Errorf("publish committed stand allocation: %w", err)
				}
			}
			return result, nil
		}
		if selected != "" {
			tried[selected] = struct{}{}
		}
		if !retryableStandAllocationError(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w after %d attempts", ErrAllocationRetriesExhausted, s.attempts)
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
	if command == CompatibleManualStand || command == IncompatibleManualOverride {
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
	request.Stand = selected
	assignment, err := persistStandAllocation(ctx, txAssignments, command, request, assignments, selection, match, conflict)
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
	}, selected, nil
}

func (s *StandAllocationService) selectStand(command StandAllocationCommand, request StandAllocationRequest, evaluation sat.StandCompatibilityEvaluation, assignments []*models.StandAssignment, blocks []*models.StandBlock, tried map[string]struct{}) (string, *sat.StandSelection, *sat.StandCompatibilityMatch, []string, string, error) {
	matches := make(map[string]sat.StandCompatibilityMatch, len(evaluation.Matches))
	for _, match := range evaluation.Matches {
		matches[standName(match.Stand.Name)] = match
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
	now := time.Now()
	result := map[string][]string{}
	for candidate, match := range matches {
		for _, assignment := range assignments {
			if assignment == nil || strings.EqualFold(assignment.Callsign, request.Callsign) || expired(assignment.ExpiresAt, now) {
				continue
			}
			if candidate == standName(assignment.Stand) {
				result[candidate] = append(result[candidate], "reserved by "+assignment.Callsign)
				continue
			}
			if blocksEachOther(match.Blocks, s.assignedBlocks(request.Airport, assignment), candidate, assignment.Stand) {
				result[candidate] = append(result[candidate], "blocked by allocated neighbor "+assignment.Stand)
			}
		}
		for _, block := range blocks {
			if block != nil && candidate == standName(block.Stand) {
				reason := "manually blocked"
				if block.Reason != nil && strings.TrimSpace(*block.Reason) != "" {
					reason += ": " + strings.TrimSpace(*block.Reason)
				}
				result[candidate] = append(result[candidate], reason)
			}
		}
	}
	return result
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

func persistStandAllocation(ctx context.Context, store repository.StandAssignmentRepository, command StandAllocationCommand, request StandAllocationRequest, current []*models.StandAssignment, selection *sat.StandSelection, match *sat.StandCompatibilityMatch, conflict string) (*models.StandAssignment, error) {
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
	now := time.Now().UTC()
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
