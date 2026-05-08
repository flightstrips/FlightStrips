package cdm

import (
	"FlightStrips/internal/models"
	"math"
	"strings"
	"time"
)

type CalcInput struct {
	Callsign   string
	Origin     string
	DepRwy     string
	Sid        string
	Eobt       string
	Tobt       string
	ReqTobt    string
	Ctot       string
	Asat       string
	TaxiMin    int
	DeIceMin   int
	HasManCtot bool
	ManCtot    string
}

type SlotEntry struct {
	Callsign   string
	Origin     string
	DepRwy     string
	Sid        string
	Ttot       string
	HasManCtot bool
	ManCtot    string
}

type CalcResult struct {
	Tsat string
	Ttot string
}

func Calculate(input CalcInput, slots []SlotEntry, config *CdmAirportConfig, now time.Time) CalcResult {
	nowHHMMSS := timeToClock(now)
	if shouldInvalidateStaleTobt(input, nowHHMMSS) {
		return CalcResult{}
	}

	base := selectCalculationBase(input)
	if base == "" {
		return CalcResult{}
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
	ttot = applyAdverseConditionFloor(ttot, resolveAdverseConditionImpact(input, config, now))

	rate := DefaultCDMRate
	if config != nil {
		rate = config.RateForRunway(input.DepRwy)
	}
	if rate <= 0 {
		rate = DefaultCDMRate
	}

	rateWindow := 60.0 / float64(rate)

	for {
		changed := false

		for _, slot := range slots {
			if !strings.EqualFold(strings.TrimSpace(slot.Origin), strings.TrimSpace(input.Origin)) {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(slot.Callsign), strings.TrimSpace(input.Callsign)) {
				continue
			}
			if !sameOrDependentRunway(input.DepRwy, slot.DepRwy, config) {
				continue
			}

			if toHHMMSS(slot.Ttot) == ttot {
				ttot = addMinutes(ttot, 0.5)
				changed = true
				break
			}

			if shouldApplyRateWindow(input.HasManCtot, slot.HasManCtot) && withinWindow(ttot, slot.Ttot, rateWindow) {
				ttot = addMinutes(ttot, 0.5)
				changed = true
				break
			}

			if config != nil {
				if interval := config.SidIntervalMinutes(input.DepRwy, input.Sid, slot.Sid); interval > 0 && withinWindow(ttot, slot.Ttot, interval) {
					ttot = addMinutes(ttot, 0.5)
					changed = true
					break
				}
			}
		}

		if !changed {
			tsat := subtractMinutes(ttot, float64(input.TaxiMin))
			if shouldInvalidateStaleTsat(input, tsat, nowHHMMSS) {
				return CalcResult{}
			}
			return CalcResult{
				Tsat: tsat,
				Ttot: ttot,
			}
		}
	}
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
	return normalizeCalculationClock(input.Asat) != ""
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
