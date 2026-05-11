package cdm

import (
	"strings"
	"time"
)

const adverseDelayMaxLookAhead = 6 * time.Hour

type adverseConditionImpact struct {
	DelayFloor string
}

func resolveAdverseConditionImpact(input CalcInput, config *CdmAirportConfig, now time.Time) adverseConditionImpact {
	if config == nil {
		return adverseConditionImpact{}
	}

	delayFloor, _ := config.DelayFloorForRunway(input.DepRwy, now)
	return adverseConditionImpact{
		DelayFloor: delayFloor,
	}
}

func applyAdverseConditionFloor(candidate string, impact adverseConditionImpact) string {
	if impact.DelayFloor == "" {
		return candidate
	}
	if candidate == "" || !isAfterOrEqual(candidate, impact.DelayFloor) {
		return impact.DelayFloor
	}
	return candidate
}

func (c *CdmAirportConfig) DelayFloorForRunway(depRwy string, now time.Time) (string, bool) {
	if c == nil {
		return "", false
	}

	latest := time.Time{}
	for _, delay := range c.Delays {
		if !delayMatchesAirport(delay.Airport, c.Airport) || !delayMatchesRunway(delay.Runway, depRwy) {
			continue
		}

		floor, ok := resolveDelayFloorTime(now.UTC(), delay.Time)
		if !ok {
			continue
		}
		if latest.IsZero() || floor.After(latest) {
			latest = floor
		}
	}

	if latest.IsZero() {
		return "", false
	}

	return latest.Format("150405"), true
}

func resolveDelayFloorTime(now time.Time, value string) (time.Time, bool) {
	normalized := normalizeCalculationClock(value)
	totalSeconds, ok := parseClock(normalized)
	if !ok || totalSeconds <= 0 {
		return time.Time{}, false
	}

	candidate := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		totalSeconds/3600,
		(totalSeconds%3600)/60,
		totalSeconds%60,
		0,
		time.UTC,
	)
	if candidate.Before(now) {
		candidate = candidate.Add(24 * time.Hour)
	}
	if candidate.Sub(now) > adverseDelayMaxLookAhead {
		return time.Time{}, false
	}

	return candidate, true
}

func delayMatchesAirport(delayAirport, configAirport string) bool {
	normalizedDelay := normalizeToken(delayAirport)
	return normalizedDelay == "" || normalizedDelay == normalizeToken(configAirport)
}

func delayMatchesRunway(configured, depRwy string) bool {
	switch strings.TrimSpace(configured) {
	case "", "*":
		return true
	default:
		return sameRunway(configured, depRwy)
	}
}
