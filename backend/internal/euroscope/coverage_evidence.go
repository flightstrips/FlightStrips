package euroscope

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"FlightStrips/internal/config"
	"FlightStrips/internal/vatsim"
)

type CoverageEvidenceState string

const (
	CoverageUnknown CoverageEvidenceState = "unknown"
	CoverageFresh   CoverageEvidenceState = "fresh"
	CoverageStale   CoverageEvidenceState = "stale"
)

type CoverageControllerEvidence struct {
	config.CoverageParticipant
	Identity, Radios CoverageEvidenceState
}

// One normalized projection per environment for a coordinator pass. It is never
// retained by an airport session or membership. Absence requires fresh inventory.
type CoverageStaffingEvidence struct {
	Environment string
	Inventory   CoverageEvidenceState
	Controllers []CoverageControllerEvidence
}

func coverageControllerKey(cid, callsign string) string {
	return strings.TrimSpace(cid) + "/" + strings.ToUpper(strings.TrimSpace(callsign))
}

func coverageSourceState(now, at time.Time, maxAge time.Duration, err error) CoverageEvidenceState {
	if at.IsZero() {
		return CoverageUnknown
	}
	if err != nil || maxAge <= 0 || now.Before(at) || now.Sub(at) >= maxAge {
		return CoverageStale
	}
	return CoverageFresh
}

// BuildCoverageEvidence consumes the shared MA-05 generations, including the
// controller inventory acquired by that source. It creates no upstream requests.
// Fresh plugin radio reports override public radio data for that exact identity.
// Unknown plugin radio reports leave the shared provider evidence intact.
func BuildCoverageEvidence(network vatsim.Snapshot, radios vatsim.TransceiverSnapshot, presences []PluginPresence, now time.Time, maxAge time.Duration) map[string]CoverageStaffingEvidence {
	byEnvironment := map[string]map[string]CoverageControllerEvidence{"LIVE": {}}
	inventory := map[string]CoverageEvidenceState{"LIVE": coverageSourceState(now, network.Timestamp, maxAge, network.LastRefreshError)}
	if !network.ControllersKnown {
		inventory["LIVE"] = CoverageUnknown
	}
	radioState := coverageSourceState(now, radios.Timestamp, maxAge, radios.LastRefreshError)
	for _, controller := range network.Controllers() {
		callsign := strings.ToUpper(strings.TrimSpace(controller.Callsign))
		if controller.Facility == 0 || strings.HasSuffix(callsign, "_ATIS") || strings.HasSuffix(callsign, "_OBS") {
			continue
		}
		key := coverageControllerKey(strconv.FormatInt(controller.CID, 10), callsign)
		byEnvironment["LIVE"][key] = CoverageControllerEvidence{
			CoverageParticipant: config.CoverageParticipant{Key: key, Callsign: callsign, PrimaryFrequency: vatsim.NormalizeFrequency(controller.Frequency), TunedFrequencies: radios.GetFrequencies(callsign)},
			Identity:            inventory["LIVE"], Radios: radioState,
		}
	}
	for _, presence := range presences {
		control := presence.Evidence
		if control == nil || !environmentName.MatchString(control.Environment) {
			continue
		}
		environment := control.Environment
		if byEnvironment[environment] == nil {
			byEnvironment[environment] = map[string]CoverageControllerEvidence{}
			inventory[environment] = CoverageUnknown
		}
		key := coverageControllerKey(presence.CID, control.Callsign)
		previous, exists := byEnvironment[environment][key]
		state := coverageSourceState(now, presence.EvidenceAt, maxAge, nil)
		if state != CoverageFresh && exists {
			// An expired plugin report cannot replace a fresh shared identity or
			// erase current provider radios. Retain source health with its data.
			continue
		}
		evidence := CoverageControllerEvidence{CoverageParticipant: config.CoverageParticipant{Key: key, Callsign: strings.ToUpper(strings.TrimSpace(control.Callsign)), PrimaryFrequency: vatsim.NormalizeFrequency(control.PrimaryFrequency), Observer: control.Observer}, Identity: state, Radios: CoverageUnknown}
		if exists {
			evidence.TunedFrequencies = slices.Clone(previous.TunedFrequencies)
			evidence.Radios = previous.Radios
		} else if environment == "LIVE" {
			// A zero-membership plugin can precede its network inventory record.
			evidence.TunedFrequencies = radios.GetFrequencies(control.Callsign)
			evidence.Radios = radioState
		}
		if control.TunedFrequenciesKnown && state == CoverageFresh {
			evidence.TunedFrequencies = nil
			for _, frequency := range control.TunedFrequencies {
				if normalized := vatsim.NormalizeFrequency(frequency); normalized != "" {
					evidence.TunedFrequencies = append(evidence.TunedFrequencies, normalized)
				}
			}
			slices.Sort(evidence.TunedFrequencies)
			evidence.TunedFrequencies = slices.Compact(evidence.TunedFrequencies)
			evidence.Radios = CoverageFresh
		}
		byEnvironment[environment][key] = evidence
	}
	result := map[string]CoverageStaffingEvidence{}
	for environment, controllers := range byEnvironment {
		entry := CoverageStaffingEvidence{Environment: environment, Inventory: inventory[environment]}
		for _, controller := range controllers {
			entry.Controllers = append(entry.Controllers, controller)
		}
		slices.SortFunc(entry.Controllers, func(a, b CoverageControllerEvidence) int { return strings.Compare(a.Key, b.Key) })
		result[environment] = entry
	}
	return result
}
