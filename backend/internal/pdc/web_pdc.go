package pdc

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"
)

// Web PDC request row statuses (pdc_web_requests.status).
const (
	WebRequestStatusPending = "pending"
	WebRequestStatusCleared = "cleared"
	WebRequestStatusFaults  = "faults"
	WebRequestStatusError   = "error"
)

// WebPDCSubmitInput is validated pilot input for SubmitWebPDCRequest.
type WebPDCSubmitInput struct {
	Callsign  string
	Atis      string
	Stand     string
	Remarks   string
	VatsimCID string
}

// StripMatch is a LIVE session plus strip for a callsign.
type StripMatch struct {
	SessionID int32
	Strip     *models.Strip
}

// FindStripsForWebCallsign returns all strips matching callsign across LIVE sessions.
func (s *Service) FindStripsForWebCallsign(ctx context.Context, callsign string) ([]StripMatch, error) {
	sessions, err := s.sessionRepo.GetByNames(ctx, "LIVE")
	if err != nil {
		return nil, err
	}
	var matches []StripMatch
	for _, sess := range sessions {
		strip, err := s.stripRepo.GetByCallsign(ctx, sess.ID, callsign)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		matches = append(matches, StripMatch{SessionID: sess.ID, Strip: strip})
	}
	return matches, nil
}

// SubmitWebPDCRequest creates a web PDC row and runs the same validation as CPDLC PDC (without aircraft-type telex match).
func (s *Service) SubmitWebPDCRequest(ctx context.Context, in WebPDCSubmitInput) (requestID int64, err error) {
	if s.queries == nil {
		return 0, fmt.Errorf("web PDC persistence not configured")
	}
	in.Callsign = strings.TrimSpace(in.Callsign)
	in.Atis = strings.TrimSpace(strings.ToUpper(in.Atis))
	in.Stand = strings.TrimSpace(in.Stand)
	in.Remarks = strings.TrimSpace(in.Remarks)

	if len(in.Atis) != 1 || !unicode.IsLetter(rune(in.Atis[0])) {
		return 0, fmt.Errorf("invalid ATIS letter")
	}

	matches, err := s.FindStripsForWebCallsign(ctx, in.Callsign)
	if err != nil {
		return 0, err
	}
	if len(matches) == 0 {
		return 0, ErrWebStripNotFound
	}
	if len(matches) > 1 {
		return 0, ErrWebAmbiguousCallsign
	}
	m := matches[0]
	strip := m.Strip
	sessionID := m.SessionID

	expires := time.Now().UTC().Add(2 * time.Hour)
	row, err := s.queries.InsertPdcWebRequest(ctx, database.InsertPdcWebRequestParams{
		SessionID: sessionID,
		Callsign:  in.Callsign,
		VatsimCid: in.VatsimCID,
		Atis:      in.Atis,
		Stand:     emptyStringPtr(in.Stand),
		Remarks:   emptyStringPtr(in.Remarks),
		Status:    WebRequestStatusPending,
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if err != nil {
		return 0, err
	}
	requestID = row.ID

	if in.Stand != "" && s.stripService != nil {
		if uerr := s.stripService.UpdateStand(ctx, sessionID, in.Callsign, in.Stand); uerr != nil {
			slog.ErrorContext(ctx, "web PDC: UpdateStand failed", slog.Any("error", uerr))
		}
	}

	faults := s.validatePDCFlightPlan(strip)
	if len(faults) > 0 {
		msg := strings.Join(faults, "; ")
		now := time.Now().UTC()
		if err := s.stripRepo.SetPdcRequested(ctx, sessionID, strip.Callsign, string(StateRequestedWithFaults), &now); err != nil {
			return requestID, fmt.Errorf("set PDC requested with faults: %w", err)
		}
		s.notifyFrontendStateChange(sessionID, strip.Callsign, StateRequestedWithFaults)
		_ = s.queries.UpdatePdcWebRequestStatus(ctx, database.UpdatePdcWebRequestStatusParams{
			ID:           requestID,
			Status:       WebRequestStatusFaults,
			ErrorMessage: &msg,
		})
		return requestID, nil
	}

	issueErr := s.IssueClearance(ctx, shared.PdcIssueClearanceParams{
		Callsign:     strip.Callsign,
		Remarks:      in.Remarks,
		CID:          in.VatsimCID,
		SessionID:    sessionID,
		Atis:         in.Atis,
		SkipCPDLC:    true,
		WebRequestID: &requestID,
	})
	if issueErr != nil {
		now := time.Now().UTC()
		if err := s.stripRepo.SetPdcRequested(ctx, sessionID, strip.Callsign, string(StateRequested), &now); err != nil {
			return requestID, fmt.Errorf("set PDC requested: %w", err)
		}
		s.notifyFrontendStateChange(sessionID, strip.Callsign, StateRequested)
		slog.InfoContext(ctx, "web PDC: clearance not auto-issued", slog.String("callsign", strip.Callsign), slog.Any("error", issueErr))
		return requestID, nil
	}

	return requestID, nil
}

func emptyStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Web PDC domain errors for HTTP mapping.
var (
	ErrWebStripNotFound      = fmt.Errorf("no strip for callsign on a LIVE session")
	ErrWebAmbiguousCallsign  = fmt.Errorf("callsign matches multiple LIVE sessions")
	ErrWebRequestNotFound    = fmt.Errorf("web request not found")
	ErrWebRequestForbidden   = fmt.Errorf("web request does not belong to this user")
)
