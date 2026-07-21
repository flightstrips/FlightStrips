package gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/report"
	"github.com/stretchr/testify/require"
)

var gateNow = time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)

type evidenceReader struct {
	values []aman.ValidationEvidence
	err    error
}

func (r *evidenceReader) ListValidationEvidence(context.Context, string) ([]aman.ValidationEvidence, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.values, nil
}

func TestEvaluatorModeAndOwnershipMatrix(t *testing.T) {
	reader := &evidenceReader{values: []aman.ValidationEvidence{validationEvidence(t, gateNow, gateNow, true, "EKCH")}}
	evaluator := newEvaluator(t, reader)
	technical := readyTechnical()

	tests := []struct {
		desired                          aman.RolloutMode
		effective                        aman.EffectiveRolloutMode
		authority, legacyWriter, amanETA bool
	}{
		{desired: aman.ModeDisabled, effective: aman.EffectiveDisabled, legacyWriter: true},
		{desired: aman.ModeShadow, effective: aman.EffectiveShadow, legacyWriter: true},
		{desired: aman.ModeReadOnly, effective: aman.EffectiveReadOnly, authority: true, amanETA: true},
		{desired: aman.ModeAuthoritative, effective: aman.EffectiveAuthoritative, authority: true, amanETA: true},
	}
	for _, test := range tests {
		t.Run(string(test.desired), func(t *testing.T) {
			decision := evaluator.EvaluateAirport(context.Background(), "EKCH", test.desired, technical)
			require.Equal(t, test.desired, decision.DesiredMode)
			require.Equal(t, test.effective, decision.EffectiveMode)
			require.Equal(t, test.authority, decision.AuthorityAllowed)
			require.Equal(t, test.legacyWriter, decision.Ownership.LegacyArrivalETAWriter)
			require.Equal(t, test.amanETA, decision.Ownership.AMANArrivalETAWriter)
			require.Empty(t, decision.BlockedReasons)
		})
	}
}

func TestEvaluatorBlocksOperationalModesWithoutLegacyFallback(t *testing.T) {
	reader := &evidenceReader{values: []aman.ValidationEvidence{validationEvidence(t, gateNow, gateNow, true, "EKCH")}}
	evaluator := newEvaluator(t, reader)
	technical := readyTechnical()
	technical.VATSIM = aman.ComponentHealth{Status: aman.HealthDegraded, Reason: "snapshot_stale"}

	for _, desired := range []aman.RolloutMode{aman.ModeReadOnly, aman.ModeAuthoritative} {
		t.Run(string(desired), func(t *testing.T) {
			decision := evaluator.EvaluateAirport(context.Background(), "EKCH", desired, technical)
			require.Equal(t, aman.EffectiveBlocked, decision.EffectiveMode)
			require.False(t, decision.AuthorityAllowed)
			require.False(t, decision.Ownership.LegacyArrivalETAWriter)
			require.False(t, decision.Ownership.AMANArrivalETAWriter)
			require.Contains(t, decision.BlockedReasons, "vatsim:snapshot_stale")

			health := decision.Apply(technical)
			require.Equal(t, desired, health.DesiredMode)
			require.Equal(t, aman.EffectiveBlocked, health.EffectiveMode)
			require.False(t, health.AuthorityAllowed)
			require.Contains(t, health.BlockedReasons, "vatsim:snapshot_stale")
		})
	}
}

func TestEvaluatorRejectsInvalidValidationEvidence(t *testing.T) {
	tests := []struct {
		name   string
		reader *evidenceReader
		want   string
	}{
		{name: "missing", reader: &evidenceReader{}, want: ReasonEvidenceMissing},
		{name: "unavailable", reader: &evidenceReader{err: errors.New("database unavailable")}, want: ReasonEvidenceUnavailable},
		{name: "corrupt", reader: &evidenceReader{values: []aman.ValidationEvidence{{ID: "bad", Airport: "EKCH", Kind: report.Version, Payload: []byte("not-json"), RecordedAt: gateNow}}}, want: ReasonEvidenceCorrupt},
		{name: "future record", reader: &evidenceReader{values: []aman.ValidationEvidence{validationEvidence(t, gateNow.Add(time.Second), gateNow, true, "EKCH")}}, want: ReasonEvidenceFuture},
		{name: "future evaluation", reader: &evidenceReader{values: []aman.ValidationEvidence{validationEvidence(t, gateNow, gateNow.Add(time.Second), true, "EKCH")}}, want: ReasonEvidenceFuture},
		{name: "stale", reader: &evidenceReader{values: []aman.ValidationEvidence{validationEvidence(t, gateNow.Add(-16*time.Minute), gateNow.Add(-16*time.Minute), true, "EKCH")}}, want: ReasonEvidenceStale},
		{name: "failed", reader: &evidenceReader{values: []aman.ValidationEvidence{validationEvidence(t, gateNow, gateNow, false, "EKCH")}}, want: ReasonEvidenceFailed},
		{name: "digest mismatch", reader: &evidenceReader{values: []aman.ValidationEvidence{validationEvidenceWithConfig(t, gateNow, gateNow, true, "EKCH", Digests{Code: "wrong", Config: "config", Policy: "policy", Navigation: "navigation", Terminal: "terminal", Holding: "holding", Weather: "weather"})}}, want: ReasonDigestMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := newEvaluator(t, test.reader).EvaluateAirport(context.Background(), "EKCH", aman.ModeAuthoritative, readyTechnical())
			require.Equal(t, aman.EffectiveBlocked, decision.EffectiveMode)
			require.Contains(t, decision.BlockedReasons, test.want)
		})
	}
}

func TestEvaluatorUsesNewestReportAndRestoresWhenFreshEvidenceReturns(t *testing.T) {
	reader := &evidenceReader{values: []aman.ValidationEvidence{
		validationEvidence(t, gateNow.Add(-time.Minute), gateNow.Add(-time.Minute), true, "EKCH"),
		{ID: "newest-corrupt", Airport: "EKCH", Kind: report.Version, Payload: []byte("bad"), RecordedAt: gateNow},
	}}
	evaluator := newEvaluator(t, reader)
	blocked := evaluator.EvaluateAirport(context.Background(), "EKCH", aman.ModeReadOnly, readyTechnical())
	require.Equal(t, aman.EffectiveBlocked, blocked.EffectiveMode)
	require.Contains(t, blocked.BlockedReasons, ReasonEvidenceCorrupt)

	reader.values = []aman.ValidationEvidence{validationEvidence(t, gateNow, gateNow, true, "EKCH")}
	restored := evaluator.EvaluateAirport(context.Background(), "EKCH", aman.ModeReadOnly, readyTechnical())
	require.Equal(t, aman.EffectiveReadOnly, restored.EffectiveMode)
	require.True(t, restored.AuthorityAllowed)
}

func TestEvaluatorTreatsMaximumEvidenceAgeAsInclusive(t *testing.T) {
	recordedAt := gateNow.Add(-15 * time.Minute)
	reader := &evidenceReader{values: []aman.ValidationEvidence{validationEvidence(t, recordedAt, recordedAt, true, "EKCH")}}
	decision := newEvaluator(t, reader).EvaluateAirport(context.Background(), "EKCH", aman.ModeReadOnly, readyTechnical())
	require.Equal(t, aman.EffectiveReadOnly, decision.EffectiveMode)
}

func TestEvaluatorReconstructsSameDecisionFromPersistedEvidence(t *testing.T) {
	reader := &evidenceReader{values: []aman.ValidationEvidence{validationEvidence(t, gateNow, gateNow, true, "EKCH")}}
	first := newEvaluator(t, reader).EvaluateAirport(context.Background(), "EKCH", aman.ModeAuthoritative, readyTechnical())
	restarted := newEvaluator(t, reader).EvaluateAirport(context.Background(), "EKCH", aman.ModeAuthoritative, readyTechnical())
	require.Equal(t, first, restarted)
}

func TestNewRejectsUnsafeConfiguration(t *testing.T) {
	_, err := New(&evidenceReader{}, Config{}, func() time.Time { return gateNow })
	require.Error(t, err)
	_, err = New(nil, validConfig(), func() time.Time { return gateNow })
	require.Error(t, err)
	_, err = New(&evidenceReader{}, validConfig(), nil)
	require.Error(t, err)
}

func newEvaluator(t *testing.T, reader aman.ValidationEvidenceReader) *Evaluator {
	t.Helper()
	evaluator, err := New(reader, validConfig(), func() time.Time { return gateNow })
	require.NoError(t, err)
	return evaluator
}

func validConfig() Config {
	return Config{MaximumEvidenceAge: 15 * time.Minute, ExpectedDigests: Digests{Code: "code", Config: "config", Policy: "policy", Navigation: "navigation", Terminal: "terminal", Holding: "holding", Weather: "weather"}}
}

func readyTechnical() aman.TechnicalHealth {
	ready := aman.ComponentHealth{Status: aman.HealthReady}
	return aman.EvaluateTechnicalHealth(aman.ModeAuthoritative, ready, ready, ready, ready, ready, ready)
}

func validationEvidence(t *testing.T, recordedAt, evaluatedAt time.Time, passed bool, airport string) aman.ValidationEvidence {
	t.Helper()
	return validationEvidenceWithConfig(t, recordedAt, evaluatedAt, passed, airport, validConfig().ExpectedDigests)
}

func validationEvidenceWithConfig(t *testing.T, recordedAt, evaluatedAt time.Time, passed bool, airport string, digests Digests) aman.ValidationEvidence {
	t.Helper()
	value := report.Report{
		Version:  report.Version,
		Metadata: report.Metadata{Airport: airport, DatasetID: "replay-fixture", DatasetDigest: "dataset", ReplayDigest: "replay", CodeDigest: digests.Code, ConfigDigest: digests.Config, PolicyDigest: digests.Policy, NavigationDigest: digests.Navigation, TerminalDigest: digests.Terminal, HoldingDigest: digests.Holding, WeatherDigest: digests.Weather, EvaluatedAt: evaluatedAt},
		Horizons: []report.HorizonMetrics{{Horizon: "terminal", Passed: passed}},
		Passed:   passed,
	}
	canonical, err := json.Marshal(value)
	require.NoError(t, err)
	digest := sha256.Sum256(canonical)
	value.Digest = hex.EncodeToString(digest[:])
	evidence, err := value.Evidence(recordedAt)
	require.NoError(t, err)
	return evidence
}
