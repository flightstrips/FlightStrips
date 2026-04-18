package pdc

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/helpers"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
)

type HoppieClientInterface interface {
	Poll(ctx context.Context, callsign string) ([]Message, error)
	SendCPDLC(ctx context.Context, from, to, packet string) error
	SendTelex(ctx context.Context, from, to, packet string) error
}

type timeoutTracker struct {
	cancel    context.CancelFunc
	callsign  string
	sessionID int32
	cid       string
	web       bool
}

type sessionInformation struct {
	id       int32
	callsign string
}

type Service struct {
	client            HoppieClientInterface
	sessionRepo       repository.SessionRepository
	stripRepo         repository.StripRepository
	sectorRepo        repository.SectorOwnerRepository
	controllerRepo    repository.ControllerRepository
	frontendHub       shared.FrontendHub
	euroscopeHub      shared.EuroscopeHub
	stripService      shared.StripService
	timeouts          map[string]*timeoutTracker
	timeoutsMutex     sync.RWMutex
	timeoutConfig     time.Duration
	webLookupLiveOnly bool
}

func NewPDCService(client HoppieClientInterface, sessionRepo repository.SessionRepository, stripRepo repository.StripRepository, sectorRepo repository.SectorOwnerRepository, controllerRepo repository.ControllerRepository) *Service {
	return &Service{
		client:         client,
		sessionRepo:    sessionRepo,
		stripRepo:      stripRepo,
		sectorRepo:     sectorRepo,
		controllerRepo: controllerRepo,
		timeouts:       make(map[string]*timeoutTracker),
		timeoutConfig:  10 * time.Minute, // Default 10 minutes
	}
}

func (s *Service) SetFrontendHub(frontendHub shared.FrontendHub) {
	s.frontendHub = frontendHub
}

func (s *Service) SetEuroscopeHub(euroscopeHub shared.EuroscopeHub) {
	s.euroscopeHub = euroscopeHub
}

func (s *Service) SetStripService(stripService shared.StripService) {
	s.stripService = stripService
}

func (s *Service) SetWebLookupLiveOnly(liveOnly bool) {
	s.webLookupLiveOnly = liveOnly
}

func normalizedStripAircraftType(strip *models.Strip) string {
	if strip == nil || strip.AircraftType == nil {
		return ""
	}
	return strings.SplitN(*strip.AircraftType, "/", 2)[0]
}

func stripAircraftTypeMatches(strip *models.Strip, aircraftType string) bool {
	expected := normalizedStripAircraftType(strip)
	return expected != "" && strings.EqualFold(expected, strings.TrimSpace(aircraftType))
}

// Helper functions

// getSessionInfo retrieves session information from the database
func (s *Service) getSessionInfo(ctx context.Context, sessionID int32) (sessionInformation, error) {
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return sessionInformation{}, fmt.Errorf("failed to get session: %w", err)
	}
	return sessionInformation{id: sessionID, callsign: session.Airport}, nil
}

// getNextSequence retrieves the next message sequence number
func (s *Service) getNextSequence(ctx context.Context, sessionID int32) (int32, error) {
	seq, err := s.sessionRepo.IncrementPdcMessageSequence(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("failed to get next message sequence: %w", err)
	}
	return seq, nil
}

// sendErrorAndReturn sends an error message to the pilot and returns the original error
func (s *Service) sendErrorAndReturn(ctx context.Context, session sessionInformation, callsign string, originalErr error, messageBuilder func(int32) string) error {
	seq, seqErr := s.getNextSequence(ctx, session.id)
	if seqErr != nil {
		return fmt.Errorf("%w; also failed to get message sequence: %w", originalErr, seqErr)
	}

	msg := messageBuilder(seq)
	if sendErr := s.client.SendCPDLC(ctx, session.callsign, callsign, msg); sendErr != nil {
		return fmt.Errorf("%w; also failed to send error message: %w", originalErr, sendErr)
	}

	return originalErr
}

func (s *Service) notifyStateChangeFrontend(sessionID int32, callsign string, state ClearanceState, remarks string) {
	if s.frontendHub != nil {
		s.frontendHub.SendPdcStateChange(sessionID, callsign, string(state), remarks)
	}
}

func (s *Service) notifyStateChangeEuroscope(sessionID int32, callsign string, state ClearanceState, remarks string) {
	if s.euroscopeHub != nil {
		s.euroscopeHub.SendPdcStateChange(sessionID, callsign, string(state), remarks)
	}
}

func (s *Service) notifyClearedFlagEuroscope(sessionID int32, callsign string, cleared bool, cid string) {
	if s.euroscopeHub != nil {
		s.euroscopeHub.SendClearedFlag(sessionID, cid, callsign, cleared)
	}
}

// notifyStateChange sends PDC state change notification to frontend and EuroScope clients
func (s *Service) notifyStateChange(ctx context.Context, sessionID int32, callsign string, state ClearanceState, remarks string) error {
	metrics.PDCStateChange(context.Background(), sessionID, string(state))
	if s.stripService != nil {
		if err := s.stripService.ReevaluatePdcInvalidValidation(ctx, sessionID, callsign, true, state == StateRequestedWithFaults); err != nil {
			return err
		}
	}
	s.notifyStateChangeFrontend(sessionID, callsign, state, remarks)
	s.notifyStateChangeEuroscope(sessionID, callsign, state, remarks)
	return nil
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	return &value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func isWebPDCRequest(strip *models.Strip) bool {
	return strip != nil &&
		strip.PdcData != nil &&
		strip.PdcData.RequestChannel != nil &&
		strings.EqualFold(*strip.PdcData.RequestChannel, models.PdcChannelWeb) &&
		strip.PdcData.Web != nil
}

func (s *Service) updatePdcData(ctx context.Context, sessionID int32, callsign string, mutate func(*models.PdcData)) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, sessionID, callsign)
	if err != nil {
		return err
	}

	pdcData := strip.PdcData.Clone()
	mutate(pdcData)
	return s.stripRepo.SetPdcData(ctx, sessionID, callsign, pdcData)
}

func (s *Service) confirmPilotAcknowledgement(ctx context.Context, sessionID int32, strip *models.Strip, clearedBay string, clearanceCid string, webAcknowledged bool) error {
	pdcData := strip.PdcData.Clone()
	if webAcknowledged {
		if pdcData.Web == nil {
			pdcData.Web = &models.PdcWebData{}
		}
		if pdcData.Web.PilotAcknowledgedAt == nil {
			now := time.Now().UTC()
			pdcData.Web.PilotAcknowledgedAt = &now
		}
	}
	pdcData.State = string(StateConfirmed)

	if s.stripService != nil {
		if err := s.stripService.ConfirmPdcClearance(ctx, sessionID, strip.Callsign, clearedBay, clearanceCid); err != nil {
			return fmt.Errorf("failed to confirm strip clearance: %w", err)
		}
	}

	if err := s.stripRepo.SetPdcData(ctx, sessionID, strip.Callsign, pdcData); err != nil {
		return fmt.Errorf("failed to persist confirmed PDC data: %w", err)
	}

	if !strip.Cleared && s.stripService != nil {
		if err := s.stripService.AutoAssumeForClearedStripByCid(ctx, sessionID, strip.Callsign, clearanceCid); err != nil {
			slog.ErrorContext(ctx, "Failed to auto-assume confirmed PDC strip", slog.Any("error", err))
		}
	}

	s.CancelTimeout(strip.Callsign, sessionID)

	if s.stripService != nil {
		if err := s.stripService.ReevaluatePdcInvalidValidation(ctx, sessionID, strip.Callsign, true, false); err != nil {
			return fmt.Errorf("failed to reevaluate PDC invalid validation: %w", err)
		}
	}

	metrics.PDCStateChange(context.Background(), sessionID, string(StateConfirmed))
	s.notifyStateChangeEuroscope(sessionID, strip.Callsign, StateConfirmed, "")
	s.notifyClearedFlagEuroscope(sessionID, strip.Callsign, true, clearanceCid)

	s.notifyStateChangeFrontend(sessionID, strip.Callsign, StateConfirmed, "")

	strip.PdcData = pdcData
	strip.PdcState = string(StateConfirmed)
	strip.PdcRequestRemarks = nil

	return nil
}

// Start begins the background polling loop
func (s *Service) Start(ctx context.Context) {
	slog.InfoContext(ctx, "PDC Service: Starting Hoppie polling loop")

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "PDC Service: Stopping polling loop")
			return
		case <-time.After(RandomPollInterval()):
			if err := s.pollAndProcess(ctx); err != nil {
				slog.ErrorContext(ctx, "PDC Service: Error during poll", slog.Any("error", err))
			}
		}
	}
}

func (s *Service) pollAndProcess(ctx context.Context) error {
	sessions, err := s.sessionRepo.GetByNames(ctx, "LIVE")
	if err != nil {
		return fmt.Errorf("failed to get sessions: %w", err)
	}

	for _, session := range sessions {
		err = s.pollAndProcessForSession(ctx, sessionInformation{id: session.ID, callsign: session.Airport})
		if err != nil {
			return fmt.Errorf("failed to poll and process for session %s: %w", session.Airport, err)
		}
	}

	return nil
}

func (s *Service) pollAndProcessForSession(ctx context.Context, session sessionInformation) error {
	slog.DebugContext(ctx, "PDC Service: Polling for messages", slog.String("callsign", session.callsign), slog.Int("session", int(session.id)))
	messages, err := s.client.Poll(ctx, session.callsign)
	if err != nil {
		return fmt.Errorf("failed to poll: %w", err)
	}
	slog.DebugContext(ctx, "PDC Service: Polling returned", slog.String("callsign", session.callsign), slog.Int("messages", len(messages)))

	for _, msg := range messages {
		if err := s.HandleIncomingMessage(ctx, &msg, session); err != nil {
			slog.ErrorContext(ctx, "PDC Service: Error handling message", slog.String("from", msg.From), slog.Any("error", err))
		}
	}

	return nil
}

// HandleIncomingMessage routes incoming messages by type
func (s *Service) HandleIncomingMessage(ctx context.Context, msg *Message, session sessionInformation) error {
	incomingMsg, err := ParseIncomingMessage(msg.From, msg.To, msg.Packet)
	if err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	slog.DebugContext(ctx, "PDC Service: Received message from CPDLC", slog.String("from", msg.From), slog.String("packet", msg.Packet))

	switch incomingMsg.Type {
	case MsgPDCRequest:
		return s.ProcessPDCRequest(ctx, incomingMsg, session)
	case MsgWilco:
		return s.HandleWilco(ctx, incomingMsg, session)
	case MsgUnable:
		return s.HandleUnable(ctx, incomingMsg.From, session)
	case MsgUnknown:
		slog.InfoContext(ctx, "PDC Service: Unknown message type", slog.String("from", msg.From), slog.String("packet", msg.Packet))
		return s.SendErrorMessage(ctx, session, msg.From)
	}

	return nil
}

// ProcessPDCRequest handles a pilot's PDC request
func (s *Service) ProcessPDCRequest(ctx context.Context, msg *IncomingMessage, session sessionInformation) error {
	req, err := parsePDCRequest(msg.Payload)
	if err != nil {
		if sendErr := s.SendErrorMessage(ctx, session, msg.From); sendErr != nil {
			return fmt.Errorf("failed to parse PDC request %w and sending error message %w", err, sendErr)
		}
		return fmt.Errorf("failed to parse PDC request: %w", err)
	}

	if req.Callsign != msg.From {
		return fmt.Errorf("callsign in request does not match from field in message")
	}

	slog.DebugContext(ctx, "PDC Service: Processing PDC request", slog.String("callsign", req.Callsign), slog.String("departure", req.Departure))

	strip, err := s.stripRepo.GetByCallsign(ctx, session.id, req.Callsign)

	if errors.Is(err, sql.ErrNoRows) {
		return s.sendErrorAndReturn(ctx, session, req.Callsign,
			fmt.Errorf("strip not found"),
			func(seq int32) string { return buildFlightPlanNotHeld(seq, req.Departure, req.Callsign) })
	}

	if err != nil {
		return s.sendErrorAndReturn(ctx, session, req.Callsign, err, buildPDCUnavailable)
	}

	if strip.Cleared {
		return s.sendErrorAndReturn(ctx, session, req.Callsign,
			fmt.Errorf("aircraft already cleared"),
			func(seq int32) string { return buildAlreadyCleared(seq, strip.Origin, req.Callsign) })
	}

	if !strings.EqualFold(strip.Origin, req.Departure) || !strings.EqualFold(strip.Destination, req.Destination) {
		return s.sendErrorAndReturn(ctx, session, req.Callsign,
			fmt.Errorf("invalid PDC request: origin/destination mismatch"),
			func(seq int32) string { return buildFlightPlanNotHeld(seq, strip.Origin, req.Callsign) })
	}

	stripAircraftType := normalizedStripAircraftType(strip)
	if !stripAircraftTypeMatches(strip, req.Aircraft) {
		return s.sendErrorAndReturn(ctx, session, req.Callsign,
			fmt.Errorf("aircraft type mismatch: expected %s, got %s", stripAircraftType, req.Aircraft),
			func(seq int32) string { return buildInvalidAircraftType(seq, strip.Origin, req.Callsign) })
	}

	currentSession, err := s.sessionRepo.GetByID(ctx, session.id)
	if err != nil {
		return s.sendErrorAndReturn(ctx, session, req.Callsign, err, buildPDCUnavailable)
	}

	requestRemarks := optionalString(req.Remarks)
	faults := s.validatePDCFlightPlan(strip, currentSession.ActiveRunways.DepartureRunways)
	if len(faults) > 0 {
		metrics.PDCRequest(ctx, session.id, "requested_with_faults")
		now := time.Now().UTC()
		if err := s.stripRepo.SetPdcRequested(ctx, session.id, strip.Callsign, string(StateRequestedWithFaults), &now, requestRemarks); err != nil {
			return fmt.Errorf("failed to set PDC requested with faults: %w", err)
		}
		if err := s.notifyStateChange(ctx, session.id, req.Callsign, StateRequestedWithFaults, req.Remarks); err != nil {
			return fmt.Errorf("failed to notify requested-with-faults state change: %w", err)
		}
		if err := s.SendStatusAck(ctx, session, req.Callsign, req.Departure); err != nil {
			return fmt.Errorf("failed to send status ack: %w", err)
		}
		slog.InfoContext(ctx, "PDC Service: PDC request has faults", slog.String("callsign", req.Callsign), slog.Any("faults", faults))
		return nil
	}

	// No faults — send ACK then auto-issue clearance
	if err := s.SendStatusAck(ctx, session, req.Callsign, req.Departure); err != nil {
		return fmt.Errorf("failed to send status ack: %w", err)
	}

	if requestRemarks != nil {
		metrics.PDCRequest(ctx, session.id, "requested_manual_review")
		now := time.Now().UTC()
		if err := s.stripRepo.SetPdcRequested(ctx, session.id, strip.Callsign, string(StateRequested), &now, requestRemarks); err != nil {
			return fmt.Errorf("failed to set PDC requested with remarks: %w", err)
		}
		if err := s.notifyStateChange(ctx, session.id, req.Callsign, StateRequested, req.Remarks); err != nil {
			return fmt.Errorf("failed to notify requested state change: %w", err)
		}
		slog.InfoContext(ctx, "PDC Service: PDC request requires manual review due to request remarks", slog.String("callsign", req.Callsign))
		return nil
	}

	if issueErr := s.IssueClearance(ctx, strip.Callsign, "", "", session.id); issueErr != nil {
		metrics.PDCRequest(ctx, session.id, "requested_pending_clearance")
		// Clearance fields not set yet — fall back to REQUESTED state
		now := time.Now().UTC()
		if err := s.stripRepo.SetPdcRequested(ctx, session.id, strip.Callsign, string(StateRequested), &now, nil); err != nil {
			return fmt.Errorf("failed to set PDC requested: %w", err)
		}
		if err := s.notifyStateChange(ctx, session.id, req.Callsign, StateRequested, ""); err != nil {
			return fmt.Errorf("failed to notify requested fallback state change: %w", err)
		}
		slog.InfoContext(ctx, "PDC Service: PDC request acknowledged (clearance fields not ready)", slog.String("callsign", req.Callsign))
	} else {
		metrics.PDCRequest(ctx, session.id, "auto_cleared")
		slog.InfoContext(ctx, "PDC Service: PDC clearance auto-issued", slog.String("callsign", req.Callsign))
	}
	return nil
}

// SendStatusAck sends the "request received" message
func (s *Service) SendStatusAck(ctx context.Context, session sessionInformation, callsign, origin string) error {
	seq, err := s.getNextSequence(ctx, session.id)
	if err != nil {
		return err
	}

	msg := buildRequestAck(seq, origin, callsign)
	return s.client.SendCPDLC(ctx, session.callsign, callsign, msg)
}

// IssueClearance sends a PDC clearance to a pilot
// The clearance data (runway, sid, squawk) should already be in the strip table
func (s *Service) IssueClearance(ctx context.Context, callsign, remarks, cid string, sessionID int32) error {
	sessionInfo, err := s.getSessionInfo(ctx, sessionID)
	if err != nil {
		return err
	}

	// Get strip data (source of truth for clearance data)
	strip, err := s.stripRepo.GetByCallsign(ctx, sessionInfo.id, callsign)
	if err != nil {
		return fmt.Errorf("strip not found for PDC clearance: %w", err)
	}

	webDelivery := isWebPDCRequest(strip)

	// Validate required clearance fields are present in strip
	if strip.Runway == nil || (strip.Sid == nil && strip.Heading == nil) {
		return fmt.Errorf("strip missing required clearance data (runway and SID or heading)")
	}

	clearanceSquawk, err := getAssignedPDCSquawk(strip)
	if err != nil {
		return fmt.Errorf("strip missing required clearance data: %w", err)
	}

	nextFreq, err := s.getNextFrequency(ctx, sessionInfo.id)
	if err != nil {
		return fmt.Errorf("failed to get next frequency: %w", err)
	}

	departureFreq, err := s.getAirborneFrequency(ctx, sessionInfo.id, strip.Sid)
	if err != nil {
		return fmt.Errorf("failed to get departure frequency: %w", err)
	}

	nextPdcSeq, err := s.sessionRepo.IncrementPdcSequence(ctx, sessionInfo.id)
	if err != nil {
		return fmt.Errorf("failed to get next PDC sequence: %w", err)
	}

	nextSeq, err := s.getNextSequence(ctx, sessionInfo.id)
	if err != nil {
		return err
	}

	options := ClearanceOptions{
		Callsign:           strip.Callsign,
		Origin:             strip.Origin,
		Destination:        strip.Destination,
		Atis:               "A",
		Runway:             *strip.Runway,
		Squawk:             clearanceSquawk,
		NextFrequency:      nextFreq,
		DepartureFrequency: departureFreq,
		Sequence:           nextSeq,
		PdcSequence:        nextPdcSeq,
		Remarks:            remarks,
	}

	if webDelivery && strip.PdcData.Web.Atis != nil && strings.TrimSpace(*strip.PdcData.Web.Atis) != "" {
		options.Atis = strings.TrimSpace(*strip.PdcData.Web.Atis)
	}

	if strip.Heading != nil && *strip.Heading != 0 && strip.ClearedAltitude != nil && *strip.ClearedAltitude > 0 {
		// Get first waypoint of route and make sure it is not the airport and DCT. If the SID is in there be sure only take the first part
		options.Heading = fmt.Sprintf("%03d", *strip.Heading)
		if *strip.ClearedAltitude > 5000 {
			options.ClimbTo = fmt.Sprintf("FL%03d", *strip.ClearedAltitude/100)
		} else {
			options.ClimbTo = fmt.Sprintf("%04d FT", *strip.ClearedAltitude)
		}
		options.Vectors = "FIRST WAYPOINT"
	} else if strip.Sid != nil {
		options.SID = *strip.Sid
	}

	clearance := buildPDCClearance(options)

	if !webDelivery {
		if err := s.client.SendCPDLC(ctx, sessionInfo.callsign, callsign, clearance); err != nil {
			return fmt.Errorf("failed to send clearance: %w", err)
		}
	}

	now := time.Now().UTC()
	err = s.stripRepo.SetPdcMessageSent(ctx, sessionInfo.id, callsign, string(StateCleared), &nextSeq, &now)

	if err != nil {
		return fmt.Errorf("failed to set PDC message sent: %w", err)
	}

	if err := s.updatePdcData(ctx, sessionInfo.id, callsign, func(pdcData *models.PdcData) {
		pdcData.State = string(StateCleared)
		pdcData.IssuedByCid = optionalString(cid)
		if webDelivery {
			if pdcData.Web == nil {
				pdcData.Web = &models.PdcWebData{}
			}
			webClearance := buildWebPDCClearance(options)
			pdcData.Web.ClearanceText = &webClearance
		}
	}); err != nil {
		return fmt.Errorf("failed to persist issued PDC data: %w", err)
	}

	if s.stripService != nil {
		if err := s.stripService.MoveToBay(ctx, sessionInfo.id, callsign, shared.BAY_CLEARED, true); err != nil {
			slog.ErrorContext(ctx, "PDC Service: Warning - failed to move strip to cleared bay", slog.Any("error", err))
		}
	}

	s.StartClearanceTimeout(ctx, strip.Origin, callsign, nextSeq, sessionInfo, cid, webDelivery)

	if err := s.notifyStateChange(ctx, sessionInfo.id, callsign, StateCleared, ""); err != nil {
		return fmt.Errorf("failed to notify cleared state change: %w", err)
	}

	slog.DebugContext(ctx, "PDC Service: Clearance issued", slog.String("callsign", callsign), slog.Int("sequence", int(nextSeq)))
	return nil
}

func (s *Service) RevertToVoice(ctx context.Context, callsign string, sessionId int32, cid string) error {
	sessionInfo, err := s.getSessionInfo(ctx, sessionId)
	if err != nil {
		return err
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, sessionId, callsign)
	if err != nil {
		return fmt.Errorf("failed to get strip: %w", err)
	}

	if strip.PdcState == string(StateNone) || strip.PdcState == string(StateRevertToVoice) || strip.PdcState == "" {
		return fmt.Errorf("cannot revert to voice, PDC state is %s", strip.PdcState)
	}

	nextSeq, err := s.getNextSequence(ctx, sessionInfo.id)
	if err != nil {
		return err
	}

	pdcMessage := buildRevertToVoice(nextSeq)
	err = s.client.SendCPDLC(ctx, sessionInfo.callsign, callsign, pdcMessage)
	if err != nil {
		return fmt.Errorf("failed to send revert to voice: %w", err)
	}

	s.CancelTimeout(callsign, sessionId)

	err = s.setPdcFailed(ctx, callsign, sessionId, StateRevertToVoice, cid)
	if err != nil {
		return fmt.Errorf("failed to set PDC failed: %w", err)
	}

	slog.DebugContext(ctx, "PDC Service: Reverting to voice", slog.String("callsign", callsign))
	return nil
}

func (s *Service) getNextFrequency(ctx context.Context, sessionID int32) (string, error) {
	owners, err := s.sectorRepo.ListBySession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get sector owners: %w", err)
	}

	// Find the owner of SQ (sequence controller).
	nextFrequency := ""
	for _, owner := range owners {
		if slices.Contains(owner.Sector, "SQ") {
			nextFrequency = owner.Position
		}
	}

	// Fallback: DEL sector owner.
	if nextFrequency == "" {
		for _, owner := range owners {
			if slices.Contains(owner.Sector, "DEL") {
				nextFrequency = owner.Position
			}
		}
	}

	if nextFrequency == "" {
		return "", fmt.Errorf("no frequency found for sector SQ or DEL")
	}

	return nextFrequency, nil
}

// getAirborneFrequency returns the frequency of the highest-priority controller
// for the strip's airborne sector, or "122.8" (UNICOM) if none is online.
func (s *Service) getAirborneFrequency(ctx context.Context, sessionID int32, sid *string) (string, error) {
	onlineFreqs := s.getOnlineControllerFrequencies(ctx, sessionID)
	controllerPriority, err := getPdcAirborneControllerPriority(sid)
	if err != nil {
		return "", err
	}

	for _, posName := range controllerPriority {
		pos, err := config.GetPositionByName(posName)
		if err != nil {
			continue
		}
		if _, online := onlineFreqs[normalizeFrequency(pos.Frequency)]; online {
			return pos.Frequency, nil
		}
	}

	return "122.8", nil
}

func getPdcAirborneControllerPriority(sid *string) ([]string, error) {
	if sid != nil {
		normalizedSID := strings.TrimSpace(*sid)
		if normalizedSID != "" {
			controllerPriority, err := config.GetAirborneControllerPriority(normalizedSID)
			if err == nil {
				return controllerPriority, nil
			}
			if !errors.Is(err, config.ErrUnknownAirborneRoute) {
				return nil, err
			}
		}
	}

	return config.GetDefaultAirborneControllerPriority()
}

func (s *Service) getOnlineControllerFrequencies(ctx context.Context, sessionID int32) map[string]struct{} {
	onlineFreqs := make(map[string]struct{})

	if s.controllerRepo != nil {
		controllers, err := s.controllerRepo.ListBySession(ctx, sessionID)
		if err == nil {
			for _, controller := range controllers {
				if frequency := normalizeFrequency(controller.Position); frequency != "" {
					onlineFreqs[frequency] = struct{}{}
				}
			}
		}
		if len(onlineFreqs) > 0 {
			return onlineFreqs
		}
	}

	owners, err := s.sectorRepo.ListBySession(ctx, sessionID)
	if err != nil {
		return onlineFreqs
	}

	for _, owner := range owners {
		if frequency := normalizeFrequency(owner.Position); frequency != "" {
			onlineFreqs[frequency] = struct{}{}
		}
	}

	return onlineFreqs
}

func getAssignedPDCSquawk(strip *models.Strip) (string, error) {
	if strip.AssignedSquawk == nil {
		return "", errors.New("assigned squawk not set")
	}

	squawk := strings.TrimSpace(*strip.AssignedSquawk)
	if squawk == "" {
		return "", errors.New("assigned squawk not set")
	}
	if !isValidPDCSquawk(squawk) {
		return "", fmt.Errorf("assigned squawk %q is not valid for PDC", squawk)
	}

	return squawk, nil
}

func isValidPDCSquawk(squawk string) bool {
	return helpers.IsValidAssignedSquawk(squawk)
}

func normalizeFrequency(frequency string) string {
	normalized := strings.TrimSpace(frequency)
	if strings.Contains(normalized, ".") {
		normalized = strings.TrimRight(normalized, "0")
		normalized = strings.TrimRight(normalized, ".")
	}
	return normalized
}

// HandleWilco processes pilot WILCO response
func (s *Service) HandleWilco(ctx context.Context, message *IncomingMessage, session sessionInformation) error {
	callsign := message.From

	wilco, err := ParseWilcoMessage(message.Payload)
	if err != nil {
		sendErr := s.SendErrorMessage(ctx, session, callsign)
		if sendErr != nil {
			return errors.Join(err, sendErr)
		}
		return fmt.Errorf("failed to parse WILCO message: %w", err)
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session.id, callsign)
	if err != nil {
		sendErr := s.SendErrorMessage(ctx, session, callsign)
		if sendErr != nil {
			return errors.Join(err, sendErr)
		}
		return fmt.Errorf("failed to get strip: %w", err)
	}

	if strip.PdcMessageSequence == nil {
		sendErr := s.SendErrorMessage(ctx, session, callsign)
		if sendErr != nil {
			return errors.Join(err, sendErr)
		}
		return fmt.Errorf("strip missing PDC message sequence")
	}

	if *strip.PdcMessageSequence != wilco.ResponseTo {
		sendErr := s.SendErrorMessage(ctx, session, callsign)
		if sendErr != nil {
			return errors.Join(err, sendErr)
		}
		return fmt.Errorf("WILCO response missing sequence or not correct response. Expected: %d, got: %d", *strip.PdcMessageSequence, wilco.ResponseTo)
	}

	key := fmt.Sprintf("%s_%d", callsign, session.id)
	clearanceCid := ""
	s.timeoutsMutex.RLock()
	if tracker, exists := s.timeouts[key]; exists {
		clearanceCid = tracker.cid
	}
	s.timeoutsMutex.RUnlock()

	if clearanceCid == "" && strip.PdcData != nil && strip.PdcData.IssuedByCid != nil {
		clearanceCid = *strip.PdcData.IssuedByCid
	}

	clearedBay := strip.Bay
	if clearedBay == "" || clearedBay == shared.BAY_UNKNOWN || clearedBay == shared.BAY_NOT_CLEARED {
		clearedBay = shared.BAY_CLEARED
	}

	if err := s.confirmPilotAcknowledgement(ctx, session.id, strip, clearedBay, clearanceCid, false); err != nil {
		return err
	}

	slog.DebugContext(ctx, "PDC Service: WILCO received", slog.String("callsign", callsign))
	return nil
}

func (s *Service) HandleUnable(ctx context.Context, callsign string, session sessionInformation) error {
	key := fmt.Sprintf("%s_%d", callsign, session.id)

	s.timeoutsMutex.RLock()
	tracker, exists := s.timeouts[key]
	s.timeoutsMutex.RUnlock()

	cid := ""
	if exists {
		cid = tracker.cid
	}

	if cid == "" {
		strip, err := s.stripRepo.GetByCallsign(ctx, session.id, callsign)
		if err == nil && strip.PdcData != nil && strip.PdcData.IssuedByCid != nil {
			cid = *strip.PdcData.IssuedByCid
		}
	}

	s.CancelTimeout(callsign, session.id)

	err := s.setPdcFailed(ctx, callsign, session.id, StateFailed, cid)
	if err != nil {
		return fmt.Errorf("failed to set PDC failed: %w", err)
	}

	return nil
}

// ManualStateChange allows manual state confirmation if pilot communicates outside Hoppie
func (s *Service) ManualStateChange(ctx context.Context, callsign string, sessionID int32, newState string) error {
	/*
		// Validate state
		validStates := map[string]bool{
			string(StateConfirmed): true,
		}

		if !validStates[newState] {
			return fmt.Errorf("invalid manual state: %s (allowed: WILCO_RECEIVED, CONFIRMED)", newState)
		}

		// Update state
		if _, err := s.queries.UpdatePDCState(ctx, database.UpdatePDCStateParams{
			Callsign: callsign,
			Session:  sessionID,
			State:    newState,
		}); err != nil {
			return fmt.Errorf("failed to update state: %w", err)
		}

		// Cancel timeout
		s.CancelTimeout(callsign, sessionID)

		// Get sequence for frontend notification
		pdc, _ := s.queries.GetPDCClearance(ctx, database.GetPDCClearanceParams{
			Callsign: callsign,
			Session:  sessionID,
		})

		// Notify frontend
		timestamp := time.Now().Format(time.RFC3339)
		if s.frontendHub != nil {
			seq := int32(0)
			if pdc.Sequence != nil {
				seq = *pdc.Sequence
			}
			s.frontendHub.SendPdcStateChange(sessionID, callsign, newState, seq, timestamp)
		}

		slog.InfoContext(ctx, "PDC manual state change", slog.String("state", string(newState)), slog.String("callsign", callsign))
	*/
	return nil
}

// SendErrorMessage sends error message to pilot for invalid PDC requests
func (s *Service) SendErrorMessage(ctx context.Context, session sessionInformation, callsign string) error {
	seq, err := s.getNextSequence(ctx, session.id)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("/data2/%d//NE/BAD PDC MESSAGE. @RESEND OR REVERT TO VOICE", seq)
	return s.client.SendCPDLC(ctx, session.callsign, callsign, msg)
}

// StartClearanceTimeout starts a timeout that reverts cleared flag if pilot doesn't respond
func (s *Service) StartClearanceTimeout(ctx context.Context, airport, callsign string, messageId int32, session sessionInformation, cid string, web bool) {
	key := fmt.Sprintf("%s_%d", callsign, session.id)

	// Cancel any existing timeout for this callsign
	s.CancelTimeout(callsign, session.id)

	// Create cancellable context
	timeoutCtx, cancel := context.WithCancel(ctx)

	tracker := &timeoutTracker{
		cancel:    cancel,
		callsign:  callsign,
		sessionID: session.id,
		cid:       cid,
		web:       web,
	}

	s.timeoutsMutex.Lock()
	s.timeouts[key] = tracker
	s.timeoutsMutex.Unlock()

	// Start timeout goroutine
	go s.handleTimeout(timeoutCtx, airport, callsign, messageId, session)

	slog.DebugContext(ctx, "PDC Service: Started timeout", slog.Duration("timeout", s.timeoutConfig), slog.String("callsign", callsign))
}

// CancelTimeout cancels an active timeout
func (s *Service) CancelTimeout(callsign string, sessionID int32) {
	key := fmt.Sprintf("%s_%d", callsign, sessionID)

	s.timeoutsMutex.Lock()
	defer s.timeoutsMutex.Unlock()

	if tracker, exists := s.timeouts[key]; exists {
		tracker.cancel()
		delete(s.timeouts, key)
		slog.Debug("PDC Service: Cancelled timeout", slog.String("callsign", callsign))
	}
}

// handleTimeout waits for timeout and reverts cleared flag if no response
func (s *Service) handleTimeout(ctx context.Context, airport, callsign string, messageId int32, session sessionInformation) {
	key := fmt.Sprintf("%s_%d", callsign, session.id)

	s.timeoutsMutex.RLock()
	tracker, exists := s.timeouts[key]
	s.timeoutsMutex.RUnlock()

	if !exists {
		return
	}

	select {
	case <-ctx.Done():
		// Timeout was cancelled (pilot responded)
		return
	case <-time.After(s.timeoutConfig):
		// Timeout expired - revert cleared flag
		slog.DebugContext(ctx, "PDC Service: Timeout expired - reverting cleared flag", slog.String("callsign", callsign))

		strip, err := s.stripRepo.GetByCallsign(ctx, session.id, callsign)
		if err != nil {
			slog.ErrorContext(ctx, "PDC Service: Failed to reload strip for timeout", slog.String("callsign", callsign), slog.Any("error", err))
			s.timeoutsMutex.Lock()
			delete(s.timeouts, key)
			s.timeoutsMutex.Unlock()
			return
		}
		if strip.PdcState != string(StateCleared) {
			s.timeoutsMutex.Lock()
			delete(s.timeouts, key)
			s.timeoutsMutex.Unlock()
			return
		}

		err = s.setPdcFailed(ctx, callsign, session.id, StateNoResponse, tracker.cid)
		if err != nil {
			slog.ErrorContext(ctx, "PDC Service: Failed to revert cleared flag", slog.String("callsign", callsign), slog.Any("error", err))
		}

		if !tracker.web {
			seq, err := s.getNextSequence(ctx, session.id)
			if err != nil {
				slog.ErrorContext(ctx, "PDC Service: Failed to get next message sequence", slog.Any("error", err))
			} else {
				msg := buildNoResponseMessage(seq, messageId, airport, callsign)
				err = s.client.SendCPDLC(ctx, session.callsign, callsign, msg)
				if err != nil {
					slog.ErrorContext(ctx, "PDC Service: Failed to send no response message", slog.Any("error", err))
				}
			}
		}

		// Clean up timeout tracker
		s.timeoutsMutex.Lock()
		delete(s.timeouts, key)
		s.timeoutsMutex.Unlock()
	}
}

func (s *Service) setPdcFailed(ctx context.Context, callsign string, sessionId int32, state ClearanceState, cid string) error {
	if cid == "" {
		strip, err := s.stripRepo.GetByCallsign(ctx, sessionId, callsign)
		if err == nil && strip.PdcData != nil && strip.PdcData.IssuedByCid != nil {
			cid = *strip.PdcData.IssuedByCid
		}
	}

	if err := s.stripRepo.UpdatePdcStatus(ctx, sessionId, callsign, string(state)); err != nil {
		return fmt.Errorf("failed to update PDC status: %w", err)
	}

	if s.stripService != nil {
		if err := s.stripService.UnclearStrip(ctx, sessionId, callsign, cid); err != nil {
			slog.ErrorContext(ctx, "PDC Service: Error unclearing strip", slog.Any("error", err))
		}
	}

	if err := s.notifyStateChange(ctx, sessionId, callsign, state, ""); err != nil {
		return err
	}

	return nil
}

// validatePDCFlightPlan validates a strip's flight plan against PDC validation config.
// Returns a list of fault descriptions (empty = no faults).
func (s *Service) validatePDCFlightPlan(strip *models.Strip, activeDepartureRunways []string) []string {
	return validationFaultMessages(validatePDCFlightPlanFaults(strip, activeDepartureRunways, time.Now().UTC()))
}
