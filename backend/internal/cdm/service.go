package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type Service struct {
	actionService          *ActionService
	syncService            *SyncService
	masterViffSync         *MasterViffSync
	broadcaster            *CdmBroadcaster
	recalculationScheduler *RecalculationScheduler
	client                 *Client
	stripRepo              CdmStripStore
	sessionRepo            repository.SessionRepository
	controllerRepo         repository.ControllerRepository
	publisher              shared.CdmEventPublisher
	euroscopeHub           shared.EuroscopeHub
	configProvider         ConfigProvider
	sequenceService        *SequenceService
	validationReevaluator  StripValidationReevaluator
	debouncer              *recalcDebouncer
	// sessionMaster tracks per-session CDM master status as an in-memory cache.
	// Populated from session.CdmMaster during syncSessions and updated
	// immediately when SetSessionCdmMaster is called.
	sessionMaster sync.Map // map[int32]bool
	// sessionUsesViff tracks whether a session is allowed to exchange data with the vIFF network.
	// Populated from session.Name during syncSessions and refreshed on demand for later runtime calls.
	sessionUsesViff sync.Map // map[int32]bool
	lastPushedViff  sync.Map // map[string]viffPushState
}

type StripValidationReevaluator interface {
	ReevaluateCtotValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error
	ReevaluateCtotValidationsForSession(ctx context.Context, session int32, publish bool) error
}

const DefaultMasterPosition = "FlightStrips"

const (
	cdmSyncInterval           = 30 * time.Second
	cdmPeriodicRecalcInterval = time.Minute
	masterEobtClampThreshold  = 45.0
	masterEobtClampTarget     = 30.0
)

func NewCdmService(client *Client, stripRepo CdmStripStore, sessionRepo repository.SessionRepository, controllerRepo repository.ControllerRepository) *Service {
	service := &Service{
		client:         client,
		stripRepo:      stripRepo,
		sessionRepo:    sessionRepo,
		controllerRepo: controllerRepo,
		debouncer:      newRecalcDebouncer(500 * time.Millisecond),
	}
	service.actionService = &ActionService{service: service}
	service.syncService = &SyncService{service: service}
	service.masterViffSync = &MasterViffSync{service: service}
	service.broadcaster = &CdmBroadcaster{service: service}
	service.recalculationScheduler = &RecalculationScheduler{service: service}
	return service
}

func detachedContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithoutCancel(ctx)
}

// Facade methods delegate to focused collaborators while preserving the public Service API.
func (s *Service) TriggerRecalculate(ctx context.Context, session int32, airport string) {
	s.recalculationScheduler.TriggerRecalculate(ctx, session, airport)
}

func (s *Service) SyncAirportLvoFromRunwayStatus(ctx context.Context, airport string, runwayStatus map[string]string) {
	s.syncService.SyncAirportLvoFromRunwayStatus(ctx, airport, runwayStatus)
}

func (s *Service) TriggerRecalculateForAirport(ctx context.Context, airport string) error {
	return s.recalculationScheduler.TriggerRecalculateForAirport(ctx, airport)
}

func (s *Service) HandleTobtUpdate(ctx context.Context, session int32, callsign string, tobt string, sourcePosition string, sourceRole string) error {
	return s.actionService.HandleTobtUpdate(ctx, session, callsign, tobt, sourcePosition, sourceRole)
}

func (s *Service) HandleEobtUpdate(ctx context.Context, session int32, callsign string, eobt string, sourcePosition string, sourceRole string) error {
	return s.actionService.HandleEobtUpdate(ctx, session, callsign, eobt, sourcePosition, sourceRole)
}

func (s *Service) PrepareEuroscopeEobtSync(session int32, data *models.CdmData, eobt string, now time.Time) (*models.CdmData, string, bool) {
	return s.actionService.PrepareEuroscopeEobtSync(session, data, eobt, now)
}

func (s *Service) normalizeMasterEobtValue(session int32, eobt string, now time.Time) (string, bool) {
	return s.actionService.normalizeMasterEobtValue(session, eobt, now)
}

func (s *Service) normalizeExistingMasterSessionEobts(ctx context.Context, session int32, airport string, now time.Time) (bool, error) {
	return s.actionService.normalizeExistingMasterSessionEobts(ctx, session, airport, now)
}

func (s *Service) normalizeMasterLookupEobts(ctx context.Context, session int32, lookup map[string]*models.CdmData, now time.Time) (bool, error) {
	return s.actionService.normalizeMasterLookupEobts(ctx, session, lookup, now)
}

func (s *Service) normalizeMasterFlightEobt(ctx context.Context, session int32, callsign string, data *models.CdmData, now time.Time) (bool, error) {
	return s.actionService.normalizeMasterFlightEobt(ctx, session, callsign, data, now)
}

func (s *Service) HandleClxTobtUpdate(ctx context.Context, session int32, callsign string, tobt string, sourcePosition string, sourceRole string) error {
	return s.actionService.HandleClxTobtUpdate(ctx, session, callsign, tobt, sourcePosition, sourceRole)
}

func (s *Service) HandleDeiceUpdate(ctx context.Context, session int32, callsign string, deiceType string) error {
	return s.actionService.HandleDeiceUpdate(ctx, session, callsign, deiceType)
}

func (s *Service) HandleAsrtToggle(ctx context.Context, session int32, callsign string, asrt string) error {
	return s.actionService.HandleAsrtToggle(ctx, session, callsign, asrt)
}

func (s *Service) HandleTsacUpdate(ctx context.Context, session int32, callsign string, tsac string) error {
	return s.actionService.HandleTsacUpdate(ctx, session, callsign, tsac)
}

func (s *Service) HandleManualCtot(ctx context.Context, session int32, callsign string, ctot string) error {
	return s.actionService.HandleManualCtot(ctx, session, callsign, ctot)
}

func (s *Service) HandleCtotRemove(ctx context.Context, session int32, callsign string) error {
	return s.actionService.HandleCtotRemove(ctx, session, callsign)
}

func (s *Service) HandleApproveReqTobt(ctx context.Context, session int32, callsign string, sourcePosition string, sourceRole string) error {
	return s.actionService.HandleApproveReqTobt(ctx, session, callsign, sourcePosition, sourceRole)
}

func (s *Service) clearReqTobtAsync(session int32, callsign string) {
	s.actionService.clearReqTobtAsync(context.Background(), session, callsign)
}

func (s *Service) HandleReadyRequest(ctx context.Context, session int32, callsign string, sourcePosition string, sourceRole string) error {
	return s.actionService.HandleReadyRequest(ctx, session, callsign, sourcePosition, sourceRole)
}

func (s *Service) SetReady(ctx context.Context, session int32, callsign string) error {
	return s.actionService.SetReady(ctx, session, callsign)
}

func (s *Service) RequestBetterTobt(ctx context.Context, session int32, callsign string) error {
	return s.actionService.RequestBetterTobt(ctx, session, callsign)
}

func (s *Service) PushTobt(ctx context.Context, session int32, callsign string, tobt string) error {
	return s.actionService.PushTobt(ctx, session, callsign, tobt)
}

func (s *Service) Start(ctx context.Context) {
	s.syncService.Start(ctx)
}

func (s *Service) syncSessions(ctx context.Context) error {
	return s.syncService.syncSessions(ctx)
}

func (s *Service) schedulePeriodicRecalculate(ctx context.Context) error {
	return s.recalculationScheduler.schedulePeriodicRecalculate(ctx)
}

func (s *Service) syncCdmData(ctx context.Context, session *models.Session) error {
	return s.syncService.syncCdmData(ctx, session)
}

func (s *Service) ensureMasterFlightExport(ctx context.Context, session int32, callsign string, local *models.CdmData, remote IFPSData) {
	s.masterViffSync.ensureMasterFlightExport(ctx, session, callsign, local, remote)
}

func (s *Service) mergeMasterViffFlight(ctx context.Context, session int32, callsign string, flight *models.CdmData, row IFPSData, nextCtot string, nextCtotSource string) (*models.CdmData, bool, error) {
	return s.masterViffSync.mergeMasterViffFlight(ctx, session, callsign, flight, row, nextCtot, nextCtotSource)
}

func (s *Service) reevaluateCtotValidationAsync(ctx context.Context, session int32, callsign string, before, after cdmSnapshot) {
	s.syncService.reevaluateCtotValidationAsync(ctx, session, callsign, before, after)
}

func (s *Service) schedulePeriodicCtotValidationReevaluation(ctx context.Context) error {
	return s.syncService.schedulePeriodicCtotValidationReevaluation(ctx)
}

func (s *Service) broadcastIfChanged(session int32, callsign string, before, after cdmSnapshot) {
	s.broadcaster.broadcastIfChanged(session, callsign, before, after)
}

func (s *Service) pushViffDataAfterRecalc(ctx context.Context, session int32, callsign string) {
	s.masterViffSync.pushViffDataAfterRecalc(ctx, session, callsign)
}

func (s *Service) pushCdmDataAfterRecalc(ctx context.Context, session int32, callsign string) {
	s.masterViffSync.pushCdmDataAfterRecalc(ctx, session, callsign)
}

func (s *Service) pushViffAfterRecalcAsync(session int32, callsign string, strip *models.Strip, data *models.CdmData) {
	s.masterViffSync.pushViffAfterRecalcAsync(context.Background(), session, callsign, strip, data)
}

func (s *Service) pushViffAfterRecalc(ctx context.Context, callsign string, strip *models.Strip, data *models.CdmData) error {
	return s.masterViffSync.pushViffAfterRecalc(ctx, callsign, strip, data)
}

func (s *Service) pushViffState(ctx context.Context, callsign string, state viffPushState) error {
	return s.masterViffSync.pushViffState(ctx, callsign, state)
}

func (s *Service) markViffPushPending(session int32, callsign string, state viffPushState) bool {
	return s.masterViffSync.markViffPushPending(session, callsign, state)
}

func (s *Service) clearPendingViffPush(session int32, callsign string, state viffPushState) {
	s.masterViffSync.clearPendingViffPush(session, callsign, state)
}

func (s *Service) pushLatestMasterCdmDataToViff(ctx context.Context, session int32, callsign string, strip *models.Strip) error {
	return s.masterViffSync.pushLatestMasterCdmDataToViff(ctx, session, callsign, strip)
}

func (s *Service) refreshMasterFlightFromViff(ctx context.Context, session int32, callsign string, airport string) error {
	return s.masterViffSync.refreshMasterFlightFromViff(ctx, session, callsign, airport)
}

func (s *Service) loadCdmActionTarget(ctx context.Context, session int32, callsign string) (*models.Strip, *models.CdmData, error) {
	return s.actionService.loadCdmActionTarget(ctx, session, callsign)
}

func (s *Service) prepareTobtUpdate(ctx context.Context, session int32, callsign string, tobt string, sourcePosition string, now time.Time) (*models.Strip, cdmSnapshot, *models.CdmData, string, bool, bool, error) {
	return s.actionService.prepareTobtUpdate(ctx, session, callsign, tobt, sourcePosition, "ATC", now)
}

func (s *Service) SyncAsatForGroundState(ctx context.Context, session int32, callsign string, groundState string) error {
	return s.actionService.SyncAsatForGroundState(ctx, session, callsign, groundState)
}

func (s *Service) pushAobtAsync(session int32, callsign, aobt string) {
	s.actionService.pushAobtAsync(context.Background(), session, callsign, aobt)
}

func (s *Service) pushCorrectedEobtToEuroscope(session int32, callsign, eobt string) {
	s.actionService.pushCorrectedEobtToEuroscope(context.Background(), session, callsign, eobt)
}

func (s *Service) persistCdmUpdate(ctx context.Context, session int32, callsign string, before cdmSnapshot, updated *models.CdmData) error {
	return s.broadcaster.persistCdmUpdate(ctx, session, callsign, before, updated)
}

func (s *Service) persistCdmUpdateSilently(ctx context.Context, session int32, callsign string, updated *models.CdmData) error {
	return s.broadcaster.persistCdmUpdateSilently(ctx, session, callsign, updated)
}

func (s *Service) masterPosition() string {
	return s.masterViffSync.masterPosition()
}

func (s *Service) registerMasterAsync(airport string) {
	s.masterViffSync.registerMasterAsync(context.Background(), airport)
}

func (s *Service) finalizeClxTobtUpdate(ctx context.Context, session int32, callsign string, airport string, shouldTriggerRecalculate bool) error {
	return s.actionService.finalizeClxTobtUpdate(ctx, session, callsign, airport, shouldTriggerRecalculate)
}

func (s *Service) canRunLocalRecalculation(session int32) bool {
	return s.recalculationScheduler.canRunLocalRecalculation(session)
}

func (s *Service) pushTobtAsync(session int32, callsign string, previousTobt string, tobt string) {
	s.actionService.pushTobtAsync(context.Background(), session, callsign, previousTobt, tobt)
}

func (s *Service) resolveTaxiMinutes(strip *models.Strip) int {
	return s.actionService.resolveTaxiMinutes(strip)
}

func (s *Service) SetFrontendHub(publisher shared.CdmEventPublisher) {
	s.publisher = publisher
}

func (s *Service) SetEuroscopeHub(euroscopeHub shared.EuroscopeHub) {
	s.euroscopeHub = euroscopeHub
}

func (s *Service) SetConfigProvider(configProvider ConfigProvider) {
	s.configProvider = configProvider
}

func (s *Service) SetSequenceService(sequenceService *SequenceService) {
	s.sequenceService = sequenceService
	if s.sequenceService != nil {
		s.sequenceService.SetAfterPersist(func(ctx context.Context, session int32, callsign string) {
			s.pushViffDataAfterRecalc(ctx, session, callsign)
		})
	}
}

func (s *Service) SetValidationReevaluator(validationReevaluator StripValidationReevaluator) {
	s.validationReevaluator = validationReevaluator
}

// isMasterSession returns true if the in-memory cache indicates this session is CDM master.
func (s *Service) isMasterSession(sessionID int32) bool {
	v, ok := s.sessionMaster.Load(sessionID)
	return ok && v.(bool)
}

func isViffEnabledSession(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "LIVE")
}

func (s *Service) usesViffSession(sessionID int32) bool {
	v, ok := s.sessionUsesViff.Load(sessionID)
	if ok {
		return v.(bool)
	}
	if s.sessionRepo == nil {
		return false
	}
	session, err := s.sessionRepo.GetByID(context.Background(), sessionID)
	if err != nil || session == nil {
		return false
	}
	usesViff := isViffEnabledSession(session.Name)
	s.sessionUsesViff.Store(sessionID, usesViff)
	return usesViff
}

// SetSessionCdmMaster persists the CDM master flag for a session, updates the in-memory cache,
// and registers or deregisters the airport with the vIFF CDM network accordingly.
func (s *Service) SetSessionCdmMaster(ctx context.Context, sessionID int32, master bool) error {
	if err := s.sessionRepo.UpdateCdmMaster(ctx, sessionID, master); err != nil {
		return fmt.Errorf("update CDM master for session %d: %w", sessionID, err)
	}

	if master {
		s.sessionMaster.Store(sessionID, true)
		// Fetch session to get airport for immediate master registration.
		sess, err := s.sessionRepo.GetByID(ctx, sessionID)
		if err == nil && sess != nil {
			usesViff := isViffEnabledSession(sess.Name)
			s.sessionUsesViff.Store(sessionID, usesViff)
			if _, err := s.normalizeExistingMasterSessionEobts(ctx, sessionID, sess.Airport, time.Now().UTC()); err != nil {
				return fmt.Errorf("normalize master EOBTs for session %d: %w", sessionID, err)
			}
			if usesViff {
				s.masterViffSync.registerMasterAsync(ctx, sess.Airport)
			}
			s.TriggerRecalculate(ctx, sessionID, sess.Airport)
		}
	} else {
		s.sessionMaster.Delete(sessionID)
		// Deregister from vIFF if client is valid.
		if s.client.isValid {
			sess, err := s.sessionRepo.GetByID(ctx, sessionID)
			if err == nil && sess != nil {
				usesViff := isViffEnabledSession(sess.Name)
				s.sessionUsesViff.Store(sessionID, usesViff)
				if usesViff && sess.Airport != "" {
					position := s.masterPosition()
					asyncCtx := detachedContext(ctx)
					go func() {
						if err := s.client.ClearMasterAirport(asyncCtx, sess.Airport, position); err != nil {
							slog.WarnContext(asyncCtx, "Failed to clear CDM master airport",
								slog.String("airport", sess.Airport),
								slog.Int("session", int(sessionID)),
								slog.Any("error", err),
							)
						}
					}()
				}
			}
		}
	}

	slog.InfoContext(ctx, "CDM master status updated",
		slog.Int("session", int(sessionID)),
		slog.Bool("master", master),
	)
	return nil
}

func truncateCDMClockValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4 {
		return value[:4]
	}

	return value
}
