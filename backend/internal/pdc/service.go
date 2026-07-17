package pdc

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/dependencies"
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/helpers"
	pkgModels "FlightStrips/pkg/models"
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

type TransceiverLookup interface {
	GetFrequencies(callsign string) []string
}

type FrontendNotifier interface {
	GetAtisCodes(session int32) (arrival string, departure string)
	SendMessage(session int32, sender, text string, recipients []string)
	SendPdcStateChange(session int32, callsign, state, remarks string)
	SendStripUpdate(session int32, callsign string)
}

type EuroscopeCommander interface {
	GetMasterCallsign(session int32) string
	GetMasterCid(session int32) string
	SendClearedFlag(session int32, cid string, callsign string, flag bool)
	SendPdcStateChange(session int32, callsign, state, remarks string)
	SendRoute(session int32, cid string, callsign string, route string)
	SendSid(session int32, cid string, callsign string, sid string)
}

type ServiceDependencies struct {
	Client               HoppieClientInterface
	Sessions             repository.SessionRepository
	Strips               PdcStripStore
	Sectors              repository.SectorOwnerRepository
	Controllers          repository.ControllerRepository
	Frontend             FrontendNotifier
	Euroscope            EuroscopeCommander
	StripService         shared.StripService
	TransceiverProviders []TransceiverLookup
	WebLookupLiveOnly    bool
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
	name     string
	airport  string
	callsign string
}

type PdcRequestTransition string

const (
	PdcRequestTransitionRequested           PdcRequestTransition = "requested"
	PdcRequestTransitionRequestedWithFaults PdcRequestTransition = "requested_with_faults"
	PdcRequestTransitionCleared             PdcRequestTransition = "cleared"
)

type PdcRequestOutcome struct {
	Transition    PdcRequestTransition
	State         ClearanceState
	MetricOutcome string
	Remarks       string
	Faults        []string
	AutoIssue     bool
}

type Service struct {
	client             HoppieClientInterface
	sessionRepo        repository.SessionRepository
	stripRepo          PdcStripStore
	sectorRepo         repository.SectorOwnerRepository
	controllerRepo     repository.ControllerRepository
	frontendHub        FrontendNotifier
	euroscopeHub       EuroscopeCommander
	stripService       shared.StripService
	transceiverLookups []TransceiverLookup
	timeouts           map[string]*timeoutTracker
	timeoutsMutex      sync.RWMutex
	timeoutConfig      time.Duration
	webLookupLiveOnly  bool
}

func NewPDCService(deps ServiceDependencies) (*Service, error) {
	required := []struct {
		name  string
		value any
	}{
		{"Hoppie client", deps.Client},
		{"session repository", deps.Sessions},
		{"strip store", deps.Strips},
		{"sector repository", deps.Sectors},
		{"controller repository", deps.Controllers},
		{"frontend publisher", deps.Frontend},
		{"EuroScope commander", deps.Euroscope},
		{"strip service", deps.StripService},
	}
	for _, dependency := range required {
		if dependencies.IsNil(dependency.value) {
			return nil, fmt.Errorf("pdc service requires %s", dependency.name)
		}
	}
	for i, provider := range deps.TransceiverProviders {
		if dependencies.IsNil(provider) {
			return nil, fmt.Errorf("pdc service transceiver provider %d is nil", i)
		}
	}

	return &Service{
		client:             deps.Client,
		sessionRepo:        deps.Sessions,
		stripRepo:          deps.Strips,
		sectorRepo:         deps.Sectors,
		controllerRepo:     deps.Controllers,
		frontendHub:        deps.Frontend,
		euroscopeHub:       deps.Euroscope,
		stripService:       deps.StripService,
		transceiverLookups: append([]TransceiverLookup(nil), deps.TransceiverProviders...),
		timeouts:           make(map[string]*timeoutTracker),
		timeoutConfig:      10 * time.Minute,
		webLookupLiveOnly:  deps.WebLookupLiveOnly,
	}, nil
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
	return sessionInformation{
		id:       sessionID,
		name:     session.Name,
		airport:  session.Airport,
		callsign: session.Airport,
	}, nil
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
	s.frontendHub.SendPdcStateChange(sessionID, callsign, string(state), remarks)
}

func (s *Service) notifyStateChangeEuroscope(sessionID int32, callsign string, state ClearanceState, remarks string) {
	s.euroscopeHub.SendPdcStateChange(sessionID, callsign, string(state), remarks)
}

func (s *Service) notifyClearedFlagEuroscope(ctx context.Context, sessionID int32, callsign string, cleared bool, cid string) {
	if strings.TrimSpace(cid) == "" {
		cid = s.resolveEuroscopeTargetCID(ctx, sessionID)
	}
	if strings.TrimSpace(cid) == "" {
		slog.WarnContext(ctx, "PDC Service: Unable to resolve EuroScope target for cleared flag",
			slog.Int("session", int(sessionID)),
			slog.String("callsign", callsign),
		)
		return
	}
	s.euroscopeHub.SendClearedFlag(sessionID, cid, callsign, cleared)
}

// notifyStateChange sends PDC state change notification to frontend and EuroScope clients
func (s *Service) notifyStateChange(ctx context.Context, sessionID int32, callsign string, state ClearanceState, remarks string) error {
	if sessionInfo, err := s.getSessionInfo(ctx, sessionID); err == nil {
		metrics.PDCStateChange(context.Background(), sessionInfo.name, sessionInfo.airport, string(state))
	}
	if err := s.stripService.ReevaluatePdcRequestValidations(ctx, sessionID, callsign, true, state == StateRequestedWithFaults); err != nil {
		return err
	}
	s.notifyStateChangeFrontend(sessionID, callsign, state, remarks)
	s.notifyStateChangeEuroscope(sessionID, callsign, state, remarks)
	return nil
}

func (s sessionInformation) recordPDCRequestReceived(ctx context.Context, channel string) {
	metrics.PDCRequestReceived(ctx, s.name, s.airport, channel)
}

func (s sessionInformation) recordPDCRequestOutcome(ctx context.Context, channel, outcome string) {
	metrics.PDCRequestOutcome(ctx, s.name, s.airport, channel, outcome)
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	return &value
}

func (o PdcRequestOutcome) RequestRemarks() *string {
	if o.Remarks == "" {
		return nil
	}

	return &o.Remarks
}

func (s *Service) EvaluatePdcRequest(strip *models.Strip, session *models.Session, remarks string) PdcRequestOutcome {
	faults := s.validatePDCFlightPlan(strip, session.ActiveRunways.DepartureRunways, session.AvailableSids)
	if len(faults) > 0 {
		return PdcRequestOutcome{
			Transition:    PdcRequestTransitionRequestedWithFaults,
			State:         StateRequestedWithFaults,
			MetricOutcome: "requested_with_faults",
			Remarks:       remarks,
			Faults:        faults,
		}
	}

	if strings.TrimSpace(remarks) != "" {
		return PdcRequestOutcome{
			Transition:    PdcRequestTransitionRequested,
			State:         StateRequested,
			MetricOutcome: "requested_manual_review",
			Remarks:       remarks,
		}
	}

	return PdcRequestOutcome{
		Transition:    PdcRequestTransitionCleared,
		State:         StateCleared,
		MetricOutcome: "auto_cleared",
		AutoIssue:     true,
	}
}

func (s *Service) PersistPdcRequestOutcome(ctx context.Context, sessionID int32, callsign string, requestedAt time.Time, outcome PdcRequestOutcome) error {
	if outcome.AutoIssue {
		return nil
	}

	if err := s.stripRepo.SetPdcRequested(ctx, sessionID, callsign, string(outcome.State), &requestedAt, outcome.RequestRemarks()); err != nil {
		return fmt.Errorf("persist PDC request outcome %s: %w", outcome.State, err)
	}
	if err := s.notifyStateChange(ctx, sessionID, callsign, outcome.State, outcome.Remarks); err != nil {
		return fmt.Errorf("notify PDC request outcome %s: %w", outcome.State, err)
	}

	return nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func (s *Service) getDepartureAtisCode(sessionID int32) string {
	_, dep := s.frontendHub.GetAtisCodes(sessionID)
	return strings.TrimSpace(dep)
}

func (s *Service) resolveEuroscopeTargetCID(ctx context.Context, sessionID int32) string {
	if masterCID := strings.TrimSpace(s.euroscopeHub.GetMasterCid(sessionID)); masterCID != "" {
		return masterCID
	}

	masterCallsign := strings.TrimSpace(s.euroscopeHub.GetMasterCallsign(sessionID))
	if masterCallsign == "" {
		return ""
	}

	controller, err := s.controllerRepo.GetByCallsign(ctx, sessionID, masterCallsign)
	if err != nil || controller == nil || controller.Cid == nil {
		return ""
	}

	return strings.TrimSpace(*controller.Cid)
}

func (s *Service) applyMandatoryRouteReview(ctx context.Context, session *models.Session, strip *models.Strip) (*mandatoryRouteReview, error) {
	if session == nil || strip == nil {
		return nil, nil
	}

	review := resolveMandatoryRouteReview(strip, session.AvailableSids)
	if review == nil {
		return nil, nil
	}

	updated := *strip
	changed := false

	if review.Route != "" && !strings.EqualFold(strings.TrimSpace(stringValue(strip.Route)), review.Route) {
		route := review.Route
		updated.Route = &route
		changed = true
	}

	if review.SID != "" && !strings.EqualFold(strings.TrimSpace(stringValue(strip.Sid)), review.SID) {
		sid := review.SID
		updated.Sid = &sid
		changed = true
	}

	if !changed {
		return review, nil
	}

	if _, err := s.stripRepo.Update(ctx, &updated); err != nil {
		return nil, fmt.Errorf("failed to persist mandatory route updates: %w", err)
	}

	strip.Route = updated.Route
	strip.Sid = updated.Sid
	strip.Version = updated.Version

	if targetCID := s.resolveEuroscopeTargetCID(ctx, session.ID); targetCID != "" {
		if review.Route != "" {
			s.euroscopeHub.SendRoute(session.ID, targetCID, strip.Callsign, review.Route)
		}
		if review.SID != "" {
			s.euroscopeHub.SendSid(session.ID, targetCID, strip.Callsign, review.SID)
		}
	}

	s.frontendHub.SendStripUpdate(session.ID, strip.Callsign)

	return review, nil
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

	if err := s.stripService.ConfirmPdcClearance(ctx, sessionID, strip.Callsign, clearedBay, clearanceCid); err != nil {
		return fmt.Errorf("failed to confirm strip clearance: %w", err)
	}

	if err := s.stripRepo.SetPdcData(ctx, sessionID, strip.Callsign, pdcData); err != nil {
		return fmt.Errorf("failed to persist confirmed PDC data: %w", err)
	}

	if !strip.Cleared {
		if err := s.stripService.AutoAssumeForClearedStripByCid(ctx, sessionID, strip.Callsign, clearanceCid); err != nil {
			slog.ErrorContext(ctx, "Failed to auto-assume confirmed PDC strip", slog.Any("error", err))
		}
	}

	s.CancelTimeout(strip.Callsign, sessionID)

	if err := s.stripService.ReevaluatePdcInvalidValidation(ctx, sessionID, strip.Callsign, true, false); err != nil {
		return fmt.Errorf("failed to reevaluate PDC invalid validation: %w", err)
	}

	s.frontendHub.SendStripUpdate(sessionID, strip.Callsign)

	if sessionInfo, err := s.getSessionInfo(context.Background(), sessionID); err == nil {
		metrics.PDCStateChange(context.Background(), sessionInfo.name, sessionInfo.airport, string(StateConfirmed))
	}
	s.notifyStateChangeEuroscope(sessionID, strip.Callsign, StateConfirmed, "")
	s.notifyClearedFlagEuroscope(ctx, sessionID, strip.Callsign, true, clearanceCid)

	s.notifyStateChangeFrontend(sessionID, strip.Callsign, StateConfirmed, "")

	strip.PdcData = pdcData
	strip.PdcState = string(StateConfirmed)
	strip.PdcRequestRemarks = nil

	return nil
}

// Start begins the background polling loop
func (s *Service) Start(ctx context.Context) {
	if restored, err := s.restorePendingTimeouts(ctx); err != nil {
		slog.ErrorContext(ctx, "PDC Service: Failed to restore pending confirmation timeouts", slog.Any("error", err))
	} else if restored > 0 {
		slog.InfoContext(ctx, "PDC Service: Restored pending confirmation timeouts", slog.Int("count", restored))
	}

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

func (s *Service) restorePendingTimeouts(ctx context.Context) (int, error) {
	sessions, err := s.sessionRepo.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list sessions for timeout recovery: %w", err)
	}

	restored := 0
	for _, session := range sessions {
		strips, err := s.stripRepo.List(ctx, session.ID)
		if err != nil {
			return restored, fmt.Errorf("failed to list strips for session %d: %w", session.ID, err)
		}

		sessionInfo := sessionInformation{
			id:       session.ID,
			name:     session.Name,
			airport:  session.Airport,
			callsign: session.Airport,
		}

		for _, strip := range strips {
			if strip == nil || strip.PdcState != string(StateCleared) || strip.PdcData == nil {
				continue
			}
			if strip.PdcData.MessageSent == nil || strip.PdcData.MessageSequence == nil {
				slog.WarnContext(ctx, "PDC Service: Skipping timeout recovery for cleared strip with incomplete PDC data",
					slog.String("callsign", strip.Callsign),
					slog.Int("session", int(session.ID)),
				)
				continue
			}

			remaining := s.timeoutConfig - time.Since(strip.PdcData.MessageSent.UTC())
			if remaining < 0 {
				remaining = 0
			}

			cid := valueOrEmpty(strip.PdcData.IssuedByCid)
			s.startClearanceTimeoutAfter(ctx, *strip.PdcData.MessageSequence, sessionInfo, strip.Callsign, cid, isWebPDCRequest(strip), remaining)
			restored++
		}
	}

	return restored, nil
}

func (s *Service) pollAndProcess(ctx context.Context) error {
	sessions, err := s.sessionRepo.GetByNames(ctx, "LIVE")
	if err != nil {
		return fmt.Errorf("failed to get sessions: %w", err)
	}

	for _, session := range sessions {
		err = s.pollAndProcessForSession(ctx, sessionInformation{
			id:       session.ID,
			name:     session.Name,
			airport:  session.Airport,
			callsign: session.Airport,
		})
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
	session.recordPDCRequestReceived(ctx, models.PdcChannelCPDLC)
	req, err := parsePDCRequest(msg.Payload)
	if err != nil {
		session.recordPDCRequestOutcome(ctx, models.PdcChannelCPDLC, "rejected")
		if sendErr := s.SendErrorMessage(ctx, session, msg.From); sendErr != nil {
			return fmt.Errorf("failed to parse PDC request %w and sending error message %w", err, sendErr)
		}
		return fmt.Errorf("failed to parse PDC request: %w", err)
	}

	if req.Callsign != msg.From {
		session.recordPDCRequestOutcome(ctx, models.PdcChannelCPDLC, "rejected")
		return fmt.Errorf("callsign in request does not match from field in message")
	}

	slog.DebugContext(ctx, "PDC Service: Processing PDC request", slog.String("callsign", req.Callsign), slog.String("departure", req.Departure))

	strip, err := s.stripRepo.GetByCallsign(ctx, session.id, req.Callsign)

	if errors.Is(err, sql.ErrNoRows) {
		session.recordPDCRequestOutcome(ctx, models.PdcChannelCPDLC, "rejected")
		return s.sendErrorAndReturn(ctx, session, req.Callsign,
			fmt.Errorf("strip not found"),
			func(seq int32) string { return buildFlightPlanNotHeld(seq, req.Departure, req.Callsign) })
	}

	if err != nil {
		session.recordPDCRequestOutcome(ctx, models.PdcChannelCPDLC, "rejected")
		return s.sendErrorAndReturn(ctx, session, req.Callsign, err, buildPDCUnavailable)
	}

	if strip.Cleared {
		session.recordPDCRequestOutcome(ctx, models.PdcChannelCPDLC, "rejected")
		return s.sendErrorAndReturn(ctx, session, req.Callsign,
			fmt.Errorf("aircraft already cleared"),
			func(seq int32) string { return buildAlreadyCleared(seq, strip.Origin, req.Callsign) })
	}

	if !strings.EqualFold(strip.Origin, req.Departure) || !strings.EqualFold(strip.Destination, req.Destination) {
		session.recordPDCRequestOutcome(ctx, models.PdcChannelCPDLC, "rejected")
		return s.sendErrorAndReturn(ctx, session, req.Callsign,
			fmt.Errorf("invalid PDC request: origin/destination mismatch"),
			func(seq int32) string { return buildFlightPlanNotHeld(seq, strip.Origin, req.Callsign) })
	}

	stripAircraftType := normalizedStripAircraftType(strip)
	if !stripAircraftTypeMatches(strip, req.Aircraft) {
		session.recordPDCRequestOutcome(ctx, models.PdcChannelCPDLC, "rejected")
		return s.sendErrorAndReturn(ctx, session, req.Callsign,
			fmt.Errorf("aircraft type mismatch: expected %s, got %s", stripAircraftType, req.Aircraft),
			func(seq int32) string { return buildInvalidAircraftType(seq, strip.Origin, req.Callsign) })
	}

	currentSession, err := s.sessionRepo.GetByID(ctx, session.id)
	if err != nil {
		return s.sendErrorAndReturn(ctx, session, req.Callsign, err, buildPDCUnavailable)
	}

	outcome := s.EvaluatePdcRequest(strip, currentSession, req.Remarks)
	requestedAt := time.Now().UTC()
	if !outcome.AutoIssue {
		session.recordPDCRequestOutcome(ctx, models.PdcChannelCPDLC, outcome.MetricOutcome)
		if err := s.PersistPdcRequestOutcome(ctx, session.id, strip.Callsign, requestedAt, outcome); err != nil {
			return err
		}
		if err := s.SendStatusAck(ctx, session, req.Callsign, req.Departure); err != nil {
			return fmt.Errorf("failed to send status ack: %w", err)
		}
		if outcome.Transition == PdcRequestTransitionRequestedWithFaults {
			slog.InfoContext(ctx, "PDC Service: PDC request has faults", slog.String("callsign", req.Callsign), slog.Any("faults", outcome.Faults))
		} else {
			slog.InfoContext(ctx, "PDC Service: PDC request requires manual review due to request remarks", slog.String("callsign", req.Callsign))
		}
		return nil
	}

	if issueErr := s.IssueClearance(ctx, strip.Callsign, "", "", session.id); issueErr != nil {
		fallbackOutcome := PdcRequestOutcome{
			Transition:    PdcRequestTransitionRequested,
			State:         StateRequested,
			MetricOutcome: "requested_pending_clearance",
		}
		session.recordPDCRequestOutcome(ctx, models.PdcChannelCPDLC, fallbackOutcome.MetricOutcome)
		if err := s.PersistPdcRequestOutcome(ctx, session.id, strip.Callsign, requestedAt, fallbackOutcome); err != nil {
			return err
		}
		if err := s.SendStatusAck(ctx, session, req.Callsign, req.Departure); err != nil {
			return fmt.Errorf("failed to send status ack: %w", err)
		}
		slog.InfoContext(ctx, "PDC Service: PDC request acknowledged (clearance fields not ready)", slog.String("callsign", req.Callsign))
	} else {
		session.recordPDCRequestOutcome(ctx, models.PdcChannelCPDLC, outcome.MetricOutcome)
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

	sessionData, err := s.sessionRepo.GetByID(ctx, sessionInfo.id)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	mandatoryRouteReview, err := s.applyMandatoryRouteReview(ctx, sessionData, strip)
	if err != nil {
		return err
	}

	webDelivery := isWebPDCRequest(strip)

	options, err := s.BuildClearanceOptions(ctx, sessionInfo, strip, remarks, webDelivery, mandatoryRouteReview)
	if err != nil {
		return err
	}
	if err := s.deliverPdcClearance(ctx, sessionInfo, callsign, options, webDelivery); err != nil {
		return err
	}
	if err := s.persistIssuedPdcClearance(ctx, sessionInfo.id, callsign, cid, options, webDelivery); err != nil {
		return err
	}

	s.moveIssuedPdcStrip(ctx, sessionInfo.id, callsign)
	s.StartClearanceTimeout(ctx, strip.Origin, callsign, options.Sequence, sessionInfo, cid, webDelivery)
	if err := s.notifyIssuedPdcClearance(ctx, sessionInfo.id, callsign); err != nil {
		return err
	}

	slog.DebugContext(ctx, "PDC Service: Clearance issued", slog.String("callsign", callsign), slog.Int("sequence", int(options.Sequence)))
	return nil
}

func (s *Service) BuildClearanceOptions(ctx context.Context, sessionInfo sessionInformation, strip *models.Strip, remarks string, webDelivery bool, mandatoryRouteReview *mandatoryRouteReview) (ClearanceOptions, error) {
	if strip.Runway == nil || (strip.Sid == nil && strip.Heading == nil) {
		return ClearanceOptions{}, fmt.Errorf("strip missing required clearance data (runway and SID or heading)")
	}

	clearanceSquawk, err := getAssignedPDCSquawk(strip)
	if err != nil {
		return ClearanceOptions{}, fmt.Errorf("strip missing required clearance data: %w", err)
	}

	nextFreq, err := s.getNextFrequency(ctx, sessionInfo.id)
	if err != nil {
		return ClearanceOptions{}, fmt.Errorf("failed to get next frequency: %w", err)
	}

	departureFreq, err := s.getAirborneFrequency(ctx, sessionInfo.id, strip.Sid)
	if err != nil {
		return ClearanceOptions{}, fmt.Errorf("failed to get departure frequency: %w", err)
	}

	nextPdcSeq, err := s.sessionRepo.IncrementPdcSequence(ctx, sessionInfo.id)
	if err != nil {
		return ClearanceOptions{}, fmt.Errorf("failed to get next PDC sequence: %w", err)
	}

	nextSeq, err := s.getNextSequence(ctx, sessionInfo.id)
	if err != nil {
		return ClearanceOptions{}, err
	}

	options := ClearanceOptions{
		Callsign:           strip.Callsign,
		Origin:             strip.Origin,
		Destination:        strip.Destination,
		Runway:             *strip.Runway,
		Squawk:             clearanceSquawk,
		NextFrequency:      nextFreq,
		DepartureFrequency: departureFreq,
		Sequence:           nextSeq,
		PdcSequence:        nextPdcSeq,
		Remarks:            remarks,
	}

	if depAtis := s.getDepartureAtisCode(sessionInfo.id); depAtis != "" {
		options.Atis = depAtis
	}

	if webDelivery && strip.PdcData.Web.Atis != nil && strings.TrimSpace(*strip.PdcData.Web.Atis) != "" {
		options.Atis = strings.TrimSpace(*strip.PdcData.Web.Atis)
	}

	if options.Atis == "" {
		slog.InfoContext(ctx,
			"PDC Service: Omitting ATIS letter from clearance because none is available",
			slog.String("callsign", strip.Callsign),
			slog.Int("session", int(sessionInfo.id)),
			slog.Bool("web_delivery", webDelivery),
		)
	}

	if strip.Heading != nil && *strip.Heading != 0 && strip.ClearedAltitude != nil && *strip.ClearedAltitude > 0 {
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

	if mandatoryRouteReview != nil {
		if strings.TrimSpace(mandatoryRouteReview.SID) == "" {
			return ClearanceOptions{}, fmt.Errorf("mandatory route %s requires a matching SID before PDC can be issued", mandatoryRouteReview.Route)
		}
		options.SID = mandatoryRouteReview.SID
		options.Route = mandatoryRouteReview.Route
	}

	return options, nil
}

func (s *Service) deliverPdcClearance(ctx context.Context, sessionInfo sessionInformation, callsign string, options ClearanceOptions, webDelivery bool) error {
	if webDelivery {
		return nil
	}

	if err := s.client.SendCPDLC(ctx, sessionInfo.callsign, callsign, buildPDCClearance(options)); err != nil {
		return fmt.Errorf("failed to send clearance: %w", err)
	}

	return nil
}

func (s *Service) persistIssuedPdcClearance(ctx context.Context, sessionID int32, callsign, cid string, options ClearanceOptions, webDelivery bool) error {
	now := time.Now().UTC()
	if err := s.stripRepo.SetPdcMessageSent(ctx, sessionID, callsign, string(StateCleared), &options.Sequence, &now); err != nil {
		return fmt.Errorf("failed to set PDC message sent: %w", err)
	}

	if err := s.updatePdcData(ctx, sessionID, callsign, func(pdcData *models.PdcData) {
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

	return nil
}

func (s *Service) moveIssuedPdcStrip(ctx context.Context, sessionID int32, callsign string) {
	if err := s.stripService.MoveToBay(ctx, sessionID, callsign, shared.BAY_CLEARED, true); err != nil {
		slog.ErrorContext(ctx, "PDC Service: Warning - failed to move strip to cleared bay", slog.Any("error", err))
	}
	s.stripService.ClearMandatoryRouteCdm(ctx, sessionID, callsign)
}

func (s *Service) notifyIssuedPdcClearance(ctx context.Context, sessionID int32, callsign string) error {
	if err := s.notifyStateChange(ctx, sessionID, callsign, StateCleared, ""); err != nil {
		return fmt.Errorf("failed to notify cleared state change: %w", err)
	}

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

func (s *Service) ConfirmVoiceClearance(ctx context.Context, callsign string, sessionID int32) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, sessionID, callsign)
	if err != nil {
		return fmt.Errorf("failed to get strip: %w", err)
	}

	if strip.PdcState == "" || strip.PdcState == string(StateNone) {
		return nil
	}

	s.CancelTimeout(callsign, sessionID)

	if err := s.stripRepo.SetPdcData(ctx, sessionID, callsign, (&models.PdcData{}).Normalize()); err != nil {
		return fmt.Errorf("failed to clear PDC state after voice clearance: %w", err)
	}

	if err := s.notifyStateChange(ctx, sessionID, callsign, StateNone, ""); err != nil {
		return err
	}

	recipients := []string{}
	if _, ok := config.GetMessageAreas()["CLR-DEL"]; ok {
		recipients = []string{"CLR-DEL"}
	}
	s.frontendHub.SendMessage(sessionID, "SYSTEM", fmt.Sprintf("%s: CLEARANCE given / confirmed over voice.", callsign), recipients)

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

	controllers, err := s.controllerRepo.ListBySession(ctx, sessionID)
	if err == nil {
		for _, controller := range controllers {
			if !shared.IsOperationalController(controller) {
				continue
			}
			if position, ok := resolvePdcOperationalPosition(controller); ok && shared.IsOperationalControllerForPosition(controller, position) {
				addNormalizedFrequency(onlineFreqs, position.Frequency)
			} else {
				addNormalizedFrequency(onlineFreqs, controller.Position)
			}
			for _, provider := range s.transceiverLookups {
				for _, frequency := range provider.GetFrequencies(controller.Callsign) {
					addNormalizedFrequency(onlineFreqs, frequency)
				}
			}
		}
	}
	if len(onlineFreqs) > 0 {
		return onlineFreqs
	}

	owners, err := s.sectorRepo.ListBySession(ctx, sessionID)
	if err != nil {
		return onlineFreqs
	}

	for _, owner := range owners {
		addNormalizedFrequency(onlineFreqs, owner.Position)
	}

	return onlineFreqs
}

func resolvePdcOperationalPosition(controller *models.Controller) (*config.Position, bool) {
	if controller == nil {
		return nil, false
	}

	if position, err := config.GetPositionByName(controller.Callsign); err == nil {
		return position, true
	}

	if position, err := config.GetPositionBasedOnFrequency(controller.Position); err == nil {
		return position, true
	}

	return nil, false
}

func addNormalizedFrequency(frequencies map[string]struct{}, frequency string) {
	if normalizedFrequency := normalizeFrequency(frequency); normalizedFrequency != "" {
		frequencies[normalizedFrequency] = struct{}{}
	}
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
func (s *Service) StartClearanceTimeout(ctx context.Context, _ string, callsign string, messageId int32, session sessionInformation, cid string, web bool) {
	s.startClearanceTimeoutAfter(ctx, messageId, session, callsign, cid, web, s.timeoutConfig)
}

func (s *Service) startClearanceTimeoutAfter(ctx context.Context, messageId int32, session sessionInformation, callsign string, cid string, web bool, delay time.Duration) {
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
	go s.handleTimeout(timeoutCtx, delay, callsign, messageId, session)

	slog.DebugContext(ctx, "PDC Service: Started timeout", slog.Duration("timeout", delay), slog.String("callsign", callsign))
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
func (s *Service) handleTimeout(ctx context.Context, delay time.Duration, callsign string, messageId int32, session sessionInformation) {
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
	case <-time.After(delay):
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
				msg := buildNoResponseMessage(seq, messageId, session.airport, callsign)
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

	if err := s.stripService.UnclearStrip(ctx, sessionId, callsign, cid); err != nil {
		slog.ErrorContext(ctx, "PDC Service: Error unclearing strip", slog.Any("error", err))
	}

	if err := s.notifyStateChange(ctx, sessionId, callsign, state, ""); err != nil {
		return err
	}

	return nil
}

// validatePDCFlightPlan validates a strip's flight plan against PDC validation config.
// Returns a list of fault descriptions (empty = no faults).
func (s *Service) validatePDCFlightPlan(strip *models.Strip, activeDepartureRunways []string, availableSids pkgModels.AvailableSids) []string {
	return validationFaultMessages(validatePDCFlightPlanFaults(strip, activeDepartureRunways, availableSids))
}
