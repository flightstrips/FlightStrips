package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"context"
	"sort"
	"strings"
	"time"
)

type SequenceService struct {
	stripRepo      repository.StripRepository
	sessionRepo    repository.SessionRepository
	configProvider ConfigProvider
	frontendHub    shared.FrontendHub
	euroscopeHub   shared.EuroscopeHub
	afterPersist   func(ctx context.Context, session int32, callsign string)
}

func NewSequenceService(stripRepo repository.StripRepository, sessionRepo repository.SessionRepository, configProvider ConfigProvider, frontendHub shared.FrontendHub, euroscopeHub shared.EuroscopeHub) *SequenceService {
	return &SequenceService{
		stripRepo:      stripRepo,
		sessionRepo:    sessionRepo,
		configProvider: configProvider,
		frontendHub:    frontendHub,
		euroscopeHub:   euroscopeHub,
	}
}

func (s *SequenceService) SetAfterPersist(fn func(ctx context.Context, session int32, callsign string)) {
	s.afterPersist = fn
}

func (s *SequenceService) RecalculateAirport(ctx context.Context, session int32, airport string) error {
	strips, err := s.stripRepo.ListByOrigin(ctx, session, airport)
	if err != nil {
		return err
	}
	sessionData, err := s.sessionRepo.GetByID(ctx, session)
	if err != nil {
		return err
	}

	config := s.configProvider.ConfigForAirport(airport)
	if config == nil {
		if defaults, ok := s.configProvider.(*CdmConfigStore); ok {
			config = defaults.DefaultConfigForAirport(airport)
		} else {
			config = &CdmAirportConfig{Airport: normalizeToken(airport), DefaultRate: 20, DefaultTaxiMinutes: 10}
		}
	}
	config = config.SnapshotWithRunways(sessionData.ActiveRunways.ArrivalRunways, sessionData.ActiveRunways.DepartureRunways)
	now := time.Now().UTC()

	candidates := make([]*models.Strip, 0, len(strips))
	for _, strip := range strips {
		if strip == nil || shouldSkipStrip(strip) {
			continue
		}
		candidates = append(candidates, strip)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return sequenceSortKey(candidates[i]) < sequenceSortKey(candidates[j])
	})

	slots := make([]SlotEntry, 0, len(candidates))
	for _, strip := range candidates {
		if !shouldRecalculateStrip(strip, now) {
			if slot, ok := existingSlotEntry(strip); ok {
				if valueOrEmpty(strip.EffectiveAsat()) != "" || valueOrEmpty(strip.EffectiveAobt()) != "" || !hasExactTtotConflict(slot, slots, config) {
					slots = append(slots, slot)
					continue
				}
			} else {
				continue
			}
		}

		// TSAT specifically expired → mark strip as invalid, keep TOBT
		if isTsatSpecificallyExpired(strip, now) {
			data := strip.CdmData
			if data == nil {
				data = (&models.CdmData{}).Normalize()
			}
			alreadyInvalid := valueOrEmpty(data.Phase) == "I" && data.Tsat == nil && data.Ttot == nil
			if !alreadyInvalid {
				updated := data.Clone()
				phase := "I"
				updated.Phase = &phase
				updated.Tsat = nil
				updated.Ttot = nil
				updated.ClearLocalRecalculationPending()
				if _, err := s.stripRepo.SetCdmData(ctx, session, strip.Callsign, updated.Normalize()); err != nil {
					return err
				}
				strip.CdmData = updated
				s.broadcast(session, strip.Callsign, updated)
				if s.afterPersist != nil {
					s.afterPersist(ctx, session, strip.Callsign)
				}
			}
			continue
		}

		result := Calculate(buildCalcInput(strip, config), slots, config, now)

		beforeTsat := strings.TrimSpace(valueOrEmpty(strip.EffectiveTsat()))
		beforeTtot := strings.TrimSpace(valueOrEmpty(strip.EffectiveTtot()))

		updated := strip.CdmData
		if updated == nil {
			updated = (&models.CdmData{}).Normalize()
		}
		updated = updated.Clone()
		beforeNeedsRecalc := updated.NeedsLocalRecalculation()
		updated.Phase = nil
		updated.Tsat = stringPointerIfPresent(result.Tsat)
		updated.Ttot = stringPointerIfPresent(result.Ttot)
		updated.ClearLocalRecalculationPending()

		if beforeTsat != result.Tsat || beforeTtot != result.Ttot || beforeNeedsRecalc {
			if _, err := s.stripRepo.SetCdmData(ctx, session, strip.Callsign, updated.Normalize()); err != nil {
				return err
			}
			strip.CdmData = updated
			s.broadcast(session, strip.Callsign, updated)
			if s.afterPersist != nil {
				s.afterPersist(ctx, session, strip.Callsign)
			}
		}

		if result.Ttot != "" {
			slots = append(slots, SlotEntry{
				Callsign:   strip.Callsign,
				Origin:     strip.Origin,
				DepRwy:     valueOrEmpty(strip.Runway),
				Sid:        valueOrEmpty(strip.Sid),
				Ttot:       result.Ttot,
				HasManCtot: updated.HasManualCtot(),
				ManCtot:    valueOrEmpty(updated.Ctot),
			})
		}
	}

	return nil
}

func buildCalcInput(strip *models.Strip, config *CdmAirportConfig) CalcInput {
	var data *models.CdmData
	if strip.CdmData != nil {
		data = strip.CdmData
	} else {
		data = (&models.CdmData{}).Normalize()
	}

	return CalcInput{
		Callsign:   strip.Callsign,
		Origin:     strip.Origin,
		DepRwy:     valueOrEmpty(strip.Runway),
		Sid:        valueOrEmpty(strip.Sid),
		Eobt:       normalizeCalculationClock(valueOrEmpty(data.EffectiveEobt())),
		Tobt:       normalizeCalculationClock(valueOrEmpty(data.EffectiveTobt())),
		ReqTobt:    normalizeCalculationClock(valueOrEmpty(data.EffectiveReqTobt())),
		Ctot:       valueOrEmpty(data.EffectiveCtot()),
		Asat:       normalizeCalculationClock(valueOrEmpty(data.EffectiveAsat())),
		TaxiMin:    config.TaxiMinutesForRunway(valueOrEmpty(strip.Runway)),
		DeIceMin:   deiceTypeToMinutes(valueOrEmpty(data.DeIce)),
		HasManCtot: data.HasManualCtot(),
		ManCtot:    valueOrEmpty(data.Ctot),
	}
}

func calculationBaseTime(strip *models.Strip) string {
	if strip == nil {
		return ""
	}
	if strip.CdmData != nil {
		if tobt := normalizeCalculationClock(valueOrEmpty(strip.CdmData.EffectiveTobt())); tobt != "" {
			return tobt
		}
		if req := normalizeCalculationClock(valueOrEmpty(strip.CdmData.EffectiveReqTobt())); req != "" {
			return req
		}
		if eobt := normalizeCalculationClock(valueOrEmpty(strip.CdmData.EffectiveEobt())); eobt != "" {
			return eobt
		}
	}
	return ""
}

func sequenceSortKey(strip *models.Strip) string {
	if base := calculationBaseTime(strip); base != "" {
		return base
	}
	if strip != nil && strip.CdmData != nil {
		if ttot := normalizeCalculationClock(valueOrEmpty(strip.CdmData.EffectiveTtot())); ttot != "" {
			return ttot
		}
	}
	return ""
}

func shouldSkipStrip(strip *models.Strip) bool {
	state := strings.ToUpper(strings.TrimSpace(valueOrEmpty(strip.State)))
	if state == "ARR" || state == "DEPA" {
		return true
	}
	return sequenceSortKey(strip) == ""
}

func shouldRecalculateStrip(strip *models.Strip, now time.Time) bool {
	if strip == nil {
		return false
	}

	data := strip.CdmData
	if data == nil {
		return true
	}
	if valueOrEmpty(data.EffectiveAsat()) != "" || valueOrEmpty(data.EffectiveAobt()) != "" {
		return false
	}
	if data.NeedsLocalRecalculation() {
		return true
	}
	// Invalid strips are not recalculated unless explicitly triggered via NeedsLocalRecalculation
	if valueOrEmpty(data.Phase) == "I" {
		return false
	}
	if localCalcTimesExpired(strip, now) {
		return true
	}
	if normalizeCalculationClock(valueOrEmpty(data.EffectiveTsat())) == "" || normalizeCalculationClock(valueOrEmpty(data.EffectiveTtot())) == "" {
		return true
	}
	return false
}

func isTsatSpecificallyExpired(strip *models.Strip, now time.Time) bool {
	if strip == nil || strip.CdmData == nil {
		return false
	}
	tsat := normalizeCalculationClock(valueOrEmpty(strip.CdmData.EffectiveTsat()))
	if tsat == "" {
		return false
	}
	return isMoreThanMinutesPast(tsat, timeToClock(now), 5)
}

func localCalcTimesExpired(strip *models.Strip, now time.Time) bool {
	if strip == nil || strip.CdmData == nil {
		return false
	}

	nowHHMMSS := timeToClock(now)

	// When a TSAT exists, use it as the expiry signal: each strip expires on its
	// own TSAT, not on the shared TOBT. This prevents all strips with the same
	// TOBT from being force-recalculated (and cleared) the moment their common
	// TOBT crosses the 5-minute threshold, even when some of those strips still
	// have a valid TSAT that was pushed forward by sequencing.
	if tsat := normalizeCalculationClock(valueOrEmpty(strip.CdmData.EffectiveTsat())); tsat != "" {
		return isMoreThanMinutesPast(tsat, nowHHMMSS, 5)
	}

	// Fall back to TOBT expiry only when no TSAT has been assigned yet.
	if tobt := normalizeCalculationClock(valueOrEmpty(strip.CdmData.EffectiveTobt())); tobt != "" {
		return isMoreThanMinutesPast(tobt, nowHHMMSS, 5)
	}

	return false
}

func existingSlotEntry(strip *models.Strip) (SlotEntry, bool) {
	if strip == nil || strip.CdmData == nil {
		return SlotEntry{}, false
	}

	ttot := normalizeCalculationClock(valueOrEmpty(strip.CdmData.EffectiveTtot()))
	if ttot == "" {
		return SlotEntry{}, false
	}

	return SlotEntry{
		Callsign:   strip.Callsign,
		Origin:     strip.Origin,
		DepRwy:     valueOrEmpty(strip.Runway),
		Sid:        valueOrEmpty(strip.Sid),
		Ttot:       ttot,
		HasManCtot: strip.CdmData.HasManualCtot(),
		ManCtot:    valueOrEmpty(strip.CdmData.Ctot),
	}, true
}

func hasExactTtotConflict(candidate SlotEntry, slots []SlotEntry, config *CdmAirportConfig) bool {
	for _, slot := range slots {
		if !strings.EqualFold(strings.TrimSpace(slot.Origin), strings.TrimSpace(candidate.Origin)) {
			continue
		}
		if !sameOrDependentRunway(candidate.DepRwy, slot.DepRwy, config) {
			continue
		}
		if toHHMMSS(slot.Ttot) == toHHMMSS(candidate.Ttot) {
			return true
		}
	}

	return false
}

func (s *SequenceService) broadcast(session int32, callsign string, data *models.CdmData) {
	if s.frontendHub != nil {
		s.frontendHub.SendCdmUpdate(
			session,
			callsign,
			truncateCDMClockValue(valueOrEmpty(data.EffectiveEobt())),
			truncateCDMClockValue(valueOrEmpty(data.EffectiveTobt())),
			truncateCDMClockValue(valueOrEmpty(data.EffectiveTsat())),
			truncateCDMClockValue(valueOrEmpty(data.Ctot)),
		)
	}
	if s.euroscopeHub != nil {
		s.euroscopeHub.Broadcast(session, buildCdmUpdateEvent(callsign, data))
	}
}

func buildCdmUpdateEvent(callsign string, data *models.CdmData) euroscopeEvents.CdmUpdateEvent {
	if data == nil {
		data = (&models.CdmData{}).Normalize()
	}
	return euroscopeEvents.CdmUpdateEvent{
		Callsign:   callsign,
		Eobt:       truncateCDMClockValue(valueOrEmpty(data.EffectiveEobt())),
		Tobt:       truncateCDMClockValue(valueOrEmpty(data.EffectiveTobt())),
		TobtSetBy:       valueOrEmpty(data.TobtSetBy),
		TobtConfirmedBy: valueOrEmpty(data.TobtConfirmedBy),
		ReqTobt:    truncateCDMClockValue(valueOrEmpty(data.EffectiveReqTobt())),
		Tsat:       truncateToHHMM(valueOrEmpty(data.EffectiveTsat())),
		Ttot:       truncateToHHMM(valueOrEmpty(data.EffectiveTtot())),
		Ctot:       truncateCDMClockValue(valueOrEmpty(data.EffectiveCtot())),
		CtotSource: valueOrEmpty(data.CtotSource),
		Asat:       truncateCDMClockValue(valueOrEmpty(data.EffectiveAsat())),
		Asrt:       truncateCDMClockValue(valueOrEmpty(data.Asrt)),
		Tsac:       valueOrEmpty(data.Tsac),
		Status:     valueOrEmpty(data.EffectiveStatus()),
		EcfmpID:    valueOrEmpty(data.EcfmpID),
		Phase:      valueOrEmpty(data.EffectivePhase()),
	}
}

func truncateToHHMM(value string) string {
	if len(value) > 4 {
		return value[:4]
	}
	return value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
