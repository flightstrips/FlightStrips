package services

import (
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
	"FlightStrips/pkg/events/euroscope"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	StageEstimated = "ESTIMATED"
	StageAssigned  = "ASSIGNED"
	StageConfirmed = "CONFIRMED"

	defaultEstimatedBefore = 45 * time.Minute
	defaultAssignedBefore  = 10 * time.Minute
	defaultConfirmedBefore = 2 * time.Minute

	assignedAltitudeThreshold  = 10000
	confirmedAltitudeThreshold = 3000
	// Once a low-altitude arrival is within two nautical miles of the airport's
	// configured stands, its operational stand must no longer be changed by the
	// planning allocator. This covers runways, taxiways, and aprons rather than
	// only the small stand-position circles.
	arrivalAirportProximity   = 2 * 1852.0
	arrivalAirportMaxAltitude = 1000
	arrivalStandRetention     = 30 * time.Minute
	arrivalRetentionRefreshAt = 10 * time.Minute

	defaultArrivalSweepInterval = 30 * time.Second
)

type ArrivalLifecycleService struct {
	allocations   *StandAllocationService
	assignments   repository.StandAssignmentRepository
	strips        repository.StripRepository
	sessions      lifecycleSessionLister
	stands        *sat.StandCapabilityRegistry
	aircraft      *sat.AircraftRegistry
	engines       *sat.AircraftEngineRegistry
	borders       *sat.AirportCountryRegistry
	now           func() time.Time
	sweepInterval time.Duration
	allowPrefiles bool
}

type ArrivalLifecycleOption func(*ArrivalLifecycleService)

func WithArrivalLifecycleClock(now func() time.Time) ArrivalLifecycleOption {
	return func(s *ArrivalLifecycleService) {
		if now != nil {
			s.now = now
		}
	}
}

func WithArrivalSweepInterval(duration time.Duration) ArrivalLifecycleOption {
	return func(s *ArrivalLifecycleService) {
		if duration > 0 {
			s.sweepInterval = duration
		}
	}
}

// WithArrivalPrefileAssignments enables automatic planning assignments for
// offline VATSIM prefiles. The production default is false: only online
// flights participate in automatic stand allocation.
func WithArrivalPrefileAssignments(enabled bool) ArrivalLifecycleOption {
	return func(s *ArrivalLifecycleService) { s.allowPrefiles = enabled }
}

func NewArrivalLifecycleService(
	allocations *StandAllocationService,
	assignments repository.StandAssignmentRepository,
	strips repository.StripRepository,
	sessions lifecycleSessionLister,
	stands *sat.StandCapabilityRegistry,
	aircraft *sat.AircraftRegistry,
	engines *sat.AircraftEngineRegistry,
	borders *sat.AirportCountryRegistry,
	options ...ArrivalLifecycleOption,
) (*ArrivalLifecycleService, error) {
	if allocations == nil || assignments == nil || strips == nil || sessions == nil || stands == nil {
		return nil, errors.New("arrival lifecycle requires allocation service, repositories, session store, and stand registry")
	}
	service := &ArrivalLifecycleService{
		allocations:   allocations,
		assignments:   assignments,
		strips:        strips,
		sessions:      sessions,
		stands:        stands,
		aircraft:      aircraft,
		engines:       engines,
		borders:       borders,
		now:           time.Now,
		sweepInterval: defaultArrivalSweepInterval,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	allocations.SetDisplacedArrivalHandler(service.reassignDisplacedArrival)
	return service, nil
}

func (s *ArrivalLifecycleService) reassignDisplacedArrival(ctx context.Context, displaced models.StandAssignment) error {
	if displaced.Direction != string(sat.AssignmentDirectionArrival) || !isArrivalStage(displaced.Stage) {
		return nil
	}
	if !visitRelocationChain(ctx, displaced.Callsign) {
		reason := "displaced arrival relocation cycle; controller action required"
		if err := s.recordDisplacedArrivalAdvisory(ctx, displaced, reason); err != nil {
			return err
		}
		return ErrRelocationCycle
	}
	strip, err := s.strips.GetByCallsign(ctx, displaced.SessionID, displaced.Callsign)
	if err != nil {
		return err
	}
	request := s.buildRequest(displaced.SessionID, strip, vatsim.ArrivalFlightInfo{}, displaced.Stage, displaced.ETA, displaced.ExpiresAt)
	if _, err = s.allocations.Allocate(ctx, request); err == nil {
		return nil
	}
	if !errors.Is(err, ErrNoAvailableStand) && !errors.Is(err, ErrNoPolicyStand) &&
		!errors.Is(err, ErrNoCompatibleStand) && !errors.Is(err, ErrAutomaticAllocationSuppressed) {
		return err
	}

	reason := "physically displaced; no compatible replacement stand available"
	if advisoryErr := s.recordDisplacedArrivalAdvisory(ctx, displaced, reason); advisoryErr != nil {
		return advisoryErr
	}
	return err
}

// recordDisplacedArrivalAdvisory preserves an actionable, non-occupying record
// when relocation cannot settle. Its version lets a controller retry automatic
// or manual assignment without resurrecting the former stand.
func (s *ArrivalLifecycleService) recordDisplacedArrivalAdvisory(ctx context.Context, displaced models.StandAssignment, reason string) error {
	advisory := displaced
	advisory.ID = 0
	advisory.Stand = ""
	advisory.ConflictReason = &reason
	advisory.ObservedStand = nil
	advisory.Acknowledged = false
	advisory.AcknowledgedAt = nil
	advisory.AcknowledgedBy = nil
	advisory.Version = 0
	if createErr := s.assignments.CreateAssignment(ctx, &advisory); createErr != nil {
		return fmt.Errorf("record displaced arrival advisory: %w", createErr)
	}
	if publishErr := s.allocations.PublishAssignment(ctx, advisory); publishErr != nil {
		return fmt.Errorf("publish displaced arrival advisory: %w", publishErr)
	}
	return nil
}

func (s *ArrivalLifecycleService) ProcessArrival(ctx context.Context, session int32, strip *models.Strip, flight vatsim.ArrivalFlightInfo) (err error) {
	defer func() { err = suppressAutomaticAllocationError(err) }()
	if strip == nil || strings.TrimSpace(strip.Callsign) == "" {
		return nil
	}
	if !flight.Online && !s.allowPrefiles {
		return s.releaseOfflinePrefileAssignment(ctx, session, strip.Callsign)
	}
	eta := arrivalETATime(strip)
	now := s.now()
	altitude := arrivalAltitude(strip)
	nearAirport := s.arrivalIsNearAirport(strip, flight.Destination)
	atAirport := s.arrivalIsAtAirport(strip, flight.Destination)
	targetStage := determineArrivalTargetStage(eta, now, altitude, atAirport, nearAirport)
	existing, err := s.assignments.GetAssignment(ctx, session, strip.Callsign)
	if err != nil && !isNotFound(err) {
		return err
	}
	if existing == nil {
		if targetStage == StageEstimated && eta != nil && eta.Sub(now) > defaultEstimatedBefore {
			return nil
		}
		// The airport-area guard only freezes an existing operational stand.
		// An unassigned arrival still needs a compatible stand.
		var expiresAt *time.Time
		if atAirport {
			expiresAt = arrivalStandExpiresAt(strip, nil, now)
			if !expiresAt.After(now) {
				// A retained strip can outlive its stand-retention window. Do not
				// recreate an assignment after ALDT + retention has elapsed.
				return nil
			}
			if observedStand, found := s.observedParkedArrivalStand(strip, flight.Destination); found {
				request := s.buildRequest(session, strip, flight, targetStage, eta, expiresAt)
				request.Stand = observedStand
				request.ObservedStand = &observedStand
				_, err := s.allocations.assignObservedStand(ctx, request)
				return err
			}
		}
		return s.ensureAssignment(ctx, session, strip, flight, eta, targetStage, expiresAt)
	}
	if strings.TrimSpace(existing.Stand) == "" {
		// A failed physical-displacement relocation deliberately leaves an
		// advisory assignment with no occupying stand. Treat it as unassigned and
		// retry selection when later feed state or departure timing frees capacity.
		// Allocate updates the advisory record atomically when a stand is found.
		return s.ensureAssignment(ctx, session, strip, flight, eta, targetStage, existing.ExpiresAt)
	}
	currentStage := existing.Stage
	if !isArrivalStage(currentStage) {
		return nil
	}
	// A low-altitude position within the airport area is operational evidence
	// that this arrival is no longer a movable planning reservation. It may
	// still advance its lifecycle stage and retention deadline in place.
	if atAirport {
		expiresAt := arrivalStandExpiresAt(strip, existing.ExpiresAt, now)
		if !expiresAt.After(now) {
			err := s.allocations.ReleaseAssignment(ctx, existing)
			recordSATExpiry(ctx, existing, "arrival_retention_elapsed", err)
			return err
		}
		// A controller-owned assignment remains authoritative. A PARK observation
		// may replace an automatic plan, but must not silently undo an explicit
		// controller decision while the aircraft still reports its previous stand.
		nextObservedStand := existing.ObservedStand
		if observedStand, found := s.observedParkedArrivalStand(strip, flight.Destination); found {
			if arrivalObservedStandBaselinePending(existing) {
				// Manual assignments created before observed-stand tracking have no
				// trustworthy physical baseline. Preserve controller intent and record
				// the first observation; only a later different stand may replace it.
				nextObservedStand = &observedStand
			} else if shouldAdoptObservedArrivalStand(existing, observedStand) {
				request := s.buildRequest(session, strip, flight, targetStage, eta, expiresAt)
				request.Stand = observedStand
				request.ObservedStand = &observedStand
				_, err := s.allocations.assignObservedStand(ctx, request)
				return err
			}
			if strings.EqualFold(existing.Stand, observedStand) {
				nextObservedStand = &observedStand
			}
		}
		if shouldPromoteArrival(currentStage, targetStage) || arrivalExpiryChanged(existing.ExpiresAt, expiresAt) || stringPointerChanged(existing.ObservedStand, nextObservedStand) {
			// Once the aircraft is in the airport area, ETA changes are no longer
			// operational stand-planning inputs. Preserve the stand and its last
			// accepted ETA while only advancing stage/retention metadata.
			return s.updateArrivalAtAirport(ctx, existing, targetStage, expiresAt, nextObservedStand)
		}
		return nil
	}
	if !shouldPromoteArrival(currentStage, targetStage) {
		// CONFIRMED is the routing-stability boundary. Once entered, feed polls,
		// ETA changes, compatibility changes, and newly introduced stand blocks
		// must never trigger automatic stand movement. Only controller intent or
		// authoritative physical-position reconciliation may move it.
		if currentStage == StageConfirmed {
			return nil
		}
		if err := s.reallocateIfBlocked(ctx, session, strip, flight, existing, eta, targetStage); err != nil {
			return err
		}
		return nil
	}
	return s.promoteArrival(ctx, session, strip, flight, existing, eta, targetStage)
}

func (s *ArrivalLifecycleService) releaseOfflinePrefileAssignment(ctx context.Context, session int32, callsign string) error {
	existing, err := s.assignments.GetAssignment(ctx, session, callsign)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	if existing.Manual || existing.Stage != StageEstimated || existing.ExpiresAt != nil || existing.ObservedStand != nil {
		return nil
	}
	return s.allocations.ReleaseAssignment(ctx, existing)
}

// ArrivalProcessingPriority mirrors the stage calculation used by
// ProcessArrival so the reconciler can apply hard arrival occupancy before
// lower-stage reservations from the same feed generation.
func (s *ArrivalLifecycleService) ArrivalProcessingPriority(strip *models.Strip, flight vatsim.ArrivalFlightInfo, assignment *models.StandAssignment) int {
	if strip == nil {
		return 0
	}
	stage := determineArrivalTargetStage(
		arrivalETATime(strip), s.now(), arrivalAltitude(strip),
		s.arrivalIsAtAirport(strip, flight.Destination),
		s.arrivalIsNearAirport(strip, flight.Destination),
	)
	priority := arrivalStagePriority(stage)
	if assignment != nil {
		priority = max(priority, arrivalStagePriority(assignment.Stage))
	}
	return priority
}

func arrivalStagePriority(stage string) int {
	switch stage {
	case StageConfirmed:
		return 3
	case StageAssigned:
		return 2
	case StageEstimated:
		return 1
	default:
		return 0
	}
}

// CancelArrival releases an automatic arrival reservation that disappeared
// before airport-area detection. Arrived assignments have a retention deadline;
// manual assignments remain controller-owned.
func (s *ArrivalLifecycleService) CancelArrival(ctx context.Context, session int32, callsign string) error {
	existing, err := s.assignments.GetAssignment(ctx, session, callsign)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	if existing == nil || existing.Manual || existing.ExpiresAt != nil ||
		existing.Direction != string(sat.AssignmentDirectionArrival) || !isArrivalStage(existing.Stage) {
		return nil
	}
	return s.allocations.ReleaseAssignment(ctx, existing)
}

func (s *ArrivalLifecycleService) observedParkedArrivalStand(strip *models.Strip, flightDestination string) (string, bool) {
	if s == nil {
		return "", false
	}
	return parkedArrivalStandAtPosition(s.stands, strip, flightDestination)
}

func parkedArrivalStandAtPosition(stands *sat.StandCapabilityRegistry, strip *models.Strip, flightDestination string) (string, bool) {
	if stands == nil || strip == nil || strip.State == nil || !strings.EqualFold(strings.TrimSpace(*strip.State), euroscope.GroundStateParked) {
		return "", false
	}
	if strip.PositionLatitude == nil || strip.PositionLongitude == nil || !validArrivalPosition(*strip.PositionLatitude, *strip.PositionLongitude) {
		return "", false
	}
	airport := strings.ToUpper(strings.TrimSpace(flightDestination))
	if airport == "" {
		airport = strings.ToUpper(strings.TrimSpace(strip.Destination))
	}
	stand, found := stands.StandAtPosition(airport, *strip.PositionLatitude, *strip.PositionLongitude)
	if !found {
		return "", false
	}
	return stand.Name, true
}

func (s *ArrivalLifecycleService) arrivalIsAtAirport(strip *models.Strip, flightDestination string) bool {
	if strip == nil || strip.PositionAltitude == nil || *strip.PositionAltitude > arrivalAirportMaxAltitude {
		return false
	}
	return s.arrivalIsNearAirport(strip, flightDestination)
}

// arrivalIsNearAirport gates altitude-driven lifecycle promotion. A low
// altitude at the departure airport must not turn a future inbound into a hard
// EKCH stand reservation before it has even departed.
func (s *ArrivalLifecycleService) arrivalIsNearAirport(strip *models.Strip, flightDestination string) bool {
	if s == nil || s.stands == nil || strip == nil || strip.PositionLatitude == nil || strip.PositionLongitude == nil {
		return false
	}
	if !validArrivalPosition(*strip.PositionLatitude, *strip.PositionLongitude) {
		return false
	}
	airport := strings.ToUpper(strings.TrimSpace(flightDestination))
	if airport == "" {
		airport = strings.ToUpper(strings.TrimSpace(strip.Destination))
	}
	if airport == "" {
		return false
	}
	if _, found := s.stands.StandAtPosition(airport, *strip.PositionLatitude, *strip.PositionLongitude); found {
		return true
	}
	return s.stands.PositionNearAirport(airport, *strip.PositionLatitude, *strip.PositionLongitude, arrivalAirportProximity)
}

func (s *ArrivalLifecycleService) ensureAssignment(ctx context.Context, session int32, strip *models.Strip, flight vatsim.ArrivalFlightInfo, eta *time.Time, stage string, expiresAt *time.Time) error {
	request := s.buildRequest(session, strip, flight, stage, eta, expiresAt)
	_, err := s.allocations.Allocate(ctx, request)
	return err
}

func (s *ArrivalLifecycleService) promoteArrival(ctx context.Context, session int32, strip *models.Strip, flight vatsim.ArrivalFlightInfo, existing *models.StandAssignment, eta *time.Time, targetStage string) error {
	request := s.buildRequest(session, strip, flight, targetStage, eta, nil)
	// Controller choices advance lifecycle metadata but are never moved by
	// automatic availability or tier optimization. Physical-position
	// reconciliation remains the sole automatic exception.
	if existing.Manual {
		return s.updateArrivalInPlace(ctx, existing, targetStage, eta, request.ETASource, existing.ExpiresAt)
	}
	available, err := s.allocations.StandAvailable(ctx, request, existing.Stand)
	if err != nil {
		return err
	}
	if !available {
		// A later-stage arrival may take over an overlapping ESTIMATED direct or
		// neighbouring reservation. Prefer the current stand so resolving that
		// soft conflict cannot swap a valid Primary assignment for an equivalent
		// Primary stand.
		request.Stand = existing.Stand
		result, reallocateErr := s.allocations.Reallocate(ctx, request)
		if reallocateErr == nil {
			s.recordArrivalStagePromotion(ctx, existing, result.Assignment)
		}
		err = reallocateErr
		return err
	}
	// Automatic Primary assignments are stable across the boundary.
	if existing.Tier == nil || *existing.Tier <= 1 {
		return s.updateArrivalInPlace(ctx, existing, targetStage, eta, request.ETASource, existing.ExpiresAt)
	}

	// A promotion is the only time a valid automatic assignment may move for
	// preference alone. Constrain the allocator to Primary so a failed retry
	// cannot churn between Secondary/Tertiary or equivalent stands. ESTIMATED
	// reservations may yield; later stages remain protected.
	request.ImproveTierBelow = 2
	request.DisplaceArrivalStages = []string{StageEstimated}
	result, reallocateErr := s.allocations.Reallocate(ctx, request)
	if reallocateErr == nil {
		s.recordArrivalStagePromotion(ctx, existing, result.Assignment)
		slog.InfoContext(ctx, "SAT assignment tier improved at stage promotion",
			slog.String("callsign", existing.Callsign), slog.String("from_stand", existing.Stand),
			slog.String("to_stand", result.Assignment.Stand), slog.String("from_stage", existing.Stage),
			slog.String("to_stage", targetStage), slog.Int("from_tier", int(*existing.Tier)),
			slog.Int("to_tier", int(valueInt32(result.Assignment.Tier))))
		metrics.RecordSATLifecycleEvent(ctx, "tier_improvement", "stage_promotion", result.Assignment.Source)
		return nil
	}
	if errors.Is(reallocateErr, ErrNoTierImprovement) || errors.Is(reallocateErr, ErrNoAvailableStand) ||
		errors.Is(reallocateErr, ErrNoPolicyStand) || errors.Is(reallocateErr, ErrAutomaticAllocationSuppressed) {
		return s.updateArrivalInPlace(ctx, existing, targetStage, eta, request.ETASource, existing.ExpiresAt)
	}
	return reallocateErr
}

func (s *ArrivalLifecycleService) recordArrivalStagePromotion(ctx context.Context, existing *models.StandAssignment, promoted models.StandAssignment) {
	if existing == nil || existing.Stage == promoted.Stage {
		return
	}
	slog.InfoContext(ctx, "SAT assignment stage changed",
		slog.String("callsign", existing.Callsign), slog.String("stand", promoted.Stand),
		slog.String("from_stage", existing.Stage), slog.String("to_stage", promoted.Stage))
	metrics.RecordSATLifecycleEvent(ctx, "stage_promotion", existing.Stage+"_to_"+promoted.Stage, promoted.Source)
}

func valueInt32(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

func (s *ArrivalLifecycleService) reallocateIfBlocked(ctx context.Context, session int32, strip *models.Strip, flight vatsim.ArrivalFlightInfo, existing *models.StandAssignment, eta *time.Time, targetStage string) error {
	request := s.buildRequest(session, strip, flight, existing.Stage, eta, nil)
	available, err := s.allocations.StandAvailable(ctx, request, existing.Stand)
	if err != nil {
		return err
	}
	if available {
		if arrivalTimingChanged(existing, eta, request.ETASource) {
			return s.updateArrivalInPlace(ctx, existing, existing.Stage, eta, request.ETASource, existing.ExpiresAt)
		}
		return nil
	}
	_, err = s.allocations.Reallocate(ctx, request)
	return err
}

func (s *ArrivalLifecycleService) updateArrivalInPlace(ctx context.Context, existing *models.StandAssignment, stage string, eta *time.Time, etaSource *string, expiresAt *time.Time) error {
	updated := *existing
	now := s.now().UTC()
	updated.Stage = stage
	updated.ETA = eta
	updated.ETASource = etaSource
	updated.ExpiresAt = expiresAt
	updated.AssignedAt = &now
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("arrival stage update version conflict for %s", existing.Callsign)
	}
	updated.Version++
	if existing.Stage != stage {
		slog.InfoContext(ctx, "SAT assignment stage changed", slog.String("callsign", existing.Callsign), slog.String("stand", existing.Stand), slog.String("from_stage", existing.Stage), slog.String("to_stage", stage))
		metrics.RecordSATLifecycleEvent(ctx, "stage_promotion", existing.Stage+"_to_"+stage, existing.Source)
	}
	if stage == StageConfirmed && existing.Stage != StageConfirmed {
		return s.allocations.PublishConfirmedArrival(ctx, updated)
	}
	return s.allocations.PublishAssignment(ctx, updated)
}

func (s *ArrivalLifecycleService) updateArrivalAtAirport(ctx context.Context, existing *models.StandAssignment, stage string, expiresAt *time.Time, observedStand *string) error {
	updated := *existing
	stageChanged := existing.Stage != stage
	updated.Stage = stage
	updated.ExpiresAt = expiresAt
	updated.ObservedStand = observedStand
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("arrival airport-area update version conflict for %s", existing.Callsign)
	}
	updated.Version++
	if !stageChanged {
		// Keep controller clients on the current optimistic-concurrency version.
		// PublishAssignment broadcasts assignment metadata without requesting a
		// stand write in EuroScope.
		return s.allocations.PublishAssignment(ctx, updated)
	}
	slog.InfoContext(ctx, "SAT assignment stage changed", slog.String("callsign", existing.Callsign), slog.String("stand", existing.Stand), slog.String("from_stage", existing.Stage), slog.String("to_stage", stage))
	metrics.RecordSATLifecycleEvent(ctx, "stage_promotion", existing.Stage+"_to_"+stage, existing.Source)
	if stage == StageConfirmed {
		return s.allocations.PublishConfirmedArrival(ctx, updated)
	}
	return s.allocations.PublishAssignment(ctx, updated)
}

func (s *ArrivalLifecycleService) ReleaseExpired(ctx context.Context) error {
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
			slog.Warn("arrival sweep cannot list session assignments",
				slog.Int("sessionID", int(session.ID)),
				slog.Any("error", err))
			continue
		}
		for _, assignment := range assignments {
			if assignment == nil {
				continue
			}
			if !isArrivalStage(assignment.Stage) {
				continue
			}
			if err := s.releaseIfDue(ctx, session.ID, assignment, now); err != nil {
				slog.Warn("arrival sweep failed to release assignment",
					slog.String("callsign", assignment.Callsign),
					slog.Any("error", err))
			}
		}
		if err := s.allocations.ReconcileUnsafeAssignments(ctx, session.ID, session.Airport); err != nil {
			slog.Warn("arrival sweep failed to reconcile unsafe stand overlaps",
				slog.Int("sessionID", int(session.ID)),
				slog.Any("error", err))
		}
	}
	return nil
}

func (s *ArrivalLifecycleService) releaseIfDue(ctx context.Context, session int32, assignment *models.StandAssignment, now time.Time) error {
	strip, err := s.strips.GetByCallsign(ctx, session, assignment.Callsign)
	if err != nil && !isNotFound(err) {
		return err
	}
	if strip == nil {
		err := s.allocations.ReleaseAssignment(ctx, assignment)
		recordSATExpiry(ctx, assignment, "strip_removed", err)
		return err
	}
	if assignment.ExpiresAt != nil && !assignment.ExpiresAt.After(now) {
		err = s.allocations.ReleaseAssignment(ctx, assignment)
		recordSATExpiry(ctx, assignment, "expired", err)
		return err
	}
	return nil
}

func recordSATExpiry(ctx context.Context, assignment *models.StandAssignment, reason string, err error) {
	if err != nil || assignment == nil {
		return
	}
	metrics.RecordSATExpiration(ctx, assignment.Direction, assignment.Stage)
	slog.InfoContext(ctx, "SAT assignment expired", slog.String("callsign", assignment.Callsign), slog.String("stand", assignment.Stand), slog.String("stage", assignment.Stage), slog.String("reason", reason))
}

func (s *ArrivalLifecycleService) StartSweep(ctx context.Context) {
	ticker := time.NewTicker(s.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ReleaseExpired(ctx); err != nil {
				slog.Warn("Arrival lifecycle sweep failed", slog.Any("error", err))
			}
		}
	}
}

func (s *ArrivalLifecycleService) buildRequest(session int32, strip *models.Strip, flight vatsim.ArrivalFlightInfo, stage string, eta *time.Time, expiresAt *time.Time) StandAllocationRequest {
	facts, assignmentFacts := s.resolveFacts(strip, flight)
	vatsimCID, vatsimRevision := resolvedVatsimIdentity(strip, flight.CID, flight.Revision)
	return StandAllocationRequest{
		SessionID:       session,
		Callsign:        strip.Callsign,
		Airport:         facts.Destination,
		Direction:       sat.AssignmentDirectionArrival,
		Stage:           stage,
		FlightFacts:     facts,
		AssignmentFacts: assignmentFacts,
		ETA:             eta,
		ETASource:       arrivalETASource(strip, eta),
		ExpiresAt:       expiresAt,
		VatsimCID:       vatsimCID,
		VatsimRevision:  vatsimRevision,
	}
}

func (s *ArrivalLifecycleService) resolveFacts(strip *models.Strip, flight vatsim.ArrivalFlightInfo) (sat.FlightCompatibilityFacts, sat.AssignmentFlightFacts) {
	aircraftType := flight.AircraftType
	origin, destination := flight.Origin, flight.Destination
	if strip != nil {
		if strip.AircraftType != nil && strings.TrimSpace(*strip.AircraftType) != "" {
			aircraftType = *strip.AircraftType
		}
		if strings.TrimSpace(strip.Origin) != "" {
			origin = strip.Origin
		}
		if strings.TrimSpace(destination) == "" && strings.TrimSpace(strip.Destination) != "" {
			destination = strip.Destination
		}
	}
	input := sat.FlightCompatibilityInput{
		Direction:      sat.Arrival,
		Origin:         origin,
		Destination:    destination,
		AircraftType:   aircraftType,
		LiveEngineType: engineTypeValue(strip),
	}
	facts := sat.ResolveFlightCompatibilityFacts(input, s.aircraft, s.engines, s.borders)
	// A freshly created EuroScope session can briefly contain a strip whose
	// source fields are incomplete or unrecognized while the same VATSIM flight
	// already has usable facts. Do not let that transient strip state downgrade
	// a known Heavy arrival to the unrestricted "unknown" fallback policy.
	retryWithFlightFacts := false
	if !facts.AircraftKnown && strings.TrimSpace(flight.AircraftType) != "" &&
		!strings.EqualFold(strings.TrimSpace(input.AircraftType), strings.TrimSpace(flight.AircraftType)) {
		input.AircraftType = flight.AircraftType
		retryWithFlightFacts = true
	}
	if facts.BorderStatus == sat.BorderStatusUnknown && strings.TrimSpace(flight.Origin) != "" &&
		!strings.EqualFold(strings.TrimSpace(input.Origin), strings.TrimSpace(flight.Origin)) {
		input.Origin = flight.Origin
		retryWithFlightFacts = true
	}
	if strings.TrimSpace(input.Destination) == "" && strings.TrimSpace(flight.Destination) != "" {
		input.Destination = flight.Destination
		retryWithFlightFacts = true
	}
	if retryWithFlightFacts {
		facts = sat.ResolveFlightCompatibilityFacts(input, s.aircraft, s.engines, s.borders)
	}
	assignmentFacts := sat.AssignmentFlightFacts{
		Callsign:     strip.Callsign,
		AircraftType: input.AircraftType,
		AircraftUse:  facts.Aircraft.UseCode,
		BorderStatus: facts.BorderStatus,
		Direction:    sat.AssignmentDirectionArrival,
	}
	return facts, assignmentFacts
}

func determineArrivalTargetStage(eta *time.Time, now time.Time, altitude *int32, atAirport, nearAirport bool) string {
	confirmedByTime := eta != nil && eta.Sub(now) <= defaultConfirmedBefore
	if atAirport || confirmedByTime || (nearAirport && altitudeBelow(altitude, confirmedAltitudeThreshold)) {
		return StageConfirmed
	}
	assignedByTime := eta != nil && eta.Sub(now) <= defaultAssignedBefore
	if assignedByTime || (nearAirport && altitudeBelow(altitude, assignedAltitudeThreshold)) {
		return StageAssigned
	}
	return StageEstimated
}

func arrivalETASource(strip *models.Strip, eta *time.Time) *string {
	if eta == nil {
		return nil
	}
	if strip != nil && strip.ArrivalETA != nil && strings.TrimSpace(strip.ArrivalETA.Source) != "" {
		source := strings.TrimSpace(strip.ArrivalETA.Source)
		return &source
	}
	source := "ARRIVAL_ETA"
	return &source
}

func arrivalTimingChanged(existing *models.StandAssignment, eta *time.Time, etaSource *string) bool {
	if existing == nil {
		return false
	}
	if (existing.ETA == nil) != (eta == nil) || (existing.ETASource == nil) != (etaSource == nil) {
		return true
	}
	if existing.ETA != nil && !existing.ETA.Equal(*eta) {
		return true
	}
	return existing.ETASource != nil && *existing.ETASource != *etaSource
}

func arrivalStandExpiresAt(strip *models.Strip, current *time.Time, now time.Time) *time.Time {
	if strip != nil {
		if aldt := strip.EffectiveAldt(); aldt != nil {
			if landedAt, ok := parseArrivalLandingClockUTC(*aldt, now); ok {
				expiresAt := landedAt.Add(arrivalStandRetention)
				return &expiresAt
			}
		}
	}
	if current != nil && current.After(now) && current.Sub(now) > arrivalRetentionRefreshAt {
		return current
	}
	// Without ALDT, keep a sliding deadline while live airport-area updates
	// continue. Refresh only near expiry so ordinary feed polls do not cause a
	// database write and full assignment publication every time.
	expiresAt := now.UTC().Add(arrivalStandRetention)
	return &expiresAt
}

func shouldAdoptObservedArrivalStand(existing *models.StandAssignment, observedStand string) bool {
	if existing == nil || strings.EqualFold(existing.Stand, observedStand) {
		return false
	}
	if !existing.Manual {
		return true
	}
	if existing.ObservedStand == nil {
		return true
	}
	return strings.TrimSpace(*existing.ObservedStand) != "" && !strings.EqualFold(*existing.ObservedStand, observedStand)
}

func arrivalObservedStandBaselinePending(existing *models.StandAssignment) bool {
	return existing != nil && existing.Manual && existing.ObservedStand != nil && strings.TrimSpace(*existing.ObservedStand) == ""
}

func parseArrivalLandingClockUTC(value string, now time.Time) (time.Time, bool) {
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
	now = now.UTC()
	landedAt := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, second, 0, time.UTC)
	if landedAt.After(now) {
		landedAt = landedAt.Add(-24 * time.Hour)
	}
	return landedAt, true
}

func arrivalExpiryChanged(current, next *time.Time) bool {
	if (current == nil) != (next == nil) {
		return true
	}
	return current != nil && !current.Equal(*next)
}

func stringPointerChanged(current, next *string) bool {
	if (current == nil) != (next == nil) {
		return true
	}
	return current != nil && !strings.EqualFold(*current, *next)
}

func shouldPromoteArrival(currentStage, targetStage string) bool {
	order := map[string]int{StageEstimated: 1, StageAssigned: 2, StageConfirmed: 3}
	currentOrder := order[currentStage]
	targetOrder := order[targetStage]
	if currentOrder == 0 || targetOrder == 0 {
		return false
	}
	return targetOrder > currentOrder
}

func arrivalETATime(strip *models.Strip) *time.Time {
	if strip == nil || strip.ArrivalETA == nil || strip.ArrivalETA.Time.IsZero() {
		return nil
	}
	return &strip.ArrivalETA.Time
}

func arrivalAltitude(strip *models.Strip) *int32 {
	if strip == nil || strip.PositionAltitude == nil ||
		strip.PositionLatitude == nil || strip.PositionLongitude == nil ||
		!validArrivalPosition(*strip.PositionLatitude, *strip.PositionLongitude) {
		return nil
	}
	return strip.PositionAltitude
}

func validArrivalPosition(latitude, longitude float64) bool {
	return latitude >= -90 && latitude <= 90 &&
		longitude >= -180 && longitude <= 180 &&
		(latitude != 0 || longitude != 0)
}

func altitudeBelow(altitude *int32, threshold int32) bool {
	if altitude == nil {
		return false
	}
	return *altitude <= threshold
}

func isArrivalStage(stage string) bool {
	return stage == StageEstimated || stage == StageAssigned || stage == StageConfirmed
}
