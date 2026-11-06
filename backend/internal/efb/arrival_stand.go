package efb

import (
	"FlightStrips/internal/models"
	"strings"
	"time"
)

func arrivalStandRestriction(strip *models.Strip, airport string, now time.Time) string {
	if strip == nil || !strings.EqualFold(strip.Destination, airport) || strings.EqualFold(strip.Origin, airport) {
		return ""
	}
	if strip.ArrivalETA == nil || strip.ArrivalETA.Time.IsZero() {
		return "Stand requests require an arrival ETA"
	}
	if strip.ArrivalETA.Time.After(now.Add(45 * time.Minute)) {
		return "Stand requests are available 45 minutes before arrival"
	}
	return ""
}

func applyArrivalStandRestriction(result *snapshot, strip *models.Strip, airport string, now time.Time) {
	if result.Phase != "ARRIVAL" {
		return
	}
	result.StandRequestUnavailableReason = arrivalStandRestriction(strip, airport, now)
	if result.StandRequestUnavailableReason != "" {
		result.Capabilities.Stand = false
	}
}
