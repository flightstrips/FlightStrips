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
	ErrNoAvailableStand              = errors.New("no safe stand is currently available")
	ErrNoPolicyStand                 = errors.New("compatible stands are available, but none is in the configured airline or fallback policy pool")
	ErrNoCompatibleStand             = errors.New("no stand is compatible with the flight")
	ErrIncompatibleManualAssignment  = errors.New("manual stand is not compatible or available")
	ErrAllocationRetriesExhausted    = errors.New("stand allocation retries exhausted")
	ErrUnknownManualOverrideStand    = errors.New("manual override stand is not configured")
	ErrAutomaticAllocationSuppressed = errors.New("automatic stand allocation suppressed after an unchanged stand shortage")
	ErrNoTierImprovement             = errors.New("no better stand tier is currently available")
	ErrRelocationCycle               = errors.New("displaced arrival relocation revisited the same callsign")
	errAllocationVersionConflict     = errors.New("stand assignment version conflict")
)

const observedDepartureConflictPrefix = "observed departure conflicts with confirmed arrival:"

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
	// DepartureTOBT is a fallback estimate used only when ExpiresAt does not
	// already describe the departure's effective stand hold.
	DepartureTOBT *time.Time
	DepartureTSAT *time.Time
	// DepartureReady is operational evidence such as START REQ or PUSH. For a
	// ready aircraft, the later of TOBT and TSAT is the expected start of stand
	// release, so START REQ can never bypass a substantially later TSAT.
	DepartureReady bool
	VatsimCID      *int64
	VatsimRevision *int64

	Stand          string
	ObservedStand  *string
	ConflictReason string
	// RequestStandSync makes a caller-originated selection reach EuroScope even
	// when the strip already contains that stand. This is needed for pilot EFB
	// requests, where the persisted value alone cannot prove that EuroScope saw
	// the selection.
	RequestStandSync bool

	DisplaceStage string
	// DisplaceArrivalStages extends DisplaceStage for callers that may displace
	// more than one arrival stage. It is deliberately arrival-only: even a
	// physical takeover must never displace another departure.
	DisplaceArrivalStages []string
	// ImproveTierBelow constrains a stage-boundary reallocation to tiers below
	// the supplied value. Arrival promotion uses 2 so only Primary is accepted,
	// preventing Secondary/Tertiary or equivalent-stand churn.
	ImproveTierBelow int32
}

// StandAllocationResult is complete only after the transaction commits. The
// eventual event/validation layer can use its decision data without rerunning
// compatibility or selection.
type StandAllocationResult struct {
	Command    StandAllocationCommand
	Assignment models.StandAssignment
	Removed    bool
	// StandChanged reports whether this transaction changed the operational
	// strip stand. Lifecycle-only assignment updates must not be mirrored to
	// EuroScope as stand writes.
	StandChanged bool
	// NotifyEuroscope is set when EuroScope needs a stand write even though the
	// strip value did not change, currently when an arrival becomes CONFIRMED.
	NotifyEuroscope     bool
	RemovedAssignments  []models.StandAssignment
	RemovedStandChanges []models.StandAssignment
	Selection           *sat.StandSelection
	MatchedVariant      *sat.StandCompatibilityMatch
	Compatibility       sat.StandCompatibilityEvaluation
	ConflictReason      string
	Attempts            int
	AvailableCandidates []string
}

// StandAllocationPreview is a read-only explanation of the stands an
// automatic allocation could currently choose for one flight.
type StandAllocationPreview struct {
	Callsign         string                    `json:"callsign"`
	Airport          string                    `json:"airport"`
	FallbackUsed     bool                      `json:"fallback_used"`
	CompatibleStands int                       `json:"compatible_stands"`
	AvailableStands  int                       `json:"available_stands"`
	Selection        sat.StandSelectionPreview `json:"selection"`
}

// StandAvailability describes whether a manually selected stand can be used
// for a flight at its current arrival ETA or departure TOBT.
type StandAvailability struct {
	Stand     string `json:"stand"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type StandAllocationPublisher func(context.Context, StandAllocationResult) error
type DisplacedArrivalHandler func(context.Context, models.StandAssignment) error

type relocationChainContextKey struct{}

type relocationChain struct {
	seen map[string]struct{}
}

func beginRelocationChain(ctx context.Context, callsign string) context.Context {
	if _, ok := ctx.Value(relocationChainContextKey{}).(*relocationChain); ok {
		return ctx
	}
	chain := &relocationChain{seen: map[string]struct{}{standName(callsign): {}}}
	return context.WithValue(ctx, relocationChainContextKey{}, chain)
}

func visitRelocationChain(ctx context.Context, callsign string) bool {
	chain, ok := ctx.Value(relocationChainContextKey{}).(*relocationChain)
	if !ok {
		return true
	}
	key := standName(callsign)
	if _, visited := chain.seen[key]; visited {
		return false
	}
	chain.seen[key] = struct{}{}
	return true
}

// StandAllocationService is the sole owner of the allocation transaction. It
// builds on the SAT persistence repositories rather than bypassing their
// versioning and transaction-bound access.
type StandAllocationService struct {
	pool                   *pgxpool.Pool
	strips                 repository.StripRepository
	assignments            repository.StandAssignmentRepository
	stands                 *sat.StandCapabilityRegistry
	policy                 *sat.AirlineAssignmentConfig
	random                 func() float64
	publish                StandAllocationPublisher
	relocate               DisplacedArrivalHandler
	attempts               int
	now                    func() time.Time
	failures               *standdiagnostics.AllocationFailureLog
	departureReleaseBuffer time.Duration

	automaticFailureMu        sync.Mutex
	automaticTerminalFailures map[string]automaticTerminalFailure
}

// automaticTerminalFailure prevents a lifecycle poll from reporting the same
// unassignable flight repeatedly. Changed flight facts or lifecycle stage form
// a new allocation decision and are deliberately allowed through.
type automaticTerminalFailure struct {
	fingerprint string
	outcome     string
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

// WithStandAllocationDepartureReleaseBuffer keeps stand availability checks
// aligned with the configured departure block extension when an observed
// position intentionally has no persisted expiry yet.
func WithStandAllocationDepartureReleaseBuffer(duration time.Duration) StandAllocationOption {
	return func(service *StandAllocationService) {
		if duration > 0 {
			service.departureReleaseBuffer = duration
		}
	}
}

func (s *StandAllocationService) SetPublisher(publisher StandAllocationPublisher) {
	s.publish = publisher
}

func (s *StandAllocationService) SetDisplacedArrivalHandler(handler DisplacedArrivalHandler) {
	s.relocate = handler
}

func (s *StandAllocationService) PublishAssignment(ctx context.Context, assignment models.StandAssignment) error {
	s.publishCommitted(ctx, StandAllocationResult{Assignment: assignment})
	return nil
}

// PublishConfirmedArrival publishes the lifecycle transition that makes an
// arrival's previously allocated stand ready for EuroScope.
func (s *StandAllocationService) PublishConfirmedArrival(ctx context.Context, assignment models.StandAssignment) error {
	s.publishCommitted(ctx, StandAllocationResult{Assignment: assignment, NotifyEuroscope: true})
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

	standChanged := strip != nil && strip.Stand != nil && strings.EqualFold(strings.TrimSpace(*strip.Stand), strings.TrimSpace(current.Stand))
	if standChanged {
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
	s.publishCommitted(ctx, StandAllocationResult{Assignment: *current, Removed: true, StandChanged: standChanged, NotifyEuroscope: standChanged})
	return nil
}

// ReconcileUnsafeAssignments removes automatic planning reservations that
// overlap a higher-priority assignment persisted by an older deployment. It
// never moves controller-owned or physically observed aircraft; conflicts
// between two such assignments remain visible for controller resolution.
func (s *StandAllocationService) ReconcileUnsafeAssignments(ctx context.Context, session int32, airport string) error {
	if session <= 0 {
		return errors.New("stand assignment reconciliation requires a session")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT id FROM sessions WHERE id = $1 FOR UPDATE", session); err != nil {
		return err
	}

	txStrips := s.strips.WithTx(tx)
	txAssignments := s.assignments.WithTx(tx)
	assignments, err := txAssignments.LockAssignments(ctx, session, "")
	if err != nil {
		return err
	}
	losers := s.unsafeAssignmentLosers(airport, assignments, s.now())
	type removal struct {
		assignment   models.StandAssignment
		standChanged bool
	}
	removals := make([]removal, 0, len(losers))
	for _, assignment := range losers {
		strip, err := txStrips.LockByCallsign(ctx, session, assignment.Callsign)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		standChanged := strip != nil && strip.Stand != nil && strings.EqualFold(strings.TrimSpace(*strip.Stand), strings.TrimSpace(assignment.Stand))
		if standChanged {
			updated, err := txStrips.UpdateStand(ctx, session, assignment.Callsign, nil, nil)
			if err != nil {
				return err
			}
			if updated != 1 {
				return errAllocationVersionConflict
			}
		}
		deleted, err := txAssignments.DeleteAssignment(ctx, session, assignment.ID, assignment.Version)
		if err != nil {
			return err
		}
		if deleted != 1 {
			return errAllocationVersionConflict
		}
		removals = append(removals, removal{assignment: *assignment, standChanged: standChanged})
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	for _, removal := range removals {
		s.publishCommitted(ctx, StandAllocationResult{
			Assignment: removal.assignment, Removed: true,
			StandChanged: removal.standChanged, NotifyEuroscope: removal.standChanged,
		})
		metrics.RecordSATLifecycleEvent(ctx, "displacement", "unsafe_reconciliation", removal.assignment.Source)
		slog.WarnContext(ctx, "SAT released unsafe overlapping assignment",
			slog.Int("session", int(session)),
			slog.String("callsign", removal.assignment.Callsign),
			slog.String("stand", removal.assignment.Stand),
			slog.String("stage", removal.assignment.Stage))
	}
	displaced := make([]models.StandAssignment, 0, len(removals))
	for _, removal := range removals {
		displaced = append(displaced, removal.assignment)
	}
	s.relocateDisplacedArrivals(ctx, displaced, "unsafe_reconciliation")
	return nil
}

func (s *StandAllocationService) unsafeAssignmentLosers(airport string, assignments []*models.StandAssignment, now time.Time) []*models.StandAssignment {
	ordered := make([]*models.StandAssignment, 0, len(assignments))
	for _, assignment := range assignments {
		if assignment != nil && strings.TrimSpace(assignment.Stand) != "" && !standAssignmentExpired(assignment, now) {
			ordered = append(ordered, assignment)
		}
	}
	slices.SortStableFunc(ordered, func(left, right *models.StandAssignment) int {
		leftProtected, rightProtected := assignmentProtectedFromReconciliation(left), assignmentProtectedFromReconciliation(right)
		if leftProtected != rightProtected {
			if leftProtected {
				return -1
			}
			return 1
		}
		if leftRank, rightRank := assignmentReconciliationRank(left), assignmentReconciliationRank(right); leftRank != rightRank {
			return rightRank - leftRank
		}
		leftStart, _, _ := assignmentOccupancyWindow(left, now)
		rightStart, _, _ := assignmentOccupancyWindow(right, now)
		if !leftStart.Equal(rightStart) {
			if leftStart.Before(rightStart) {
				return -1
			}
			return 1
		}
		if left.ID < right.ID {
			return -1
		}
		if left.ID > right.ID {
			return 1
		}
		return strings.Compare(strings.ToUpper(left.Callsign), strings.ToUpper(right.Callsign))
	})

	kept := make([]*models.StandAssignment, 0, len(ordered))
	losers := make([]*models.StandAssignment, 0)
	for _, candidate := range ordered {
		unsafe := false
		for _, winner := range kept {
			if !s.assignmentsOccupancyConflict(airport, candidate, winner, now) {
				continue
			}
			if assignmentProtectedFromReconciliation(candidate) && assignmentProtectedFromReconciliation(winner) {
				continue
			}
			unsafe = true
			break
		}
		if unsafe {
			losers = append(losers, candidate)
			continue
		}
		kept = append(kept, candidate)
	}
	return losers
}

func (s *StandAllocationService) assignmentsOccupancyConflict(airport string, left, right *models.StandAssignment, now time.Time) bool {
	leftStand, rightStand := standName(left.Stand), standName(right.Stand)
	direct := leftStand == rightStand
	adjacent := blocksEachOther(s.assignedBlocks(airport, left), s.assignedBlocks(airport, right), leftStand, rightStand)
	if !direct && !adjacent {
		return false
	}
	leftStart, leftEnd, leftActive := assignmentOccupancyWindow(left, now)
	rightStart, rightEnd, rightActive := assignmentOccupancyWindow(right, now)
	if !leftActive || !rightActive {
		return false
	}
	return occupancyWindowsOverlap(leftStart, leftEnd, rightStart, rightEnd, now)
}

func assignmentProtectedFromReconciliation(assignment *models.StandAssignment) bool {
	if assignment == nil {
		return false
	}
	if assignment.Manual || assignment.Stage == StageDepartureBlock ||
		(assignment.Direction == string(sat.AssignmentDirectionArrival) &&
			(assignment.Stage == StageConfirmed || assignment.ExpiresAt != nil)) {
		return true
	}
	return assignment.ObservedStand != nil && strings.EqualFold(strings.TrimSpace(*assignment.ObservedStand), strings.TrimSpace(assignment.Stand))
}

func assignmentReconciliationRank(assignment *models.StandAssignment) int {
	if assignment == nil {
		return 0
	}
	switch assignment.Stage {
	case StageConfirmed:
		return 50
	case StageAssigned:
		return 40
	case StageDepartureBlock:
		return 35
	case StageReserved:
		return 30
	case StageEstimated:
		return 20
	default:
		return 10
	}
}

func assignmentOccupancyWindow(assignment *models.StandAssignment, now time.Time) (time.Time, *time.Time, bool) {
	if assignment == nil || standAssignmentExpired(assignment, now) {
		return time.Time{}, nil, false
	}
	if assignment.ExpiresAt != nil {
		return now, assignment.ExpiresAt, true
	}
	if assignment.Direction == string(sat.AssignmentDirectionDeparture) && assignment.ProjectedReleaseAt != nil {
		if !assignment.ProjectedReleaseAt.After(now) {
			// A missed projection cannot overrule a still-observed aircraft. Keep
			// the stand occupied until fresh timing or position evidence arrives.
			return now, nil, true
		}
		return now, assignment.ProjectedReleaseAt, true
	}
	if assignment.Direction == string(sat.AssignmentDirectionArrival) && assignment.ETA != nil {
		end := assignment.ETA.Add(arrivalStandRetention)
		if !end.After(now) {
			// A stale ETA is not proof that a delayed inbound disappeared. Treat
			// it as active now with an unknown release until lifecycle facts move.
			return now, nil, true
		}
		return *assignment.ETA, &end, true
	}
	return now, nil, true
}

func requestOccupancyWindow(request StandAllocationRequest, now time.Time, departureReleaseBuffer time.Duration) (time.Time, *time.Time, bool) {
	if request.ExpiresAt != nil {
		if !request.ExpiresAt.After(now) {
			return time.Time{}, nil, false
		}
		return now, request.ExpiresAt, true
	}
	if request.Direction == sat.AssignmentDirectionArrival {
		if request.ETA == nil {
			return now, nil, true
		}
		end := request.ETA.Add(arrivalStandRetention)
		if !end.After(now) {
			// A stale ETA cannot prove that a delayed inbound has vacated.
			return now, nil, true
		}
		return *request.ETA, &end, true
	}
	if request.Direction == sat.AssignmentDirectionDeparture {
		release := projectedDepartureRelease(request, departureReleaseBuffer)
		if release == nil {
			return now, nil, true
		}
		if !release.After(now) {
			return time.Time{}, nil, false
		}
		return now, release, true
	}
	return now, nil, true
}

func projectedDepartureRelease(request StandAllocationRequest, departureReleaseBuffer time.Duration) *time.Time {
	release := request.DepartureTOBT
	if request.DepartureTSAT != nil && (release == nil || request.DepartureTSAT.After(*release)) {
		release = request.DepartureTSAT
	}
	if release == nil {
		return nil
	}
	effectiveRelease := *release
	if !request.DepartureReady {
		if departureReleaseBuffer <= 0 {
			departureReleaseBuffer = defaultDepartureBlockExtension
		}
		effectiveRelease = effectiveRelease.Add(departureReleaseBuffer)
	}
	return &effectiveRelease
}

func assignmentBlocksRequest(assignment *models.StandAssignment, request StandAllocationRequest, now time.Time, departureReleaseBuffer time.Duration) bool {
	// Sharing is allowed only when both sides have a bounded or scheduled
	// occupancy window. Missing timing cannot prove that a future use is safe.
	if assignmentTimingUnknown(assignment) || requestTimingUnknown(request) {
		return true
	}
	assignmentStart, assignmentEnd, assignmentActive := assignmentOccupancyWindow(assignment, now)
	requestStart, requestEnd, requestActive := requestOccupancyWindow(request, now, departureReleaseBuffer)
	if !assignmentActive || !requestActive {
		return false
	}
	return occupancyWindowsOverlap(assignmentStart, assignmentEnd, requestStart, requestEnd, now)
}

func assignmentTimingUnknown(assignment *models.StandAssignment) bool {
	if assignment == nil || assignment.ExpiresAt != nil {
		return false
	}
	if assignment.Direction == string(sat.AssignmentDirectionDeparture) {
		return assignment.ProjectedReleaseAt == nil
	}
	return assignment.ETA == nil
}

func requestTimingUnknown(request StandAllocationRequest) bool {
	if request.ExpiresAt != nil {
		return false
	}
	if request.Direction == sat.AssignmentDirectionArrival {
		return request.ETA == nil
	}
	return request.Direction == sat.AssignmentDirectionDeparture && request.DepartureTOBT == nil && request.DepartureTSAT == nil
}

func occupancyWindowsOverlap(leftStart time.Time, leftEnd *time.Time, rightStart time.Time, rightEnd *time.Time, now time.Time) bool {
	if leftEnd == nil && leftStart.Equal(now) && rightStart.After(now) {
		return false
	}
	if rightEnd == nil && rightStart.Equal(now) && leftStart.After(now) {
		return false
	}
	if leftEnd != nil && !leftEnd.After(rightStart) {
		return false
	}
	return rightEnd == nil || rightEnd.After(leftStart)
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
		failures:               standdiagnostics.NewAllocationFailureLog(100),
		departureReleaseBuffer: defaultDepartureBlockExtension,
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

// assignObservedStand adopts the stand where EuroScope saw an aircraft. The
// aircraft is already physically present, so stand capability preferences must
// not trigger a relocation by themselves. Departures retain their existing
// wrong-stand warning flow when the observed stand is unavailable; confirmed
// arrival plans remain stable and surface a conflict for controller action.
// Multiple departures physically observed on the same stand are retained as
// physical truth instead of assigning one of them a fictional alternative.
func (s *StandAllocationService) assignObservedStand(ctx context.Context, request StandAllocationRequest) (*StandAllocationResult, error) {
	// A departure observed on a stand is physically there. Its presence takes
	// precedence over provisional arrival plans. A CONFIRMED plan is retained
	// as a warned conflict until a controller moves it. Other departures and
	// callsign-less controller blocks remain protected by availability checks.
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
	ctx = beginRelocationChain(ctx, request.Callsign)
	previousAutomaticOutcome := ""
	if isAutomaticStandAllocation(command) {
		skipAttempt, previousOutcome := s.automaticAllocationSuppression(request)
		if skipAttempt {
			return nil, ErrAutomaticAllocationSuppressed
		}
		previousAutomaticOutcome = previousOutcome
	} else if command == observedStandAllocation {
		// Physical conflicts must still be retried on every observation so a
		// newly freed stand is claimed immediately. Remember the last identical
		// outcome only to suppress repeated telemetry.
		_, previousAutomaticOutcome = s.automaticAllocationSuppression(request)
	}
	tried := map[string]struct{}{}
	for attempt := 1; attempt <= s.attempts; attempt++ {
		result, selected, err := s.allocateOnce(ctx, command, request, tried, attempt)
		if err == nil {
			s.clearAutomaticTerminalFailure(request)
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
			if command == observedStandAllocation {
				metrics.RecordSATLifecycleEvent(ctx, "physical_takeover", "observed_position", result.Assignment.Source)
			}
			for _, displaced := range result.RemovedAssignments {
				metrics.RecordSATLifecycleEvent(ctx, "displacement", string(command), displaced.Source)
				slog.WarnContext(ctx, "SAT assignment displaced",
					slog.String("callsign", displaced.Callsign), slog.String("stand", displaced.Stand),
					slog.String("stage", displaced.Stage), slog.String("source", displaced.Source),
					slog.String("reason", string(command)), slog.String("takeover_callsign", request.Callsign))
			}
			slog.InfoContext(ctx, "SAT stand allocation committed",
				slog.String("callsign", request.Callsign), slog.Int("session", int(request.SessionID)),
				slog.String("command", string(command)), slog.String("stand", result.Assignment.Stand),
				slog.String("stage", result.Assignment.Stage), slog.String("source", result.Assignment.Source),
				slog.String("rule_id", rule), slog.Int("tier", tier), slog.Int("attempt", attempt))
			s.publishCommitted(ctx, *result)
			s.relocateDisplacedArrivals(ctx, result.RemovedAssignments, string(command))
			return result, nil
		}
		if selected != "" {
			tried[selected] = struct{}{}
		}
		if !retryableStandAllocationError(err) {
			if errors.Is(err, ErrNoTierImprovement) {
				// Promotion probes are expected optimization misses. The lifecycle
				// retains the valid current stand, so this is neither an operational
				// error nor an allocation-failure diagnostic.
				s.clearAutomaticTerminalFailure(request)
				return nil, err
			}
			outcome := standAllocationFailureOutcome(err)
			if command == observedStandAllocation && previousAutomaticOutcome == outcome {
				return nil, ErrAutomaticAllocationSuppressed
			}
			if isAutomaticStandAllocation(command) && previousAutomaticOutcome == outcome && isTransientAutomaticStandShortage(err) {
				// Capacity is probed on every lifecycle poll so a newly freed stand
				// is claimed promptly. An unchanged shortage is reported only once;
				// repeated logs, metrics, and diagnostic records are suppressed.
				return nil, ErrAutomaticAllocationSuppressed
			}
			if command == observedStandAllocation {
				s.noteAutomaticTerminalFailure(request, outcome)
				metrics.RecordSATLifecycleEvent(ctx, "unresolved_conflict", "protected_occupancy", "PHYSICAL")
			}
			if isAutomaticStandAllocation(command) {
				if isTerminalAutomaticStandShortage(err) {
					s.noteAutomaticTerminalFailure(request, outcome)
				} else {
					s.clearAutomaticTerminalFailure(request)
				}
			}
			metrics.RecordSATOutcome(ctx, outcome, string(request.Direction))
			logStandAllocationRejection(ctx, command, request, outcome, err)
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

func (s *StandAllocationService) relocateDisplacedArrivals(ctx context.Context, displacedAssignments []models.StandAssignment, reason string) {
	if s.relocate == nil {
		return
	}
	for _, displaced := range displacedAssignments {
		if displaced.Direction != string(sat.AssignmentDirectionArrival) || !isArrivalStage(displaced.Stage) {
			continue
		}
		if err := s.relocate(ctx, displaced); err != nil {
			metrics.RecordSATLifecycleEvent(ctx, "failed_relocation", reason, displaced.Source)
			slog.WarnContext(ctx, "SAT displaced arrival relocation failed",
				slog.String("callsign", displaced.Callsign), slog.String("former_stand", displaced.Stand),
				slog.String("stage", displaced.Stage), slog.String("source", displaced.Source), slog.Any("error", err))
			continue
		}
		metrics.RecordSATLifecycleEvent(ctx, "relocation", reason, displaced.Source)
	}
}

func logStandAllocationRejection(ctx context.Context, command StandAllocationCommand, request StandAllocationRequest, outcome string, err error) {
	attributes := []any{
		slog.String("callsign", request.Callsign), slog.String("command", string(command)),
		slog.String("outcome", outcome), slog.String("stage", request.Stage), slog.Any("error", err),
	}
	if standdiagnostics.SeverityForStage(request.Stage) == standdiagnostics.SeverityError {
		slog.ErrorContext(ctx, "SAT stand allocation rejected", attributes...)
		return
	}
	slog.WarnContext(ctx, "SAT stand allocation warning", attributes...)
}

func isAutomaticStandAllocation(command StandAllocationCommand) bool {
	return command == AutomaticStandAllocation || command == AutomaticStandReallocation
}

func (s *StandAllocationService) automaticAllocationSuppressed(request StandAllocationRequest) bool {
	skipAttempt, previousOutcome := s.automaticAllocationSuppression(request)
	return skipAttempt || previousOutcome != ""
}

func (s *StandAllocationService) automaticAllocationSuppression(request StandAllocationRequest) (skipAttempt bool, previousOutcome string) {
	key, fingerprint := automaticFailureKeyAndFingerprint(request)
	s.automaticFailureMu.Lock()
	defer s.automaticFailureMu.Unlock()
	failure, ok := s.automaticTerminalFailures[key]
	if !ok || failure.fingerprint != fingerprint {
		return false, ""
	}
	if failure.outcome == "no_compatible_stand" {
		return true, failure.outcome
	}
	return false, failure.outcome
}

func (s *StandAllocationService) noteAutomaticTerminalFailure(request StandAllocationRequest, outcome string) {
	key, fingerprint := automaticFailureKeyAndFingerprint(request)
	s.automaticFailureMu.Lock()
	defer s.automaticFailureMu.Unlock()
	if s.automaticTerminalFailures == nil {
		s.automaticTerminalFailures = make(map[string]automaticTerminalFailure)
	}
	s.automaticTerminalFailures[key] = automaticTerminalFailure{fingerprint: fingerprint, outcome: outcome}
}

func (s *StandAllocationService) clearAutomaticTerminalFailure(request StandAllocationRequest) {
	key, _ := automaticFailureKeyAndFingerprint(request)
	s.automaticFailureMu.Lock()
	defer s.automaticFailureMu.Unlock()
	delete(s.automaticTerminalFailures, key)
}

func isTerminalAutomaticStandShortage(err error) bool {
	// Compatibility is stable for an unchanged fingerprint and can skip the
	// allocation attempt. Availability is transient and is still probed on each
	// poll, while duplicate observability emissions are suppressed.
	return errors.Is(err, ErrNoCompatibleStand) || isTransientAutomaticStandShortage(err)
}

func isTransientAutomaticStandShortage(err error) bool {
	return errors.Is(err, ErrNoAvailableStand) || errors.Is(err, ErrNoPolicyStand)
}

func automaticFailureKeyAndFingerprint(request StandAllocationRequest) (string, string) {
	facts := request.FlightFacts
	assignmentFacts := request.AssignmentFacts
	key := fmt.Sprintf("%d:%s", request.SessionID, strings.ToUpper(strings.TrimSpace(request.Callsign)))
	fingerprint := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%t|%s|%.6f|%.6f|%.6f|%.3f|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		strings.ToUpper(strings.TrimSpace(request.Airport)), request.Direction, strings.ToUpper(strings.TrimSpace(request.Stage)),
		standName(request.Stand), standName(valueString(request.ObservedStand)), facts.Origin, facts.Destination, facts.AircraftKnown, facts.Aircraft.Type,
		facts.Aircraft.WingspanMetres, facts.Aircraft.LengthMetres, facts.Aircraft.HeightMetres,
		facts.Aircraft.MTOWKilograms, facts.Aircraft.UseCode, facts.EngineType, facts.WTC,
		facts.BorderEndpoint, facts.BorderStatus, assignmentFacts.AircraftType,
		assignmentFacts.AircraftUse, assignmentFacts.BorderStatus, assignmentFacts.Direction,
		assignmentFacts.Special)
	return key, fingerprint
}

func suppressAutomaticAllocationError(err error) error {
	if isTerminalAutomaticStandShortage(err) ||
		errors.Is(err, ErrAutomaticAllocationSuppressed) ||
		errors.Is(err, ErrNoTierImprovement) {
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
	case errors.Is(err, ErrNoPolicyStand):
		return "no_policy_stand"
	case errors.Is(err, ErrNoTierImprovement):
		return "no_tier_improvement"
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
		Severity:       standdiagnostics.SeverityForStage(request.Stage),
		Reason:         reason,
		Direction:      string(request.Direction),
		Stage:          request.Stage,
		AttemptedStand: request.Stand,
		AircraftType:   request.AssignmentFacts.AircraftType,
		EngineType:     string(request.FlightFacts.EngineType),
		WTC:            request.FlightFacts.WTC,
		BorderStatus:   string(request.FlightFacts.BorderStatus),
		ETA:            cloneTimePointer(request.ETA),
		ETASource:      valueString(request.ETASource),
		ExpiresAt:      cloneTimePointer(request.ExpiresAt),
		DepartureTOBT:  cloneTimePointer(request.DepartureTOBT),
		DepartureTSAT:  cloneTimePointer(request.DepartureTSAT),
		DepartureReady: request.DepartureReady,
		Attempts:       attempts,
	})
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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
	if isAutomaticStandAllocation(command) && s.selectedHasOverlappingEstimatedReservation(
		assignments, selected, matchBlocks(match), request, s.now(), s.departureReleaseBuffer,
	) {
		request.addDisplaceArrivalStage(StageEstimated)
	}
	var removedAssignments []models.StandAssignment
	var removedStandChanges []models.StandAssignment
	if request.displacesArrivalStage() && selected != "" {
		selectedBlocks := []string(nil)
		if match != nil {
			selectedBlocks = match.Blocks
		}
		removedAssignments, removedStandChanges, err = s.displaceAssignments(ctx, txStrips, txAssignments, request, selected, selectedBlocks, assignments)
		if err != nil {
			return nil, selected, err
		}
	}
	request.Stand = selected
	assignment, err := s.persistStandAllocation(ctx, txAssignments, command, request, assignments, selection, match, conflict)
	if err != nil {
		return nil, selected, err
	}
	standChanged := strip.Stand == nil || !strings.EqualFold(strings.TrimSpace(*strip.Stand), strings.TrimSpace(selected))
	if standChanged {
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
		RemovedAssignments: removedAssignments, RemovedStandChanges: removedStandChanges,
		StandChanged: standChanged, NotifyEuroscope: standChanged || request.RequestStandSync,
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

// ConfirmedArrivalConflictAtStand reports only a timing-overlapping CONFIRMED
// arrival. It ignores provisional arrivals that a physical departure may
// displace and does not misclassify protected departures or manual blocks.
func (s *StandAllocationService) ConfirmedArrivalConflictAtStand(ctx context.Context, request StandAllocationRequest, stand string) (bool, error) {
	target := standName(stand)
	if target == "" {
		return false, errors.New("stand conflict check requires a stand")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT id FROM sessions WHERE id = $1 FOR UPDATE", request.SessionID); err != nil {
		return false, err
	}
	store := s.assignments.WithTx(tx)
	assignments, err := store.LockAssignments(ctx, request.SessionID, request.Callsign)
	if err != nil {
		return false, err
	}
	if _, known := s.stands.Lookup(request.Airport, target); !known {
		return false, nil
	}
	return s.confirmedArrivalConflicts(request, target, assignments), nil
}

// AvailableStands evaluates every configured stand for an explicit selection.
// It uses the same compatibility and timing rules as a manual assignment but
// does not modify an assignment or reserve a stand.
func (s *StandAllocationService) AvailableStands(ctx context.Context, request StandAllocationRequest) ([]StandAvailability, error) {
	if err := validateStandAllocationRequest(AutomaticStandAllocation, &request); err != nil {
		return nil, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT id FROM sessions WHERE id = $1 FOR UPDATE", request.SessionID); err != nil {
		return nil, err
	}
	txAssignments := s.assignments.WithTx(tx)
	assignments, err := txAssignments.LockAssignments(ctx, request.SessionID, request.Callsign)
	if err != nil {
		return nil, err
	}
	blocks, err := txAssignments.LockActiveManualBlocks(ctx, request.SessionID)
	if err != nil {
		return nil, err
	}

	evaluation := s.stands.EvaluateManualCompatibility(request.Airport, request.FlightFacts)
	matches := make(map[string]sat.StandCompatibilityMatch, len(evaluation.Matches))
	for _, match := range evaluation.Matches {
		matches[standName(match.Stand.Name)] = match
	}
	unavailable := s.availability(request, assignments, blocks, matches)
	stands := s.stands.Stands(request.Airport)
	result := make([]StandAvailability, 0, len(stands))
	for _, stand := range stands {
		name := standName(stand.Name)
		if _, compatible := matches[name]; !compatible {
			result = append(result, StandAvailability{Stand: name, Reason: compatibilityReason(name, evaluation.Rejections)})
			continue
		}
		if reasons := unavailable[name]; len(reasons) > 0 {
			result = append(result, StandAvailability{Stand: name, Reason: joinAllocationReasons(reasons)})
			continue
		}
		result = append(result, StandAvailability{Stand: name, Available: true})
	}
	return result, nil
}

// Preview reports the current policy candidates without reserving, updating,
// or publishing anything. It is intended for controller diagnostics.
func (s *StandAllocationService) Preview(ctx context.Context, request StandAllocationRequest) (StandAllocationPreview, error) {
	if err := validateStandAllocationRequest(AutomaticStandAllocation, &request); err != nil {
		return StandAllocationPreview{}, err
	}
	assignments, err := s.assignments.ListAssignments(ctx, request.SessionID)
	if err != nil {
		return StandAllocationPreview{}, err
	}
	blocks, err := s.assignments.ListBlocks(ctx, request.SessionID)
	if err != nil {
		return StandAllocationPreview{}, err
	}
	activeBlocks := make([]*models.StandBlock, 0, len(blocks))
	for _, block := range blocks {
		if block != nil && block.Manual && !expired(block.ExpiresAt, s.now()) {
			activeBlocks = append(activeBlocks, block)
		}
	}

	evaluation := s.stands.EvaluateCompatibility(request.Airport, request.FlightFacts)
	matches := make(map[string]sat.StandCompatibilityMatch, len(evaluation.Matches))
	for _, match := range evaluation.Matches {
		matches[standName(match.Stand.Name)] = match
	}
	available, selection, err := s.automaticStandPool(request, assignments, activeBlocks, matches, nil)
	if err != nil {
		return StandAllocationPreview{}, err
	}
	preview := StandAllocationPreview{
		Callsign: request.Callsign, Airport: request.Airport, FallbackUsed: selection.FallbackUsed,
		CompatibleStands: len(matches), AvailableStands: len(available), Selection: selection,
	}
	return preview, nil
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
			if request.Direction == sat.AssignmentDirectionDeparture {
				// Aircraft can spawn on the same stand, or on a heavy stand and a
				// medium stand that it nominally blocks. When the only conflict is
				// another physically observed departure, retain and report both real
				// positions. Manual blocks and arrival reservations still use the
				// normal protection rules.
				withoutObservedDepartures := slices.DeleteFunc(slices.Clone(assignments), func(assignment *models.StandAssignment) bool {
					return assignment != nil &&
						assignment.Direction == string(sat.AssignmentDirectionDeparture) &&
						assignment.Stage == StageDepartureBlock &&
						assignment.ObservedStand != nil &&
						standName(*assignment.ObservedStand) == standName(assignment.Stand)
				})
				if len(withoutObservedDepartures) != len(assignments) &&
					len(s.availability(request, withoutObservedDepartures, blocks, matches)[target]) == 0 {
					return target, nil, &match, []string{target}, "", nil
				}
				// Physical truth still wins, but a CONFIRMED arrival is a routing
				// commitment. Permit the observed departure to coexist with that
				// protected plan and surface the overlap as an advisory. An explicit
				// controller AUTO or manual action may then move the arrival.
				withoutConfirmed := slices.DeleteFunc(slices.Clone(withoutObservedDepartures), func(assignment *models.StandAssignment) bool {
					return assignment != nil && assignment.Direction == string(sat.AssignmentDirectionArrival) && assignment.Stage == StageConfirmed
				})
				if s.confirmedArrivalConflicts(request, target, assignments) && len(s.availability(request, withoutConfirmed, blocks, matches)[target]) == 0 {
					return target, nil, &match, []string{target}, observedDepartureConflictPrefix + " " + strings.Join(availability[target], "; "), nil
				}
			}
			if request.Direction == sat.AssignmentDirectionArrival && request.Stage == StageConfirmed {
				// A parked arrival's observed position is physical truth. Keep any
				// confirmed booking or manual block as a visible conflict, but adopt
				// the occupied stand instead of instructing the aircraft to move.
				return target, nil, &match, []string{target}, "observed parked arrival: " + strings.Join(availability[target], "; "), nil
			}
			return target, nil, nil, nil, "", fmt.Errorf("%w: %s", ErrIncompatibleManualAssignment, target)
		}
		return target, nil, &match, []string{target}, "", nil
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
	if isAutomaticStandAllocation(command) && request.Stand != "" {
		// A stage promotion may need to evict an overlapping ESTIMATED
		// reservation while retaining its own operationally valid stand. Prefer
		// that stand when only a neighbouring soft reservation blocks it. A
		// direct reservation gets the open-pool selection first so it is displaced
		// only when no equal policy candidate remains.
		preferredAvailability := availability
		if request.Stage != StageEstimated {
			preferredAvailability = s.availabilityYieldingEstimated(request, assignments, blocks, matches)
		}
		if match, compatible := matches[request.Stand]; compatible &&
			len(preferredAvailability[request.Stand]) == 0 &&
			!hasDirectOverlappingEstimatedReservation(assignments, request.Stand, request, s.now(), s.departureReleaseBuffer) {
			selection, selectionErr := s.policy.SelectStand(request.AssignmentFacts, []string{request.Stand}, s.random)
			if selectionErr != nil {
				return "", nil, nil, nil, "", selectionErr
			}
			if selection != nil && (request.ImproveTierBelow == 0 || int32(selection.Tier) < request.ImproveTierBelow) {
				return request.Stand, selection, &match, []string{request.Stand}, "", nil
			}
		}
	}
	if len(matches) == 0 {
		return "", nil, nil, nil, "", ErrNoCompatibleStand
	}

	available, _, err := s.automaticStandPool(request, assignments, blocks, matches, tried)
	if err != nil {
		return "", nil, nil, nil, "", err
	}
	selection, err := s.policy.SelectStand(request.AssignmentFacts, available, s.random)
	if err != nil {
		return "", nil, nil, available, "", err
	}
	if selection == nil {
		if len(available) > 0 {
			return "", nil, nil, available, "", s.policyExhaustionError(request, available, matches, assignments, blocks)
		}
		return "", nil, nil, available, "", ErrNoAvailableStand
	}
	if request.ImproveTierBelow > 0 && int32(selection.Tier) >= request.ImproveTierBelow {
		return "", nil, nil, available, "", ErrNoTierImprovement
	}
	match := matches[selection.Stand]
	return selection.Stand, selection, &match, available, "", nil
}

func (s *StandAllocationService) policyExhaustionError(request StandAllocationRequest, available []string, matches map[string]sat.StandCompatibilityMatch, assignments []*models.StandAssignment, blocks []*models.StandBlock) error {
	compatible := make([]string, 0, len(matches))
	for stand := range matches {
		compatible = append(compatible, stand)
	}
	slices.Sort(compatible)
	matchedRule := ""
	if match, err := s.policy.MatchRule(request.AssignmentFacts); err == nil && match != nil && match.Rule != nil {
		matchedRule = match.Rule.ID
	}
	policyPreview, _ := s.policy.PreviewStandSelection(request.AssignmentFacts, compatible)
	fallbackPreview, _ := s.policy.PreviewFallbackStandSelection(request.AssignmentFacts, compatible)
	policyCandidates := append(previewCandidateStands(policyPreview), previewCandidateStands(fallbackPreview)...)
	slices.Sort(policyCandidates)
	policyCandidates = slices.Compact(policyCandidates)
	unavailable := s.availability(request, assignments, blocks, matches)
	blockedPolicy := make([]string, 0, len(policyCandidates))
	blockingAssignments := make([]string, 0)
	for _, stand := range policyCandidates {
		if reasons := unavailable[stand]; len(reasons) > 0 {
			blockedPolicy = append(blockedPolicy, stand+"["+strings.Join(reasons, ", ")+"]")
			for _, assignment := range assignments {
				if assignment == nil || strings.EqualFold(assignment.Callsign, request.Callsign) {
					continue
				}
				reasonText := strings.Join(reasons, " ")
				if !strings.Contains(reasonText, assignment.Callsign) && !strings.Contains(reasonText, "neighbor "+assignment.Stand) {
					continue
				}
				blockingAssignments = append(blockingAssignments, formatBlockingAssignment(assignment))
			}
		}
	}
	slices.Sort(blockingAssignments)
	blockingAssignments = slices.Compact(blockingAssignments)
	return fmt.Errorf("%w: available compatible stands=%s; matched rule=%s; compatible policy path=%s:%s; compatible fallback=%s:%s; unavailable policy candidates=%s; blocking assignments=%s",
		ErrNoPolicyStand, strings.Join(available, ","), matchedRule,
		policyPreview.RuleID, strings.Join(previewCandidateStands(policyPreview), ","),
		fallbackPreview.RuleID, strings.Join(previewCandidateStands(fallbackPreview), ","), strings.Join(blockedPolicy, "; "), strings.Join(blockingAssignments, "; "))
}

func formatBlockingAssignment(assignment *models.StandAssignment) string {
	eta, expires := "none", "none"
	if assignment.ETA != nil {
		eta = assignment.ETA.UTC().Format(time.RFC3339)
	}
	if assignment.ExpiresAt != nil {
		expires = assignment.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("%s@%s(%s/%s,eta=%s,expires=%s)", assignment.Callsign, assignment.Stand, assignment.Direction, assignment.Stage, eta, expires)
}

func previewCandidateStands(preview sat.StandSelectionPreview) []string {
	stands := make([]string, 0, len(preview.Candidates))
	for _, candidate := range preview.Candidates {
		stands = append(stands, candidate.Stand)
	}
	slices.Sort(stands)
	return slices.Compact(stands)
}

func (s *StandAllocationService) confirmedArrivalConflicts(request StandAllocationRequest, target string, assignments []*models.StandAssignment) bool {
	now := s.now()
	targetBlocks := s.configuredStandBlocks(request.Airport, target)
	for _, assignment := range assignments {
		if assignment == nil || assignment.Direction != string(sat.AssignmentDirectionArrival) || assignment.Stage != StageConfirmed || standAssignmentExpired(assignment, now) {
			continue
		}
		overlaps := standName(assignment.Stand) == standName(target) ||
			blocksEachOther(targetBlocks, s.assignedBlocks(request.Airport, assignment), target, assignment.Stand)
		if !overlaps {
			continue
		}
		if !assignmentBlocksRequest(assignment, request, now, s.departureReleaseBuffer) {
			continue
		}
		return true
	}
	return false
}

// automaticStandPool keeps ESTIMATED arrivals as soft reservations. It only
// releases those reservations when doing so prevents the requesting aircraft
// from falling to a later rule/tier, a fallback, or no stand at all.
func (s *StandAllocationService) automaticStandPool(request StandAllocationRequest, assignments []*models.StandAssignment, blocks []*models.StandBlock, matches map[string]sat.StandCompatibilityMatch, tried map[string]struct{}) ([]string, sat.StandSelectionPreview, error) {
	strict := availableStandNames(matches, s.availability(request, assignments, blocks, matches), tried)
	strictPreview, err := s.policy.PreviewStandSelection(request.AssignmentFacts, strict)
	if err != nil {
		return nil, sat.StandSelectionPreview{}, err
	}
	relaxed := availableStandNames(matches, s.availabilityYieldingEstimated(request, assignments, blocks, matches), tried)
	relaxedPreview, err := s.policy.PreviewStandSelection(request.AssignmentFacts, relaxed)
	if err != nil {
		return nil, sat.StandSelectionPreview{}, err
	}
	if relaxedSelectionImproves(strictPreview, relaxedPreview) {
		return relaxed, relaxedPreview, nil
	}
	return strict, strictPreview, nil
}

func availableStandNames(matches map[string]sat.StandCompatibilityMatch, availability map[string][]string, tried map[string]struct{}) []string {
	available := make([]string, 0, len(matches))
	for stand := range matches {
		if len(availability[stand]) != 0 {
			continue
		}
		if _, retrying := tried[stand]; retrying {
			continue
		}
		available = append(available, stand)
	}
	slices.Sort(available)
	return available
}

func relaxedSelectionImproves(strict, relaxed sat.StandSelectionPreview) bool {
	strictCandidate, strictOK := selectablePreviewCandidate(strict)
	relaxedCandidate, relaxedOK := selectablePreviewCandidate(relaxed)
	if !relaxedOK {
		return false
	}
	if !strictOK {
		return true
	}
	if strictCandidate.FallbackUsed != relaxedCandidate.FallbackUsed {
		return strictCandidate.FallbackUsed && !relaxedCandidate.FallbackUsed
	}
	if strictCandidate.RuleID != relaxedCandidate.RuleID {
		// The relaxed pool is a superset of the strict pool, so a different
		// selected rule can only be an earlier preferred policy path.
		return true
	}
	return relaxedCandidate.Tier < strictCandidate.Tier
}

func selectablePreviewCandidate(preview sat.StandSelectionPreview) (sat.StandSelectionCandidate, bool) {
	for _, candidate := range preview.Candidates {
		if candidate.Selectable {
			return candidate, true
		}
	}
	return sat.StandSelectionCandidate{}, false
}

func (s *StandAllocationService) availability(request StandAllocationRequest, assignments []*models.StandAssignment, blocks []*models.StandBlock, matches map[string]sat.StandCompatibilityMatch) map[string][]string {
	return s.availabilityWithEstimated(request, assignments, blocks, matches, false)
}

func (s *StandAllocationService) availabilityYieldingEstimated(request StandAllocationRequest, assignments []*models.StandAssignment, blocks []*models.StandBlock, matches map[string]sat.StandCompatibilityMatch) map[string][]string {
	return s.availabilityWithEstimated(request, assignments, blocks, matches, true)
}

func (s *StandAllocationService) availabilityWithEstimated(request StandAllocationRequest, assignments []*models.StandAssignment, blocks []*models.StandBlock, matches map[string]sat.StandCompatibilityMatch, yieldEstimated bool) map[string][]string {
	now := s.now()
	result := map[string][]string{}
	for candidate, match := range matches {
		for _, assignment := range assignments {
			if assignment == nil || strings.EqualFold(assignment.Callsign, request.Callsign) || standAssignmentExpired(assignment, now) {
				continue
			}
			direct := candidate == standName(assignment.Stand)
			adjacent := blocksEachOther(match.Blocks, s.assignedBlocks(request.Airport, assignment), candidate, assignment.Stand)
			if !direct && !adjacent {
				continue
			}
			if !assignmentBlocksRequest(assignment, request, now, s.departureReleaseBuffer) {
				continue
			}
			if request.displacesAssignment(assignment) ||
				(yieldEstimated && estimatedReservationCanYield(request, assignment)) {
				continue
			}
			if direct {
				reason := "reserved by " + assignment.Callsign
				if assignment.Stage == StageEstimated {
					reason = "soft-reserved by " + assignment.Callsign
				}
				result[candidate] = append(result[candidate], reason)
				continue
			}
			result[candidate] = append(result[candidate], "blocked by allocated neighbor "+assignment.Stand)
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

func estimatedReservationCanYield(request StandAllocationRequest, assignment *models.StandAssignment) bool {
	if assignment == nil || !strings.EqualFold(assignment.Stage, StageEstimated) {
		return false
	}
	// Only a later lifecycle stage may take priority over an overlapping
	// ESTIMATED reservation. Non-overlapping reservations already share safely.
	return !strings.EqualFold(request.Stage, StageEstimated)
}

// futureArrivalBlocksRequest treats a future arrival as a reservation from its
// ETA. A physically active request uses its persisted release deadline; a
// departure without one falls back to TOBT plus the stand-release buffer.
// Planned arrival reservations conflict whenever their retention windows
// overlap, regardless of which booking was inserted first.
func futureArrivalBlocksRequest(arrivalETA time.Time, request StandAllocationRequest, departureReleaseBuffer time.Duration) bool {
	if requestTimingUnknown(request) {
		return true
	}
	now := arrivalETA.Add(-24 * time.Hour)
	arrivalEnd := arrivalETA.Add(arrivalStandRetention)
	requestStart, requestEnd, active := requestOccupancyWindow(request, now, departureReleaseBuffer)
	return active && occupancyWindowsOverlap(requestStart, requestEnd, arrivalETA, &arrivalEnd, now)
}

func standAssignmentExpired(assignment *models.StandAssignment, now time.Time) bool {
	if assignment == nil {
		return true
	}
	if assignment.ExpiresAt != nil {
		return !assignment.ExpiresAt.After(now)
	}
	return false
}

func arrivalReservationWindowsOverlap(leftETA, rightETA time.Time) bool {
	leftEnd := leftETA.Add(arrivalStandRetention)
	rightEnd := rightETA.Add(arrivalStandRetention)
	return leftETA.Before(rightEnd) && rightETA.Before(leftEnd)
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
	next.ObservedStand = request.ObservedStand
	next.ETA, next.ETASource, next.AssignedAt, next.ExpiresAt = request.ETA, request.ETASource, &now, request.ExpiresAt
	next.ProjectedReleaseAt = nil
	if request.Direction == sat.AssignmentDirectionDeparture {
		next.ProjectedReleaseAt = projectedDepartureRelease(request, s.departureReleaseBuffer)
	}
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

func (s *StandAllocationService) displaceAssignments(ctx context.Context, strips repository.StripRepository, assignments repository.StandAssignmentRepository, request StandAllocationRequest, selected string, selectedBlocks []string, current []*models.StandAssignment) ([]models.StandAssignment, []models.StandAssignment, error) {
	if !request.displacesArrivalStage() || selected == "" {
		return nil, nil, nil
	}
	removed := []models.StandAssignment{}
	standChanges := []models.StandAssignment{}
	for _, assignment := range current {
		if assignment == nil || strings.EqualFold(assignment.Callsign, request.Callsign) {
			continue
		}
		if !request.displacesAssignment(assignment) {
			continue
		}
		if !assignmentBlocksRequest(assignment, request, s.now(), s.departureReleaseBuffer) {
			continue
		}
		if !s.assignmentOverlapsSelectedStand(request.Airport, selected, selectedBlocks, assignment) {
			continue
		}
		strip, err := strips.LockByCallsign(ctx, request.SessionID, assignment.Callsign)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, err
		}
		standChanged := strip != nil && strip.Stand != nil
		if standChanged {
			if _, err := strips.UpdateStand(ctx, request.SessionID, assignment.Callsign, nil, nil); err != nil {
				return nil, nil, err
			}
		}
		deleted, err := assignments.DeleteAssignment(ctx, request.SessionID, assignment.ID, assignment.Version)
		if err != nil {
			return nil, nil, err
		}
		if deleted != 1 {
			return nil, nil, errAllocationVersionConflict
		}
		removed = append(removed, *assignment)
		if standChanged {
			standChanges = append(standChanges, *assignment)
		}
	}
	return removed, standChanges, nil
}

func (s *StandAllocationService) assignmentOverlapsSelectedStand(airport, selected string, selectedBlocks []string, assignment *models.StandAssignment) bool {
	if assignment == nil {
		return false
	}
	if standName(assignment.Stand) == standName(selected) {
		return true
	}
	return blocksEachOther(
		selectedBlocks,
		s.assignedBlocks(airport, assignment),
		selected,
		assignment.Stand,
	)
}

func (request StandAllocationRequest) displacesArrivalStage() bool {
	return request.DisplaceStage != "" || len(request.DisplaceArrivalStages) > 0
}

func (request *StandAllocationRequest) addDisplaceArrivalStage(stage string) {
	if request == nil || stage == "" || request.DisplaceStage == stage || slices.Contains(request.DisplaceArrivalStages, stage) {
		return
	}
	if request.DisplaceStage == "" {
		request.DisplaceStage = stage
		return
	}
	request.DisplaceArrivalStages = append(request.DisplaceArrivalStages, stage)
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

func matchBlocks(match *sat.StandCompatibilityMatch) []string {
	if match == nil {
		return nil
	}
	return match.Blocks
}

func hasDirectOverlappingEstimatedReservation(assignments []*models.StandAssignment, selected string, request StandAllocationRequest, now time.Time, departureReleaseBuffer time.Duration) bool {
	selected = standName(selected)
	for _, assignment := range assignments {
		if assignment == nil || strings.EqualFold(assignment.Callsign, request.Callsign) {
			continue
		}
		if assignment.Direction == string(sat.AssignmentDirectionArrival) &&
			assignment.Stage == StageEstimated &&
			standName(assignment.Stand) == selected &&
			assignmentBlocksRequest(assignment, request, now, departureReleaseBuffer) {
			return true
		}
	}
	return false
}

func (s *StandAllocationService) selectedHasOverlappingEstimatedReservation(assignments []*models.StandAssignment, selected string, selectedBlocks []string, request StandAllocationRequest, now time.Time, departureReleaseBuffer time.Duration) bool {
	for _, assignment := range assignments {
		if assignment == nil || strings.EqualFold(assignment.Callsign, request.Callsign) ||
			assignment.Direction != string(sat.AssignmentDirectionArrival) || assignment.Stage != StageEstimated ||
			!assignmentBlocksRequest(assignment, request, now, departureReleaseBuffer) {
			continue
		}
		if s.assignmentOverlapsSelectedStand(request.Airport, selected, selectedBlocks, assignment) {
			return true
		}
	}
	return false
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
