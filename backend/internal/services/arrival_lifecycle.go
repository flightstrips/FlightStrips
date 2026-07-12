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
	if allocations == nil || assignments == nil || strips == nil || stands == nil {
		return nil, errors.New("arrival lifecycle requires allocation service, repositories, and stand registry")
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
	return service, nil
}

func (s *ArrivalLifecycleService) ProcessArrival(ctx context.Context, session int32, strip *models.Strip, flight vatsim.ArrivalFlightInfo) error {
	if strip == nil || strings.TrimSpace(strip.Callsign) == "" {
		return nil
	}
	eta := arrivalETATime(strip)
	if eta == nil {
		return nil
	}
	now := s.now()
	timeUntilETA := eta.Sub(now)
	altitude := arrivalAltitude(strip)
	targetStage := determineArrivalTargetStage(timeUntilETA, altitude)
	if targetStage == "" {
		return nil
	}
	existing, err := s.assignments.GetAssignment(ctx, session, strip.Callsign)
	if err != nil && !isNotFound(err) {
		return err
	}
	if existing == nil {
		return s.ensureAssignment(ctx, session, strip, flight, eta, targetStage)
	}
	currentStage := existing.Stage
	if !isArrivalStage(currentStage) {
		return nil
	}
	if !shouldPromoteArrival(currentStage, targetStage) {
		if err := s.reallocateIfBlocked(ctx, session, strip, flight, existing, eta, targetStage); err != nil {
			return err
		}
		return nil
	}
	return s.promoteArrival(ctx, session, strip, flight, existing, eta, targetStage)
}

func (s *ArrivalLifecycleService) ensureAssignment(ctx context.Context, session int32, strip *models.Strip, flight vatsim.ArrivalFlightInfo, eta *time.Time, stage string) error {
	request := s.buildRequest(session, strip, flight, stage, eta, nil)
	_, err := s.allocations.Allocate(ctx, request)
	return err
}

func (s *ArrivalLifecycleService) promoteArrival(ctx context.Context, session int32, strip *models.Strip, flight vatsim.ArrivalFlightInfo, existing *models.StandAssignment, eta *time.Time, targetStage string) error {
	request := s.buildRequest(session, strip, flight, targetStage, eta, nil)
	request.DisplaceStage = StageEstimated
	available, err := s.allocations.StandAvailable(ctx, request, existing.Stand)
	if err != nil {
		return err
	}
	if available && s.currentStandIsOptimal(ctx, request, existing) {
		return s.updateStageInPlace(ctx, existing, targetStage, eta)
	}
	_, err = s.allocations.Reallocate(ctx, request)
	return err
}

func (s *ArrivalLifecycleService) reallocateIfBlocked(ctx context.Context, session int32, strip *models.Strip, flight vatsim.ArrivalFlightInfo, existing *models.StandAssignment, eta *time.Time, targetStage string) error {
	request := s.buildRequest(session, strip, flight, existing.Stage, eta, nil)
	request.DisplaceStage = StageEstimated
	available, err := s.allocations.StandAvailable(ctx, request, existing.Stand)
	if err != nil {
		return err
	}
	if available {
		return nil
	}
	if s.blockedByPastDeparture(ctx, session, strip, existing, eta) {
		return nil
	}
	_, err = s.allocations.Reallocate(ctx, request)
	return err
}

func (s *ArrivalLifecycleService) blockedByPastDeparture(ctx context.Context, session int32, strip *models.Strip, existing *models.StandAssignment, eta *time.Time) bool {
	if eta == nil {
		return false
	}
	assignments, err := s.assignments.ListAssignments(ctx, session)
	if err != nil {
		return false
	}
	stand := standName(existing.Stand)
	for _, assignment := range assignments {
		if assignment == nil || strings.EqualFold(assignment.Callsign, strip.Callsign) {
			continue
		}
		if assignment.Direction != string(sat.AssignmentDirectionDeparture) {
			continue
		}
		if standName(assignment.Stand) != stand {
			continue
		}
		if assignment.ExpiresAt != nil && !assignment.ExpiresAt.After(*eta) {
			return true
		}
	}
	return false
}

func (s *ArrivalLifecycleService) currentStandIsOptimal(ctx context.Context, request StandAllocationRequest, existing *models.StandAssignment) bool {
	if existing.Tier == nil {
		return false
	}
	evaluation := s.stands.EvaluateCompatibility(request.Airport, request.FlightFacts)
	available := make([]string, 0)
	for _, match := range evaluation.Matches {
		candidate := standName(match.Stand.Name)
		free, err := s.allocations.StandAvailable(ctx, request, candidate)
		if err != nil {
			return false
		}
		if free {
			available = append(available, candidate)
		}
	}
	selection, err := s.allocations.policy.SelectStand(request.AssignmentFacts, available, s.allocations.random)
	if err != nil || selection == nil {
		return true
	}
	if standName(selection.Stand) == standName(existing.Stand) {
		return true
	}
	if existing.RuleID == nil || !strings.EqualFold(*existing.RuleID, selection.RuleID) {
		return false
	}
	return selection.Tier >= int(*existing.Tier)
}

func (s *ArrivalLifecycleService) updateStageInPlace(ctx context.Context, existing *models.StandAssignment, stage string, eta *time.Time) error {
	updated := *existing
	now := s.now().UTC()
	updated.Stage = stage
	updated.ETA = eta
	updated.AssignedAt = &now
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("arrival stage update version conflict for %s", existing.Callsign)
	}
	return nil
}

func (s *ArrivalLifecycleService) ReleaseExpired(ctx context.Context) error {
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
	}
	return nil
}

func (s *ArrivalLifecycleService) releaseIfDue(ctx context.Context, session int32, assignment *models.StandAssignment, now time.Time) error {
	strip, err := s.strips.GetByCallsign(ctx, session, assignment.Callsign)
	if err != nil && !isNotFound(err) {
		return err
	}
	if strip == nil {
		_, err := s.assignments.DeleteAssignment(ctx, session, assignment.ID, assignment.Version)
		return err
	}
	if assignment.ExpiresAt != nil && !assignment.ExpiresAt.After(now) {
		if _, err := s.strips.UpdateStand(ctx, session, assignment.Callsign, nil, nil); err != nil {
			return err
		}
		_, err = s.assignments.DeleteAssignment(ctx, session, assignment.ID, assignment.Version)
		return err
	}
	if assignment.ETA != nil && now.After(assignment.ETA.Add(30*time.Minute)) {
		if _, err := s.strips.UpdateStand(ctx, session, assignment.Callsign, nil, nil); err != nil {
			return err
		}
		_, err = s.assignments.DeleteAssignment(ctx, session, assignment.ID, assignment.Version)
		return err
	}
	return nil
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
	revision := flight.Revision
	etaSource := "ARRIVAL_ETA"
	return StandAllocationRequest{
		SessionID:       session,
		Callsign:        strip.Callsign,
		Airport:         strings.ToUpper(strings.TrimSpace(strip.Destination)),
		Direction:       sat.AssignmentDirectionArrival,
		Stage:           stage,
		FlightFacts:     facts,
		AssignmentFacts: assignmentFacts,
		ETA:             eta,
		ETASource:       &etaSource,
		ExpiresAt:       expiresAt,
		VatsimCID:       parseCID(flight.CID),
		VatsimRevision:  &revision,
	}
}

func (s *ArrivalLifecycleService) resolveFacts(strip *models.Strip, flight vatsim.ArrivalFlightInfo) (sat.FlightCompatibilityFacts, sat.AssignmentFlightFacts) {
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
		Direction:      sat.Arrival,
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
		Direction:    sat.AssignmentDirectionArrival,
	}
	return facts, assignmentFacts
}

func determineArrivalTargetStage(timeUntilETA time.Duration, altitude *int32) string {
	if timeUntilETA <= defaultConfirmedBefore || altitudeBelow(altitude, confirmedAltitudeThreshold) {
		return StageConfirmed
	}
	if timeUntilETA <= defaultAssignedBefore || altitudeBelow(altitude, assignedAltitudeThreshold) {
		return StageAssigned
	}
	if timeUntilETA <= defaultEstimatedBefore {
		return StageEstimated
	}
	return ""
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
	if strip == nil || strip.PositionAltitude == nil {
		return nil
	}
	return strip.PositionAltitude
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
