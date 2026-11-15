package euroscope

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"FlightStrips/internal/config"
	"FlightStrips/internal/vatsim"
)

// SimulatedStaffingFile reads an operator-selected, authoritative exercise feed.
// The producer must atomically replace this file with a newly observed complete
// roster, including controllers without FlightStrips. Reading never refreshes
// observation timestamps. Missing/invalid input cannot confirm absence.
// This deliberately does not derive inventory from connected plugin instances.
func SimulatedStaffingFile(path string) func(context.Context) ([]CoverageStaffingEvidence, error) {
	return func(ctx context.Context) ([]CoverageStaffingEvidence, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("read simulated staffing: %w", err)
		}
		defer f.Close()
		return decodeSimulatedStaffing(io.LimitReader(f, 1<<20))
	}
}

type simulatedStaffingDocument struct {
	Version      string                         `json:"version"`
	Environments []simulatedStaffingEnvironment `json:"environments"`
}

type simulatedStaffingEnvironment struct {
	Environment         string                        `json:"environment"`
	ObservedAt          time.Time                     `json:"observed_at"`
	ControllersComplete bool                          `json:"controllers_complete"`
	Controllers         []simulatedStaffingController `json:"controllers"`
}

type simulatedStaffingController struct {
	CID                   string   `json:"cid"`
	Callsign              string   `json:"callsign"`
	Observer              bool     `json:"observer"`
	PrimaryFrequency      string   `json:"primary_frequency"`
	TunedFrequenciesKnown bool     `json:"tuned_frequencies_known"`
	TunedFrequencies      []string `json:"tuned_frequencies"`
}

var simulatedCID = regexp.MustCompile(`^[0-9]+$`)
var simulatedFrequency = regexp.MustCompile(`^[0-9]{3}\.[0-9]{3}$`)

func decodeSimulatedStaffing(reader io.Reader) ([]CoverageStaffingEvidence, error) {
	var document simulatedStaffingDocument
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode simulated staffing: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("simulated staffing must contain exactly one JSON document")
	}
	if document.Version != "flightstrips-simulated-staffing/v1" || document.Environments == nil {
		return nil, errors.New("simulated staffing requires version and environments")
	}
	seen := map[string]bool{}
	frames := make([]CoverageStaffingEvidence, 0, len(document.Environments))
	for _, environment := range document.Environments {
		name := environment.Environment
		if name == "LIVE" || name != strings.ToUpper(name) || !environmentName.MatchString(name) || seen[name] {
			return nil, fmt.Errorf("invalid or duplicate simulated staffing environment %q", name)
		}
		seen[name] = true
		if environment.ObservedAt.IsZero() || environment.Controllers == nil {
			return nil, fmt.Errorf("simulated staffing %s requires observation time and explicit controller inventory", name)
		}
		frame := CoverageStaffingEvidence{Environment: name, Timestamp: environment.ObservedAt, Inventory: CoverageUnknown}
		if environment.ControllersComplete {
			frame.Inventory = CoverageFresh
		}
		identities := map[string]bool{}
		for _, controller := range environment.Controllers {
			callsign := strings.ToUpper(strings.TrimSpace(controller.Callsign))
			key := coverageControllerKey(controller.CID, callsign)
			if !simulatedCID.MatchString(controller.CID) || callsign == "" || identities[key] {
				return nil, fmt.Errorf("invalid or duplicate simulated controller %q", key)
			}
			identities[key] = true
			primary := vatsim.NormalizeFrequency(controller.PrimaryFrequency)
			if !controller.Observer && !simulatedFrequency.MatchString(primary) {
				return nil, fmt.Errorf("invalid simulated primary frequency for %s", key)
			}
			entry := CoverageControllerEvidence{CoverageParticipant: config.CoverageParticipant{Key: key, Callsign: callsign, PrimaryFrequency: primary, Observer: controller.Observer}, Identity: CoverageFresh, Radios: CoverageUnknown}
			if sweatboxPrimaryOnly(name) {
				frame.Controllers = append(frame.Controllers, entry)
				continue
			}
			if controller.TunedFrequenciesKnown {
				if controller.TunedFrequencies == nil {
					return nil, fmt.Errorf("known simulated radios require explicit tuned_frequencies for %s", key)
				}
				entry.Radios = CoverageFresh
			}
			for _, frequency := range controller.TunedFrequencies {
				normalized := vatsim.NormalizeFrequency(frequency)
				if !simulatedFrequency.MatchString(normalized) {
					return nil, fmt.Errorf("invalid simulated tuned frequency for %s", key)
				}
				if controller.TunedFrequenciesKnown {
					entry.TunedFrequencies = append(entry.TunedFrequencies, normalized)
				}
			}
			slices.Sort(entry.TunedFrequencies)
			entry.TunedFrequencies = slices.Compact(entry.TunedFrequencies)
			frame.Controllers = append(frame.Controllers, entry)
		}
		frames = append(frames, frame)
	}
	return frames, nil
}
