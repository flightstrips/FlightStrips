package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/sat"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrStandActionUnauthorized = errors.New("stand action requires an authenticated controller position")
	ErrStandActionStaleVersion = errors.New("stand assignment version is stale")
	ErrStandBlockNotOwned      = errors.New("stand block is not owned by this controller")
)

// StandActionService translates authenticated controller intent into allocator
// commands. Compatibility, occupancy, override and transaction policy remain in
// StandAllocationService.
type StandActionService struct {
	allocations *StandAllocationService
	assignments repository.StandAssignmentRepository
	strips      repository.StripRepository
	aircraft    *sat.AircraftRegistry
	engines     *sat.AircraftEngineRegistry
	borders     *sat.AirportCountryRegistry
}

func NewStandActionService(allocations *StandAllocationService, assignments repository.StandAssignmentRepository, strips repository.StripRepository, aircraft *sat.AircraftRegistry, engines *sat.AircraftEngineRegistry, borders *sat.AirportCountryRegistry) *StandActionService {
	if allocations == nil || assignments == nil || strips == nil {
		return nil
	}
	return &StandActionService{allocations: allocations, assignments: assignments, strips: strips, aircraft: aircraft, engines: engines, borders: borders}
}

func (s *StandActionService) Allocate(ctx context.Context, session int32, airport, position, callsign string, version int32) (*StandAllocationResult, error) {
	req, err := s.request(ctx, session, airport, position, callsign, version)
	if err != nil {
		return nil, err
	}
	return s.allocations.Allocate(ctx, req)
}

func (s *StandActionService) AssignManually(ctx context.Context, session int32, airport, position, callsign, stand string, version int32) (*StandAllocationResult, error) {
	req, err := s.request(ctx, session, airport, position, callsign, version)
	if err != nil {
		return nil, err
	}
	req.Stand = stand
	return s.allocations.AssignManually(ctx, req)
}

func (s *StandActionService) Override(ctx context.Context, session int32, airport, position, callsign, stand, reason string, version int32) (*StandAllocationResult, error) {
	req, err := s.request(ctx, session, airport, position, callsign, version)
	if err != nil {
		return nil, err
	}
	req.Stand, req.ConflictReason = stand, strings.TrimSpace(reason)
	if req.ConflictReason == "" {
		return nil, errors.New("confirmed override requires a reason")
	}
	return s.allocations.OverrideManually(ctx, req)
}

func (s *StandActionService) request(ctx context.Context, session int32, airport, position, callsign string, version int32) (StandAllocationRequest, error) {
	if strings.TrimSpace(position) == "" {
		return StandAllocationRequest{}, ErrStandActionUnauthorized
	}
	strip, err := s.strips.GetByCallsign(ctx, session, strings.TrimSpace(callsign))
	if err != nil {
		return StandAllocationRequest{}, fmt.Errorf("load strip: %w", err)
	}
	existing, err := s.assignments.GetAssignment(ctx, session, strip.Callsign)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return StandAllocationRequest{}, fmt.Errorf("load stand assignment: %w", err)
		}
		if version != 0 {
			return StandAllocationRequest{}, ErrStandActionStaleVersion
		}
		existing = nil
	} else if existing != nil && existing.Version != version {
		return StandAllocationRequest{}, ErrStandActionStaleVersion
	}
	direction := sat.AssignmentDirectionArrival
	compatDirection := sat.Arrival
	if strings.EqualFold(strip.Origin, airport) {
		direction, compatDirection = sat.AssignmentDirectionDeparture, sat.Departure
	}
	facts := sat.ResolveFlightCompatibilityFacts(sat.FlightCompatibilityInput{Direction: compatDirection, Origin: strip.Origin, Destination: strip.Destination, AircraftType: valueString(strip.AircraftType), LiveEngineType: strip.EngineType}, s.aircraft, s.engines, s.borders)
	stage := StageConfirmed
	var expiresAt *time.Time
	if direction == sat.AssignmentDirectionDeparture {
		stage = StageReserved
		expires := time.Now().UTC().Add(defaultDepartureHoldDuration)
		expiresAt = &expires
	}
	if existing != nil && existing.Stage != "" {
		stage = existing.Stage
		expiresAt = existing.ExpiresAt
	}
	return StandAllocationRequest{SessionID: session, Callsign: strip.Callsign, Airport: strings.ToUpper(airport), Direction: direction, Stage: stage,
		FlightFacts: facts, AssignmentFacts: sat.AssignmentFlightFacts{Callsign: strip.Callsign, AircraftType: valueString(strip.AircraftType), AircraftUse: facts.Aircraft.UseCode, BorderStatus: facts.BorderStatus, Direction: direction}, ETA: arrivalETATime(strip), ETASource: existingETASource(existing), ExpiresAt: expiresAt, VatsimRevision: strip.VatsimRevision}, nil
}

func (s *StandActionService) Acknowledge(ctx context.Context, session int32, position, callsign string, version int32) (*models.StandAssignment, error) {
	if strings.TrimSpace(position) == "" {
		return nil, ErrStandActionUnauthorized
	}
	a, err := s.assignments.GetAssignment(ctx, session, callsign)
	if err != nil {
		return nil, err
	}
	if a.Version != version {
		return nil, ErrStandActionStaleVersion
	}
	now := time.Now().UTC()
	a.Acknowledged, a.AcknowledgedAt, a.AcknowledgedBy = true, &now, &position
	count, err := s.assignments.UpdateAssignment(ctx, a)
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, ErrStandActionStaleVersion
	}
	a.Version++
	return a, nil
}

func (s *StandActionService) CreateBlock(ctx context.Context, session int32, airport, position, stand, reason string) (*models.StandBlock, error) {
	if strings.TrimSpace(position) == "" {
		return nil, ErrStandActionUnauthorized
	}
	b := &models.StandBlock{SessionID: session, Stand: strings.ToUpper(strings.TrimSpace(stand)), BlockType: "MANUAL", Source: "CONTROLLER", Reason: &reason, CreatedBy: &position, Manual: true}
	if err := s.allocations.CreateManualBlock(ctx, airport, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *StandActionService) RemoveBlock(ctx context.Context, session int32, position string, id int64, version int32) (*models.StandBlock, error) {
	if strings.TrimSpace(position) == "" {
		return nil, ErrStandActionUnauthorized
	}
	b, err := s.assignments.GetBlock(ctx, session, id)
	if err != nil {
		return nil, err
	}
	if b.CreatedBy == nil || !strings.EqualFold(*b.CreatedBy, position) {
		return nil, ErrStandBlockNotOwned
	}
	if b.Version != version {
		return nil, ErrStandActionStaleVersion
	}
	count, err := s.allocations.DeleteManualBlock(ctx, session, id, version)
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, ErrStandActionStaleVersion
	}
	return b, nil
}

func valueString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func existingETASource(a *models.StandAssignment) *string {
	if a == nil {
		return nil
	}
	return a.ETASource
}
