package services

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"
	"context"
	"fmt"
	"log/slog"
	"strconv"
)

// CreateManualFPL processes a create_manual_fpl action from the frontend.
// It validates that the callsign exists in the session, updates the strip with
// the provided IFR flight-plan fields, marks it as manually created, moves it
// to the correct uncleared bay, and notifies both the frontend and EuroScope.
func (s *StripService) CreateManualFPL(ctx context.Context, session int32, req frontend.CreateManualFPLAction, cid string, airport string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, req.Callsign)
	if err != nil {
		return fmt.Errorf("callsign %q not found in session: %w", req.Callsign, err)
	}

	// For aircraft without an existing FPL the origin field is empty; use the session airport.
	origin := strip.Origin
	if origin == "" {
		origin = airport
	}

	// Convert optional string fields to pointers (empty string → nil = keep existing).
	sid := nilIfEmpty(req.SID)
	ssr := nilIfEmpty(req.SSR)
	eobt := nilIfEmpty(req.EOBT)
	aircraftType := nilIfEmpty(req.AircraftType)
	route := nilIfEmpty(req.Route)
	stand := nilIfEmpty(req.Stand)
	runway := nilIfEmpty(req.RwyDep)

	// Parse FL string (e.g. "330") into an altitude integer (33000).
	var requestedAltitude *int32
	if req.FL != "" {
		if fl, err := strconv.Atoi(req.FL); err == nil {
			alt := int32(fl * 100)
			requestedAltitude = &alt
		}
	}

	affected, err := s.stripRepo.UpdateIFRManualFPLFields(ctx, session, req.Callsign, req.ADES, sid, ssr, eobt, aircraftType, requestedAltitude, route, stand, runway)
	if err != nil {
		return fmt.Errorf("UpdateIFRManualFPLFields: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("no strip updated for callsign %q", req.Callsign)
	}

	// Determine the target bay from the callsign prefix using existing bay-routing logic.
	targetBay := routingBayForCallsign(req.Callsign)

	// Move to bay (sends bay event).
	if err := s.MoveToBay(ctx, session, req.Callsign, targetBay, true); err != nil {
		return fmt.Errorf("MoveToBay: %w", err)
	}

	if err := s.recalculateRouteForStrip(session, req.Callsign); err != nil {
		slog.Error("Error updating route after manual FPL update", slog.String("callsign", req.Callsign), slog.Any("error", err))
	}

	// Broadcast full strip update to all frontend clients.
	s.frontendHub.SendStripUpdate(session, req.Callsign)

	// Notify EuroScope so it can create the FPL in its session.
	s.euroscopeHub.SendCreateFPL(session, cid, euroscope.CreateFPLEvent{
		Callsign:          req.Callsign,
		Origin:            origin,
		Destination:       req.ADES,
		Sid:               req.SID,
		AssignedSquawk:    req.SSR,
		Eobt:              req.EOBT,
		AircraftType:      req.AircraftType,
		RequestedAltitude: int32Value(requestedAltitude),
		Route:             req.Route,
		Stand:             req.Stand,
		Runway:            req.RwyDep,
	})

	return nil
}

// CreateVFRFPL processes a create_vfr_fpl action from the frontend.
// It validates that the callsign exists, updates VFR-specific fields, moves the
// strip to the CONTROLZONE bay, and notifies both the frontend and EuroScope.
func (s *StripService) CreateVFRFPL(ctx context.Context, session int32, req frontend.CreateVFRFPLAction, cid string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, req.Callsign)
	if err != nil {
		return fmt.Errorf("callsign %q not found in session: %w", req.Callsign, err)
	}

	ssr := req.SSR
	if ssr == "" {
		ssr = "7000"
	}

	aircraftType := nilIfEmpty(req.AircraftType)
	fplType := nilIfEmpty(req.FPLType)
	language := nilIfEmpty(req.Language)
	remarks := nilIfEmpty(req.Remarks)

	var personsOnBoard *int32
	if req.PersonsOnBoard > 0 {
		pob := int32(req.PersonsOnBoard)
		personsOnBoard = &pob
	}

	affected, err := s.stripRepo.UpdateVFRManualFPLFields(ctx, session, req.Callsign, aircraftType, personsOnBoard, ssr, fplType, language, remarks, shared.BAY_CONTROLZONE)
	if err != nil {
		return fmt.Errorf("UpdateVFRManualFPLFields: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("no strip updated for callsign %q", req.Callsign)
	}

	// Broadcast full strip update to all frontend clients.
	s.frontendHub.SendStripUpdate(session, req.Callsign)

	// Notify EuroScope.
	s.euroscopeHub.SendCreateFPL(session, cid, euroscope.CreateFPLEvent{
		Callsign:       req.Callsign,
		Origin:         strip.Origin,
		AircraftType:   req.AircraftType,
		AssignedSquawk: ssr,
		Remarks:        req.Remarks,
		FplType:        req.FPLType,
		Language:       req.Language,
		PersonsOnBoard: req.PersonsOnBoard,
	})

	return nil
}

// routingBayForCallsign returns the NOT_CLEARED bay for a callsign.
// Further sub-bay routing (SAS/NORWEGIAN/OTHERS) is handled downstream when
// the strip reaches the CLRDEL position.
func routingBayForCallsign(_ string) string {
	return shared.BAY_NOT_CLEARED
}

// nilIfEmpty converts an empty string to nil, otherwise returns a pointer to the string.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// int32Value safely dereferences an *int32, returning 0 for nil.
func int32Value(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}
