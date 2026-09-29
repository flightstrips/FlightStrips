package cdm

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strings"
	"time"

	"FlightStrips/internal/models"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/helpers"

	"github.com/jackc/pgx/v5"
)

type ActionService struct {
	service *Service
}

type preparedEobtUpdate struct {
	updated                  *models.CdmData
	normalizedEobt           string
	previousEobt             string
	previousTobt             string
	clamped                  bool
	markerChanged            bool
	shouldForceRecalculate   bool
	shouldTriggerRecalculate bool
	shouldSyncTobt           bool
}

func (c *ActionService) HandleTobtUpdate(ctx context.Context, session int32, callsign string, tobt string, sourcePosition string, sourceRole string) error {
	s := c.service
	callsign = strings.TrimSpace(callsign)
	tobt = strings.TrimSpace(tobt)
	if callsign == "" || !isValidHHMM(tobt) {
		return nil
	}

	strip, before, updated, previousTobt, changed, shouldTriggerRecalculate, err := c.prepareTobtUpdate(ctx, session, callsign, tobt, sourcePosition, sourceRole, time.Now().UTC())
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
	c.pushTobtAsync(ctx, session, callsign, previousTobt, tobt)
	return nil
}

func (c *ActionService) HandleEobtUpdate(ctx context.Context, session int32, callsign string, eobt string, sourcePosition string, sourceRole string) error {
	s := c.service
	callsign = strings.TrimSpace(callsign)
	eobt = strings.TrimSpace(eobt)
	if callsign == "" {
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

	now := time.Now().UTC()
	before := snapshotCdm(cdmData)
	prepared := c.prepareEobtUpdate(session, cdmData, eobt, now, true)
	if prepared.previousEobt == prepared.normalizedEobt && !prepared.shouldForceRecalculate && !prepared.markerChanged {
		if prepared.clamped && eobt != prepared.normalizedEobt {
			c.pushCorrectedEobtToEuroscope(ctx, session, callsign, prepared.normalizedEobt)
		}
		return nil
	}

	if err := s.persistCdmUpdate(ctx, session, callsign, before, prepared.updated); err != nil {
		return err
	}
	if prepared.clamped && eobt != prepared.normalizedEobt {
		c.pushCorrectedEobtToEuroscope(ctx, session, callsign, prepared.normalizedEobt)
	}

	if prepared.shouldTriggerRecalculate {
		s.TriggerRecalculate(ctx, session, strip.Origin)
		if prepared.shouldSyncTobt {
			c.pushTobtAsync(ctx, session, callsign, prepared.previousTobt, prepared.normalizedEobt)
		}
	}
	return nil
}

func (c *ActionService) prepareEobtUpdate(session int32, data *models.CdmData, eobt string, now time.Time, replaceCapMarker bool) preparedEobtUpdate {
	updated := data.Clone()
	normalizedEobt, clamped := c.normalizeMasterEobtValue(session, eobt, now)
	markerChanged := setEobtCapReasonMarker(updated, clamped, replaceCapMarker)
	shouldForceRecalculate := shouldForceRecalculateForStaleSequence(updated, now)
	previousEobt := helpers.ValueOrDefault(updated.EffectiveEobt())
	previousTobt := helpers.ValueOrDefault(updated.EffectiveTobt())
	updated.Eobt = &normalizedEobt
	shouldAlignTobtWithEobt := shouldSyncEobtToTobt(normalizedEobt, now)
	shouldSyncTobt := shouldAlignTobtWithEobt && !hasProtectedConfirmedTobt(updated, previousEobt)
	shouldTriggerRecalculate := clamped || shouldAlignTobtWithEobt || shouldForceRecalculate
	if shouldSyncTobt {
		applyAutoSyncedTobtUpdate(updated, normalizedEobt)
	}
	if shouldTriggerRecalculate {
		updated.MarkLocalRecalculationPending()
	}

	return preparedEobtUpdate{
		updated:                  updated,
		normalizedEobt:           normalizedEobt,
		previousEobt:             previousEobt,
		previousTobt:             previousTobt,
		clamped:                  clamped,
		markerChanged:            markerChanged,
		shouldForceRecalculate:   shouldForceRecalculate,
		shouldTriggerRecalculate: shouldTriggerRecalculate,
		shouldSyncTobt:           shouldSyncTobt,
	}
}

// PrepareEuroscopeEobtSync applies the same EOBT/TOBT rules used by controller
// EOBT actions before an incoming EuroScope strip is persisted. This keeps the
// initial session sync from bypassing master-session clamping and confirmation
// metadata cleanup.
func (c *ActionService) PrepareEuroscopeEobtSync(session int32, data *models.CdmData, eobt string, now time.Time) (*models.CdmData, string, bool) {
	if data != nil && data.TobtAutoAdjusted && strings.TrimSpace(eobt) == helpers.ValueOrDefault(data.EffectiveEobt()) {
		return data.Clone(), strings.TrimSpace(eobt), false
	}
	prepared := c.prepareEobtUpdate(session, data, strings.TrimSpace(eobt), now.UTC(), true)
	return prepared.updated, prepared.normalizedEobt, prepared.clamped
}

// PrepareEuroscopeLogonSync initializes CDM timing without rewriting the filed
// EOBT. An explicit estimate from a pilot or controller remains authoritative.
func (c *ActionService) PrepareEuroscopeLogonSync(data *models.CdmData, eobt string, now time.Time) *models.CdmData {
	updated := data.Clone()
	eobt = truncateCDMClockValue(normalizeCalculationClock(eobt))
	updated.Eobt = &eobt
	if isExplicitlyConfirmedTobt(updated) {
		return updated
	}
	tobt := eobt
	adjusted := false
	if _, valid := parseClock(eobt); valid && minutesBetween(timeToClock(now), toHHMMSS(eobt)) > 40 {
		tobt = truncateCDMClockValue(addMinutes(timeToClock(now), 30))
		adjusted = true
	}
	if tobt != helpers.ValueOrDefault(updated.EffectiveTobt()) {
		applyAutoSyncedTobtUpdate(updated, tobt)
		updated.TobtAutoAdjusted = adjusted
		updated.MarkLocalRecalculationPending()
	}
	return updated
}

func (c *ActionService) HandleClearanceTobt(ctx context.Context, session int32, callsign string) error {
	strip, data, err := c.loadCdmActionTarget(ctx, session, callsign)
	if err != nil || strip == nil || data == nil {
		return err
	}
	now := time.Now().UTC()
	current := normalizeCalculationClock(helpers.ValueOrDefault(data.EffectiveTobt()))
	tobt, adjust := clearanceTobt(data, now)
	if !adjust {
		return nil
	}
	updated := data.Clone()
	applyAutoSyncedTobtUpdate(updated, tobt)
	if applyTobtRecalculationPolicy(updated, tobt) {
		updated.MarkLocalRecalculationPending()
	}
	if err := c.service.persistCdmUpdate(ctx, session, callsign, snapshotCdm(data), updated); err != nil {
		return err
	}
	c.service.TriggerRecalculate(ctx, session, strip.Origin)
	c.pushTobtAsync(ctx, session, callsign, current, tobt)
	return nil
}

func clearanceTobt(data *models.CdmData, now time.Time) (string, bool) {
	if data == nil || isExplicitlyConfirmedTobt(data) {
		return "", false
	}
	current := normalizeCalculationClock(helpers.ValueOrDefault(data.EffectiveTobt()))
	if _, valid := parseClock(current); !valid || minutesBetween(timeToClock(now), current) <= 30 {
		return "", false
	}
	return truncateCDMClockValue(addMinutes(timeToClock(now), 15)), true
}

func isExplicitlyConfirmedTobt(data *models.CdmData) bool {
	return data != nil && !data.TobtAutoSynced && (data.TobtManuallyConfirmed || strings.TrimSpace(helpers.ValueOrDefault(data.TobtConfirmedBy)) != "")
}

func (c *ActionService) normalizeMasterEobtValue(session int32, eobt string, now time.Time) (string, bool) {
	normalized := truncateCDMClockValue(normalizeCalculationClock(eobt))
	if _, ok := parseClock(normalized); !ok {
		return truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget)), true
	}
	if minutesBetween(timeToClock(now), toHHMMSS(normalized)) <= masterEobtClampThreshold {
		return normalized, false
	}
	return truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget)), true
}

func (c *ActionService) normalizeExistingMasterSessionEobts(ctx context.Context, session int32, airport string, now time.Time) (bool, error) {
	s := c.service
	if strings.TrimSpace(airport) == "" {
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

func (c *ActionService) normalizeMasterLookupEobts(ctx context.Context, session int32, lookup map[string]*models.CdmData, now time.Time) (bool, error) {
	s := c.service
	if len(lookup) == 0 {
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

func (c *ActionService) normalizeMasterFlightEobt(ctx context.Context, session int32, callsign string, data *models.CdmData, now time.Time) (bool, error) {
	s := c.service
	if data == nil {
		return false, nil
	}
	if data.TobtAutoAdjusted {
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
	c.pushCorrectedEobtToEuroscope(ctx, session, callsign, normalizedEobt)
	if shouldSyncTobt {
		c.pushTobtAsync(ctx, session, callsign, previousTobt, normalizedEobt)
	}

	return true, nil
}

func (c *ActionService) HandleClxTobtUpdate(ctx context.Context, session int32, callsign string, tobt string, sourcePosition string, sourceRole string) error {
	s := c.service
	callsign = strings.TrimSpace(callsign)
	tobt = strings.TrimSpace(tobt)
	if callsign == "" || !isValidHHMM(tobt) {
		return nil
	}

	strip, _, updated, previousTobt, changed, shouldTriggerRecalculate, err := c.prepareTobtUpdate(ctx, session, callsign, tobt, sourcePosition, sourceRole, time.Now().UTC())
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

	c.pushTobtAsync(ctx, session, callsign, previousTobt, tobt)
	return nil
}

func (c *ActionService) HandleDeiceUpdate(ctx context.Context, session int32, callsign string, deiceType string) error {
	s := c.service
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

func (c *ActionService) HandleAsrtToggle(ctx context.Context, session int32, callsign string, asrt string) error {
	s := c.service
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

func (c *ActionService) HandleTsacUpdate(ctx context.Context, session int32, callsign string, tsac string) error {
	s := c.service
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

func (c *ActionService) HandleManualCtot(ctx context.Context, session int32, callsign string, ctot string) error {
	s := c.service
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

func (c *ActionService) HandleCtotRemove(ctx context.Context, session int32, callsign string) error {
	s := c.service
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

func (c *ActionService) HandleReadyRequest(ctx context.Context, session int32, callsign string, sourcePosition string, sourceRole string) error {
	s := c.service
	now := time.Now().UTC()
	strip, current, err := c.loadCdmActionTarget(ctx, session, callsign)
	if err != nil || strip == nil || current == nil {
		return err
	}
	tobt, changeTobt := readyRequestTobt(current, now)
	before := snapshotCdm(current)
	updated := current.Clone()
	changed, shouldRecalculate := false, false
	if changeTobt {
		_, before, updated, _, changed, shouldRecalculate, err = c.prepareTobtUpdate(ctx, session, callsign, tobt, sourcePosition, sourceRole, now)
	}
	if err != nil {
		return err
	}
	if strip == nil || updated == nil {
		return nil
	}
	readyTime := now.Format("1504")
	if strings.TrimSpace(helpers.ValueOrDefault(updated.Asrt)) == "" {
		updated.Asrt = &readyTime
	}
	rea := "REA"
	updated.Status = &rea
	currentEobt := normalizeCalculationClock(helpers.ValueOrDefault(updated.EffectiveEobt()))
	if currentEobt == "" || !isAfterOrEqual(readyTime, currentEobt) {
		updated.Eobt = &readyTime
	}
	updated.ReadySyncPending = s.client.isValid && s.usesViffSession(session)
	if changed || snapshotCdm(updated) != before || updated.ReadySyncPending {
		if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
			return err
		}
	}
	if !updated.ReadySyncPending {
		if shouldRecalculate {
			s.TriggerRecalculate(ctx, session, strip.Origin)
		}
		s.publisher.SendCdmWait(session, callsign)
		return nil
	}
	if err := c.completeReadyViffSync(ctx, session, strip, updated, shouldRecalculate); err != nil {
		return err
	}
	s.publisher.SendCdmWait(session, callsign)
	return nil
}

func readyRequestTobt(data *models.CdmData, now time.Time) (string, bool) {
	current := now.UTC().Format("1504")
	if data == nil {
		return current, true
	}
	tsat := normalizeCalculationClock(helpers.ValueOrDefault(data.EffectiveTsat()))
	if _, ok := parseClock(tsat); !ok {
		return current, true
	}
	delta := minutesBetween(timeToClock(now), tsat)
	if delta < -8 {
		return truncateCDMClockValue(addMinutes(timeToClock(now), 10)), true
	}
	if delta >= -8 && delta < 10 && strings.TrimSpace(helpers.ValueOrDefault(data.EffectiveCtot())) != "" {
		return helpers.ValueOrDefault(data.EffectiveTobt()), false
	}
	return current, true
}

// ReadPushbackCtot reads only the vIFF CTOT. FlightStrips is authoritative for TSAT.
func (c *ActionService) ReadPushbackCtot(ctx context.Context, session int32, callsign string) (string, error) {
	s := c.service
	if !s.client.isValid || !s.usesViffSession(session) {
		return "", errors.New("vIFF is unavailable for pushback validation")
	}
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	payload, err := s.client.IFPSByCallsign(readCtx, callsign)
	if err != nil {
		return "", err
	}
	row, err := parseIFPSByCallsignResponse(payload)
	if err != nil {
		return "", err
	}
	if row == nil || !strings.EqualFold(row.Callsign, callsign) {
		return "", errors.New("vIFF did not confirm the flight for pushback")
	}
	ctot, _ := effectiveIfpsCtotAndSource(*row)
	return ctot, nil
}

func (c *ActionService) completeReadyViffSync(ctx context.Context, session int32, strip *models.Strip, data *models.CdmData, shouldRecalculate bool) error {
	s := c.service
	if strip == nil || data == nil || !s.client.isValid || !s.usesViffSession(session) {
		return nil
	}
	// REA/1 must be acknowledged before the READY-derived TOBT is exported;
	// otherwise upstream may assign a later CTOT while processing the TOBT.
	if err := s.client.IFPSDpi(ctx, strip.Callsign, "REA/1"); err != nil {
		return err
	}
	if shouldRecalculate && s.sequenceService != nil && strings.TrimSpace(strip.Origin) != "" {
		if err := s.sequenceService.RecalculateAirport(ctx, session, strip.Origin); err != nil {
			return err
		}
		var err error
		data, err = s.stripRepo.GetCdmDataForCallsign(ctx, session, strip.Callsign)
		if err != nil {
			return err
		}
	}
	if err := s.masterViffSync.pushAuthoritativeViffState(ctx, strip.Callsign, strip, data); err != nil {
		return err
	}
	if data.ReadySyncPending {
		updated := data.Clone()
		updated.ReadySyncPending = false
		if err := s.persistCdmUpdateSilently(ctx, session, strip.Callsign, updated); err != nil {
			return err
		}
	}
	return nil
}

func (c *ActionService) RequestBetterTobt(ctx context.Context, session int32, callsign string) error {
	s := c.service
	if !s.client.isValid {
		return nil
	}
	now := time.Now().UTC()
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

func (c *ActionService) PushTobt(ctx context.Context, session int32, callsign string, tobt string) error {
	s := c.service
	if !s.client.isValid || !s.usesViffSession(session) {
		return nil
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	taxiMinutes := s.resolveTaxiMinutes(strip)
	return s.client.IFPSSetTobt(ctx, callsign, tobt, taxiMinutes)
}

func (c *ActionService) loadCdmActionTarget(ctx context.Context, session int32, callsign string) (*models.Strip, *models.CdmData, error) {
	s := c.service
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

func (c *ActionService) prepareTobtUpdate(ctx context.Context, session int32, callsign string, tobt string, sourcePosition string, sourceRole string, now time.Time) (*models.Strip, cdmSnapshot, *models.CdmData, string, bool, bool, error) {
	s := c.service
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
	prospective := updated.Clone()
	applyConfirmedTobtUpdate(prospective, tobt, sourcePosition, sourceRole)
	shouldTriggerRecalculate := applyTobtRecalculationPolicy(prospective, tobt)
	if prospective.RecalculationMode == models.CdmRecalculationRequired && prospective.HasManualCtot() {
		configSnapshot := c.configSnapshotForStrip(strip)
		taxiAndDeiceMinutes := resolveTaxiMinutesForStrip(strip, configSnapshot) +
			deiceTypeToMinutes(configSnapshot, helpers.ValueOrDefault(prospective.DeIce))
		earliestTtot := addMinutes(toHHMMSS(tobt), float64(taxiAndDeiceMinutes))
		manualCtot := toHHMMSS(helpers.ValueOrDefault(prospective.Ctot))
		if earliestTtot != "" && manualCtot != "" && minutesBetween(manualCtot, earliestTtot) > 0 {
			prospective.Ctot = nil
			prospective.CtotSource = nil
		}
	}
	metadataChanged := snapshotCdm(prospective) != before
	if previousTobt == tobt && !shouldTriggerRecalculate && !metadataChanged {
		return strip, before, updated, previousTobt, false, false, nil
	}

	updated = prospective
	return strip, before, updated, previousTobt, true, shouldTriggerRecalculate, nil
}

// applyTobtRecalculationPolicy applies the universal TOBT/TSAT protection
// window. The proposed TOBT is always retained. An existing TSAT within five
// minutes on either side is frozen, including when it is slightly earlier than
// TOBT. Outside the window a later TOBT requires a new assignment, while an
// earlier TOBT may only improve the existing assignment.
func applyTobtRecalculationPolicy(updated *models.CdmData, tobt string) bool {
	if updated == nil {
		return false
	}
	wasPending := updated.NeedsLocalRecalculation()
	previousMode := updated.RecalculationMode
	ts := normalizeCalculationClock(helpers.ValueOrDefault(updated.EffectiveTsat()))
	delta, valid := tobtTsatWindow(tobt, ts)
	if valid && math.Abs(delta) <= 5 {
		if wasPending && previousMode == models.CdmRecalculationImproveOnly {
			// The improvement belonged to an older TOBT outside the window. The
			// replacement TOBT is protected, so that work is now obsolete.
			updated.ClearLocalRecalculationPending()
			return false
		}
		// The protected window freezes this TOBT change's effect on the
		// assignment, but it must not cancel work requested by another input
		// such as de-ice, runway, EOBT or CTOT.
		return wasPending
	}
	if !valid || delta > 5 {
		updated.Tsat = nil
		updated.Ttot = nil
		if !updated.HasManualCtot() {
			updated.Ctot = nil
			updated.CtotSource = nil
			updated.MostPenalizingAirspace = nil
			updated.EcfmpID = nil
		}
		updated.MarkLocalRecalculationPending()
		return true
	}
	if wasPending && previousMode != models.CdmRecalculationImproveOnly {
		// A TOBT improvement is weaker than an already-required recalculation.
		// Keep the stronger mode so other changed inputs cannot be ignored.
		updated.MarkLocalRecalculationPending()
		return true
	}
	updated.MarkLocalImprovementPending()
	return true
}

func (c *ActionService) SyncAsatForGroundState(ctx context.Context, session int32, callsign string, groundState string) error {
	s := c.service
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
	shouldRecordAobt := groundState == euroscopeEvents.GroundStatePush && helpers.ValueOrDefault(cdmData.Aobt) == ""

	before := snapshotCdm(cdmData)
	updated := cdmData.Clone()
	changed := false

	switch {
	case shouldHaveAsat && currentAsat == "":
		now := time.Now().UTC().Format("1504")
		updated.Asat = &now
		changed = true
	case !shouldHaveAsat && currentAsat != "":
		updated.Asat = nil
		updated.MarkLocalRecalculationPending()
		changed = true
	}
	if shouldRecordAobt {
		now := time.Now().UTC().Format("1504")
		updated.Aobt = &now
		changed = true
	}

	if !changed {
		return nil
	}

	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return err
	}
	if shouldRecordAobt {
		c.pushAobtAsync(ctx, session, callsign, *updated.Aobt)
	}
	return nil
}

func (c *ActionService) RecordAobtForTransfer(ctx context.Context, session int32, callsign string) error {
	_, data, err := c.loadCdmActionTarget(ctx, session, callsign)
	if err != nil || data == nil || helpers.ValueOrDefault(data.Aobt) != "" {
		return err
	}
	now := time.Now().UTC().Format("1504")
	updated := data.Clone()
	updated.Aobt = &now
	if err := c.service.persistCdmUpdate(ctx, session, callsign, snapshotCdm(data), updated); err != nil {
		return err
	}
	c.pushAobtAsync(ctx, session, callsign, now)
	return nil
}

func (c *ActionService) RecordTakeoffClearanceAtot(ctx context.Context, session int32, callsign string) error {
	_, data, err := c.loadCdmActionTarget(ctx, session, callsign)
	if err != nil || data == nil {
		return err
	}
	if helpers.ValueOrDefault(data.Atot) != "" {
		c.pushPendingAtotAsync(ctx, session, callsign)
		return nil
	}
	now := time.Now().UTC().Format("1504")
	updated := data.Clone()
	updated.Atot = &now
	updated.AtotViffPending = c.service.client.isValid && c.service.usesViffSession(session)
	if err := c.service.persistCdmUpdate(ctx, session, callsign, snapshotCdm(data), updated); err != nil {
		return err
	}
	c.pushPendingAtotAsync(ctx, session, callsign)
	return nil
}

func (c *ActionService) pushPendingAtotAsync(ctx context.Context, session int32, callsign string) {
	if !c.service.client.isValid || !c.service.usesViffSession(session) {
		return
	}
	asyncCtx := detachedContext(ctx)
	go func() {
		if err := c.sendPendingAtot(asyncCtx, session, callsign); err != nil {
			slog.WarnContext(asyncCtx, "Failed to push ATOT to CDM backend", slog.String("callsign", callsign), slog.Any("error", err))
		}
	}()
}

func (c *ActionService) sendPendingAtot(ctx context.Context, session int32, callsign string) error {
	s := c.service
	key := viffPushKey(session, callsign)
	if _, busy := s.atotPushInFlight.LoadOrStore(key, struct{}{}); busy {
		return nil
	}
	defer s.atotPushInFlight.Delete(key)
	data, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	if data == nil || !data.AtotViffPending || helpers.ValueOrDefault(data.Atot) == "" {
		return nil
	}
	if err := s.client.IFPSDpi(ctx, callsign, "ATOT/"+helpers.ValueOrDefault(data.Atot)); err != nil {
		return err
	}
	updated := data.Clone()
	updated.AtotViffPending = false
	return s.persistCdmUpdateSilently(ctx, session, callsign, updated)
}

// PreparePushback updates an expired or premature TOBT and returns the final
// local assignment together with whether vIFF acknowledged the proposal.
func (c *ActionService) PreparePushback(ctx context.Context, session int32, callsign string) (string, string, bool, error) {
	s := c.service
	strip, data, err := c.loadCdmActionTarget(ctx, session, callsign)
	if err != nil || strip == nil || data == nil {
		return "", "", false, err
	}
	now := time.Now().UTC()
	tobt := now.Format("1504")
	_, before, updated, _, changed, shouldRecalculate, err := c.prepareTobtUpdate(ctx, session, callsign, tobt, "FlightStrips", "ATC", now)
	if err != nil {
		return "", "", false, err
	}
	if shouldRecalculate {
		// Startup approval sets ASAT before pushback. Allow this explicit
		// correction to replace the old TSAT while the flight is still on block.
		updated.PushbackRecalculate = true
	}
	if changed || shouldRecalculate {
		if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
			return "", "", false, err
		}
	}
	if s.sequenceService == nil {
		return "", "", false, nil
	}
	if err := s.sequenceService.RecalculateAirport(ctx, session, strip.Origin); err != nil {
		return "", "", false, err
	}
	data, err = s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return "", "", false, err
	}
	if !s.client.isValid || !s.usesViffSession(session) {
		return helpers.ValueOrDefault(data.EffectiveTsat()), helpers.ValueOrDefault(data.EffectiveCtot()), false, nil
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.masterViffSync.pushAuthoritativeViffState(verifyCtx, callsign, strip, data); err != nil {
		return helpers.ValueOrDefault(data.EffectiveTsat()), helpers.ValueOrDefault(data.EffectiveCtot()), false, nil
	}
	matched := 0
	for attempt := 0; attempt < 3; attempt++ {
		payload, readErr := s.client.IFPSByCallsign(verifyCtx, callsign)
		if readErr == nil {
			row, parseErr := parseIFPSByCallsignResponse(payload)
			if parseErr == nil && row != nil && (truncateCDMClockValue(row.TOBT) == tobt || truncateCDMClockValue(row.CDMData.TOBT) == tobt) {
				matched++
				if err := s.masterViffSync.refreshMasterFlightFromViff(verifyCtx, session, callsign, strip.Origin); err == nil {
					data, err = s.stripRepo.GetCdmDataForCallsign(verifyCtx, session, callsign)
					if err == nil {
						if helpers.ValueOrDefault(data.EffectiveCtot()) != "" || matched >= 2 {
							return helpers.ValueOrDefault(data.EffectiveTsat()), helpers.ValueOrDefault(data.EffectiveCtot()), true, nil
						}
					}
				}
			}
		}
		select {
		case <-verifyCtx.Done():
			return helpers.ValueOrDefault(data.EffectiveTsat()), helpers.ValueOrDefault(data.EffectiveCtot()), false, nil
		case <-time.After(500 * time.Millisecond):
		}
	}
	return helpers.ValueOrDefault(data.EffectiveTsat()), helpers.ValueOrDefault(data.EffectiveCtot()), false, nil
}

func (c *ActionService) pushAobtAsync(ctx context.Context, session int32, callsign, aobt string) {
	s := c.service
	if aobt == "" || !s.client.isValid || !s.usesViffSession(session) {
		return
	}
	value := "AOBT/" + aobt
	asyncCtx := detachedContext(ctx)
	go func() {
		if err := s.client.IFPSDpi(asyncCtx, callsign, value); err != nil {
			slog.Warn("Failed to push AOBT to CDM backend",
				slog.String("callsign", callsign),
				slog.String("value", value),
				slog.Any("error", err),
			)
		}
	}()
}

func (c *ActionService) pushCorrectedEobtToEuroscope(ctx context.Context, session int32, callsign, eobt string) {
	s := c.service
	if strings.TrimSpace(eobt) == "" {
		return
	}

	masterCallsign := strings.TrimSpace(s.euroscopeHub.GetMasterCallsign(session))
	if masterCallsign != "" {
		controller, err := s.controllerRepo.GetByCallsign(ctx, session, masterCallsign)
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

func (c *ActionService) finalizeClxTobtUpdate(ctx context.Context, session int32, callsign string, airport string, shouldTriggerRecalculate bool) error {
	s := c.service
	if shouldTriggerRecalculate && s.sequenceService != nil && airport != "" && s.canRunLocalRecalculation(session) {
		// A TOBT change can move more than the edited flight. Use the publishing
		// path so every persisted assignment reaches frontend, EuroScope and vIFF.
		return s.sequenceService.RecalculateAirport(ctx, session, airport)
	}
	s.pushCdmDataAfterRecalc(ctx, session, callsign)
	return nil
}

func (c *ActionService) pushTobtAsync(ctx context.Context, session int32, callsign string, previousTobt string, tobt string) {
	s := c.service
	if !s.client.isValid {
		return
	}
	if strings.TrimSpace(previousTobt) == tobt {
		return
	}
	asyncCtx := detachedContext(ctx)
	go func() {
		if err := s.PushTobt(asyncCtx, session, callsign, tobt); err != nil {
			slog.Warn("Failed to push TOBT to CDM backend",
				slog.Int("session", int(session)),
				slog.String("callsign", callsign),
				slog.String("tobt", tobt),
				slog.Any("error", err),
			)
		}
	}()
}

func (c *ActionService) resolveTaxiMinutes(strip *models.Strip) int {
	if strip == nil {
		return DefaultCDMTaxiMinutes
	}
	return resolveTaxiMinutesForStrip(strip, c.configSnapshotForStrip(strip))
}

func (c *ActionService) configSnapshotForStrip(strip *models.Strip) *CdmAirportConfig {
	s := c.service
	if strip == nil {
		return NewDefaultAirportConfig("")
	}
	configSnapshot := NewDefaultAirportConfig(strip.Origin)
	if s.configProvider != nil {
		if configForAirport := s.configProvider.ConfigForAirport(strip.Origin); configForAirport != nil {
			configSnapshot = configForAirport
		}
	}
	return configSnapshot
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

func applyConfirmedTobtUpdate(updated *models.CdmData, tobt string, sourcePosition string, sourceRole ...string) {
	if updated == nil {
		return
	}
	updated.Tobt = &tobt
	setBy := strings.TrimSpace(sourcePosition)
	updated.TobtSetBy = &setBy
	confirmedBy := models.TobtConfirmedByATC
	if len(sourceRole) > 0 && strings.EqualFold(strings.TrimSpace(sourceRole[0]), "pilot") {
		confirmedBy = models.TobtConfirmedByPilot
	}
	updated.TobtConfirmedBy = &confirmedBy
	updated.TobtAutoSynced = false
	updated.TobtManuallyConfirmed = true
}

func groundStateAllowsAsat(groundState string) bool {
	switch strings.ToUpper(strings.TrimSpace(groundState)) {
	case "STUP",
		euroscopeEvents.GroundStateStartup,
		euroscopeEvents.GroundStatePush,
		euroscopeEvents.GroundStateTaxi,
		euroscopeEvents.GroundStateLineup,
		euroscopeEvents.GroundStateDepart:
		return true
	default:
		return false
	}
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
	updated.TobtAutoAdjusted = false
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
