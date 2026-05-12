package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type sequencingCandidate struct {
	strip       *models.Strip
	input       CalcInput
	baseTime    string
	naturalTtot string
	slot        SlotEntry
	hasSlot     bool
	hasCtot     bool
	started     bool
	staleBase   bool
}

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
	return s.recalculateAirport(ctx, session, airport, true)
}

func (s *SequenceService) RecalculateAirportSilently(ctx context.Context, session int32, airport string) error {
	return s.recalculateAirport(ctx, session, airport, false)
}

func (s *SequenceService) recalculateAirport(ctx context.Context, session int32, airport string, notify bool) (err error) {
	ctx, span := otel.Tracer("cdm").Start(ctx, "cdm.recalculate_airport",
		trace.WithAttributes(
			attribute.Int("session", int(session)),
			attribute.String("airport", strings.ToUpper(strings.TrimSpace(airport))),
			attribute.Bool("notify", notify),
		),
	)
	start := time.Now()
	defer func() {
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
		} else {
			span.SetStatus(codes.Ok, "")
		}
		slog.InfoContext(ctx, "CDM recalculation finished",
			slog.Int("session", int(session)),
			slog.String("airport", airport),
			slog.Bool("notify", notify),
			slog.Duration("duration", time.Since(start)),
		)
		span.End()
	}()

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
	nowHHMMSS := timeToClock(now)

	candidates := make([]sequencingCandidate, 0, len(strips))
	for _, strip := range strips {
		if strip == nil || shouldSkipStrip(strip) {
			continue
		}
		calcInput := buildCalcInput(strip, config)
		slot, hasSlot := existingSlotEntry(strip)
		candidates = append(candidates, sequencingCandidate{
			strip:       strip,
			input:       calcInput,
			baseTime:    selectCalculationBase(calcInput),
			naturalTtot: unconstrainedTtot(calcInput, config, now),
			slot:        slot,
			hasSlot:     hasSlot,
			hasCtot:     strings.TrimSpace(valueOrEmpty(strip.EffectiveCtot())) != "",
			started:     valueOrEmpty(strip.EffectiveAsat()) != "" || valueOrEmpty(strip.EffectiveAobt()) != "",
			staleBase:   shouldInvalidateStaleTobt(calcInput, nowHHMMSS),
		})
	}
	span.SetAttributes(attribute.Int("candidate_count", len(candidates)))

	preserved := make([]sequencingCandidate, 0, len(candidates))
	recalculate := make([]sequencingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if shouldRecalculateStrip(candidate.strip, now) || !candidate.hasSlot {
			recalculate = append(recalculate, candidate)
			continue
		}
		preserved = append(preserved, candidate)
	}

	sort.SliceStable(preserved, func(i, j int) bool {
		return compareSequencingCandidates(preserved[i], preserved[j], now, true) < 0
	})

	slots := make([]SlotEntry, 0, len(candidates))
	for _, candidate := range preserved {
		if candidate.started {
			slots = append(slots, candidate.slot)
			continue
		}
		if !hasPreservedSlotConflict(candidate.slot, slots, config) &&
			!preservedSlotBlocksHigherPriorityCandidate(candidate, recalculate, config, now) {
			slots = append(slots, candidate.slot)
			continue
		}
		recalculate = append(recalculate, candidate)
	}

	sort.SliceStable(recalculate, func(i, j int) bool {
		return compareSequencingCandidates(recalculate[i], recalculate[j], now, false) < 0
	})

	for _, candidate := range recalculate {
		strip := candidate.strip
		if !shouldRecalculateStrip(strip, now) {
			if slot, ok := existingSlotEntry(strip); ok {
				if valueOrEmpty(strip.EffectiveAsat()) != "" || valueOrEmpty(strip.EffectiveAobt()) != "" || canRetainExistingSlot(candidate, slot, slots, config, now) {
					slots = append(slots, slot)
					continue
				}
			} else {
				continue
			}
		}

		calcInput := candidate.input

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
				applyCalculationSnapshot(updated, calcInput, valueOrEmpty(strip.Runway), models.CdmInvalidReasonStaleTsat)
				setCalculationReasonMarkers(updated, invalidReasonMarker(updated))
				updated.ClearLocalRecalculationPending()
				rows, err := s.stripRepo.SetCdmData(ctx, session, strip.Callsign, updated.Normalize())
				if err != nil {
					return err
				}
				if rows != 1 {
					return fmt.Errorf("failed to persist recalculated CDM data for %s session %d", strip.Callsign, session)
				}
				strip.CdmData = updated
				if notify {
					s.broadcast(session, strip.Callsign, updated)
				}
				if notify && s.afterPersist != nil {
					s.afterPersist(ctx, session, strip.Callsign)
				}
			}
			continue
		}

		result, trace := calculateWithTrace(calcInput, slots, config, now)

		beforeTsat := strings.TrimSpace(valueOrEmpty(strip.EffectiveTsat()))
		beforeTtot := strings.TrimSpace(valueOrEmpty(strip.EffectiveTtot()))
		beforeTaxiMinutes := intValue(updatedTaxiMinutes(strip))
		beforeTaxiRunway := strings.TrimSpace(valueOrEmpty(updatedTaxiRunway(strip)))

		updated := strip.CdmData
		if updated == nil {
			updated = (&models.CdmData{}).Normalize()
		}
		updated = updated.Clone()
		beforeNeedsRecalc := updated.NeedsLocalRecalculation()
		updated.Phase = nil
		updated.Tsat = stringPointerIfPresent(result.Tsat)
		updated.Ttot = stringPointerIfPresent(result.Ttot)
		applyCalculationSnapshot(updated, calcInput, valueOrEmpty(strip.Runway), calculationInvalidReason(calcInput, result, now))
		setCalculationReasonMarkers(updated, movementReasonMarkersFromTrace(trace))
		updated.ClearLocalRecalculationPending()

		if beforeTsat != result.Tsat || beforeTtot != result.Ttot || beforeNeedsRecalc || beforeTaxiMinutes != calcInput.TaxiMin || beforeTaxiRunway != strings.TrimSpace(valueOrEmpty(updatedTaxiRunwayFromData(updated))) {
			rows, err := s.stripRepo.SetCdmData(ctx, session, strip.Callsign, updated.Normalize())
			if err != nil {
				return err
			}
			if rows != 1 {
				return fmt.Errorf("failed to persist recalculated CDM data for %s session %d", strip.Callsign, session)
			}
			strip.CdmData = updated
			if notify {
				s.broadcast(session, strip.Callsign, updated)
			}
			if notify && s.afterPersist != nil {
				s.afterPersist(ctx, session, strip.Callsign)
			}
		}

		if result.Ttot != "" {
			slots = append(slots, SlotEntry{
				Callsign:    strip.Callsign,
				Origin:      strip.Origin,
				Destination: strip.Destination,
				DepRwy:      valueOrEmpty(strip.Runway),
				Sid:         valueOrEmpty(strip.Sid),
				WakeCat:     valueOrEmpty(strip.AircraftCategory),
				Ttot:        result.Ttot,
				HasManCtot:  updated.HasManualCtot(),
				ManCtot:     valueOrEmpty(updated.Ctot),
			})
		}
	}

	return nil
}

func compareSequencingCandidates(left, right sequencingCandidate, anchor time.Time, preserveExistingSlots bool) int {
	if preserveExistingSlots {
		if left.started != right.started {
			if left.started {
				return -1
			}
			return 1
		}
	}

	if left.hasCtot != right.hasCtot {
		if left.hasCtot {
			return -1
		}
		return 1
	}

	if preserveExistingSlots {
		if cmp := compareClockForSort(left.slot.Ttot, right.slot.Ttot, anchor); cmp != 0 {
			return cmp
		}
	}

	if left.staleBase != right.staleBase {
		if left.staleBase {
			return 1
		}
		return -1
	}

	if cmp := compareClockForSort(left.naturalTtot, right.naturalTtot, anchor); cmp != 0 {
		return cmp
	}
	if cmp := compareClockForSort(left.baseTime, right.baseTime, anchor); cmp != 0 {
		return cmp
	}

	return strings.Compare(strings.TrimSpace(left.strip.Callsign), strings.TrimSpace(right.strip.Callsign))
}

func buildCalcInput(strip *models.Strip, config *CdmAirportConfig) CalcInput {
	var data *models.CdmData
	if strip.CdmData != nil {
		data = strip.CdmData
	} else {
		data = (&models.CdmData{}).Normalize()
	}

	return CalcInput{
		Callsign:    strip.Callsign,
		Origin:      strip.Origin,
		Destination: strip.Destination,
		DepRwy:      valueOrEmpty(strip.Runway),
		Sid:         valueOrEmpty(strip.Sid),
		WakeCat:     valueOrEmpty(strip.AircraftCategory),
		Eobt:        normalizeCalculationClock(valueOrEmpty(data.EffectiveEobt())),
		Tobt:        normalizeCalculationClock(valueOrEmpty(data.EffectiveTobt())),
		ReqTobt:     normalizeCalculationClock(valueOrEmpty(data.EffectiveReqTobt())),
		Ctot:        valueOrEmpty(data.EffectiveCtot()),
		Aobt:        normalizeCalculationClock(valueOrEmpty(data.EffectiveAobt())),
		Asat:        normalizeCalculationClock(valueOrEmpty(data.EffectiveAsat())),
		TaxiMin:     resolveTaxiMinutesForStrip(strip, config),
		DeIceMin:    deiceTypeToMinutes(config, valueOrEmpty(data.DeIce)),
		HasManCtot:  data.HasManualCtot(),
		ManCtot:     valueOrEmpty(data.Ctot),
	}
}

func resolveTaxiMinutesForStrip(strip *models.Strip, config *CdmAirportConfig) int {
	if strip == nil {
		return DefaultCDMTaxiMinutes
	}

	depRwy := valueOrEmpty(strip.Runway)
	if config == nil {
		return DefaultCDMTaxiMinutes
	}

	if strip != nil && strip.PositionLatitude != nil && strip.PositionLongitude != nil {
		if minutes, ok := config.TaxiMinutesForPosition(depRwy, *strip.PositionLatitude, *strip.PositionLongitude); ok {
			return minutes
		}
	}

	if strip.CdmData != nil {
		if minutes := strip.CdmData.EffectiveTaxiMinutesForRunway(depRwy); minutes != nil && *minutes > 0 {
			return *minutes
		}
	}

	return config.TaxiMinutesForRunway(depRwy)
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
		Callsign:    strip.Callsign,
		Origin:      strip.Origin,
		Destination: strip.Destination,
		DepRwy:      valueOrEmpty(strip.Runway),
		Sid:         valueOrEmpty(strip.Sid),
		WakeCat:     valueOrEmpty(strip.AircraftCategory),
		Ttot:        ttot,
		HasManCtot:  strip.CdmData.HasManualCtot(),
		ManCtot:     valueOrEmpty(strip.CdmData.Ctot),
	}, true
}

func hasPreservedSlotConflict(candidate SlotEntry, slots []SlotEntry, config *CdmAirportConfig) bool {
	for _, slot := range slots {
		if !strings.EqualFold(strings.TrimSpace(slot.Origin), strings.TrimSpace(candidate.Origin)) {
			continue
		}
		if violatesSameDestinationSeparation(candidate.Ttot, candidate.Destination, slot.Ttot, slot.Destination) {
			return true
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

func preservedSlotBlocksHigherPriorityCandidate(candidate sequencingCandidate, recalculate []sequencingCandidate, config *CdmAirportConfig, now time.Time) bool {
	if candidate.started {
		return false
	}

	for _, pending := range recalculate {
		if pending.started || !pending.hasCtot || pending.naturalTtot == "" {
			continue
		}
		if compareSequencingCandidates(pending, candidate, now, false) >= 0 {
			continue
		}
		if calculateWithSinglePreservedSlot(candidate.slot, pending, config, now) != pending.naturalTtot {
			return true
		}
	}

	return false
}

func canRetainExistingSlot(candidate sequencingCandidate, slot SlotEntry, slots []SlotEntry, config *CdmAirportConfig, now time.Time) bool {
	if candidate.started {
		return true
	}

	return toHHMMSS(Calculate(candidate.input, slots, config, now).Ttot) == toHHMMSS(slot.Ttot)
}

func calculateWithSinglePreservedSlot(preserved SlotEntry, candidate sequencingCandidate, config *CdmAirportConfig, now time.Time) string {
	return Calculate(candidate.input, []SlotEntry{preserved}, config, now).Ttot
}

func (s *SequenceService) broadcast(session int32, callsign string, data *models.CdmData) {
	if s.frontendHub != nil {
		s.frontendHub.SendCdmUpdate(session, shared.BuildFrontendCdmDataEvent(callsign, data))
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
		Callsign:        callsign,
		Eobt:            truncateCDMClockValue(valueOrEmpty(data.EffectiveEobt())),
		Tobt:            truncateCDMClockValue(valueOrEmpty(data.EffectiveTobt())),
		TobtSetBy:       valueOrEmpty(data.TobtSetBy),
		TobtConfirmedBy: valueOrEmpty(data.TobtConfirmedBy),
		ReqTobt:         truncateCDMClockValue(valueOrEmpty(data.EffectiveReqTobt())),
		ReqTobtType:     valueOrEmpty(data.EffectiveReqTobtType()),
		Tsat:            truncateToHHMM(valueOrEmpty(data.EffectiveTsat())),
		Ttot:            truncateToHHMM(valueOrEmpty(data.EffectiveTtot())),
		Ctot:            truncateCDMClockValue(valueOrEmpty(data.EffectiveCtot())),
		CtotSource:      valueOrEmpty(data.CtotSource),
		Asat:            truncateCDMClockValue(valueOrEmpty(data.EffectiveAsat())),
		Asrt:            truncateCDMClockValue(valueOrEmpty(data.Asrt)),
		Tsac:            valueOrEmpty(data.Tsac),
		Status:          valueOrEmpty(data.EffectiveStatus()),
		EcfmpID:         valueOrEmpty(data.EcfmpID),
		Phase:           valueOrEmpty(data.EffectivePhase()),
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

func intPointerIfPositive(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func updatedTaxiMinutes(strip *models.Strip) *int {
	if strip == nil || strip.CdmData == nil {
		return nil
	}
	return updatedTaxiMinutesFromData(strip.CdmData)
}

func updatedTaxiRunway(strip *models.Strip) *string {
	if strip == nil || strip.CdmData == nil {
		return nil
	}
	return updatedTaxiRunwayFromData(strip.CdmData)
}

func updatedTaxiMinutesFromData(data *models.CdmData) *int {
	if data == nil || data.Calculation == nil {
		return nil
	}
	return data.Calculation.TaxiMinutes
}

func updatedTaxiRunwayFromData(data *models.CdmData) *string {
	if data == nil || data.Calculation == nil {
		return nil
	}
	return data.Calculation.TaxiRunway
}

func applyCalculationSnapshot(data *models.CdmData, input CalcInput, runway string, invalidReason string) {
	if data == nil {
		return
	}

	baseTime, baseSource := selectCalculationBaseWithSource(input)
	calculation := data.Calculation.Clone()
	if calculation == nil {
		calculation = &models.CdmCalculation{}
	}
	calculation.BaseTime = stringPointerIfPresent(baseTime)
	calculation.BaseSource = stringPointerIfPresent(baseSource)
	calculation.TaxiMinutes = intPointerIfPositive(input.TaxiMin)
	calculation.TaxiRunway = stringPointerIfPresent(runway)
	calculation.InvalidReason = stringPointerIfPresent(invalidReason)

	data.Calculation = calculation
	data.Normalize()
}

func calculationInvalidReason(input CalcInput, result CalcResult, now time.Time) string {
	if result.Tsat != "" || result.Ttot != "" {
		return ""
	}
	if shouldInvalidateStaleTobt(input, timeToClock(now)) {
		return models.CdmInvalidReasonStaleTobt
	}
	return ""
}
