package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/sat"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
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

type manualStandAssignmentError struct {
	reason string
}

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

// StandAllocationService holds shared allocation policy inputs. Candidate
// planners resolve accepted facts and commit decisions through the owner writer.
type StandAllocationService struct {
	stands                 *sat.StandCapabilityRegistry
	policy                 *sat.AirlineAssignmentConfig
	random                 func() float64
	now                    func() time.Time
	departureReleaseBuffer time.Duration

	// Candidate planning snapshots use committed adjacency for existing holds.
	planningBlocks         map[string][]string
	planningBlockAdjacency map[string][]string
	planningOccupancy      map[string]string
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

func isAutomaticStandAllocation(command StandAllocationCommand) bool {
	return command == AutomaticStandAllocation || command == AutomaticStandReallocation
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

func suppressAutomaticAllocationError(err error) error {
	if isTerminalAutomaticStandShortage(err) ||
		errors.Is(err, ErrAutomaticAllocationSuppressed) ||
		errors.Is(err, ErrNoTierImprovement) {
		return nil
	}
	return err
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
		if !compatible {
			return request.Stand, nil, nil, nil, "", manualStandAssignmentError{reason: fmt.Sprintf("%s is incompatible: %s",
				request.Stand, compatibilityReason(request.Stand, evaluation.Rejections))}
		}
		if reasons := availability[request.Stand]; len(reasons) > 0 {
			return request.Stand, nil, nil, nil, "", manualStandAssignmentError{reason: fmt.Sprintf("%s is unavailable: %s",
				request.Stand, joinAllocationReasons(reasons))}
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
		for callsign, stand := range s.planningOccupancy {
			if strings.EqualFold(callsign, request.Callsign) {
				continue
			}
			if candidate == stand || blocksEachOther(match.Blocks, s.configuredStandBlocks(request.Airport, stand), candidate, stand) {
				result[candidate] = append(result[candidate], "physically occupied by "+callsign)
			}
		}
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
			blockedNeighbours := s.configuredStandBlocks(request.Airport, blockedStand)
			if committed, ok := s.planningBlockAdjacency[blockedStand]; ok {
				blockedNeighbours = committed
			}
			adjacencyBlocked := blocksEachOther(s.configuredStandBlocks(request.Airport, candidate), blockedNeighbours, candidate, blockedStand)
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

func standAssignmentExpired(assignment *models.StandAssignment, now time.Time) bool {
	if assignment == nil {
		return true
	}
	if assignment.ExpiresAt != nil {
		return !assignment.ExpiresAt.After(now)
	}
	return false
}

func (s *StandAllocationService) configuredStandBlocks(airport, standName string) []string {
	stand, found := s.stands.Lookup(airport, standName)
	if !found {
		return nil
	}
	return stand.Blocks
}

func (s *StandAllocationService) assignedBlocks(airport string, assignment *models.StandAssignment) []string {
	if blocks, ok := s.planningBlocks[assignment.Callsign]; ok {
		return slices.Clone(blocks)
	}
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

func (s *StandAllocationService) persistStandAllocation(ctx context.Context, store lifecycleAssignments, command StandAllocationCommand, request StandAllocationRequest, current []*models.StandAssignment, selection *sat.StandSelection, match *sat.StandCompatibilityMatch, conflict string) (*models.StandAssignment, error) {
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

func (s *StandAllocationService) displaceAssignments(ctx context.Context, strips lifecycleStrips, assignments lifecycleAssignments, request StandAllocationRequest, selected string, selectedBlocks []string, current []*models.StandAssignment) ([]models.StandAssignment, []models.StandAssignment, error) {
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
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
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

func (e manualStandAssignmentError) Error() string { return e.reason }
func (e manualStandAssignmentError) Unwrap() error { return ErrIncompatibleManualAssignment }
