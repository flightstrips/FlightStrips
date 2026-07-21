package aman

import (
	"context"
	"slices"
	"strings"
	"time"
)

// HealthStatus describes technical readiness. It deliberately has no
// provider-approval or transport-delivery states: neither is an AMAN gate.
type HealthStatus string

const (
	HealthDisabled    HealthStatus = "disabled"
	HealthReady       HealthStatus = "ready"
	HealthDegraded    HealthStatus = "degraded"
	HealthUnavailable HealthStatus = "unavailable"
)

// ComponentHealth captures one bounded technical dependency. Reason is a
// stable, operator-facing reason code; details belong in structured logs.
type ComponentHealth struct {
	Status     HealthStatus `json:"status"`
	Reason     string       `json:"reason,omitempty"`
	UpdatedAt  *time.Time   `json:"updated_at,omitempty"`
	AgeSeconds *float64     `json:"age_seconds,omitempty"`
}

// TechnicalHealth is the complete AMAN readiness snapshot. It exposes the
// inputs that can technically block authority without making a policy or
// provider-approval decision itself.
type TechnicalHealth struct {
	Enabled          bool            `json:"enabled"`
	Mode             RolloutMode     `json:"mode"`
	Ready            bool            `json:"ready"`
	Status           HealthStatus    `json:"status"`
	BlockedReasons   []string        `json:"blocked_reasons,omitempty"`
	VATSIM           ComponentHealth `json:"vatsim"`
	Navigation       ComponentHealth `json:"navigation"`
	Weather          ComponentHealth `json:"weather"`
	Repository       ComponentHealth `json:"repository"`
	Predictor        ComponentHealth `json:"predictor"`
	ReplayValidation ComponentHealth `json:"replay_validation"`
}

// TechnicalHealthReporter is the narrow runtime seam for a concrete health
// owner. It stays independent of HTTP, WebSocket hubs, and provider approval.
type TechnicalHealthReporter interface {
	Component
	TechnicalHealth(context.Context) TechnicalHealth
}

// EvaluateTechnicalHealth creates one deterministic snapshot from concrete
// component checks. Every non-ready component is named in BlockedReasons so
// operators can identify the technical authority blocker directly.
func EvaluateTechnicalHealth(mode RolloutMode, vatsim, navigation, weather, repository, predictor, replay ComponentHealth) TechnicalHealth {
	report := TechnicalHealth{
		Enabled: mode != ModeDisabled,
		Mode:    mode,
		VATSIM:  vatsim, Navigation: navigation, Weather: weather,
		Repository: repository, Predictor: predictor, ReplayValidation: replay,
	}
	if !report.Enabled {
		report.Status = HealthDisabled
		return report
	}
	checks := []struct {
		name  string
		value ComponentHealth
	}{
		{"vatsim", vatsim}, {"navigation", navigation}, {"weather", weather},
		{"repository", repository}, {"predictor", predictor}, {"replay_validation", replay},
	}
	for _, check := range checks {
		if check.value.Status != HealthReady {
			report.BlockedReasons = append(report.BlockedReasons, check.name+":"+healthReason(check.value))
		}
	}
	report.Ready = len(report.BlockedReasons) == 0
	if report.Ready {
		report.Status = HealthReady
	} else {
		report.Status = HealthDegraded
	}
	return report
}

// Health returns the injected concrete report, with runtime configuration
// applied as the effective mode. A missing reporter is visible as a technical
// blocker rather than being mistaken for a healthy authoritative runtime.
func (r *Runtime) Health(ctx context.Context) TechnicalHealth {
	mode := ModeDisabled
	if r != nil {
		mode = r.config.Mode
	}
	if mode == ModeDisabled {
		return EvaluateTechnicalHealth(mode, ComponentHealth{}, ComponentHealth{}, ComponentHealth{}, ComponentHealth{}, ComponentHealth{}, ComponentHealth{})
	}
	reporter, ok := r.deps.HealthService.(TechnicalHealthReporter)
	if !ok {
		unavailable := ComponentHealth{Status: HealthUnavailable, Reason: "health_reporter_unavailable"}
		return EvaluateTechnicalHealth(mode, unavailable, unavailable, unavailable, unavailable, unavailable, unavailable)
	}
	report := reporter.TechnicalHealth(ctx)
	report.Enabled, report.Mode = true, mode
	return normalizeTechnicalHealth(report)
}

func normalizeTechnicalHealth(report TechnicalHealth) TechnicalHealth {
	report.BlockedReasons = slices.Clone(report.BlockedReasons)
	if report.Status == "" {
		report = EvaluateTechnicalHealth(report.Mode, report.VATSIM, report.Navigation, report.Weather, report.Repository, report.Predictor, report.ReplayValidation)
	}
	return report
}

func healthReason(value ComponentHealth) string {
	if reason := strings.TrimSpace(value.Reason); reason != "" {
		return reason
	}
	if value.Status == "" {
		return "not_reported"
	}
	return string(value.Status)
}
