package cdm

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/pkg/helpers"
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

		if session.CdmMaster {
			s.sessionMaster.Store(session.ID, true)
			if usesViff {
				s.masterViffSync.registerMasterAsync(ctx, session.Airport)
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

func (c *SyncService) syncCdmData(ctx context.Context, session *models.Session) error {
	s := c.service
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
			current, err := c.syncMasterFlight(ctx, session, row, flight, nextCtot, nextCtotSource)
			if err != nil {
				return err
			}
			lookup[row.Callsign] = current
			continue
		}

		if err := c.syncSlaveFlight(ctx, session.ID, row, flight, nextCtot, nextCtotSource, time.Now().UTC()); err != nil {
			return err
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

func (c *SyncService) syncMasterFlight(ctx context.Context, session *models.Session, row IFPSData, flight *models.CdmData, nextCtot string, nextCtotSource string) (*models.CdmData, error) {
	s := c.service
	// Master: only CTOT and REQTOBT are relevant from the API; local calculation handles the rest.
	current, needsRecalculate, err := s.mergeMasterViffFlight(ctx, session.ID, row.Callsign, flight, row, nextCtot, nextCtotSource)
	if err != nil {
		return nil, err
	}
	if needsRecalculate {
		s.TriggerRecalculate(ctx, session.ID, session.Airport)
	}

	s.ensureMasterFlightExport(ctx, session.ID, row.Callsign, current, row)
	return current, nil
}

func (c *SyncService) syncSlaveFlight(ctx context.Context, session int32, row IFPSData, flight *models.CdmData, nextCtot string, nextCtotSource string, now time.Time) error {
	before, updated, changed := diffRemoteCdmData(flight, row, nextCtot, nextCtotSource, now)
	if !changed {
		return nil
	}
	return c.persistSyncedCdmData(ctx, session, row.Callsign, before, updated)
}

func diffRemoteCdmData(flight *models.CdmData, row IFPSData, nextCtot string, nextCtotSource string, now time.Time) (cdmSnapshot, *models.CdmData, bool) {
	nextAsat := helpers.ValueOrDefault(flight.Asat)
	if nextAsat == "" && statusImpliesAsat(row.CDMStatus) {
		nextAsat = timeToClock(now)
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
		return cdmSnapshot{}, nil, false
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
	updated.MostPenalizingAirspace = stringPointerIfPresent(row.MostPenalizingAirspace)
	updated.EcfmpID = stringPointerIfPresent(row.CDMData.Reason)
	updated.Calculation = nil
	return before, updated, true
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

func (c *SyncService) schedulePeriodicCtotValidationReevaluation(ctx context.Context) error {
	s := c.service
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
