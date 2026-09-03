package cdm

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"FlightStrips/internal/models"

	"github.com/jackc/pgx/v5"
)

type SyncService struct {
	service *Service
}

func (c *SyncService) SyncAirportLvoFromRunwayStatus(ctx context.Context, airport string, runwayStatus map[string]string) {
	s := c.service
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

func (c *SyncService) Start(ctx context.Context) {
	s := c.service
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

func (c *SyncService) syncSessions(ctx context.Context) error {
	s := c.service
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

		if usesViff {
			s.masterViffSync.registerMasterAsync(ctx, session.Airport)
		}
		s.SyncAirportLvoFromRunwayStatus(ctx, session.Airport, session.ActiveRunways.RunwayStatus)

		if usesViff {
			if err := s.syncCdmData(ctx, session); err != nil {
				return err
			}
		}

		s.TriggerRecalculate(ctx, session.ID, session.Airport)
	}

	return nil
}

func (c *SyncService) syncCdmData(ctx context.Context, session *models.Session) error {
	s := c.service
	if !s.client.isValid {
		return nil
	}
	if session != nil {
		s.sessionUsesViff.Store(session.ID, isViffEnabledSession(session.Name))
	}

	airport := session.Airport

	lookup, err := c.loadCdmLookup(ctx, session.ID)
	if err != nil {
		return err
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
		current, recalculatedAirport, err := c.syncMasterFlight(ctx, session, row, flight, nextCtot, nextCtotSource)
		if err != nil {
			return err
		}
		if recalculatedAirport {
			// RecalculateAirport can update every departure. Refresh the complete
			// snapshot before another vIFF row can persist a stale whole record.
			lookup, err = c.loadCdmLookup(ctx, session.ID)
			if err != nil {
				return err
			}
		} else {
			lookup[row.Callsign] = current
		}
	}

	// A READY export can fail before the flight appears in vIFF's airport
	// response. Retry from local pending state rather than depending on that
	// response to contain the callsign.
	readyPendingCallsigns := make([]string, 0)
	for callsign, flight := range lookup {
		if flight != nil && flight.ReadySyncPending {
			readyPendingCallsigns = append(readyPendingCallsigns, callsign)
		}
	}
	readyRecalculatedAirport := false
	for _, callsign := range readyPendingCallsigns {
		// An earlier READY retry may have recalculated this flight as part of
		// the same airport. Always use the latest record for the ordered export.
		flight, err := s.stripRepo.GetCdmDataForCallsign(ctx, session.ID, callsign)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return err
		}
		if flight == nil || !flight.ReadySyncPending {
			continue
		}
		strip, err := s.stripRepo.GetByCallsign(ctx, session.ID, callsign)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return err
		}
		shouldRecalculate := flight.NeedsLocalRecalculation()
		if err := s.actionService.completeReadyViffSync(ctx, session.ID, strip, flight, shouldRecalculate); err != nil {
			return err
		}
		readyRecalculatedAirport = readyRecalculatedAirport ||
			(shouldRecalculate && s.sequenceService != nil && strings.TrimSpace(strip.Origin) != "")
		current, err := s.stripRepo.GetCdmDataForCallsign(ctx, session.ID, callsign)
		if err != nil {
			return err
		}
		lookup[callsign] = current
	}
	if readyRecalculatedAirport {
		lookup, err = c.loadCdmLookup(ctx, session.ID)
		if err != nil {
			return err
		}
	}

	normalized, err := s.normalizeMasterLookupEobts(ctx, session.ID, lookup, time.Now().UTC())
	if err != nil {
		return err
	}
	if normalized {
		s.TriggerRecalculate(ctx, session.ID, airport)
	}

	return nil
}

func (c *SyncService) loadCdmLookup(ctx context.Context, session int32) (map[string]*models.CdmData, error) {
	rows, err := c.service.stripRepo.GetCdmData(ctx, session)
	if err != nil {
		return nil, err
	}
	lookup := make(map[string]*models.CdmData, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		lookup[row.Callsign] = row.Data
	}
	return lookup, nil
}

func (c *SyncService) syncMasterFlight(ctx context.Context, session *models.Session, row IFPSData, flight *models.CdmData, nextCtot string, nextCtotSource string) (*models.CdmData, bool, error) {
	s := c.service
	recalculatedAirport := false
	if flight != nil && flight.ReadySyncPending {
		strip, err := s.stripRepo.GetByCallsign(ctx, session.ID, row.Callsign)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, false, err
		}
		if err == nil {
			shouldRecalculate := flight.NeedsLocalRecalculation()
			if err := s.actionService.completeReadyViffSync(ctx, session.ID, strip, flight, shouldRecalculate); err != nil {
				return nil, false, err
			}
			recalculatedAirport = shouldRecalculate && s.sequenceService != nil && strings.TrimSpace(strip.Origin) != ""
			flight, err = s.stripRepo.GetCdmDataForCallsign(ctx, session.ID, row.Callsign)
			if err != nil {
				return nil, false, err
			}
		}
	}
	requestedTobt := truncateCDMClockValue(strings.TrimSpace(row.CDMData.ReqTOBT))
	requestSource := strings.ToUpper(strings.TrimSpace(row.CDMData.ReqTOBTType))
	if requestSource == "" {
		requestSource = "VIFF"
	}
	if flight != nil && flight.ViffRequestSyncPending && isValidHHMM(requestedTobt) &&
		valueOrEmpty(flight.Tobt) == requestedTobt && valueOrEmpty(flight.TobtConfirmedBy) == requestSource {
		if flight.NeedsLocalRecalculation() {
			if s.sequenceService == nil || strings.TrimSpace(session.Airport) == "" {
				return nil, false, errors.New("cannot retry vIFF TOBT request before local recalculation")
			}
			if err := s.sequenceService.RecalculateAirport(ctx, session.ID, session.Airport); err != nil {
				return nil, false, err
			}
			recalculatedAirport = true
			var err error
			flight, err = s.stripRepo.GetCdmDataForCallsign(ctx, session.ID, row.Callsign)
			if err != nil {
				return nil, false, err
			}
			if flight.NeedsLocalRecalculation() {
				return nil, false, errors.New("vIFF TOBT request remained pending after local recalculation")
			}
		}
		strip, err := s.stripRepo.GetByCallsign(ctx, session.ID, row.Callsign)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, false, err
		}
		if err := s.masterViffSync.pushAuthoritativeViffState(ctx, row.Callsign, strip, flight); err != nil {
			return nil, false, err
		}
		if err := s.client.IFPSDpi(ctx, row.Callsign, "REQTOBT/NULL/NULL"); err != nil {
			return nil, false, err
		}
		updated := flight.Clone()
		updated.ViffRequestSyncPending = false
		if err := s.persistCdmUpdateSilently(ctx, session.ID, row.Callsign, updated); err != nil {
			return nil, false, err
		}
		return updated, recalculatedAirport, nil
	}
	// Master: only CTOT and REQTOBT are relevant from the API; local calculation handles the rest.
	current, needsRecalculate, err := s.mergeMasterViffFlight(ctx, session.ID, row.Callsign, flight, row, nextCtot, nextCtotSource)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(row.CDMData.ReqTOBT) != "" && !isValidHHMM(requestedTobt) {
		slog.WarnContext(ctx, "Clearing malformed vIFF TOBT request",
			slog.String("callsign", row.Callsign),
			slog.String("requested_tobt", row.CDMData.ReqTOBT),
		)
		if err := s.client.IFPSDpi(ctx, row.Callsign, "REQTOBT/NULL/NULL"); err != nil {
			return nil, false, err
		}
		if current.ViffRequestSyncPending {
			updated := current.Clone()
			updated.ViffRequestSyncPending = false
			if err := s.persistCdmUpdateSilently(ctx, session.ID, row.Callsign, updated); err != nil {
				return nil, false, err
			}
			current = updated
		}
		return current, recalculatedAirport, nil
	}
	if requestedTobt == "" && current != nil && current.ViffRequestSyncPending {
		// The remote clear may have succeeded while persisting the local marker
		// failed. Finish the transaction from authoritative local state instead
		// of waiting for a REQTOBT that no longer exists upstream.
		if current.NeedsLocalRecalculation() {
			if s.sequenceService == nil || strings.TrimSpace(session.Airport) == "" {
				return nil, false, errors.New("cannot recover cleared vIFF TOBT request before local recalculation")
			}
			if err := s.sequenceService.RecalculateAirport(ctx, session.ID, session.Airport); err != nil {
				return nil, false, err
			}
			recalculatedAirport = true
			current, err = s.stripRepo.GetCdmDataForCallsign(ctx, session.ID, row.Callsign)
			if err != nil {
				return nil, false, err
			}
			if current.NeedsLocalRecalculation() {
				return nil, false, errors.New("cleared vIFF TOBT request remained pending after local recalculation")
			}
		}
		strip, err := s.stripRepo.GetByCallsign(ctx, session.ID, row.Callsign)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, false, err
		}
		if err := s.masterViffSync.pushAuthoritativeViffState(ctx, row.Callsign, strip, current); err != nil {
			return nil, false, err
		}
		updated := current.Clone()
		updated.ViffRequestSyncPending = false
		if err := s.persistCdmUpdateSilently(ctx, session.ID, row.Callsign, updated); err != nil {
			return nil, false, err
		}
		return updated, recalculatedAirport, nil
	}
	if requestedTobt != "" {
		if needsRecalculate && s.sequenceService != nil && strings.TrimSpace(session.Airport) != "" {
			if err := s.sequenceService.RecalculateAirport(ctx, session.ID, session.Airport); err != nil {
				return nil, false, err
			}
			recalculatedAirport = true
			current, err = s.stripRepo.GetCdmDataForCallsign(ctx, session.ID, row.Callsign)
			if err != nil {
				return nil, false, err
			}
		}
		strip, err := s.stripRepo.GetByCallsign(ctx, session.ID, row.Callsign)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, false, err
		}
		if err := s.masterViffSync.pushAuthoritativeViffState(ctx, row.Callsign, strip, current); err != nil {
			return nil, false, err
		}
		if err := s.client.IFPSDpi(ctx, row.Callsign, "REQTOBT/NULL/NULL"); err != nil {
			return nil, false, err
		}
		if current.ViffRequestSyncPending {
			updated := current.Clone()
			updated.ViffRequestSyncPending = false
			if err := s.persistCdmUpdateSilently(ctx, session.ID, row.Callsign, updated); err != nil {
				return nil, false, err
			}
			current = updated
		}
		return current, recalculatedAirport, nil
	}
	if needsRecalculate {
		s.TriggerRecalculate(ctx, session.ID, session.Airport)
	}

	s.ensureMasterFlightExport(ctx, session.ID, row.Callsign, current, row)
	return current, recalculatedAirport, nil
}

func (c *SyncService) persistSyncedCdmData(ctx context.Context, session int32, callsign string, before cdmSnapshot, updated *models.CdmData) error {
	s := c.service
	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return err
	}
	s.reevaluateCtotValidationAsync(ctx, session, callsign, before, snapshotCdm(updated))
	return nil
}

func (c *SyncService) reevaluateCtotValidationAsync(ctx context.Context, session int32, callsign string, before, after cdmSnapshot) {
	s := c.service
	if before.Ctot == after.Ctot {
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

func (c *SyncService) schedulePeriodicCtotValidationReevaluation(ctx context.Context) error {
	s := c.service
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

func hasLowVisRunwayStatus(runwayStatus map[string]string) bool {
	for _, status := range runwayStatus {
		if strings.EqualFold(strings.TrimSpace(status), "LOW_VIS") {
			return true
		}
	}
	return false
}
