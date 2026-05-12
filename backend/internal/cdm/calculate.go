package cdm

import (
	"FlightStrips/internal/models"
	"math"
	"strings"
	"time"
)

const sameDestinationSeparationMinutes = 3.0

type CalcInput struct {
	Callsign    string
	Origin      string
	Destination string
	DepRwy      string
	Sid         string
	WakeCat     string
	Eobt        string
	Tobt        string
	ReqTobt     string
	Ctot        string
	Aobt        string
	Asat        string
	TaxiMin     int
	DeIceMin    int
	HasManCtot  bool
	ManCtot     string
}

type SlotEntry struct {
	Callsign    string
	Origin      string
	Destination string
	DepRwy      string
	Sid         string
	WakeCat     string
	Ttot        string
	HasManCtot  bool
	ManCtot     string
}

type CalcResult struct {
	Tsat string
	Ttot string
}

type calculationTraceEntry struct {
	Kind                   string
	AgainstCallsign        string
	AgainstTtot            string
	AgainstRunway          string
	AgainstSid             string
	RequiredSpacingMinutes float64
	FromTtot               string
	ToTtot                 string
}

func Calculate(input CalcInput, slots []SlotEntry, config *CdmAirportConfig, now time.Time) CalcResult {
	result, _ := calculateWithTrace(input, slots, config, now)
	return result
}

func calculateWithTrace(input CalcInput, slots []SlotEntry, config *CdmAirportConfig, now time.Time) (CalcResult, []calculationTraceEntry) {
	nowHHMMSS := timeToClock(now)
	if shouldInvalidateStaleTobt(input, nowHHMMSS) {
		return CalcResult{}, nil
	}

	ttot := unconstrainedTtot(input, config, now)
	if ttot == "" {
		return CalcResult{}, nil
	}

	rate := DefaultCDMRate
	if config != nil {
		rate = config.RateForRunway(input.DepRwy)
	}
	if rate <= 0 {
		rate = DefaultCDMRate
	}

	rateWindow := 60.0 / float64(rate)
	trace := make([]calculationTraceEntry, 0)

	for {
		changed := false

		for _, slot := range slots {
			if !strings.EqualFold(strings.TrimSpace(slot.Origin), strings.TrimSpace(input.Origin)) {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(slot.Callsign), strings.TrimSpace(input.Callsign)) {
				continue
			}
			if violatesSameDestinationSeparation(ttot, input.Destination, slot.Ttot, slot.Destination) {
				nextTtot := addMinutes(ttot, 0.5)
				trace = append(trace, calculationTraceEntry{
					Kind:                   "same_destination_separation",
					AgainstCallsign:        slot.Callsign,
					AgainstTtot:            slot.Ttot,
					AgainstRunway:          slot.DepRwy,
					AgainstSid:             slot.Sid,
					RequiredSpacingMinutes: sameDestinationSeparationMinutes,
					FromTtot:               ttot,
					ToTtot:                 nextTtot,
				})
				ttot = nextTtot
				changed = true
				break
			}
			if !sameOrDependentRunway(input.DepRwy, slot.DepRwy, config) {
				continue
			}

			if toHHMMSS(slot.Ttot) == ttot {
				nextTtot := addMinutes(ttot, 0.5)
				trace = append(trace, calculationTraceEntry{
					Kind:            "runway_slot_collision",
					AgainstCallsign: slot.Callsign,
					AgainstTtot:     slot.Ttot,
					AgainstRunway:   slot.DepRwy,
					AgainstSid:      slot.Sid,
					FromTtot:        ttot,
					ToTtot:          nextTtot,
				})
				ttot = nextTtot
				changed = true
				break
			}

			if shouldApplyRateWindow(input.HasManCtot, slot.HasManCtot) && withinWindow(ttot, slot.Ttot, rateWindow) {
				nextTtot := addMinutes(ttot, 0.5)
				trace = append(trace, calculationTraceEntry{
					Kind:                   "runway_rate_window",
					AgainstCallsign:        slot.Callsign,
					AgainstTtot:            slot.Ttot,
					AgainstRunway:          slot.DepRwy,
					AgainstSid:             slot.Sid,
					RequiredSpacingMinutes: rateWindow,
					FromTtot:               ttot,
					ToTtot:                 nextTtot,
				})
				ttot = nextTtot
				changed = true
				break
			}

			if sameRunway(input.DepRwy, slot.DepRwy) {
				wakeMinutes := wakeSeparationMinutes(input.WakeCat, slot.WakeCat)
				if !isAfterOrEqual(ttot, slot.Ttot) {
					wakeMinutes = wakeSeparationMinutes(slot.WakeCat, input.WakeCat)
				}
				if wakeMinutes > 0 && withinWindow(ttot, slot.Ttot, float64(wakeMinutes)) {
					nextTtot := addMinutes(ttot, 0.5)
					trace = append(trace, calculationTraceEntry{
						Kind:                   "wake_separation",
						AgainstCallsign:        slot.Callsign,
						AgainstTtot:            slot.Ttot,
						AgainstRunway:          slot.DepRwy,
						AgainstSid:             slot.Sid,
						RequiredSpacingMinutes: float64(wakeMinutes),
						FromTtot:               ttot,
						ToTtot:                 nextTtot,
					})
					ttot = nextTtot
					changed = true
					break
				}
			}

			if config != nil {
				if interval := config.SidIntervalMinutes(input.DepRwy, input.Sid, slot.Sid); interval > 0 && withinWindow(ttot, slot.Ttot, interval) {
					nextTtot := addMinutes(ttot, 0.5)
					trace = append(trace, calculationTraceEntry{
						Kind:                   "sid_interval",
						AgainstCallsign:        slot.Callsign,
						AgainstTtot:            slot.Ttot,
						AgainstRunway:          slot.DepRwy,
						AgainstSid:             slot.Sid,
						RequiredSpacingMinutes: interval,
						FromTtot:               ttot,
						ToTtot:                 nextTtot,
					})
					ttot = nextTtot
					changed = true
					break
				}
			}
		}

		if !changed {
			tsat := subtractMinutes(ttot, float64(input.TaxiMin))
			if shouldInvalidateStaleTsat(input, tsat, nowHHMMSS) {
				return CalcResult{}, trace
			}
			return CalcResult{
				Tsat: tsat,
				Ttot: ttot,
			}, trace
		}
	}
}

func unconstrainedTtot(input CalcInput, config *CdmAirportConfig, now time.Time) string {
	base := selectCalculationBase(input)
	if base == "" {
		return ""
	}

	ttot := addMinutes(toHHMMSS(base), float64(input.TaxiMin+input.DeIceMin))
	if input.HasManCtot && strings.TrimSpace(input.ManCtot) != "" {
		manual := toHHMMSS(input.ManCtot)
		if !isAfterOrEqual(ttot, manual) {
			ttot = manual
		}
	}
	if ctot := toHHMMSS(strings.TrimSpace(input.Ctot)); ctot != "" {
		if !isAfterOrEqual(ttot, ctot) {
			ttot = ctot
		}
	}

	return applyAdverseConditionFloor(ttot, resolveAdverseConditionImpact(input, config, now))
}

func selectCalculationBase(input CalcInput) string {
	base, _ := selectCalculationBaseWithSource(input)
	return base
}

func selectCalculationBaseWithSource(input CalcInput) (string, string) {
	base := normalizeCalculationClock(input.Tobt)
	source := models.CdmCalculationBaseTobt
	if base == "" {
		base = normalizeCalculationClock(input.ReqTobt)
		source = models.CdmCalculationBaseReqTobt
	}

	eobt := normalizeCalculationClock(input.Eobt)
	if base == "" {
		if eobt == "" {
			return "", ""
		}
		return eobt, models.CdmCalculationBaseEobt
	}
	if eobt != "" && !isAfterOrEqual(base, eobt) {
		return eobt, models.CdmCalculationBaseEobt
	}

	return base, source
}

func shouldInvalidateStaleTobt(input CalcInput, nowHHMMSS string) bool {
	if hasStarted(input) {
		return false
	}

	tobt := normalizeCalculationClock(input.Tobt)
	if tobt == "" {
		return false
	}

	if base := selectCalculationBase(input); base != "" && toHHMMSS(base) != toHHMMSS(tobt) {
		return false
	}

	return isMoreThanMinutesPast(tobt, nowHHMMSS, 5)
}

func shouldInvalidateStaleTsat(input CalcInput, tsat string, nowHHMMSS string) bool {
	if hasStarted(input) {
		return false
	}

	return tsat != "" && isMoreThanMinutesPast(tsat, nowHHMMSS, 5)
}

func hasStarted(input CalcInput) bool {
	return normalizeCalculationClock(input.Asat) != "" || normalizeCalculationClock(input.Aobt) != ""
}

func shouldApplyRateWindow(currentManual, existingManual bool) bool {
	if currentManual == existingManual {
		return true
	}
	return !currentManual && !existingManual
}

func sameOrDependentRunway(current, other string, config *CdmAirportConfig) bool {
	if sameRunway(current, other) {
		return true
	}
	if config == nil {
		return false
	}

	for _, dependent := range config.DependentRunways(current) {
		if sameRunway(dependent, other) {
			return true
		}
	}

	for _, dependent := range config.DependentRunways(other) {
		if sameRunway(dependent, current) {
			return true
		}
	}

	return false
}

func withinWindow(candidate, slot string, windowMinutes float64) bool {
	if windowMinutes <= 0 {
		return false
	}
	diff := math.Abs(minutesBetween(slot, candidate))
	return diff > 0 && diff < windowMinutes
}

func violatesSameDestinationSeparation(candidateTtot, candidateDestination, slotTtot, slotDestination string) bool {
	if !sameDestination(candidateDestination, slotDestination) {
		return false
	}

	return toHHMMSS(slotTtot) == toHHMMSS(candidateTtot) || withinWindow(candidateTtot, slotTtot, sameDestinationSeparationMinutes)
}

func sameDestination(current, other string) bool {
	normalizedCurrent := normalizeToken(current)
	return normalizedCurrent != "" && normalizedCurrent == normalizeToken(other)
}
