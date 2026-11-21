package euroscope

import (
	"regexp"
	"strings"
	"time"

	"FlightStrips/internal/config"
	"FlightStrips/internal/vatsim"
)

var visiblePrimaryFrequency = regexp.MustCompile(`^[0-9]{3}\.[0-9]{3}$`)

// Each instance observes its complete relevant controller roster through the
// SDK. Never merge another instance/environment's view into this observation.
// The list includes non-plugin peers; it is not built from backend presence.
func visibleCoverageEvidence(presence PluginPresence, now time.Time, maxAge time.Duration) CoverageStaffingEvidence {
	control := presence.Evidence
	frame := CoverageStaffingEvidence{Inventory: CoverageUnknown, ESOnly: true}
	if control == nil || !environmentName.MatchString(control.Environment) {
		return frame
	}
	frame.Environment = control.Environment
	report := control.VisibleStaffing
	if report == nil || !report.Complete || report.Revision == 0 || report.ObservationAgeMs < 0 {
		return frame
	}
	// Bound before multiplying to avoid duration overflow on malformed input.
	if maxAge <= 0 || report.ObservationAgeMs >= maxAge.Milliseconds() {
		frame.Inventory = CoverageStale
		return frame
	}
	frame.Timestamp = presence.EvidenceAt.Add(-time.Duration(report.ObservationAgeMs) * time.Millisecond)
	frame.Inventory = coverageSourceState(now, frame.Timestamp, maxAge, nil)
	if transportState := coverageSourceState(now, presence.EvidenceAt, maxAge, nil); transportState != CoverageFresh {
		frame.Inventory = transportState
	}
	seen := map[string]bool{}
	selfCallsign := strings.ToUpper(strings.TrimSpace(control.Callsign))
	foundSelf := false
	for _, controller := range report.Controllers {
		callsign := strings.ToUpper(strings.TrimSpace(controller.GetCallsign()))
		frequency := vatsim.NormalizeFrequency(controller.GetPrimaryFrequency())
		if callsign == "" || seen[callsign] || (!controller.GetObserver() && !visiblePrimaryFrequency.MatchString(frequency)) {
			frame.Inventory = CoverageUnknown
			return frame
		}
		seen[callsign] = true
		if strings.HasSuffix(callsign, "_ATIS") || strings.HasSuffix(callsign, "_OBS") || controller.GetObserver() {
			continue
		}
		key := coverageControllerKey("", callsign)
		if callsign == selfCallsign {
			if frequency != vatsim.NormalizeFrequency(control.PrimaryFrequency) || control.Observer {
				frame.Inventory = CoverageUnknown
				return frame
			}
			foundSelf = true
		}
		frame.Controllers = append(frame.Controllers, CoverageControllerEvidence{
			CoverageParticipant: config.CoverageParticipant{Key: key, Callsign: callsign, PrimaryFrequency: frequency},
			Identity:            frame.Inventory, Radios: CoverageUnknown,
		})
	}
	if !foundSelf {
		// A not-yet-ready/empty SDK view cannot invent the current controller.
		frame.Inventory = CoverageUnknown
	}
	return frame
}
