package frontend

import (
	frontendEvents "FlightStrips/pkg/events/frontend"
	"context"
	"errors"
	"strings"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
)

type frontendStripUpdateUseCase interface {
	UpdateStrip(ctx context.Context, req FrontendStripUpdateRequest) error
}

type FrontendStripUpdateRequest struct {
	Session  int32
	Cid      string
	Position string
	Event    frontendEvents.UpdateStripDataEvent
}

type frontendStripUpdatePublisher interface {
	SendStripUpdate(session int32, callsign string)
}

type frontendStripUpdateStandUpdater interface {
	UpdateStand(ctx context.Context, session int32, callsign string, stand string) error
}

type frontendStripUpdateEuroscopeSender interface {
	SendRoute(session int32, cid string, callsign string, route string)
	SendAircraftInfoAndRemarks(session int32, cid string, callsign string, aircraftType string, remarks string)
	SendAircraftInfo(session int32, cid string, callsign string, aircraftType string)
	SendRemarks(session int32, cid string, callsign string, remarks string)
	SendSid(session int32, cid string, callsign string, sid string)
	SendStand(session int32, cid string, callsign string, stand string)
	SendRunway(session int32, cid string, callsign string, runway string)
	SendEobt(session int32, cid string, callsign string, eobt string)
	SendClearedAltitude(session int32, cid string, callsign string, altitude int32)
	SendHeading(session int32, cid string, callsign string, heading int32)
}

type FrontendStripUpdateService struct {
	stripRepo            FrontendStripUpdateStore
	sessionRepo          repository.SessionRepository
	euroscopeSender      frontendStripUpdateEuroscopeSender
	cdmService           shared.CdmService
	standUpdater         frontendStripUpdateStandUpdater
	pdcReevaluator       pdcInvalidValidationStripReevaluator
	departureReevaluator departureValidationStripReevaluator
	stripUpdatePublisher frontendStripUpdatePublisher
}

func NewFrontendStripUpdateService(
	stripRepo FrontendStripUpdateStore,
	sessionRepo repository.SessionRepository,
	euroscopeSender frontendStripUpdateEuroscopeSender,
	cdmService shared.CdmService,
	standUpdater frontendStripUpdateStandUpdater,
	pdcReevaluator pdcInvalidValidationStripReevaluator,
	departureReevaluator departureValidationStripReevaluator,
	stripUpdatePublisher frontendStripUpdatePublisher,
) *FrontendStripUpdateService {
	return &FrontendStripUpdateService{
		stripRepo:            stripRepo,
		sessionRepo:          sessionRepo,
		euroscopeSender:      euroscopeSender,
		cdmService:           cdmService,
		standUpdater:         standUpdater,
		pdcReevaluator:       pdcReevaluator,
		departureReevaluator: departureReevaluator,
		stripUpdatePublisher: stripUpdatePublisher,
	}
}

func (s *FrontendStripUpdateService) UpdateStrip(ctx context.Context, req FrontendStripUpdateRequest) error {
	if req.Event.Route != nil && req.Event.Sid != nil {
		return errors.New("cannot update both route and sid at the same time")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, req.Session, req.Event.Callsign)
	if err != nil {
		return err
	}

	isOwner := strip.Owner == nil || *strip.Owner == "" || *strip.Owner == req.Position
	if !isOwner {
		if req.Event.Sid != nil || req.Event.Route != nil || req.Event.Stand != nil || req.Event.Runway != nil || req.Event.Altitude != nil || req.Event.Heading != nil || req.Event.Remarks != nil || req.Event.Aircraft != nil {
			return errors.New("non-owner cannot modify strip fields")
		}
		return nil
	}

	if err := s.handleRouteUpdate(req, strip); err != nil {
		return err
	}
	if err := s.handleAircraftAndRemarksUpdate(req, strip); err != nil {
		return err
	}
	if err := s.handleSidUpdate(ctx, req, strip); err != nil {
		return err
	}
	if err := s.handleStandUpdate(ctx, req, strip); err != nil {
		return err
	}
	if err := s.handleRunwayUpdate(ctx, req, strip); err != nil {
		return err
	}
	if err := s.handleEobtUpdate(ctx, req, strip); err != nil {
		return err
	}
	if err := s.handleAltitudeUpdate(ctx, req, strip); err != nil {
		return err
	}
	if err := s.handleHeadingUpdate(ctx, req, strip); err != nil {
		return err
	}

	return nil
}

func (s *FrontendStripUpdateService) handleRouteUpdate(req FrontendStripUpdateRequest, strip *internalModels.Strip) error {
	if req.Event.Route == nil || stringPtrValue(strip.Route) == *req.Event.Route {
		return nil
	}

	s.euroscopeSender.SendRoute(req.Session, req.Cid, req.Event.Callsign, *req.Event.Route)
	return nil
}

func (s *FrontendStripUpdateService) handleAircraftAndRemarksUpdate(req FrontendStripUpdateRequest, strip *internalModels.Strip) error {
	aircraftChanged := req.Event.Aircraft != nil && stringPtrValue(strip.AircraftType) != *req.Event.Aircraft
	remarksChanged := req.Event.Remarks != nil && stringPtrValue(strip.Remarks) != *req.Event.Remarks

	switch {
	case aircraftChanged && remarksChanged:
		s.euroscopeSender.SendAircraftInfoAndRemarks(req.Session, req.Cid, req.Event.Callsign, *req.Event.Aircraft, *req.Event.Remarks)
	case aircraftChanged:
		s.euroscopeSender.SendAircraftInfo(req.Session, req.Cid, req.Event.Callsign, *req.Event.Aircraft)
	case remarksChanged:
		s.euroscopeSender.SendRemarks(req.Session, req.Cid, req.Event.Callsign, *req.Event.Remarks)
	}

	return nil
}

func (s *FrontendStripUpdateService) handleSidUpdate(ctx context.Context, req FrontendStripUpdateRequest, strip *internalModels.Strip) error {
	if req.Event.Sid == nil || stringPtrValue(strip.Sid) == *req.Event.Sid {
		return nil
	}

	s.euroscopeSender.SendSid(req.Session, req.Cid, req.Event.Callsign, *req.Event.Sid)
	if err := s.stripRepo.AppendControllerModifiedField(ctx, req.Session, req.Event.Callsign, "sid"); err != nil {
		return err
	}
	if s.pdcReevaluator == nil {
		return nil
	}

	sessionData, err := s.sessionRepo.GetByID(ctx, req.Session)
	if err != nil {
		return err
	}
	updatedStrip := *strip
	updatedStrip.Sid = req.Event.Sid

	return s.pdcReevaluator.ReevaluatePdcInvalidValidationForStrip(ctx, req.Session, &updatedStrip, sessionData.ActiveRunways.DepartureRunways, true, false)
}

func (s *FrontendStripUpdateService) handleStandUpdate(ctx context.Context, req FrontendStripUpdateRequest, strip *internalModels.Strip) error {
	if req.Event.Stand == nil || stringPtrValue(strip.Stand) == *req.Event.Stand {
		return nil
	}

	s.euroscopeSender.SendStand(req.Session, req.Cid, req.Event.Callsign, *req.Event.Stand)
	if err := s.stripRepo.AppendControllerModifiedField(ctx, req.Session, req.Event.Callsign, "stand"); err != nil {
		return err
	}
	if s.standUpdater != nil {
		return s.standUpdater.UpdateStand(ctx, req.Session, req.Event.Callsign, *req.Event.Stand)
	}

	return nil
}

func (s *FrontendStripUpdateService) handleRunwayUpdate(ctx context.Context, req FrontendStripUpdateRequest, strip *internalModels.Strip) error {
	if req.Event.Runway == nil || stringPtrValue(strip.Runway) == *req.Event.Runway {
		return nil
	}

	s.euroscopeSender.SendRunway(req.Session, req.Cid, req.Event.Callsign, *req.Event.Runway)
	if _, err := s.stripRepo.UpdateRunway(ctx, req.Session, req.Event.Callsign, req.Event.Runway, nil); err != nil {
		return err
	}
	if err := s.stripRepo.AppendControllerModifiedField(ctx, req.Session, req.Event.Callsign, "runway"); err != nil {
		return err
	}
	if s.pdcReevaluator != nil {
		if err := s.pdcReevaluator.ReevaluatePdcInvalidValidation(ctx, req.Session, req.Event.Callsign, true, false); err != nil {
			return err
		}
	}
	if s.departureReevaluator != nil {
		return s.departureReevaluator.ReevaluateDepartureValidation(ctx, req.Session, req.Event.Callsign, true, false)
	}

	return nil
}

func (s *FrontendStripUpdateService) handleEobtUpdate(ctx context.Context, req FrontendStripUpdateRequest, strip *internalModels.Strip) error {
	if req.Event.Eobt == nil {
		return nil
	}

	eobt := strings.TrimSpace(*req.Event.Eobt)
	if stringPtrValue(strip.EffectiveEobt()) == eobt {
		return nil
	}
	if !isValidFrontendClockValue(eobt) {
		return errors.New("invalid eobt: expected HHMM")
	}

	s.euroscopeSender.SendEobt(req.Session, req.Cid, req.Event.Callsign, eobt)
	if s.cdmService == nil {
		return errors.New("CDM service not available")
	}
	if err := s.cdmService.HandleEobtUpdate(ctx, req.Session, req.Event.Callsign, eobt, req.Position, "ATC"); err != nil {
		return err
	}
	if s.stripUpdatePublisher != nil {
		s.stripUpdatePublisher.SendStripUpdate(req.Session, req.Event.Callsign)
	}

	return nil
}

func (s *FrontendStripUpdateService) handleAltitudeUpdate(ctx context.Context, req FrontendStripUpdateRequest, strip *internalModels.Strip) error {
	if req.Event.Altitude == nil || int32PointerEquals(strip.ClearedAltitude, req.Event.Altitude) {
		return nil
	}

	s.euroscopeSender.SendClearedAltitude(req.Session, req.Cid, req.Event.Callsign, *req.Event.Altitude)
	return s.stripRepo.AppendControllerModifiedField(ctx, req.Session, req.Event.Callsign, "cleared_altitude")
}

func (s *FrontendStripUpdateService) handleHeadingUpdate(ctx context.Context, req FrontendStripUpdateRequest, strip *internalModels.Strip) error {
	if req.Event.Heading == nil || int32PointerEquals(strip.Heading, req.Event.Heading) {
		return nil
	}

	s.euroscopeSender.SendHeading(req.Session, req.Cid, req.Event.Callsign, *req.Event.Heading)
	return s.stripRepo.AppendControllerModifiedField(ctx, req.Session, req.Event.Callsign, "heading")
}

func int32PointerEquals(left *int32, right *int32) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
