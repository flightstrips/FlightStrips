package pdc

import (
	"FlightStrips/internal/models"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

var (
	ErrWebInvalidAtis          = errors.New("invalid atis letter")
	ErrWebAircraftTypeRequired = errors.New("aircraft type is required")
	ErrWebAircraftTypeMismatch = errors.New("aircraft type does not match the live strip")
	ErrWebAlreadyRequested     = errors.New("a web pdc has already been submitted for this aircraft")
	ErrWebStripNotFound        = errors.New("no strip found for callsign")
	ErrWebAmbiguousCallsign    = errors.New("callsign exists in multiple sessions")
	ErrWebAlreadyCleared       = errors.New("strip is already cleared")
	ErrWebNotWebRequest        = errors.New("strip does not have a web pdc request")
	ErrWebClearanceNotReady    = errors.New("web pdc clearance is not ready")
)

type WebStripMatch struct {
	SessionID int32
	Strip     *models.Strip
}

func (s *Service) FindWebStripByCallsign(ctx context.Context, callsign string) (WebStripMatch, error) {
	normalizedCallsign := strings.ToUpper(strings.TrimSpace(callsign))
	if normalizedCallsign == "" {
		return WebStripMatch{}, ErrWebStripNotFound
	}

	var (
		sessions []*models.Session
		err      error
	)

	if s.webLookupLiveOnly {
		sessions, err = s.sessionRepo.GetByNames(ctx, "LIVE")
		if err != nil {
			return WebStripMatch{}, fmt.Errorf("get live sessions: %w", err)
		}
	} else {
		sessions, err = s.sessionRepo.List(ctx)
		if err != nil {
			return WebStripMatch{}, fmt.Errorf("list sessions: %w", err)
		}
	}

	matches := make([]WebStripMatch, 0, 1)
	for _, session := range sessions {
		strip, err := s.stripRepo.GetByCallsign(ctx, session.ID, normalizedCallsign)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return WebStripMatch{}, fmt.Errorf("get strip by callsign: %w", err)
		}

		matches = append(matches, WebStripMatch{
			SessionID: session.ID,
			Strip:     strip,
		})
	}

	switch len(matches) {
	case 0:
		return WebStripMatch{}, ErrWebStripNotFound
	case 1:
		return matches[0], nil
	default:
		return WebStripMatch{}, ErrWebAmbiguousCallsign
	}
}

func WebPDCCanSubmit(state string) bool {
	switch state {
	case string(StateNone), "", string(StateFailed), string(StateNoResponse), string(StateRevertToVoice):
		return true
	default:
		return false
	}
}

func (s *Service) SubmitWebPDCRequest(ctx context.Context, callsign string, atis string, stand string, remarks string, aircraftType string) error {
	normalizedCallsign := strings.ToUpper(strings.TrimSpace(callsign))
	normalizedAtis := strings.ToUpper(strings.TrimSpace(atis))
	trimmedRemarks := strings.TrimSpace(remarks)
	normalizedAircraftType := strings.ToUpper(strings.TrimSpace(aircraftType))

	if len(normalizedAtis) != 1 || normalizedAtis[0] < 'A' || normalizedAtis[0] > 'Z' {
		return ErrWebInvalidAtis
	}
	if normalizedAircraftType == "" {
		return ErrWebAircraftTypeRequired
	}

	match, err := s.FindWebStripByCallsign(ctx, normalizedCallsign)
	if err != nil {
		return err
	}

	sessionInfo := sessionInformation{}
	if info, infoErr := s.getSessionInfo(ctx, match.SessionID); infoErr == nil {
		sessionInfo = info
		sessionInfo.recordPDCRequestReceived(ctx, models.PdcChannelWeb)
	}

	if isWebPDCRequest(match.Strip) && !WebPDCCanSubmit(match.Strip.PdcState) {
		if sessionInfo.name != "" {
			sessionInfo.recordPDCRequestOutcome(ctx, models.PdcChannelWeb, "rejected")
		}
		return ErrWebAlreadyRequested
	}
	if match.Strip.Cleared {
		if sessionInfo.name != "" {
			sessionInfo.recordPDCRequestOutcome(ctx, models.PdcChannelWeb, "rejected")
		}
		return ErrWebAlreadyCleared
	}
	if !stripAircraftTypeMatches(match.Strip, normalizedAircraftType) {
		if sessionInfo.name != "" {
			sessionInfo.recordPDCRequestOutcome(ctx, models.PdcChannelWeb, "rejected")
		}
		return ErrWebAircraftTypeMismatch
	}

	pdcData := match.Strip.PdcData.Clone()
	requestChannel := models.PdcChannelWeb
	requestedAt := time.Now().UTC()
	pdcData.RequestChannel = &requestChannel
	pdcData.RequestRemarks = optionalString(trimmedRemarks)
	pdcData.RequestedAt = &requestedAt
	pdcData.MessageSequence = nil
	pdcData.MessageSent = nil
	pdcData.IssuedByCid = nil

	if pdcData.Web == nil {
		pdcData.Web = &models.PdcWebData{}
	}
	pdcData.Web.Atis = &normalizedAtis
	pdcData.Web.Stand = nil
	pdcData.Web.ClearanceText = nil
	pdcData.Web.PilotAcknowledgedAt = nil

	if err := s.stripRepo.SetPdcData(ctx, match.SessionID, normalizedCallsign, pdcData); err != nil {
		return fmt.Errorf("persist web pdc data: %w", err)
	}

	session, err := s.sessionRepo.GetByID(ctx, match.SessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if sessionInfo.name == "" {
		sessionInfo = sessionInformation{
			id:       session.ID,
			name:     session.Name,
			airport:  session.Airport,
			callsign: session.Airport,
		}
	}

	faults := s.validatePDCFlightPlan(match.Strip, session.ActiveRunways.DepartureRunways)
	switch {
	case len(faults) > 0:
		sessionInfo.recordPDCRequestOutcome(ctx, models.PdcChannelWeb, "requested_with_faults")
		if err := s.stripRepo.SetPdcRequested(ctx, match.SessionID, normalizedCallsign, string(StateRequestedWithFaults), &requestedAt, optionalString(trimmedRemarks)); err != nil {
			return fmt.Errorf("persist requested-with-faults state: %w", err)
		}
		if err := s.notifyStateChange(ctx, match.SessionID, normalizedCallsign, StateRequestedWithFaults, trimmedRemarks); err != nil {
			return fmt.Errorf("notify requested-with-faults state change: %w", err)
		}
		return nil
	case trimmedRemarks != "":
		sessionInfo.recordPDCRequestOutcome(ctx, models.PdcChannelWeb, "requested_manual_review")
		if err := s.stripRepo.SetPdcRequested(ctx, match.SessionID, normalizedCallsign, string(StateRequested), &requestedAt, optionalString(trimmedRemarks)); err != nil {
			return fmt.Errorf("persist requested state: %w", err)
		}
		if err := s.notifyStateChange(ctx, match.SessionID, normalizedCallsign, StateRequested, trimmedRemarks); err != nil {
			return fmt.Errorf("notify requested state change: %w", err)
		}
		return nil
	}

	if err := s.IssueClearance(ctx, normalizedCallsign, "", "", match.SessionID); err != nil {
		sessionInfo.recordPDCRequestOutcome(ctx, models.PdcChannelWeb, "requested_pending_clearance")
		slog.WarnContext(ctx, "Web PDC auto-issue failed, leaving request pending", slog.String("callsign", normalizedCallsign), slog.Any("error", err))
		if err := s.stripRepo.SetPdcRequested(ctx, match.SessionID, normalizedCallsign, string(StateRequested), &requestedAt, nil); err != nil {
			return fmt.Errorf("persist fallback requested state: %w", err)
		}
		if err := s.notifyStateChange(ctx, match.SessionID, normalizedCallsign, StateRequested, ""); err != nil {
			return fmt.Errorf("notify fallback requested state change: %w", err)
		}
		return nil
	}

	sessionInfo.recordPDCRequestOutcome(ctx, models.PdcChannelWeb, "auto_cleared")
	return nil
}
