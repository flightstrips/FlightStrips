package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/helpers"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

type Service struct {
	client                *Client
	stripRepo             repository.StripRepository
	sessionRepo           repository.SessionRepository
	controllerRepo        repository.ControllerRepository
	publisher             shared.CdmEventPublisher
	euroscopeHub          shared.EuroscopeHub
	configProvider        ConfigProvider
	sequenceService       *SequenceService
	validationReevaluator StripValidationReevaluator
	debouncer             *recalcDebouncer
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

type cdmSnapshot struct {
	Eobt, Tobt, Tsat, Ctot, CtotSource, Ttot, Asat, Asrt, Tsac, Aobt, Status, ReqTobt, ReqTobtType, EcfmpID, TobtSetBy, TobtConfirmedBy, Phase string
	EcfmpRestrictionsJSON                                                                                                                      string
	TobtAutoSynced, TobtManuallyConfirmed                                                                                                      bool
}

type viffPushState struct {
	Suspend bool
	Params  SetCdmDataParams
}

func NewCdmService(client *Client, stripRepo repository.StripRepository, sessionRepo repository.SessionRepository, controllerRepo repository.ControllerRepository) *Service {
	return &Service{
		client:         client,
		stripRepo:      stripRepo,
		sessionRepo:    sessionRepo,
		controllerRepo: controllerRepo,
		debouncer:      newRecalcDebouncer(500 * time.Millisecond),
	}
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
				s.registerMasterAsync(sess.Airport)
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
					go func() {
						if err := s.client.ClearMasterAirport(context.Background(), sess.Airport, position); err != nil {
							slog.WarnContext(ctx, "Failed to clear CDM master airport",
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

func (s *Service) TriggerRecalculate(ctx context.Context, session int32, airport string) {
	if s.sequenceService == nil || airport == "" {
		return
	}
	if !s.canRunLocalRecalculation(session) {
		return
	}
	normalizedAirport := strings.ToUpper(strings.TrimSpace(airport))
	s.debouncer.Schedule(recalcDebounceKey(session, normalizedAirport), func() {
		if err := s.sequenceService.RecalculateAirport(context.Background(), session, airport); err != nil {
			slog.ErrorContext(ctx, "CDM recalculation failed", slog.Int("session", int(session)), slog.String("airport", airport), slog.Any("error", err))
		}
	})
}

func (s *Service) SyncAirportLvoFromRunwayStatus(ctx context.Context, airport string, runwayStatus map[string]string) {
	if s.configProvider == nil || strings.TrimSpace(airport) == "" {
		return
	}

	active := hasLowVisRunwayStatus(runwayStatus)
	s.configProvider.SetLvo(airport, active)
	slog.DebugContext(ctx, "Synchronized CDM LVO state from runway status",
		slog.String("airport", strings.ToUpper(strings.TrimSpace(airport))),
		slog.Bool("active", active),
	)
}

func (s *Service) TriggerRecalculateForAirport(ctx context.Context, airport string) error {
	if s.sequenceService == nil || strings.TrimSpace(airport) == "" {
		return nil
	}

	sessions, err := s.sessionRepo.List(ctx)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if session == nil || !strings.EqualFold(session.Airport, airport) {
			continue
		}
		s.TriggerRecalculate(ctx, session.ID, session.Airport)
	}

	return nil
}

func recalcDebounceKey(session int32, airport string) string {
	return strconv.FormatInt(int64(session), 10) + ":" + airport
}

func (s *Service) HandleTobtUpdate(ctx context.Context, session int32, callsign string, tobt string, sourcePosition string, sourceRole string) error {
	callsign = strings.TrimSpace(callsign)
	tobt = strings.TrimSpace(tobt)
	if callsign == "" || !isValidHHMM(tobt) {
		return nil
	}

	strip, before, updated, previousTobt, changed, shouldTriggerRecalculate, err := s.prepareTobtUpdate(ctx, session, callsign, tobt, sourcePosition, time.Now().UTC())
	if err != nil {
		return err
	}
	if strip == nil || updated == nil || !changed {
		return nil
	}

	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return err
	}

	if shouldTriggerRecalculate {
		s.TriggerRecalculate(ctx, session, strip.Origin)
	}
	s.pushTobtAsync(session, callsign, previousTobt, tobt)
	return nil
}

func (s *Service) HandleEobtUpdate(ctx context.Context, session int32, callsign string, eobt string, sourcePosition string, sourceRole string) error {
	callsign = strings.TrimSpace(callsign)
	eobt = strings.TrimSpace(eobt)
	if callsign == "" {
		return nil
	}
	if eobt == "" && !s.isMasterSession(session) {
		return nil
	}
	if eobt != "" && !isValidHHMM(eobt) {
		return nil
	}

	strip, cdmData, err := s.loadCdmActionTarget(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip == nil || cdmData == nil {
		return nil
	}

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()
	now := time.Now().UTC()
	normalizedEobt, clamped := s.normalizeMasterEobtValue(session, eobt, now)
	markerChanged := setEobtCapReasonMarker(updated, clamped, true)
	shouldForceRecalculate := shouldForceRecalculateForStaleSequence(updated, now)
	previousEobt := helpers.ValueOrDefault(updated.EffectiveEobt())
	if previousEobt == normalizedEobt && !shouldForceRecalculate && !markerChanged {
		if clamped && eobt != normalizedEobt {
			s.pushCorrectedEobtToEuroscope(session, callsign, normalizedEobt)
		}
		return nil
	}

	updated.Eobt = &normalizedEobt
	previousTobt := helpers.ValueOrDefault(updated.EffectiveTobt())
	shouldAlignTobtWithEobt := shouldSyncEobtToTobt(normalizedEobt, now)
	shouldSyncTobt := shouldAlignTobtWithEobt && !hasProtectedConfirmedTobt(updated, previousEobt)
	shouldTriggerRecalculate := clamped || shouldAlignTobtWithEobt || shouldForceRecalculate
	if shouldSyncTobt {
		applyAutoSyncedTobtUpdate(updated, normalizedEobt)
	}
	if shouldTriggerRecalculate {
		updated.MarkLocalRecalculationPending()
	}

	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return err
	}
	if clamped && eobt != normalizedEobt {
		s.pushCorrectedEobtToEuroscope(session, callsign, normalizedEobt)
	}

	if shouldTriggerRecalculate {
		s.TriggerRecalculate(ctx, session, strip.Origin)
		if shouldSyncTobt {
			s.pushTobtAsync(session, callsign, previousTobt, normalizedEobt)
		}
	}
	return nil
}

func (s *Service) normalizeMasterEobtValue(session int32, eobt string, now time.Time) (string, bool) {
	normalized := truncateCDMClockValue(normalizeCalculationClock(eobt))
	if !s.isMasterSession(session) {
		return normalized, false
	}
	if normalized == "" {
		return truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget)), true
	}
	if minutesBetween(timeToClock(now), toHHMMSS(normalized)) <= masterEobtClampThreshold {
		return normalized, false
	}
	return truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget)), true
}

func (s *Service) normalizeExistingMasterSessionEobts(ctx context.Context, session int32, airport string, now time.Time) (bool, error) {
	if !s.isMasterSession(session) || strings.TrimSpace(airport) == "" {
		return false, nil
	}

	strips, err := s.stripRepo.ListByOrigin(ctx, session, airport)
	if err != nil {
		return false, err
	}

	normalizedAny := false
	for _, strip := range strips {
		if strip == nil || strip.CdmData == nil {
			continue
		}
		normalized, err := s.normalizeMasterFlightEobt(ctx, session, strip.Callsign, strip.CdmData, now)
		if err != nil {
			return normalizedAny, err
		}
		normalizedAny = normalizedAny || normalized
	}

	return normalizedAny, nil
}

func (s *Service) normalizeMasterLookupEobts(ctx context.Context, session int32, lookup map[string]*models.CdmData, now time.Time) (bool, error) {
	if !s.isMasterSession(session) || len(lookup) == 0 {
		return false, nil
	}

	normalizedAny := false
	for callsign, data := range lookup {
		if strings.TrimSpace(callsign) == "" || data == nil {
			continue
		}
		normalized, err := s.normalizeMasterFlightEobt(ctx, session, callsign, data, now)
		if err != nil {
			return normalizedAny, err
		}
		normalizedAny = normalizedAny || normalized
	}

	return normalizedAny, nil
}

func (s *Service) normalizeMasterFlightEobt(ctx context.Context, session int32, callsign string, data *models.CdmData, now time.Time) (bool, error) {
	if !s.isMasterSession(session) || data == nil {
		return false, nil
	}

	currentEobt := helpers.ValueOrDefault(data.EffectiveEobt())
	normalizedEobt, clamped := s.normalizeMasterEobtValue(session, currentEobt, now)
	if !clamped {
		return false, nil
	}

	before := snapshotCdm(data)
	updated := data.Clone()
	markerChanged := setEobtCapReasonMarker(updated, true, false)
	previousEobt := helpers.ValueOrDefault(updated.EffectiveEobt())
	if previousEobt == normalizedEobt && !markerChanged {
		return false, nil
	}

	updated.Eobt = &normalizedEobt
	previousTobt := helpers.ValueOrDefault(updated.EffectiveTobt())
	shouldAlignTobtWithEobt := shouldSyncEobtToTobt(normalizedEobt, now)
	shouldSyncTobt := shouldAlignTobtWithEobt && !hasProtectedConfirmedTobt(updated, previousEobt)
	if shouldSyncTobt {
		applyAutoSyncedTobtUpdate(updated, normalizedEobt)
	}
	updated.MarkLocalRecalculationPending()

	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return false, err
	}
	s.pushCorrectedEobtToEuroscope(session, callsign, normalizedEobt)
	if shouldSyncTobt {
		s.pushTobtAsync(session, callsign, previousTobt, normalizedEobt)
	}

	return true, nil
}

func (s *Service) HandleClxTobtUpdate(ctx context.Context, session int32, callsign string, tobt string, sourcePosition string, sourceRole string) error {
	callsign = strings.TrimSpace(callsign)
	tobt = strings.TrimSpace(tobt)
	if callsign == "" || !isValidHHMM(tobt) {
		return nil
	}

	strip, _, updated, previousTobt, changed, shouldTriggerRecalculate, err := s.prepareTobtUpdate(ctx, session, callsign, tobt, sourcePosition, time.Now().UTC())
	if err != nil {
		return err
	}
	if strip == nil || updated == nil {
		return nil
	}

	if changed {
		if err := s.persistCdmUpdateSilently(ctx, session, callsign, updated); err != nil {
			return err
		}
	}

	if err := s.finalizeClxTobtUpdate(ctx, session, callsign, strip.Origin, shouldTriggerRecalculate); err != nil {
		return err
	}

	s.pushTobtAsync(session, callsign, previousTobt, tobt)
	return nil
}

func (s *Service) HandleDeiceUpdate(ctx context.Context, session int32, callsign string, deiceType string) error {
	callsign = strings.TrimSpace(callsign)
	deiceType = strings.ToUpper(strings.TrimSpace(deiceType))
	if callsign == "" || !isValidDeiceType(deiceType) {
		return nil
	}

	strip, cdmData, err := s.loadCdmActionTarget(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip == nil || cdmData == nil {
		return nil
	}

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()

	current := helpers.ValueOrDefault(updated.DeIce)
	if current == deiceType {
		return nil
	}
	if deiceType == "" {
		updated.DeIce = nil
	} else {
		updated.DeIce = &deiceType
	}
	updated.MarkLocalRecalculationPending()

	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return err
	}
	s.TriggerRecalculate(ctx, session, strip.Origin)
	return nil
}

func (s *Service) HandleAsrtToggle(ctx context.Context, session int32, callsign string, asrt string) error {
	callsign = strings.TrimSpace(callsign)
	asrt = strings.TrimSpace(asrt)
	if callsign == "" {
		return nil
	}

	_, cdmData, err := s.loadCdmActionTarget(ctx, session, callsign)
	if err != nil {
		return err
	}
	if cdmData == nil {
		return nil
	}

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()
	if asrt == "" {
		updated.Asrt = nil
	} else {
		updated.Asrt = &asrt
	}
	return s.persistCdmUpdate(ctx, session, callsign, before, updated)
}

func (s *Service) HandleTsacUpdate(ctx context.Context, session int32, callsign string, tsac string) error {
	callsign = strings.TrimSpace(callsign)
	tsac = strings.TrimSpace(tsac)
	if callsign == "" {
		return nil
	}

	_, cdmData, err := s.loadCdmActionTarget(ctx, session, callsign)
	if err != nil {
		return err
	}
	if cdmData == nil {
		return nil
	}

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()
	if tsac == "" {
		updated.Tsac = nil
	} else {
		updated.Tsac = &tsac
	}
	return s.persistCdmUpdate(ctx, session, callsign, before, updated)
}

func (s *Service) HandleManualCtot(ctx context.Context, session int32, callsign string, ctot string) error {
	callsign = strings.TrimSpace(callsign)
	ctot = strings.TrimSpace(ctot)
	if callsign == "" || !isValidHHMM(ctot) {
		return nil
	}

	strip, cdmData, err := s.loadCdmActionTarget(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip == nil || cdmData == nil {
		return nil
	}

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()

	if helpers.ValueOrDefault(updated.Ctot) == ctot && helpers.ValueOrDefault(updated.CtotSource) == models.CtotSourceManual {
		return nil
	}

	updated.Ctot = &ctot
	src := models.CtotSourceManual
	updated.CtotSource = &src
	updated.MarkLocalRecalculationPending()

	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return err
	}
	s.reevaluateCtotValidationAsync(ctx, session, callsign, before, snapshotCdm(updated))
	s.TriggerRecalculate(ctx, session, strip.Origin)
	return nil
}

func (s *Service) HandleCtotRemove(ctx context.Context, session int32, callsign string) error {
	callsign = strings.TrimSpace(callsign)
	if callsign == "" {
		return nil
	}

	strip, cdmData, err := s.loadCdmActionTarget(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip == nil || cdmData == nil {
		return nil
	}

	if !cdmData.HasManualCtot() {
		return nil
	}

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()
	updated.Ctot = nil
	updated.CtotSource = nil
	updated.MarkLocalRecalculationPending()

	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return err
	}
	s.reevaluateCtotValidationAsync(ctx, session, callsign, before, snapshotCdm(updated))
	s.TriggerRecalculate(ctx, session, strip.Origin)
	return nil
}

func (s *Service) HandleApproveReqTobt(ctx context.Context, session int32, callsign string, sourcePosition string, sourceRole string) error {
	callsign = strings.TrimSpace(callsign)
	if callsign == "" {
		return nil
	}

	strip, cdmData, err := s.loadCdmActionTarget(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip == nil || cdmData == nil || helpers.ValueOrDefault(cdmData.EffectiveReqTobt()) == "" {
		return nil
	}

	if err := s.HandleTobtUpdate(ctx, session, callsign, helpers.ValueOrDefault(cdmData.EffectiveReqTobt()), sourcePosition, sourceRole); err != nil {
		return err
	}
	s.clearReqTobtAsync(session, callsign)
	return nil
}

func (s *Service) clearReqTobtAsync(session int32, callsign string) {
	if !s.client.isValid || !s.isMasterSession(session) || !s.usesViffSession(session) {
		return
	}
	go func() {
		if err := s.client.IFPSDpi(context.Background(), callsign, "REQTOBT/NULL/NULL"); err != nil {
			slog.Warn("Failed to clear REQTOBT on CDM backend",
				slog.String("callsign", callsign),
				slog.Any("error", err),
			)
		}
	}()
}

func (s *Service) HandleReadyRequest(ctx context.Context, session int32, callsign string, sourcePosition string, sourceRole string) error {
	if s.client.isValid && !s.isMasterSession(session) {
		return s.SetReady(ctx, session, callsign)
	}

	now := time.Now().UTC()
	tobt := now.Format("1504")

	strip, cdmData, err := s.loadCdmActionTarget(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip == nil || cdmData == nil {
		return nil
	}

	updated := cdmData.Clone()
	previousEobt := helpers.ValueOrDefault(updated.EffectiveEobt())
	previousTobt := helpers.ValueOrDefault(updated.EffectiveTobt())
	applyConfirmedTobtUpdate(updated, tobt, sourcePosition)
	currentEobt := normalizeCalculationClock(helpers.ValueOrDefault(updated.EffectiveEobt()))
	if currentEobt == "" || !isAfterOrEqual(tobt, currentEobt) {
		updated.Eobt = &tobt
	}
	updated.MarkLocalRecalculationPending()
	if err := s.persistCdmUpdateSilently(ctx, session, callsign, updated); err != nil {
		return err
	}

	if err := s.finalizeClxTobtUpdate(ctx, session, callsign, strip.Origin, true); err != nil {
		return err
	}
	if nextEobt := helpers.ValueOrDefault(updated.EffectiveEobt()); strings.TrimSpace(previousEobt) != strings.TrimSpace(nextEobt) {
		s.pushCorrectedEobtToEuroscope(session, callsign, nextEobt)
	}

	masterViffSession := s.client.isValid && s.isMasterSession(session) && s.usesViffSession(session)
	if masterViffSession {
		if strings.TrimSpace(previousTobt) != tobt {
			if err := s.PushTobt(ctx, session, callsign, tobt); err != nil {
				return err
			}
		}
		if err := s.pushLatestMasterCdmDataToViff(ctx, session, callsign, strip); err != nil {
			return err
		}
	}

	if err := s.SetReady(ctx, session, callsign); err != nil {
		return err
	}
	if masterViffSession {
		if err := s.refreshMasterFlightFromViff(ctx, session, callsign, strip.Origin); err != nil {
			return err
		}
	} else {
		s.pushTobtAsync(session, callsign, previousTobt, tobt)
	}
	return nil
}

func (s *Service) SetReady(ctx context.Context, session int32, callsign string) error {
	if s.client.isValid && s.usesViffSession(session) {
		if err := s.client.IFPSDpi(ctx, callsign, "REA/1"); err != nil {
			return err
		}
		if !s.isMasterSession(session) {
			return nil
		}
	}

	cdmData, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	if cdmData.EffectiveStatus() != nil && *cdmData.EffectiveStatus() == "REA" {
		return nil
	}

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()
	rea := "REA"
	updated.Status = &rea
	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return err
	}

	if s.publisher != nil {
		s.publisher.SendCdmWait(session, callsign)
	}

	return nil
}

func (s *Service) RequestBetterTobt(ctx context.Context, session int32, callsign string) error {
	if !s.client.isValid {
		return nil
	}
	now := time.Now()
	format := now.Format("1504")
	status := "REQTOBT/" + format + "/ATC"

	cdmData, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if cdmData.EffectiveStatus() != nil && *cdmData.EffectiveStatus() == status {
		return nil
	}

	if s.usesViffSession(session) {
		err = s.client.IFPSDpi(ctx, callsign, status)
		if err != nil {
			return err
		}
	}

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()
	updated.Status = &status
	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return err
	}

	s.publisher.SendCdmWait(session, callsign)

	return nil
}

func (s *Service) PushTobt(ctx context.Context, session int32, callsign string, tobt string) error {
	if !s.client.isValid || !s.isMasterSession(session) || !s.usesViffSession(session) {
		return nil
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	taxiMinutes := s.resolveTaxiMinutes(strip)
	return s.client.IFPSSetTobt(ctx, callsign, tobt, taxiMinutes)
}

func (s *Service) Start(ctx context.Context) {
	syncEnabled := s.client.isValid
	localRecalcEnabled := s.sequenceService != nil
	validationEnabled := s.validationReevaluator != nil

	if !syncEnabled {
		slog.WarnContext(ctx, "CDM client is not valid, CDM data will not be synced")
	}
	if !syncEnabled && !localRecalcEnabled && !validationEnabled {
		return
	}

	if syncEnabled || localRecalcEnabled {
		if err := s.syncSessions(ctx); err != nil {
			slog.ErrorContext(ctx, "Failed to initialize CDM session state", slog.Any("error", err))
		}
	}

	var syncTicker *time.Ticker
	if syncEnabled {
		syncTicker = time.NewTicker(cdmSyncInterval)
		defer syncTicker.Stop()
	}

	var recalcTicker *time.Ticker
	if localRecalcEnabled {
		recalcTicker = time.NewTicker(cdmPeriodicRecalcInterval)
		defer recalcTicker.Stop()
	}

	var validationTicker *time.Ticker
	if validationEnabled {
		validationTicker = time.NewTicker(time.Minute)
		defer validationTicker.Stop()
	}

	var syncCh <-chan time.Time
	if syncTicker != nil {
		syncCh = syncTicker.C
	}

	var recalcCh <-chan time.Time
	if recalcTicker != nil {
		recalcCh = recalcTicker.C
	}

	var validationCh <-chan time.Time
	if validationTicker != nil {
		validationCh = validationTicker.C
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-syncCh:
			if err := s.syncSessions(ctx); err != nil {
				slog.ErrorContext(ctx, "Failed to sync CDM data", slog.Any("error", err))
			}
		case <-recalcCh:
			if err := s.schedulePeriodicRecalculate(ctx); err != nil {
				slog.ErrorContext(ctx, "Failed to schedule periodic CDM recalculation", slog.Any("error", err))
			}
		case <-validationCh:
			if err := s.schedulePeriodicCtotValidationReevaluation(ctx); err != nil {
				slog.ErrorContext(ctx, "Failed to reevaluate CTOT validations", slog.Any("error", err))
			}
		}
	}
}

func (s *Service) syncSessions(ctx context.Context) error {
	sessions, err := s.sessionRepo.List(ctx)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if session == nil {
			continue
		}

		usesViff := isViffEnabledSession(session.Name)
		s.sessionUsesViff.Store(session.ID, usesViff)
		slog.DebugContext(ctx, "Syncing CDM data", slog.String("session", session.Name), slog.Int("id", int(session.ID)), slog.String("airport", session.Airport))

		if session.CdmMaster {
			s.sessionMaster.Store(session.ID, true)
			if usesViff {
				s.registerMasterAsync(session.Airport)
			}
		} else {
			s.sessionMaster.Delete(session.ID)
		}
		s.SyncAirportLvoFromRunwayStatus(ctx, session.Airport, session.ActiveRunways.RunwayStatus)

		if usesViff {
			if err := s.syncCdmData(ctx, session); err != nil {
				return err
			}
		}

		if session.CdmMaster {
			s.TriggerRecalculate(ctx, session.ID, session.Airport)
		}
	}

	return nil
}

func (s *Service) schedulePeriodicRecalculate(ctx context.Context) error {
	if s.sequenceService == nil {
		return nil
	}

	sessions, err := s.sessionRepo.List(ctx)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if session == nil || strings.TrimSpace(session.Airport) == "" {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		s.TriggerRecalculate(ctx, session.ID, session.Airport)
	}

	return nil
}

func (s *Service) syncCdmData(ctx context.Context, session *models.Session) error {
	if !s.client.isValid {
		return nil
	}
	if session != nil {
		s.sessionUsesViff.Store(session.ID, isViffEnabledSession(session.Name))
	}

	airport := session.Airport

	currentData, err := s.stripRepo.GetCdmData(ctx, session.ID)
	if err != nil {
		return err
	}

	lookup := make(map[string]*models.CdmData)
	for _, row := range currentData {
		lookup[row.Callsign] = row.Data
	}

	newData, err := s.client.IFPSByDepartureAirport(ctx, airport)
	if err != nil {
		return err
	}

	for _, row := range newData {
		flight, ok := lookup[row.Callsign]
		if !ok {
			continue
		}

		nextCtot, nextCtotSource := effectiveIfpsCtotAndSource(row)

		if s.isMasterSession(session.ID) {
			// Master: only CTOT and REQTOBT are relevant from the API; local calculation handles the rest.
			current, needsRecalculate, err := s.mergeMasterViffFlight(ctx, session.ID, row.Callsign, flight, row, nextCtot, nextCtotSource)
			if err != nil {
				return err
			}
			lookup[row.Callsign] = current
			if needsRecalculate {
				s.TriggerRecalculate(ctx, session.ID, session.Airport)
			}

			s.ensureMasterFlightExport(ctx, session.ID, row.Callsign, current, row)
		} else {
			// Slave: sync all CDM fields from the API.
			nextAsat := helpers.ValueOrDefault(flight.Asat)
			if nextAsat == "" && statusImpliesAsat(row.CDMStatus) {
				nextAsat = timeToClock(time.Now().UTC())
			}

			changed := helpers.ValueOrDefault(flight.Status) != row.CDMStatus ||
				helpers.ValueOrDefault(flight.Aobt) != row.AOBT ||
				helpers.ValueOrDefault(flight.Eobt) != row.EOBT ||
				helpers.ValueOrDefault(flight.Ctot) != nextCtot ||
				helpers.ValueOrDefault(flight.Asat) != nextAsat ||
				helpers.ValueOrDefault(flight.Asrt) != row.CDMData.ReqASRT ||
				helpers.ValueOrDefault(flight.Tobt) != row.TOBT ||
				helpers.ValueOrDefault(flight.Tsat) != truncateCDMClockValue(row.CDMData.TSAT) ||
				helpers.ValueOrDefault(flight.Ttot) != truncateCDMClockValue(row.CDMData.TTOT) ||
				helpers.ValueOrDefault(flight.ReqTobt) != row.CDMData.ReqTOBT ||
				helpers.ValueOrDefault(flight.ReqTobtType) != row.CDMData.ReqTOBTType ||
				helpers.ValueOrDefault(flight.EcfmpID) != row.CDMData.Reason

			if !changed {
				continue
			}

			before := snapshotCdm(flight)
			updated := flight.Clone()
			updated.Tobt = &row.TOBT
			updated.ReqTobt = stringPointerIfPresent(row.CDMData.ReqTOBT)
			updated.ReqTobtType = stringPointerIfPresent(row.CDMData.ReqTOBTType)
			updated.Tsat = stringPointerIfPresent(truncateCDMClockValue(row.CDMData.TSAT))
			updated.Ttot = stringPointerIfPresent(truncateCDMClockValue(row.CDMData.TTOT))
			updated.Asrt = stringPointerIfPresent(row.CDMData.ReqASRT)
			if nextCtot != "" {
				updated.Ctot = &nextCtot
				updated.CtotSource = &nextCtotSource
			} else if !flight.HasManualCtot() {
				updated.Ctot = nil
				updated.CtotSource = nil
			}
			updated.Aobt = &row.AOBT
			updated.Asat = stringPointerIfPresent(nextAsat)
			updated.Eobt = &row.EOBT
			updated.Status = &row.CDMStatus
			updated.EcfmpID = stringPointerIfPresent(row.CDMData.Reason)
			updated.Calculation = nil

			if err := s.persistCdmUpdate(ctx, session.ID, row.Callsign, before, updated); err != nil {
				return err
			}
			s.reevaluateCtotValidationAsync(ctx, session.ID, row.Callsign, before, snapshotCdm(updated))
		}
	}

	if s.isMasterSession(session.ID) {
		normalized, err := s.normalizeMasterLookupEobts(ctx, session.ID, lookup, time.Now().UTC())
		if err != nil {
			return err
		}
		if normalized {
			s.TriggerRecalculate(ctx, session.ID, airport)
		}
	}

	if s.sequenceService != nil && !s.isMasterSession(session.ID) {
		strips, err := s.stripRepo.ListByOrigin(ctx, session.ID, airport)
		if err != nil {
			return err
		}
		markerUpdates := buildStoredSequenceMarkerUpdates(strips, s.isMasterSession(session.ID), time.Now().UTC())
		for _, strip := range strips {
			if strip == nil {
				continue
			}
			updated, ok := markerUpdates[strip.Callsign]
			if !ok {
				continue
			}
			if err := s.persistCdmUpdateSilently(ctx, session.ID, strip.Callsign, updated); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Service) ensureMasterFlightExport(ctx context.Context, session int32, callsign string, local *models.CdmData, remote IFPSData) {
	if !s.client.isValid || !s.usesViffSession(session) || local == nil || local.NeedsLocalRecalculation() || !masterFlightNeedsExport(local, remote) {
		return
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.WarnContext(ctx, "Failed to load strip for master CDM export",
			slog.Int("session", int(session)),
			slog.String("callsign", callsign),
			slog.Any("error", err),
		)
	}

	s.pushViffAfterRecalcAsync(session, callsign, strip, local)
}

func (s *Service) mergeMasterViffFlight(ctx context.Context, session int32, callsign string, flight *models.CdmData, row IFPSData, nextCtot string, nextCtotSource string) (*models.CdmData, bool, error) {
	if flight == nil {
		return nil, false, nil
	}

	ctotChanged := helpers.ValueOrDefault(flight.Ctot) != nextCtot
	reqTobtChanged := helpers.ValueOrDefault(flight.ReqTobt) != row.CDMData.ReqTOBT
	reqTobtTypeChanged := helpers.ValueOrDefault(flight.ReqTobtType) != row.CDMData.ReqTOBTType
	changed := ctotChanged ||
		reqTobtChanged ||
		reqTobtTypeChanged ||
		helpers.ValueOrDefault(flight.EcfmpID) != row.CDMData.Reason
	if !changed {
		return flight, false, nil
	}

	before := snapshotCdm(flight)
	updated := flight.Clone()
	if nextCtot != "" {
		updated.Ctot = &nextCtot
		updated.CtotSource = &nextCtotSource
		updated.EcfmpID = stringPointerIfPresent(row.CDMData.Reason)
	} else if !flight.HasManualCtot() {
		updated.Ctot = nil
		updated.CtotSource = nil
		updated.EcfmpID = nil
	}
	updated.ReqTobt = stringPointerIfPresent(row.CDMData.ReqTOBT)
	updated.ReqTobtType = stringPointerIfPresent(row.CDMData.ReqTOBTType)
	needsRecalculate := ctotChanged || reqTobtChanged
	if needsRecalculate {
		updated.MarkLocalRecalculationPending()
	}

	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return nil, false, err
	}
	s.reevaluateCtotValidationAsync(ctx, session, callsign, before, snapshotCdm(updated))
	return updated, needsRecalculate, nil
}

func masterFlightNeedsExport(local *models.CdmData, remote IFPSData) bool {
	if local == nil {
		return false
	}

	if helpers.ValueOrDefault(local.EffectivePhase()) == "I" {
		return !statusImpliesInvalidation(remote.CDMStatus)
	}

	localTsat := truncateCDMClockValue(helpers.ValueOrDefault(local.EffectiveTsat()))
	if localTsat == "" {
		return false
	}

	localTobt := truncateCDMClockValue(helpers.ValueOrDefault(local.EffectiveTobt()))
	localTtot := truncateCDMClockValue(helpers.ValueOrDefault(local.EffectiveTtot()))
	localCtot := truncateCDMClockValue(helpers.ValueOrDefault(local.EffectiveCtot()))
	localAsrt := truncateCDMClockValue(helpers.ValueOrDefault(local.Asrt))
	localReason := helpers.ValueOrDefault(local.EcfmpID)

	remoteTobt := truncateCDMClockValue(remote.TOBT)
	remoteTsat := truncateCDMClockValue(remote.CDMData.TSAT)
	remoteTtot := truncateCDMClockValue(remote.CDMData.TTOT)
	remoteCtot, _ := effectiveIfpsCtotAndSource(remote)
	remoteAsrt := truncateCDMClockValue(remote.CDMData.ReqASRT)
	remoteReason := remote.CDMData.Reason

	return localTobt != remoteTobt ||
		localTsat != remoteTsat ||
		localTtot != remoteTtot ||
		localCtot != remoteCtot ||
		localAsrt != remoteAsrt ||
		localReason != remoteReason
}

func (s *Service) reevaluateCtotValidationAsync(ctx context.Context, session int32, callsign string, before, after cdmSnapshot) {
	if s.validationReevaluator == nil || before.Ctot == after.Ctot {
		return
	}
	if err := s.validationReevaluator.ReevaluateCtotValidation(ctx, session, callsign, true, false); err != nil {
		slog.WarnContext(ctx, "Failed to reevaluate CTOT validation",
			slog.Int("session", int(session)),
			slog.String("callsign", callsign),
			slog.Any("error", err),
		)
	}
}

func (s *Service) schedulePeriodicCtotValidationReevaluation(ctx context.Context) error {
	if s.validationReevaluator == nil {
		return nil
	}

	sessions, err := s.sessionRepo.List(ctx)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if session == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := s.validationReevaluator.ReevaluateCtotValidationsForSession(ctx, session.ID, true); err != nil {
			return err
		}
	}

	return nil
}

func truncateCDMClockValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4 {
		return value[:4]
	}

	return value
}

func effectiveIfpsCtotAndSource(row IFPSData) (string, string) {
	if ctot := truncateCDMClockValue(row.CTOT); ctot != "" {
		return ctot, models.CtotSourceATFCM
	}
	if ctot := truncateCDMClockValue(row.CDMData.CTOT); ctot != "" {
		return ctot, models.CtotSourceEvent
	}
	return "", ""
}

func statusImpliesAsat(status string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	if normalized == "" {
		return false
	}

	for _, token := range strings.Split(normalized, "/") {
		switch token {
		case "STUP", "ST-UP", "PUSH", "TAXI", "DEPA":
			return true
		}
	}

	return false
}

func statusImpliesInvalidation(status string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	if normalized == "" {
		return false
	}

	for _, token := range strings.Split(normalized, "/") {
		switch token {
		case "SUSP", "SUS":
			return true
		}
	}

	return false
}

func snapshotCdm(data *models.CdmData) cdmSnapshot {
	if data == nil {
		return cdmSnapshot{}
	}
	ecfmpJSON, _ := json.Marshal(data.EcfmpRestrictions)
	return cdmSnapshot{
		Eobt:                  truncateCDMClockValue(helpers.ValueOrDefault(data.Eobt)),
		Tobt:                  truncateCDMClockValue(helpers.ValueOrDefault(data.Tobt)),
		Tsat:                  truncateCDMClockValue(helpers.ValueOrDefault(data.Tsat)),
		Ctot:                  truncateCDMClockValue(helpers.ValueOrDefault(data.Ctot)),
		CtotSource:            helpers.ValueOrDefault(data.CtotSource),
		Ttot:                  truncateCDMClockValue(helpers.ValueOrDefault(data.Ttot)),
		Asat:                  truncateCDMClockValue(helpers.ValueOrDefault(data.Asat)),
		Asrt:                  truncateCDMClockValue(helpers.ValueOrDefault(data.Asrt)),
		Tsac:                  helpers.ValueOrDefault(data.Tsac),
		Aobt:                  truncateCDMClockValue(helpers.ValueOrDefault(data.Aobt)),
		Status:                helpers.ValueOrDefault(data.Status),
		ReqTobt:               truncateCDMClockValue(helpers.ValueOrDefault(data.ReqTobt)),
		ReqTobtType:           helpers.ValueOrDefault(data.ReqTobtType),
		EcfmpID:               helpers.ValueOrDefault(data.EcfmpID),
		TobtSetBy:             helpers.ValueOrDefault(data.TobtSetBy),
		TobtConfirmedBy:       helpers.ValueOrDefault(data.TobtConfirmedBy),
		Phase:                 helpers.ValueOrDefault(data.Phase),
		EcfmpRestrictionsJSON: string(ecfmpJSON),
		TobtAutoSynced:        data.TobtAutoSynced,
		TobtManuallyConfirmed: data.TobtManuallyConfirmed,
	}
}

func (s *Service) broadcastIfChanged(session int32, callsign string, before, after cdmSnapshot) {
	if before == after {
		return
	}

	if s.publisher != nil {
		cdmData := &models.CdmData{
			Eobt:        stringPointerIfPresent(after.Eobt),
			Tobt:        stringPointerIfPresent(after.Tobt),
			ReqTobt:     stringPointerIfPresent(after.ReqTobt),
			ReqTobtType: stringPointerIfPresent(after.ReqTobtType),
			Tsat:        stringPointerIfPresent(after.Tsat),
			Ttot:        stringPointerIfPresent(after.Ttot),
			Ctot:        stringPointerIfPresent(after.Ctot),
			CtotSource:  stringPointerIfPresent(after.CtotSource),
			Aobt:        stringPointerIfPresent(after.Aobt),
			Asat:        stringPointerIfPresent(after.Asat),
			Asrt:        stringPointerIfPresent(after.Asrt),
			Tsac:        stringPointerIfPresent(after.Tsac),
			Status:      stringPointerIfPresent(after.Status),
			EcfmpID:     stringPointerIfPresent(after.EcfmpID),
			Phase:       stringPointerIfPresent(after.Phase),
		}
		if before.EcfmpRestrictionsJSON != after.EcfmpRestrictionsJSON {
			storedData, err := s.stripRepo.GetCdmDataForCallsign(context.Background(), session, callsign)
			if err == nil && storedData != nil {
				cdmData.EcfmpRestrictions = storedData.EcfmpRestrictions
			}
		}

		s.publisher.SendCdmUpdates(session, []frontendEvents.CdmDataEvent{shared.BuildFrontendCdmDataEvent(callsign, cdmData)})
	}

	if s.euroscopeHub != nil {
		data, err := s.stripRepo.GetCdmDataForCallsign(context.Background(), session, callsign)
		if err == nil {
			s.euroscopeHub.BroadcastCdmUpdates(session, []euroscopeEvents.CdmUpdateEvent{buildCdmUpdateEvent(callsign, data)})
		}
	}
}

func (s *Service) pushViffDataAfterRecalc(ctx context.Context, session int32, callsign string) {
	if !s.client.isValid || !s.usesViffSession(session) {
		return
	}

	data, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		slog.WarnContext(ctx, "Failed to load recalculated CDM data",
			slog.Int("session", int(session)),
			slog.String("callsign", callsign),
			slog.Any("error", err),
		)
		return
	}

	// Load strip for departure info (runway) needed by setCdmData
	strip, _ := s.stripRepo.GetByCallsign(ctx, session, callsign)
	s.pushViffAfterRecalcAsync(session, callsign, strip, data)
}

func (s *Service) pushCdmDataAfterRecalc(ctx context.Context, session int32, callsign string) {
	if s.publisher == nil && s.euroscopeHub == nil && (!s.client.isValid || !s.usesViffSession(session)) {
		return
	}

	data, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		slog.WarnContext(ctx, "Failed to load recalculated CDM data",
			slog.Int("session", int(session)),
			slog.String("callsign", callsign),
			slog.Any("error", err),
		)
		return
	}

	if s.publisher != nil {
		s.publisher.SendCdmUpdates(session, []frontendEvents.CdmDataEvent{shared.BuildFrontendCdmDataEvent(callsign, data)})
	}
	if s.euroscopeHub != nil {
		s.euroscopeHub.BroadcastCdmUpdates(session, []euroscopeEvents.CdmUpdateEvent{buildCdmUpdateEvent(callsign, data)})
	}

	if !s.client.isValid || !s.usesViffSession(session) {
		return
	}

	strip, _ := s.stripRepo.GetByCallsign(ctx, session, callsign)
	s.pushViffAfterRecalcAsync(session, callsign, strip, data)
}

func (s *Service) pushViffAfterRecalcAsync(session int32, callsign string, strip *models.Strip, data *models.CdmData) {
	if !s.client.isValid || !s.usesViffSession(session) {
		return
	}
	state, ok := buildViffPushState(callsign, strip, data)
	if !ok || !s.markViffPushPending(session, callsign, state) {
		return
	}
	go func() {
		ctx := context.Background()
		if err := s.pushViffState(ctx, callsign, state); err != nil {
			s.clearPendingViffPush(session, callsign, state)
			slog.WarnContext(ctx, "Failed to push CDM data to CDM backend",
				slog.String("callsign", callsign),
				slog.Any("error", err),
			)
		}
	}()
}

func (s *Service) pushViffAfterRecalc(ctx context.Context, callsign string, strip *models.Strip, data *models.CdmData) error {
	state, ok := buildViffPushState(callsign, strip, data)
	if !ok {
		return nil
	}
	return s.pushViffState(ctx, callsign, state)
}

func (s *Service) pushViffState(ctx context.Context, callsign string, state viffPushState) error {
	if state.Suspend {
		return s.client.IFPSDpi(ctx, callsign, "SUSP")
	}
	return s.client.IFPSSetCdmData(ctx, state.Params)
}

func buildViffPushState(callsign string, strip *models.Strip, data *models.CdmData) (viffPushState, bool) {
	if data == nil {
		return viffPushState{}, false
	}
	if helpers.ValueOrDefault(data.EffectivePhase()) == "I" {
		return viffPushState{Suspend: true}, true
	}

	tsat := normalizeViffCdmTime(helpers.ValueOrDefault(data.EffectiveTsat()))
	if tsat == "" {
		return viffPushState{}, false
	}

	depInfo := ""
	if strip != nil && strip.Runway != nil {
		depInfo = *strip.Runway
	}

	return viffPushState{
		Params: SetCdmDataParams{
			Callsign: callsign,
			Tobt:     normalizeViffCdmTime(helpers.ValueOrDefault(data.EffectiveTobt())),
			Tsat:     tsat,
			Ttot:     normalizeViffCdmTime(helpers.ValueOrDefault(data.EffectiveTtot())),
			Ctot:     truncateCDMClockValue(helpers.ValueOrDefault(data.EffectiveCtot())),
			Reason:   helpers.ValueOrDefault(data.EcfmpID),
			Asrt:     normalizeViffCdmTime(helpers.ValueOrDefault(data.Asrt)),
			DepInfo:  depInfo,
		},
	}, true
}

func (s *Service) markViffPushPending(session int32, callsign string, state viffPushState) bool {
	key := viffPushKey(session, callsign)
	current, ok := s.lastPushedViff.Load(key)
	if ok && current.(viffPushState) == state {
		return false
	}
	s.lastPushedViff.Store(key, state)
	return true
}

func (s *Service) clearPendingViffPush(session int32, callsign string, state viffPushState) {
	key := viffPushKey(session, callsign)
	current, ok := s.lastPushedViff.Load(key)
	if ok && current.(viffPushState) == state {
		s.lastPushedViff.Delete(key)
	}
}

func viffPushKey(session int32, callsign string) string {
	return strconv.Itoa(int(session)) + ":" + callsign
}

func (s *Service) pushLatestMasterCdmDataToViff(ctx context.Context, session int32, callsign string, strip *models.Strip) error {
	data, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	return s.pushViffAfterRecalc(ctx, callsign, strip, data)
}

func (s *Service) refreshMasterFlightFromViff(ctx context.Context, session int32, callsign string, airport string) error {
	payload, err := s.client.IFPSByCallsign(ctx, callsign)
	if err != nil {
		return err
	}

	row, err := parseIFPSByCallsignResponse(payload)
	if err != nil || row == nil {
		return err
	}

	flight, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	nextCtot, nextCtotSource := effectiveIfpsCtotAndSource(*row)
	_, needsRecalculate, err := s.mergeMasterViffFlight(ctx, session, callsign, flight, *row, nextCtot, nextCtotSource)
	if err != nil {
		return err
	}
	if !needsRecalculate || s.sequenceService == nil || airport == "" || !s.canRunLocalRecalculation(session) {
		return nil
	}

	if err := s.sequenceService.RecalculateAirportSilently(ctx, session, airport); err != nil {
		return err
	}
	s.pushCdmDataAfterRecalc(ctx, session, callsign)
	return nil
}

func parseIFPSByCallsignResponse(payload []byte) (*IFPSData, error) {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" || trimmed == "null" || trimmed == "true" || trimmed == "false" {
		return nil, nil
	}

	var single IFPSData
	if err := json.Unmarshal(payload, &single); err == nil {
		if strings.TrimSpace(single.Callsign) == "" {
			return nil, nil
		}
		return &single, nil
	}

	var many []IFPSData
	if err := json.Unmarshal(payload, &many); err != nil {
		return nil, err
	}
	for _, row := range many {
		if strings.TrimSpace(row.Callsign) != "" {
			result := row
			return &result, nil
		}
	}
	return nil, nil
}

func normalizeViffCdmTime(value string) string {
	value = normalizeCalculationClock(value)
	if value == "" {
		return ""
	}
	return toHHMMSS(value)
}

func isValidHHMM(value string) bool {
	if len(value) != 4 {
		return false
	}
	_, ok := parseClock(value)
	if !ok {
		return false
	}
	hours := value[0:2]
	minutes := value[2:4]
	return hours < "24" && minutes < "60"
}

func isValidDeiceType(value string) bool {
	switch value {
	case "", "L", "M", "H", "J":
		return true
	default:
		return false
	}
}

func stringPointerIfPresent(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	result := value
	return &result
}

func (s *Service) loadCdmActionTarget(ctx context.Context, session int32, callsign string) (*models.Strip, *models.CdmData, error) {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	cdmData, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return strip, (&models.CdmData{}).Normalize(), nil
		}
		return nil, nil, err
	}
	return strip, cdmData, nil
}

func (s *Service) prepareTobtUpdate(ctx context.Context, session int32, callsign string, tobt string, sourcePosition string, now time.Time) (*models.Strip, cdmSnapshot, *models.CdmData, string, bool, bool, error) {
	strip, cdmData, err := s.loadCdmActionTarget(ctx, session, callsign)
	if err != nil {
		return nil, cdmSnapshot{}, nil, "", false, false, err
	}
	if strip == nil || cdmData == nil {
		return strip, cdmSnapshot{}, nil, "", false, false, nil
	}

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()
	previousTobt := helpers.ValueOrDefault(updated.Tobt)
	shouldTriggerRecalculate := shouldTriggerClockRecalculation(tobt, now) || shouldForceRecalculateForStaleSequence(updated, now)
	prospective := updated.Clone()
	applyConfirmedTobtUpdate(prospective, tobt, sourcePosition)
	metadataChanged := snapshotCdm(prospective) != before
	if previousTobt == tobt && helpers.ValueOrDefault(updated.ReqTobt) == "" && !shouldTriggerRecalculate && !metadataChanged {
		return strip, before, updated, previousTobt, false, false, nil
	}

	updated = prospective
	if shouldTriggerRecalculate {
		updated.MarkLocalRecalculationPending()
	}
	return strip, before, updated, previousTobt, true, shouldTriggerRecalculate, nil
}

func applyConfirmedTobtUpdate(updated *models.CdmData, tobt string, sourcePosition string) {
	if updated == nil {
		return
	}
	updated.Tobt = &tobt
	setBy := strings.TrimSpace(sourcePosition)
	updated.TobtSetBy = &setBy
	confirmedBy := models.TobtConfirmedByATC
	updated.TobtConfirmedBy = &confirmedBy
	updated.TobtAutoSynced = false
	updated.TobtManuallyConfirmed = true
	updated.ReqTobt = nil
	updated.ReqTobtType = nil
}

func (s *Service) SyncAsatForGroundState(ctx context.Context, session int32, callsign string, groundState string) error {
	callsign = strings.TrimSpace(callsign)
	groundState = strings.ToUpper(strings.TrimSpace(groundState))
	if callsign == "" {
		return nil
	}

	_, cdmData, err := s.loadCdmActionTarget(ctx, session, callsign)
	if err != nil {
		return err
	}
	if cdmData == nil {
		return nil
	}

	currentAsat := helpers.ValueOrDefault(cdmData.Asat)
	shouldHaveAsat := groundStateAllowsAsat(groundState)

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()
	changed := false

	switch {
	case shouldHaveAsat && currentAsat == "":
		now := time.Now().UTC().Format("1504")
		updated.Asat = &now
		updated.Aobt = &now
		changed = true
		s.pushAobtAsync(session, callsign, now)
	case !shouldHaveAsat && currentAsat != "":
		updated.Asat = nil
		updated.Aobt = nil
		updated.MarkLocalRecalculationPending()
		changed = true
		s.pushAobtAsync(session, callsign, "")
	}

	if !changed {
		return nil
	}

	return s.persistCdmUpdate(ctx, session, callsign, before, updated)
}

func (s *Service) pushAobtAsync(session int32, callsign, aobt string) {
	if !s.client.isValid || !s.isMasterSession(session) || !s.usesViffSession(session) {
		return
	}
	value := "AOBT/NULL"
	if aobt != "" {
		value = "AOBT/" + aobt
	}
	go func() {
		if err := s.client.IFPSDpi(context.Background(), callsign, value); err != nil {
			slog.Warn("Failed to push AOBT to CDM backend",
				slog.String("callsign", callsign),
				slog.String("value", value),
				slog.Any("error", err),
			)
		}
	}()
}

func (s *Service) pushCorrectedEobtToEuroscope(session int32, callsign, eobt string) {
	if s.euroscopeHub == nil || !s.isMasterSession(session) || strings.TrimSpace(eobt) == "" {
		return
	}

	masterCallsign := strings.TrimSpace(s.euroscopeHub.GetMasterCallsign(session))
	if masterCallsign != "" && s.controllerRepo != nil {
		controller, err := s.controllerRepo.GetByCallsign(context.Background(), session, masterCallsign)
		if err == nil && controller != nil && controller.Cid != nil && strings.TrimSpace(*controller.Cid) != "" {
			s.euroscopeHub.SendEobt(session, strings.TrimSpace(*controller.Cid), callsign, eobt)
			return
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("Failed to resolve master controller CID for EOBT sync",
				slog.Int("session", int(session)),
				slog.String("master_callsign", masterCallsign),
				slog.Any("error", err),
			)
		}
	}

	s.euroscopeHub.Broadcast(session, euroscopeEvents.EobtEvent{
		Callsign: callsign,
		Eobt:     eobt,
	})
}

func groundStateAllowsAsat(groundState string) bool {
	switch strings.ToUpper(strings.TrimSpace(groundState)) {
	case "STUP", "ST-UP", "PUSH", "TAXI", "DEPA":
		return true
	default:
		return false
	}
}

func (s *Service) persistCdmUpdate(ctx context.Context, session int32, callsign string, before cdmSnapshot, updated *models.CdmData) error {
	rows, err := s.stripRepo.SetCdmData(ctx, session, callsign, updated.Normalize())
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("failed to persist CDM data for %s session %d", callsign, session)
	}
	s.broadcastIfChanged(session, callsign, before, snapshotCdm(updated))
	return nil
}

func (s *Service) persistCdmUpdateSilently(ctx context.Context, session int32, callsign string, updated *models.CdmData) error {
	rows, err := s.stripRepo.SetCdmData(ctx, session, callsign, updated.Normalize())
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("failed to persist CDM data for %s session %d", callsign, session)
	}
	return nil
}

func (s *Service) masterPosition() string {
	return DefaultMasterPosition
}

func (s *Service) registerMasterAsync(airport string) {
	if !s.client.isValid || airport == "" {
		return
	}
	position := s.masterPosition()
	go func() {
		if err := s.client.SetMasterAirport(context.Background(), airport, position); err != nil {
			slog.Warn("Failed to register CDM master airport",
				slog.String("airport", airport),
				slog.String("position", position),
				slog.Any("error", err),
			)
		}
	}()
}

func (s *Service) finalizeClxTobtUpdate(ctx context.Context, session int32, callsign string, airport string, shouldTriggerRecalculate bool) error {
	if shouldTriggerRecalculate && s.sequenceService != nil && airport != "" && s.canRunLocalRecalculation(session) {
		if err := s.sequenceService.RecalculateAirportSilently(ctx, session, airport); err != nil {
			return err
		}
	}
	s.pushCdmDataAfterRecalc(ctx, session, callsign)
	return nil
}

func shouldTriggerClockRecalculation(value string, now time.Time) bool {
	normalized := normalizeCalculationClock(value)
	if normalized == "" {
		return false
	}
	return !isMoreThanMinutesPast(normalized, now.UTC().Format("1504"), 0)
}

func applyAutoSyncedTobtUpdate(updated *models.CdmData, tobt string) {
	if updated == nil {
		return
	}
	updated.Tobt = &tobt
	updated.TobtSetBy = nil
	updated.TobtConfirmedBy = nil
	updated.TobtAutoSynced = true
	updated.TobtManuallyConfirmed = false
}

func shouldSyncEobtToTobt(eobt string, now time.Time) bool {
	return shouldTriggerClockRecalculation(eobt, now)
}

func hasProtectedConfirmedTobt(data *models.CdmData, currentEobt string) bool {
	if data == nil {
		return false
	}

	currentTobt := normalizeCalculationClock(helpers.ValueOrDefault(data.EffectiveTobt()))
	if currentTobt == "" {
		return false
	}

	if data.TobtAutoSynced {
		return false
	}

	confirmedBy := strings.TrimSpace(helpers.ValueOrDefault(data.TobtConfirmedBy))
	if confirmedBy == "" {
		return false
	}

	if data.TobtManuallyConfirmed {
		return true
	}

	// Legacy auto-follow TOBTs were stored as ATC-confirmed while mirroring EOBT exactly.
	// Allow those to keep following subsequent EOBT updates.
	return !(confirmedBy == models.TobtConfirmedByATC && currentTobt == normalizeCalculationClock(currentEobt))
}

func shouldForceRecalculateForStaleSequence(data *models.CdmData, now time.Time) bool {
	if data == nil {
		return false
	}

	if helpers.ValueOrDefault(data.EffectivePhase()) == "I" {
		return true
	}

	nowClock := timeToClock(now)
	tsat := normalizeCalculationClock(helpers.ValueOrDefault(data.EffectiveTsat()))
	return tsat != "" && isMoreThanMinutesPast(tsat, nowClock, 5)
}

func (s *Service) canRunLocalRecalculation(session int32) bool {
	return s.isMasterSession(session)
}

func hasLowVisRunwayStatus(runwayStatus map[string]string) bool {
	for _, status := range runwayStatus {
		if strings.EqualFold(strings.TrimSpace(status), "LOW_VIS") {
			return true
		}
	}
	return false
}

func (s *Service) pushTobtAsync(session int32, callsign string, previousTobt string, tobt string) {
	if !s.client.isValid {
		return
	}
	if strings.TrimSpace(previousTobt) == tobt {
		return
	}
	go func() {
		if err := s.PushTobt(context.Background(), session, callsign, tobt); err != nil {
			slog.Warn("Failed to push TOBT to CDM backend",
				slog.Int("session", int(session)),
				slog.String("callsign", callsign),
				slog.String("tobt", tobt),
				slog.Any("error", err),
			)
		}
	}()
}

func (s *Service) resolveTaxiMinutes(strip *models.Strip) int {
	if strip == nil {
		return DefaultCDMTaxiMinutes
	}
	configSnapshot := NewDefaultAirportConfig(strip.Origin)
	if s.configProvider != nil {
		if configForAirport := s.configProvider.ConfigForAirport(strip.Origin); configForAirport != nil {
			configSnapshot = configForAirport
		}
	}
	return resolveTaxiMinutesForStrip(strip, configSnapshot)
}
