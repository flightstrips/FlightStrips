package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/helpers"
	"context"
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
	client           *Client
	stripRepo        repository.StripRepository
	sessionRepo      repository.SessionRepository
	controllerRepo   repository.ControllerRepository
	frontendHub      shared.FrontendHub
	euroscopeHub     shared.EuroscopeHub
	configProvider   ConfigProvider
	sequenceService  *SequenceService
	debouncer      *recalcDebouncer
	masterPosition string
	// sessionMaster tracks per-session CDM master status as an in-memory cache.
	// Populated from session.CdmMaster during syncLiveSessions and updated
	// immediately when SetSessionCdmMaster is called.
	sessionMaster sync.Map // map[int32]bool
}

const DefaultMasterPosition = "FlightStrips"

const (
	cdmSyncInterval           = 30 * time.Second
	cdmPeriodicRecalcInterval = time.Minute
)

type cdmSnapshot struct {
	Eobt, Tobt, Tsat, Ctot, CtotSource, Ttot, Asat, Asrt, Tsac, Aobt, Status, ReqTobt, EcfmpID, TobtSetBy, TobtConfirmedBy, Phase string
}

func NewCdmService(client *Client, stripRepo repository.StripRepository, sessionRepo repository.SessionRepository, controllerRepo repository.ControllerRepository) *Service {
	return &Service{
		client:           client,
		stripRepo:        stripRepo,
		sessionRepo:      sessionRepo,
		controllerRepo:   controllerRepo,
		debouncer:      newRecalcDebouncer(500 * time.Millisecond),
		masterPosition: DefaultMasterPosition,
	}
}

func (s *Service) SetMasterPosition(position string) {
	if strings.TrimSpace(position) != "" {
		s.masterPosition = strings.TrimSpace(position)
	}
}

func (s *Service) SetFrontendHub(frontendHub shared.FrontendHub) {
	s.frontendHub = frontendHub
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
			s.pushCdmDataAfterRecalc(ctx, session, callsign)
		})
	}
}

// isMasterSession returns true if the in-memory cache indicates this session is CDM master.
func (s *Service) isMasterSession(sessionID int32) bool {
	v, ok := s.sessionMaster.Load(sessionID)
	return ok && v.(bool)
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
			s.registerMasterAsync(sess.Airport)
		}
	} else {
		s.sessionMaster.Delete(sessionID)
		// Deregister from vIFF if client is valid.
		if s.client.isValid {
			sess, err := s.sessionRepo.GetByID(ctx, sessionID)
			if err == nil && sess != nil && sess.Airport != "" {
				go func() {
					if err := s.client.ClearMasterAirport(context.Background(), sess.Airport, s.masterPosition); err != nil {
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
	if !s.isMasterSession(session) {
		return
	}
	normalizedAirport := strings.ToUpper(strings.TrimSpace(airport))
	s.debouncer.Schedule(recalcDebounceKey(session, normalizedAirport), func() {
		if err := s.sequenceService.RecalculateAirport(context.Background(), session, airport); err != nil {
			slog.ErrorContext(ctx, "CDM recalculation failed", slog.Int("session", int(session)), slog.String("airport", airport), slog.Any("error", err))
		}
	})
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

	strip, cdmData, err := s.loadCdmActionTarget(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip == nil || cdmData == nil {
		return nil
	}

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()

	if helpers.ValueOrDefault(updated.Tobt) == tobt &&
		helpers.ValueOrDefault(updated.ReqTobt) == "" {
		return nil
	}

	updated.Tobt = &tobt
	setBy := strings.TrimSpace(sourcePosition)
	updated.TobtSetBy = &setBy
	confirmedBy := models.TobtConfirmedByATC
	updated.TobtConfirmedBy = &confirmedBy
	updated.ReqTobt = nil
	updated.MarkLocalRecalculationPending()

	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return err
	}

	s.TriggerRecalculate(ctx, session, strip.Origin)
	s.pushTobtAsync(session, callsign, strip, cdmData, tobt)
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
	if !s.client.isValid || !s.isMasterSession(session) {
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

func (s *Service) HandleReadyRequest(ctx context.Context, session int32, callsign string) error {
	if !s.client.isValid {
		return nil
	}

	return s.RequestBetterTobt(ctx, session, callsign)
}

func (s *Service) SetReady(ctx context.Context, session int32, callsign string) error {
	if !s.client.isValid || !s.isMasterSession(session) {
		return nil
	}
	cdmData, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if cdmData.EffectiveStatus() != nil && *cdmData.EffectiveStatus() == "REA" {
		return nil
	}

	if err := s.client.IFPSDpi(ctx, callsign, "REA/1"); err != nil {
		return err
	}

	updated := cdmData.Clone()
	rea := "REA"
	updated.Status = &rea
	rows, err := s.stripRepo.SetCdmData(ctx, session, callsign, updated)
	if err != nil {
		return err
	}

	if rows != 1 {
		return fmt.Errorf("failed to update CDM status for %s session %d", callsign, session)
	}

	s.frontendHub.SendCdmWait(session, callsign)

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

	err = s.client.IFPSDpi(ctx, callsign, status)
	if err != nil {
		return err
	}

	updated := cdmData.Clone()
	updated.Status = &status
	rows, err := s.stripRepo.SetCdmData(ctx, session, callsign, updated)
	if err != nil {
		return err
	}

	if rows != 1 {
		return fmt.Errorf("failed to update CDM status for %s session %d", callsign, session)
	}

	s.frontendHub.SendCdmWait(session, callsign)

	return nil
}

func (s *Service) PushTobt(ctx context.Context, session int32, callsign string, tobt string) error {
	if !s.client.isValid || !s.isMasterSession(session) {
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

	if !syncEnabled {
		slog.WarnContext(ctx, "CDM client is not valid, CDM data will not be synced")
	}
	if !syncEnabled && !localRecalcEnabled {
		return
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

	var syncCh <-chan time.Time
	if syncTicker != nil {
		syncCh = syncTicker.C
	}

	var recalcCh <-chan time.Time
	if recalcTicker != nil {
		recalcCh = recalcTicker.C
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-syncCh:
			if err := s.syncLiveSessions(ctx); err != nil {
				slog.ErrorContext(ctx, "Failed to sync CDM data", slog.Any("error", err))
			}
		case <-recalcCh:
			if err := s.schedulePeriodicRecalculate(ctx); err != nil {
				slog.ErrorContext(ctx, "Failed to schedule periodic CDM recalculation", slog.Any("error", err))
			}
		}
	}
}

func (s *Service) syncLiveSessions(ctx context.Context) error {
	sessions, err := s.sessionRepo.List(ctx)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if session == nil || session.Name != "LIVE" {
			continue
		}

		slog.DebugContext(ctx, "Syncing CDM data", slog.String("session", session.Name), slog.Int("id", int(session.ID)), slog.String("airport", session.Airport))

		if session.CdmMaster {
			s.sessionMaster.Store(session.ID, true)
			s.registerMasterAsync(session.Airport)
		}

		if err := s.syncCdmData(ctx, session); err != nil {
			return err
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
			changed := helpers.ValueOrDefault(flight.Ctot) != nextCtot ||
				helpers.ValueOrDefault(flight.ReqTobt) != row.CDMData.ReqTOBT ||
				helpers.ValueOrDefault(flight.EcfmpID) != row.CDMData.Reason

			if !changed {
				continue
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

			if _, err := s.stripRepo.SetCdmData(ctx, session.ID, row.Callsign, updated.Normalize()); err != nil {
				return err
			}
			s.broadcastIfChanged(session.ID, row.Callsign, before, snapshotCdm(updated))
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
				helpers.ValueOrDefault(flight.EcfmpID) != row.CDMData.Reason

			if !changed {
				continue
			}

			before := snapshotCdm(flight)
			updated := flight.Clone()
			updated.Tobt = &row.TOBT
			updated.ReqTobt = stringPointerIfPresent(row.CDMData.ReqTOBT)
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

			if _, err := s.stripRepo.SetCdmData(ctx, session.ID, row.Callsign, updated.Normalize()); err != nil {
				return err
			}
			s.broadcastIfChanged(session.ID, row.Callsign, before, snapshotCdm(updated))
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

func snapshotCdm(data *models.CdmData) cdmSnapshot {
	if data == nil {
		return cdmSnapshot{}
	}
	return cdmSnapshot{
		Eobt:       truncateCDMClockValue(helpers.ValueOrDefault(data.Eobt)),
		Tobt:       truncateCDMClockValue(helpers.ValueOrDefault(data.Tobt)),
		Tsat:       truncateCDMClockValue(helpers.ValueOrDefault(data.Tsat)),
		Ctot:       truncateCDMClockValue(helpers.ValueOrDefault(data.Ctot)),
		CtotSource: helpers.ValueOrDefault(data.CtotSource),
		Ttot:       truncateCDMClockValue(helpers.ValueOrDefault(data.Ttot)),
		Asat:       truncateCDMClockValue(helpers.ValueOrDefault(data.Asat)),
		Asrt:       truncateCDMClockValue(helpers.ValueOrDefault(data.Asrt)),
		Tsac:       helpers.ValueOrDefault(data.Tsac),
		Aobt:       truncateCDMClockValue(helpers.ValueOrDefault(data.Aobt)),
		Status:     helpers.ValueOrDefault(data.Status),
		ReqTobt:    truncateCDMClockValue(helpers.ValueOrDefault(data.ReqTobt)),
		EcfmpID:    helpers.ValueOrDefault(data.EcfmpID),
		TobtSetBy:       helpers.ValueOrDefault(data.TobtSetBy),
		TobtConfirmedBy: helpers.ValueOrDefault(data.TobtConfirmedBy),
		Phase:           helpers.ValueOrDefault(data.Phase),
	}
}

func (s *Service) broadcastIfChanged(session int32, callsign string, before, after cdmSnapshot) {
	if before == after {
		return
	}

	if s.frontendHub != nil {
		s.frontendHub.SendCdmUpdate(session, callsign, after.Eobt, after.Tobt, after.Tsat, after.Ctot)
	}

	if s.euroscopeHub != nil {
		data, err := s.stripRepo.GetCdmDataForCallsign(context.Background(), session, callsign)
		if err == nil {
			s.euroscopeHub.Broadcast(session, buildCdmUpdateEvent(callsign, data))
		}
	}
}

func (s *Service) pushCdmDataAfterRecalc(ctx context.Context, session int32, callsign string) {
	if s.frontendHub == nil && s.euroscopeHub == nil {
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

	if s.frontendHub != nil {
		s.frontendHub.SendCdmUpdate(
			session,
			callsign,
			truncateCDMClockValue(helpers.ValueOrDefault(data.EffectiveEobt())),
			truncateCDMClockValue(helpers.ValueOrDefault(data.EffectiveTobt())),
			truncateCDMClockValue(helpers.ValueOrDefault(data.EffectiveTsat())),
			truncateCDMClockValue(helpers.ValueOrDefault(data.Ctot)),
		)
	}
	if s.euroscopeHub != nil {
		s.euroscopeHub.Broadcast(session, buildCdmUpdateEvent(callsign, data))
	}

	// Load strip for departure info (runway) needed by setCdmData
	var strip *models.Strip
	if s.client.isValid {
		strip, _ = s.stripRepo.GetByCallsign(ctx, session, callsign)
	}
	s.pushViffAfterRecalcAsync(callsign, strip, data)
}

func (s *Service) pushViffAfterRecalcAsync(callsign string, strip *models.Strip, data *models.CdmData) {
	if !s.client.isValid {
		return
	}
	go func() {
		ctx := context.Background()
		if helpers.ValueOrDefault(data.EffectivePhase()) == "I" {
			if err := s.client.IFPSDpi(ctx, callsign, "SUSP"); err != nil {
				slog.WarnContext(ctx, "Failed to send SUSP to CDM backend",
					slog.String("callsign", callsign),
					slog.Any("error", err),
				)
			}
			return
		}
		tsat := truncateCDMClockValue(helpers.ValueOrDefault(data.EffectiveTsat()))
		if tsat == "" {
			return
		}
		depInfo := ""
		if strip != nil && strip.Runway != nil {
			depInfo = *strip.Runway
		}
		params := SetCdmDataParams{
			Callsign: callsign,
			Tobt:     truncateCDMClockValue(helpers.ValueOrDefault(data.EffectiveTobt())),
			Tsat:     tsat,
			Ttot:     truncateCDMClockValue(helpers.ValueOrDefault(data.EffectiveTtot())),
			Ctot:     truncateCDMClockValue(helpers.ValueOrDefault(data.EffectiveCtot())),
			Reason:   helpers.ValueOrDefault(data.EcfmpID),
			Asrt:     truncateCDMClockValue(helpers.ValueOrDefault(data.Asrt)),
			DepInfo:  depInfo,
		}
		if err := s.client.IFPSSetCdmData(ctx, params); err != nil {
			slog.WarnContext(ctx, "Failed to push CDM data to CDM backend",
				slog.String("callsign", callsign),
				slog.Any("error", err),
			)
		}
	}()
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
	if !s.client.isValid || !s.isMasterSession(session) {
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

func (s *Service) registerMasterAsync(airport string) {
	if !s.client.isValid || airport == "" {
		return
	}
	position := s.masterPosition
	if position == "" {
		position = DefaultMasterPosition
	}
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

func (s *Service) pushTobtAsync(session int32, callsign string, strip *models.Strip, cdmData *models.CdmData, tobt string) {
	if !s.client.isValid || strip == nil || cdmData == nil {
		return
	}
	if helpers.ValueOrDefault(cdmData.Tobt) == tobt {
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
	runway := ""
	if strip.Runway != nil {
		runway = *strip.Runway
	}
	return configSnapshot.TaxiMinutesForRunway(runway)
}
