package efb

import (
	"FlightStrips/internal/models"
	"testing"
	"time"
)

func TestPDCDeliveryDelay(t *testing.T) {
	requested := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	clearance := "CLEARED TO EKYT"
	for _, tc := range []struct {
		name        string
		state       string
		elapsed     time.Duration
		requestedAt *time.Time
		wantPending bool
		wantRetry   int64
	}{
		{name: "immediate", state: "CLEARED", requestedAt: &requested, wantPending: true, wantRetry: 10000},
		{name: "just before deadline", state: "CLEARED", elapsed: 10*time.Second - time.Nanosecond, requestedAt: &requested, wantPending: true, wantRetry: 1},
		{name: "at deadline", state: "CLEARED", elapsed: 10 * time.Second, requestedAt: &requested},
		{name: "manual clearance after deadline", state: "CLEARED", elapsed: time.Minute, requestedAt: &requested},
		{name: "already confirmed", state: "CONFIRMED", requestedAt: &requested},
		{name: "legacy missing request time", state: "CLEARED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := snapshot{PDCState: tc.state, PDCRequiresPilotAction: tc.state == "CLEARED", PDCClearanceText: &clearance}
			data := &models.PdcData{RequestedAt: tc.requestedAt, Web: &models.PdcWebData{ClearanceText: &clearance}}
			applyPDCDeliveryDelay(&result, data, requested.Add(tc.elapsed))
			if tc.wantPending {
				if result.PDCState != "REQUESTED" || result.PDCRequiresPilotAction || result.PDCClearanceText != nil || result.PDCRetryAfterMS != tc.wantRetry {
					t.Fatalf("clearance exposed prematurely: %+v", result)
				}
			} else if result.PDCState != tc.state || result.PDCClearanceText == nil || result.PDCRetryAfterMS != 0 || result.PDCRequiresPilotAction != (tc.state == "CLEARED") {
				t.Fatalf("clearance unexpectedly changed: %+v", result)
			}
		})
	}
}
