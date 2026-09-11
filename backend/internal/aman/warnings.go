package aman

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// WarningSource identifies the authoritative current state that produced a
// warning. Sources replace their warnings wholesale; this model owns no
// acknowledgement or history.
type WarningSource string

const (
	WarningSourceTechnicalHealth WarningSource = "technical_health"
	WarningSourceSequence        WarningSource = "sequence"
)

// WarningSeverity orders conditions that prevent safe operation before
// degraded conditions whose output remains usable.
type WarningSeverity string

const (
	WarningSeverityError   WarningSeverity = "error"
	WarningSeverityWarning WarningSeverity = "warning"
)

// Warning is one current operator-facing condition. ID deliberately excludes
// Message so presentation or localization changes preserve item identity.
type Warning struct {
	ID              string
	Source          WarningSource
	Component       *string
	Severity        WarningSeverity
	Code            string
	RunwayGroupID   *RunwayGroupID
	FlightID        *FlightID
	RelatedFlightID *FlightID
	Message         string
}

// Identity derives the stable source-aware identity for a warning.
func (w Warning) Identity() string {
	return "warning:" + strings.Join([]string{
		strconv.Quote(string(w.Source)), optionalWarningIdentity(w.Component), strconv.Quote(w.Code),
		optionalWarningIdentity(w.RunwayGroupID), optionalWarningIdentity(w.FlightID), optionalWarningIdentity(w.RelatedFlightID),
	}, "/")
}

func optionalWarningIdentity[T ~string](value *T) string {
	if value == nil {
		return "-"
	}
	return strconv.Quote(string(*value))
}

// WarningSnapshot is the complete current warning replacement. An empty
// slice means every previously published warning has resolved.
type WarningSnapshot struct {
	Warnings []Warning
}

// CurrentWarningSnapshot combines the current technical report with warnings
// persisted on the committed airport state. Equivalent inputs always produce
// the same deduplicated ordering, including after restart or replay.
func CurrentWarningSnapshot(technical TechnicalHealth, state AirportState) WarningSnapshot {
	byID := make(map[string]Warning)
	add := func(warning Warning) {
		warning.ID = warning.Identity()
		prior, exists := byID[warning.ID]
		if !exists || warningLess(warning, prior) {
			byID[warning.ID] = warning
		}
	}

	components := []struct {
		name   string
		health ComponentHealth
	}{
		{"vatsim", technical.VATSIM},
		{"navigation", technical.Navigation},
		{"weather", technical.Weather},
		{"repository", technical.Repository},
		{"predictor", technical.Predictor},
		{"replay_validation", technical.ReplayValidation},
	}
	componentIDs := make(map[string]struct{}, len(components))
	if technical.Enabled {
		if technical.EffectiveMode == EffectiveBlocked {
			add(Warning{Source: WarningSourceTechnicalHealth, Severity: WarningSeverityError,
				Code: "authority_blocked", Message: "AMAN operation is blocked"})
		}
		for _, component := range components {
			if component.health.Status == HealthReady {
				continue
			}
			severity := WarningSeverityError
			if component.health.Status == HealthDegraded {
				severity = WarningSeverityWarning
			}
			name := component.name
			warning := Warning{Source: WarningSourceTechnicalHealth, Component: &name, Severity: severity,
				Code: healthReason(component.health), Message: technicalWarningMessage(name, component.health)}
			componentIDs[warning.Identity()] = struct{}{}
			add(warning)
		}
		for _, reason := range technical.BlockedReasons {
			component, code := blockedWarningIdentity(reason)
			if code == "" {
				continue
			}
			warning := Warning{Source: WarningSourceTechnicalHealth, Component: component,
				Severity: WarningSeverityError, Code: code, Message: fmt.Sprintf("AMAN operation is blocked: %s", reason)}
			if _, represented := componentIDs[warning.Identity()]; !represented {
				add(warning)
			}
		}
	}

	for _, group := range state.RunwayGroups {
		for _, current := range group.SequenceWarnings {
			groupID, flightID, relatedID := group.ID, current.FlightID, current.RelatedFlightID
			add(Warning{Source: WarningSourceSequence, Severity: WarningSeverityError, Code: current.Code,
				RunwayGroupID: &groupID, FlightID: &flightID, RelatedFlightID: &relatedID,
				Message: fmt.Sprintf("Flights %s and %s conflict with protected %s spacing on runway group %s",
					current.FlightID, current.RelatedFlightID, current.STARFamily, group.ID)})
		}
	}

	warnings := make([]Warning, 0, len(byID))
	for _, warning := range byID {
		warnings = append(warnings, warning)
	}
	sort.Slice(warnings, func(i, j int) bool { return warningLess(warnings[i], warnings[j]) })
	return WarningSnapshot{Warnings: warnings}
}

func warningLess(left, right Warning) bool {
	severityRank := func(severity WarningSeverity) int {
		if severity == WarningSeverityError {
			return 0
		}
		return 1
	}
	if severityRank(left.Severity) != severityRank(right.Severity) {
		return severityRank(left.Severity) < severityRank(right.Severity)
	}
	if left.ID != right.ID {
		return left.ID < right.ID
	}
	return left.Message < right.Message
}

func blockedWarningIdentity(reason string) (*string, string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, ""
	}
	component, code, found := strings.Cut(reason, ":")
	if !found || !knownWarningComponent(component) || strings.TrimSpace(code) == "" {
		return nil, reason
	}
	component, code = strings.TrimSpace(component), strings.TrimSpace(code)
	return &component, code
}

func knownWarningComponent(component string) bool {
	switch component {
	case "vatsim", "navigation", "weather", "repository", "predictor", "replay_validation":
		return true
	default:
		return false
	}
}

func technicalWarningMessage(component string, health ComponentHealth) string {
	status := health.Status
	if status == "" {
		status = HealthUnavailable
	}
	return fmt.Sprintf("AMAN %s is %s: %s", strings.ReplaceAll(component, "_", " "), status, healthReason(health))
}
