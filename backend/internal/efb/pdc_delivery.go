package efb

import (
	"FlightStrips/internal/models"
	"time"
)

const pdcDeliveryDelay = 10 * time.Second

func applyPDCDeliveryDelay(result *snapshot, data *models.PdcData, now time.Time) {
	if result.PDCState != "CLEARED" || data == nil || data.Web == nil || data.RequestedAt == nil {
		return
	}
	remaining := data.RequestedAt.Add(pdcDeliveryDelay).Sub(now)
	if remaining <= 0 {
		return
	}
	result.PDCState = "REQUESTED"
	result.PDCRequiresPilotAction = false
	result.PDCClearanceText = nil
	result.PDCRetryAfterMS = (remaining.Nanoseconds() + int64(time.Millisecond) - 1) / int64(time.Millisecond)
}
