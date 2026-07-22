// Package gate applies the technical and replay-validation prerequisites for
// AMAN operational rollout modes. It consumes the canonical validation report
// rather than defining a second evidence payload.
package gate

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/report"
)

const (
	ReasonEvidenceUnavailable = "validation_evidence_unavailable"
	ReasonEvidenceMissing     = "validation_evidence_missing"
	ReasonEvidenceCorrupt     = "validation_evidence_corrupt"
	ReasonEvidenceFuture      = "validation_evidence_future"
	ReasonEvidenceStale       = "validation_evidence_stale"
	ReasonEvidenceFailed      = "validation_evidence_failed"
	ReasonDigestMismatch      = "validation_evidence_digest_mismatch"
)

// Digests are the active inputs that must match a replay result before it can
// grant operational AMAN authority.
type Digests struct {
	Code       string
	Config     string
	Policy     string
	Navigation string
	Terminal   string
	Holding    string
	Weather    string
}

func (d Digests) validate() error {
	for _, value := range []string{d.Code, d.Config, d.Policy, d.Navigation, d.Terminal, d.Holding, d.Weather} {
		if strings.TrimSpace(value) == "" {
			return errors.New("rollout gate requires every active validation digest")
		}
	}
	return nil
}

// Config controls validation-result freshness and the active inputs against
// which report metadata is compared.
type Config struct {
	MaximumEvidenceAge time.Duration
	ExpectedDigests    Digests
}

func (c Config) validate() error {
	if c.MaximumEvidenceAge <= 0 {
		return errors.New("rollout gate maximum evidence age must be greater than 0")
	}
	return c.ExpectedDigests.validate()
}

// Decision exposes desired and effective state separately. Blocked is an
// effective state only; it never rewrites the configured desired mode.
type Decision struct {
	DesiredMode      aman.RolloutMode
	EffectiveMode    aman.EffectiveRolloutMode
	AuthorityAllowed bool
	Ownership        aman.Ownership
	BlockedReasons   []string
}

// Apply annotates the health payload consumed by controller-facing health
// endpoints. A blocked decision is degraded and keeps the complete technical
// and validation reason list visible.
func (d Decision) Apply(health aman.TechnicalHealth) aman.TechnicalHealth {
	health.Mode = d.DesiredMode
	health.DesiredMode = d.DesiredMode
	health.EffectiveMode = d.EffectiveMode
	health.AuthorityAllowed = d.AuthorityAllowed
	if d.EffectiveMode == aman.EffectiveBlocked {
		health.Ready = false
		health.Status = aman.HealthDegraded
		health.BlockedReasons = appendUnique(health.BlockedReasons, d.BlockedReasons)
	}
	return health
}

// Evaluator reads persisted canonical evidence. Now is injected so stale and
// future boundaries are deterministic in production and replay tests.
type Evaluator struct {
	evidence aman.ValidationEvidenceReader
	config   Config
	now      func() time.Time
}

func New(evidence aman.ValidationEvidenceReader, config Config, now func() time.Time) (*Evaluator, error) {
	if evidence == nil {
		return nil, errors.New("rollout gate requires validation evidence reader")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	if now == nil {
		return nil, errors.New("rollout gate requires clock")
	}
	return &Evaluator{evidence: evidence, config: config, now: now}, nil
}

// EvaluateAirport is fail-closed for read-only and authoritative desired
// modes. Disabled and shadow remain deterministic desired modes; shadow
// continues to collect comparison evidence but never consumes it to grant
// authority.
func (e *Evaluator) EvaluateAirport(ctx context.Context, airport string, desired aman.RolloutMode, technical aman.TechnicalHealth) Decision {
	if !desired.Valid() {
		return blocked(desired, []string{"invalid_desired_mode"})
	}
	if !isOperational(desired) {
		return allowed(desired)
	}
	health := aman.EvaluateTechnicalHealth(desired, technical.VATSIM, technical.Navigation, technical.Weather, technical.Repository, technical.Predictor, technical.ReplayValidation)
	reasons := slices.Clone(health.BlockedReasons)
	reasons = append(reasons, e.currentEvidenceReasons(ctx, airport)...)
	if len(reasons) > 0 {
		return blocked(desired, reasons)
	}
	return allowed(desired)
}

func (e *Evaluator) currentEvidenceReasons(ctx context.Context, airport string) []string {
	if strings.TrimSpace(airport) == "" {
		return []string{ReasonEvidenceMissing}
	}
	evidence, err := e.evidence.ListValidationEvidence(ctx, airport)
	if err != nil {
		return []string{ReasonEvidenceUnavailable}
	}
	latest, ok := newestReport(evidence)
	if !ok {
		return []string{ReasonEvidenceMissing}
	}
	now := e.now()
	if now.IsZero() || latest.RecordedAt.IsZero() || latest.RecordedAt.After(now) {
		return []string{ReasonEvidenceFuture}
	}
	reportValue, err := report.Decode(bytes.NewReader(latest.Payload))
	if err != nil {
		return []string{ReasonEvidenceCorrupt}
	}
	if reportValue.Metadata.Airport != airport {
		return []string{ReasonEvidenceCorrupt}
	}
	if reportValue.Metadata.EvaluatedAt.After(now) {
		return []string{ReasonEvidenceFuture}
	}
	if now.Sub(latest.RecordedAt) > e.config.MaximumEvidenceAge || now.Sub(reportValue.Metadata.EvaluatedAt) > e.config.MaximumEvidenceAge {
		return []string{ReasonEvidenceStale}
	}
	if !reportValue.Passed {
		return []string{ReasonEvidenceFailed}
	}
	if !e.matches(reportValue.Metadata) {
		return []string{ReasonDigestMismatch}
	}
	return nil
}

func (e *Evaluator) matches(metadata report.Metadata) bool {
	expected := e.config.ExpectedDigests
	return metadata.CodeDigest == expected.Code && metadata.ConfigDigest == expected.Config && metadata.PolicyDigest == expected.Policy && metadata.NavigationDigest == expected.Navigation && metadata.TerminalDigest == expected.Terminal && metadata.HoldingDigest == expected.Holding && metadata.WeatherDigest == expected.Weather
}

func newestReport(values []aman.ValidationEvidence) (aman.ValidationEvidence, bool) {
	candidates := make([]aman.ValidationEvidence, 0, len(values))
	for _, value := range values {
		if value.Kind == report.Version {
			candidates = append(candidates, value)
		}
	}
	if len(candidates) == 0 {
		return aman.ValidationEvidence{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].RecordedAt.Equal(candidates[j].RecordedAt) {
			return candidates[i].ID > candidates[j].ID
		}
		return candidates[i].RecordedAt.After(candidates[j].RecordedAt)
	})
	return candidates[0], true
}

func allowed(desired aman.RolloutMode) Decision {
	authority := isOperational(desired)
	return Decision{DesiredMode: desired, EffectiveMode: aman.EffectiveModeFor(desired), AuthorityAllowed: authority, Ownership: aman.OwnershipForRolloutGate(desired, authority)}
}

func blocked(desired aman.RolloutMode, reasons []string) Decision {
	reasons = appendUnique(nil, reasons)
	return Decision{DesiredMode: desired, EffectiveMode: aman.EffectiveBlocked, Ownership: aman.OwnershipForRolloutGate(desired, false), BlockedReasons: reasons}
}

func isOperational(mode aman.RolloutMode) bool {
	return mode == aman.ModeReadOnly || mode == aman.ModeAuthoritative
}

func appendUnique(base []string, values []string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	for _, value := range base {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, found := seen[value]; !found {
			seen[value] = struct{}{}
			base = append(base, value)
		}
	}
	return base
}
