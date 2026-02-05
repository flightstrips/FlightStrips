package pdc

import (
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
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
}

type sessionInformation struct {
	id       int32
	callsign string
}

type Service struct {
	client        HoppieClientInterface
	sessionRepo   repository.SessionRepository
	stripRepo     repository.StripRepository
	sectorRepo    repository.SectorOwnerRepository
	frontendHub   shared.FrontendHub
	stripService  shared.StripService
	timeouts      map[string]*timeoutTracker
	timeoutsMutex sync.RWMutex
	timeoutConfig time.Duration
}

func NewPDCService(client *Client, sessionRepo repository.SessionRepository, stripRepo repository.StripRepository, sectorRepo repository.SectorOwnerRepository) *Service {
	return &Service{
		client:        client,
		sessionRepo:   sessionRepo,
		stripRepo:     stripRepo,
		sectorRepo:    sectorRepo,
		timeouts:      make(map[string]*timeoutTracker),
		timeoutConfig: 10 * time.Minute, // Default 10 minutes
	}
}

func (s *Service) SetFrontendHub(frontendHub shared.FrontendHub) {
	s.frontendHub = frontendHub
}

func (s *Service) SetStripService(stripService shared.StripService) {
	s.stripService = stripService
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

// notifyFrontendStateChange sends PDC state change notification to frontend
func (s *Service) notifyFrontendStateChange(sessionID int32, callsign string, state ClearanceState) {
	if s.frontendHub != nil {
		s.frontendHub.SendPdcStateChange(sessionID, callsign, string(state))
	}
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

	if !strings.EqualFold(strip.Origin, req.Departure) || !strings.EqualFold(strip.Destination, req.Destination) {
		return s.sendErrorAndReturn(ctx, session, req.Callsign,
			fmt.Errorf("invalid PDC request: origin/destination mismatch"),
			func(seq int32) string { return buildFlightPlanNotHeld(seq, strip.Origin, req.Callsign) })
	}

	if strip.AircraftType == nil || !strings.EqualFold(*strip.AircraftType, req.Aircraft) {
		return s.sendErrorAndReturn(ctx, session, req.Callsign,
			fmt.Errorf("aircraft type mismatch: expected %s, got %s", *strip.AircraftType, req.Aircraft),
			func(seq int32) string { return buildInvalidAircraftType(seq, strip.Origin, req.Callsign) })
	}

	now := time.Now().UTC()
	err = s.stripRepo.SetPdcRequested(ctx, session.id, strip.Callsign, string(StateRequested), &now)
	if err != nil {
		return fmt.Errorf("failed to set PDC requested: %w", err)
	}

	// Send status acknowledgement
	if err := s.SendStatusAck(ctx, session, req.Callsign, req.Departure); err != nil {
		return fmt.Errorf("failed to send status ack: %w", err)
	}

	s.notifyFrontendStateChange(session.id, req.Callsign, StateRequested)

	slog.InfoContext(ctx, "PDC Service: PDC request acknowledged", slog.String("callsign", req.Callsign))
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

	// Validate required clearance fields are present in strip
	if strip.Runway == nil || strip.Squawk == nil || (strip.Sid == nil && strip.Heading == nil) || strip.AssignedSquawk == nil {
		return fmt.Errorf("strip missing required clearance data (runway/squawk/sid/squawk)")
	}

	nextFreq, err := s.getNextFrequency(ctx, sessionInfo.id)
	if err != nil {
		return fmt.Errorf("failed to get next frequency: %w", err)
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
		Callsign:      strip.Callsign,
		Origin:        strip.Origin,
		Destination:   strip.Destination,
		Atis:          "A",
		Runway:        *strip.Runway,
		Squawk:        *strip.Squawk,
		NextFrequency: nextFreq,
		Sequence:      nextSeq,
		PdcSequence:   nextPdcSeq,
		Remarks:       remarks,
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

	if err := s.client.SendCPDLC(ctx, sessionInfo.callsign, callsign, clearance); err != nil {
		return fmt.Errorf("failed to send clearance: %w", err)
	}

	now := time.Now().UTC()
	err = s.stripRepo.SetPdcMessageSent(ctx, sessionInfo.id, callsign, string(StateCleared), &nextSeq, &now)

	if err != nil {
		return fmt.Errorf("failed to set PDC message sent: %w", err)
	}

	if s.stripService != nil {
		if err := s.stripService.ClearStrip(ctx, sessionInfo.id, callsign, cid); err != nil {
			slog.ErrorContext(ctx, "PDC Service: Warning - failed to clear strip", slog.Any("error", err))
		}
	}

	s.StartClearanceTimeout(ctx, strip.Origin, callsign, nextSeq, sessionInfo, cid)

	s.notifyFrontendStateChange(sessionInfo.id, callsign, StateCleared)

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

	s.notifyFrontendStateChange(sessionInfo.id, callsign, StateRevertToVoice)

	slog.DebugContext(ctx, "PDC Service: Reverting to voice", slog.String("callsign", callsign))
	return nil
}

func (s *Service) getNextFrequency(ctx context.Context, sessionID int32) (string, error) {
	owners, err := s.sectorRepo.ListBySession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get sector owners: %w", err)
	}

	// Find the owner of SQ
	nextFrequency := ""
	for _, owner := range owners {
		if slices.Contains(owner.Sector, "SQ") {
			nextFrequency = owner.Position
		}
	}

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

	err = s.stripRepo.UpdatePdcStatus(ctx, session.id, callsign, string(StateConfirmed))
	if err != nil {
		return fmt.Errorf("failed to update PDC status: %w", err)
	}

	s.CancelTimeout(callsign, session.id)

	s.notifyFrontendStateChange(session.id, callsign, StateConfirmed)

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

		slog.Info("PDC manual state change", slog.String("state", string(newState)), slog.String("callsign", callsign))
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
func (s *Service) StartClearanceTimeout(ctx context.Context, airport, callsign string, messageId int32, session sessionInformation, cid string) {
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
	}

	s.timeoutsMutex.Lock()
	s.timeouts[key] = tracker
	s.timeoutsMutex.Unlock()

	// Start timeout goroutine
	go s.handleTimeout(timeoutCtx, airport, callsign, messageId, session)

	slog.Debug("PDC Service: Started timeout", slog.Duration("timeout", s.timeoutConfig), slog.String("callsign", callsign))
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

		err := s.setPdcFailed(ctx, callsign, session.id, StateNoResponse, tracker.cid)
		if err != nil {
			slog.ErrorContext(ctx, "PDC Service: Failed to revert cleared flag", slog.String("callsign", callsign), slog.Any("error", err))
		}

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

		// Clean up timeout tracker
		s.timeoutsMutex.Lock()
		delete(s.timeouts, key)
		s.timeoutsMutex.Unlock()
	}
}

func (s *Service) setPdcFailed(ctx context.Context, callsign string, sessionId int32, state ClearanceState, cid string) error {
	err := s.stripRepo.UpdatePdcStatus(ctx, sessionId, callsign, string(state))
	if err != nil {
		slog.ErrorContext(ctx, "PDC Service: Failed to update PDC status", slog.Any("error", err))
	}

	if s.stripService != nil {
		if err := s.stripService.UnclearStrip(ctx, sessionId, callsign, cid); err != nil {
			slog.ErrorContext(ctx, "PDC Service: Error unclearing strip", slog.Any("error", err))
		}
	}

	s.notifyFrontendStateChange(sessionId, callsign, state)

	return nil
}
