// Package report compiles deterministic AMAN accuracy and sequence-integrity
// evidence from provider-neutral replay inputs.
package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/replay"
)

const Version = "aman-validation-report/v1"

var ErrInvalidReport = errors.New("invalid AMAN validation report")

// Horizon is an inclusive range of seconds between a forecast being generated
// and independently recorded landing truth. Thresholds use seconds so their
// JSON representation cannot silently lose duration precision.
type Horizon struct {
	Name                string  `json:"name"`
	MinimumSeconds      int64   `json:"minimum_seconds"`
	MaximumSeconds      int64   `json:"maximum_seconds"`
	MaximumMAESeconds   float64 `json:"maximum_mae_seconds"`
	MaximumRMSESeconds  float64 `json:"maximum_rmse_seconds"`
	MinimumAvailability float64 `json:"minimum_availability"`
}

type Policy struct {
	Version  string    `json:"version"`
	Horizons []Horizon `json:"horizons"`
}

// Provenance is supplied by the replay composition for normalized navigation
// fragments that are intentionally not vendor DTOs. All digests are required
// before evidence can become rollout input.
type Provenance struct {
	NavigationDigest string `json:"navigation_digest"`
	TerminalDigest   string `json:"terminal_digest"`
	HoldingDigest    string `json:"holding_digest"`
}

type IntegrityKind string

const (
	IntegrityDuplicateFlight        IntegrityKind = "duplicate_flight"
	IntegrityDuplicateOrder         IntegrityKind = "duplicate_order"
	IntegrityDuplicateRevision      IntegrityKind = "duplicate_revision"
	IntegrityWTCViolation           IntegrityKind = "wtc_violation"
	IntegrityUnauthorizedFreeze     IntegrityKind = "unauthorized_freeze_movement"
	IntegrityCommandIdempotency     IntegrityKind = "command_idempotency"
	IntegrityUnauthorizedDirectFact IntegrityKind = "unauthorized_direct_fact"
)

func (k IntegrityKind) Valid() bool {
	switch k {
	case IntegrityDuplicateFlight, IntegrityDuplicateOrder, IntegrityDuplicateRevision, IntegrityWTCViolation, IntegrityUnauthorizedFreeze, IntegrityCommandIdempotency, IntegrityUnauthorizedDirectFact:
		return true
	default:
		return false
	}
}

// Finding lets a pure sequence or route-fact validator contribute a concrete,
// provider-neutral violation without defining another replay transport schema.
type Finding struct {
	Kind     IntegrityKind `json:"kind"`
	FlightID aman.FlightID `json:"flight_id,omitempty"`
	Detail   string        `json:"detail"`
}

type Input struct {
	Dataset     replay.Dataset
	Replay      replay.Result
	Policy      Policy
	Provenance  Provenance
	EvaluatedAt time.Time
	Findings    []Finding
}

type Metadata struct {
	Airport          string    `json:"airport"`
	DatasetID        string    `json:"dataset_id"`
	DatasetDigest    string    `json:"dataset_digest"`
	ReplayDigest     string    `json:"replay_digest"`
	CodeDigest       string    `json:"code_digest"`
	ConfigDigest     string    `json:"config_digest"`
	PolicyDigest     string    `json:"policy_digest"`
	NavigationDigest string    `json:"navigation_digest"`
	TerminalDigest   string    `json:"terminal_digest"`
	HoldingDigest    string    `json:"holding_digest"`
	WeatherDigest    string    `json:"weather_digest"`
	EvaluatedAt      time.Time `json:"evaluated_at"`
}

type HorizonMetrics struct {
	Horizon                        string  `json:"horizon"`
	EligibleCount                  int     `json:"eligible_count"`
	ExcludedMissingTruthCount      int     `json:"excluded_missing_truth_count"`
	ExcludedMissingPredictionCount int     `json:"excluded_missing_prediction_count"`
	BiasSeconds                    float64 `json:"bias_seconds"`
	MAESeconds                     float64 `json:"mae_seconds"`
	RMSESeconds                    float64 `json:"rmse_seconds"`
	MedianAbsoluteErrorSeconds     float64 `json:"median_absolute_error_seconds"`
	P90AbsoluteErrorSeconds        float64 `json:"p90_absolute_error_seconds"`
	Availability                   float64 `json:"availability"`
	Passed                         bool    `json:"passed"`
}

type IntegrityCounts struct {
	DuplicateFlights        int `json:"duplicate_flights"`
	DuplicateOrders         int `json:"duplicate_orders"`
	DuplicateRevisions      int `json:"duplicate_revisions"`
	WTCViolations           int `json:"wtc_violations"`
	UnauthorizedFreezeMoves int `json:"unauthorized_freeze_movements"`
	CommandIdempotency      int `json:"command_idempotency"`
	UnauthorizedDirectFacts int `json:"unauthorized_direct_facts"`
}

func (c IntegrityCounts) Critical() int {
	return c.DuplicateFlights + c.DuplicateOrders + c.DuplicateRevisions + c.WTCViolations + c.UnauthorizedFreezeMoves + c.CommandIdempotency + c.UnauthorizedDirectFacts
}

type Report struct {
	Version   string           `json:"version"`
	Metadata  Metadata         `json:"metadata"`
	Horizons  []HorizonMetrics `json:"horizons"`
	Integrity IntegrityCounts  `json:"integrity"`
	Passed    bool             `json:"passed"`
	Digest    string           `json:"digest"`
}

func Compile(input Input) (Report, error) {
	if err := input.Dataset.Validate(); err != nil {
		return Report{}, err
	}
	if err := input.Policy.validate(); err != nil {
		return Report{}, err
	}
	if !utc(input.EvaluatedAt) || !input.Provenance.valid() {
		return Report{}, fmt.Errorf("%w: incomplete provenance or evaluation time", ErrInvalidReport)
	}
	datasetDigest, err := input.Dataset.Digest()
	if err != nil {
		return Report{}, err
	}
	if input.Replay.DatasetDigest != datasetDigest || strings.TrimSpace(input.Replay.OutputDigest) == "" {
		return Report{}, fmt.Errorf("%w: replay result does not match dataset", ErrInvalidReport)
	}
	policyDigest, err := digest(input.Policy)
	if err != nil {
		return Report{}, err
	}
	report := Report{Version: Version, Metadata: Metadata{
		Airport: input.Dataset.Metadata.Airport, DatasetID: input.Dataset.Metadata.ID, DatasetDigest: datasetDigest, ReplayDigest: input.Replay.OutputDigest,
		CodeDigest: input.Dataset.Metadata.CodeDigest, ConfigDigest: input.Dataset.Metadata.ConfigDigest, PolicyDigest: policyDigest,
		NavigationDigest: input.Provenance.NavigationDigest, TerminalDigest: input.Provenance.TerminalDigest, HoldingDigest: input.Provenance.HoldingDigest,
		WeatherDigest: input.Dataset.Metadata.WeatherDigest, EvaluatedAt: input.EvaluatedAt,
	}}
	samples, missingTruth, missingPrediction := landingSamples(input.Dataset, input.Replay)
	for _, horizon := range input.Policy.Horizons {
		report.Horizons = append(report.Horizons, metrics(horizon, samples, missingTruth, missingPrediction))
	}
	report.Integrity = integrity(input.Dataset, input.Replay, input.Findings)
	report.Passed = report.Integrity.Critical() == 0
	for _, metric := range report.Horizons {
		report.Passed = report.Passed && metric.Passed
	}
	bytes, err := report.canonicalWithoutDigest()
	if err != nil {
		return Report{}, err
	}
	sum := sha256.Sum256(bytes)
	report.Digest = hex.EncodeToString(sum[:])
	return report, nil
}

type sample struct{ horizon, errorSeconds int64 }

func landingSamples(dataset replay.Dataset, result replay.Result) ([]sample, int, int) {
	generated := map[aman.FlightID]time.Time{}
	samples := []sample{}
	missingTruth, missingPrediction := 0, 0
	for _, output := range result.Outputs {
		if int(output.Index) >= len(dataset.Records) {
			continue
		}
		record := dataset.Records[output.Index]
		if record.Observation != nil && output.Outcome.Prediction != nil {
			generated[record.Observation.FlightID] = output.Outcome.Prediction.GeneratedAt
		}
		for _, comparison := range output.Outcome.LandingComparisons {
			at, ok := generated[comparison.FlightID]
			if !ok || comparison.ErrorSeconds == nil {
				missingPrediction++
				continue
			}
			horizon := comparison.LandedAt.Sub(at)
			if horizon < 0 {
				missingPrediction++
				continue
			}
			samples = append(samples, sample{horizon: int64(horizon / time.Second), errorSeconds: *comparison.ErrorSeconds})
		}
	}
	for _, record := range dataset.Records {
		if record.Landing != nil {
			missingTruth++
		}
	}
	// Landing comparisons represent truth that was actually matched. Do not let
	// repeated comparisons hide an omitted truth record.
	matched := 0
	for _, output := range result.Outputs {
		matched += len(output.Outcome.LandingComparisons)
	}
	if missingTruth >= matched {
		missingTruth -= matched
	} else {
		missingTruth = 0
	}
	return samples, missingTruth, missingPrediction
}

func metrics(h Horizon, samples []sample, missingTruth, missingPrediction int) HorizonMetrics {
	m := HorizonMetrics{Horizon: h.Name, ExcludedMissingTruthCount: missingTruth, ExcludedMissingPredictionCount: missingPrediction}
	errors := []float64{}
	for _, sample := range samples {
		if sample.horizon >= h.MinimumSeconds && sample.horizon <= h.MaximumSeconds {
			errors = append(errors, float64(sample.errorSeconds))
		}
	}
	m.EligibleCount = len(errors)
	if len(errors) > 0 {
		var sum, squares, absolute float64
		abs := make([]float64, 0, len(errors))
		for _, value := range errors {
			sum += value
			squares += value * value
			absolute += math.Abs(value)
			abs = append(abs, math.Abs(value))
		}
		m.BiasSeconds = sum / float64(len(errors))
		m.MAESeconds = absolute / float64(len(errors))
		m.RMSESeconds = math.Sqrt(squares / float64(len(errors)))
		sort.Float64s(abs)
		m.MedianAbsoluteErrorSeconds = percentile(abs, .5)
		m.P90AbsoluteErrorSeconds = percentile(abs, .9)
	}
	denominator := m.EligibleCount + m.ExcludedMissingTruthCount + m.ExcludedMissingPredictionCount
	if denominator > 0 {
		m.Availability = float64(m.EligibleCount) / float64(denominator)
	}
	m.Passed = m.MAESeconds <= h.MaximumMAESeconds && m.RMSESeconds <= h.MaximumRMSESeconds && m.Availability >= h.MinimumAvailability
	return m
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Ceil(p*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}

func integrity(dataset replay.Dataset, result replay.Result, findings []Finding) IntegrityCounts {
	counts := IntegrityCounts{}
	revisions := map[aman.SequenceRevision]struct{}{}
	previous := map[aman.FlightID]aman.AMANFlight{}
	commands := map[string][]byte{}
	for _, output := range result.Outputs {
		outcome := output.Outcome
		if state := outcome.AirportState; state != nil {
			if _, exists := revisions[state.Revision]; exists {
				counts.DuplicateRevisions++
			}
			revisions[state.Revision] = struct{}{}
			flights := map[aman.FlightID]struct{}{}
			orders := map[string]struct{}{}
			for _, flight := range state.Flights {
				if _, exists := flights[flight.ID]; exists {
					counts.DuplicateFlights++
				}
				flights[flight.ID] = struct{}{}
				if flight.Order != nil {
					group := ""
					if flight.SelectedRunwayGroup != nil {
						group = string(*flight.SelectedRunwayGroup)
					}
					key := fmt.Sprintf("%s/%d", group, *flight.Order)
					if _, exists := orders[key]; exists {
						counts.DuplicateOrders++
					}
					orders[key] = struct{}{}
				}
				if old, exists := previous[flight.ID]; exists && old.FreezeReason == aman.FreezeSuperstable && flight.FreezeReason == aman.FreezeSuperstable && !freezeEqual(old, flight) && !freezeMoveAuthorized(dataset, output, outcome) {
					counts.UnauthorizedFreezeMoves++
				}
				previous[flight.ID] = flight
			}
		}
		if command := outcome.CommandOutcome; command != nil {
			if prior, exists := commands[command.CommandID]; exists && !bytes.Equal(prior, command.Payload) {
				counts.CommandIdempotency++
			}
			commands[command.CommandID] = slices.Clone(command.Payload)
		}
	}
	for _, finding := range findings {
		switch finding.Kind {
		case IntegrityDuplicateFlight:
			counts.DuplicateFlights++
		case IntegrityDuplicateOrder:
			counts.DuplicateOrders++
		case IntegrityDuplicateRevision:
			counts.DuplicateRevisions++
		case IntegrityWTCViolation:
			counts.WTCViolations++
		case IntegrityUnauthorizedFreeze:
			counts.UnauthorizedFreezeMoves++
		case IntegrityCommandIdempotency:
			counts.CommandIdempotency++
		case IntegrityUnauthorizedDirectFact:
			counts.UnauthorizedDirectFacts++
		}
	}
	return counts
}

func freezeMoveAuthorized(dataset replay.Dataset, output replay.Output, outcome replay.Outcome) bool {
	if outcome.Lifecycle != nil && outcome.Lifecycle.To == aman.StateGoAround {
		return true
	}
	if int(output.Index) >= len(dataset.Records) {
		return false
	}
	command := dataset.Records[output.Index].Command
	return command != nil && command.ManualFreeze != nil
}

func freezeEqual(a, b aman.AMANFlight) bool {
	if !timeEqual(a.FrozenOperationalTETA, b.FrozenOperationalTETA) {
		return false
	}
	if a.Slot == nil || b.Slot == nil {
		return a.Slot == nil && b.Slot == nil
	}
	return a.Slot.Time.Equal(b.Slot.Time) && a.Slot.Sequence == b.Slot.Sequence && a.Slot.RunwayGroupID == b.Slot.RunwayGroupID
}

func timeEqual(a, b *time.Time) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Equal(*b))
}

func (p Policy) validate() error {
	if strings.TrimSpace(p.Version) == "" || len(p.Horizons) == 0 {
		return fmt.Errorf("%w: policy is incomplete", ErrInvalidReport)
	}
	names := map[string]struct{}{}
	for _, h := range p.Horizons {
		if strings.TrimSpace(h.Name) == "" || h.MinimumSeconds < 0 || h.MaximumSeconds < h.MinimumSeconds || h.MaximumMAESeconds < 0 || h.MaximumRMSESeconds < 0 || h.MinimumAvailability < 0 || h.MinimumAvailability > 1 {
			return fmt.Errorf("%w: invalid horizon", ErrInvalidReport)
		}
		if _, exists := names[h.Name]; exists {
			return fmt.Errorf("%w: duplicate horizon", ErrInvalidReport)
		}
		names[h.Name] = struct{}{}
	}
	return nil
}
func (p Provenance) valid() bool {
	return strings.TrimSpace(p.NavigationDigest) != "" && strings.TrimSpace(p.TerminalDigest) != "" && strings.TrimSpace(p.HoldingDigest) != ""
}
func utc(t time.Time) bool { return !t.IsZero() && t.Location() == time.UTC }
func digest(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), nil
}

func (r Report) canonicalWithoutDigest() ([]byte, error) { r.Digest = ""; return json.Marshal(r) }
func (r Report) CanonicalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(r)
}
func (r Report) Validate() error {
	if r.Version != Version || !utc(r.Metadata.EvaluatedAt) || strings.TrimSpace(r.Metadata.Airport) == "" || strings.TrimSpace(r.Metadata.DatasetID) == "" || strings.TrimSpace(r.Metadata.DatasetDigest) == "" || strings.TrimSpace(r.Metadata.ReplayDigest) == "" || strings.TrimSpace(r.Metadata.CodeDigest) == "" || strings.TrimSpace(r.Metadata.ConfigDigest) == "" || strings.TrimSpace(r.Metadata.PolicyDigest) == "" || strings.TrimSpace(r.Metadata.NavigationDigest) == "" || strings.TrimSpace(r.Metadata.TerminalDigest) == "" || strings.TrimSpace(r.Metadata.HoldingDigest) == "" || strings.TrimSpace(r.Metadata.WeatherDigest) == "" || strings.TrimSpace(r.Digest) == "" {
		return ErrInvalidReport
	}
	if len(r.Horizons) == 0 || r.Integrity.Critical() < 0 {
		return ErrInvalidReport
	}
	bytes, err := r.canonicalWithoutDigest()
	if err != nil {
		return err
	}
	sum := sha256.Sum256(bytes)
	if r.Digest != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("%w: digest mismatch", ErrInvalidReport)
	}
	return nil
}

func Decode(reader io.Reader) (Report, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var report Report
	if err := decoder.Decode(&report); err != nil {
		return Report{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Report{}, fmt.Errorf("%w: trailing content", ErrInvalidReport)
	}
	if err := report.Validate(); err != nil {
		return Report{}, err
	}
	return report, nil
}

func (r Report) Markdown() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# AMAN validation report\\n\\nStatus: %s\\n\\n", map[bool]string{true: "PASS", false: "FAIL"}[r.Passed])
	fmt.Fprintf(&b, "Dataset: `%s`  \\nDigest: `%s`\\n\\n", r.Metadata.DatasetID, r.Digest)
	b.WriteString("| Horizon | Eligible | MAE (s) | RMSE (s) | Availability | Result |\\n| --- | ---: | ---: | ---: | ---: | --- |\\n")
	for _, m := range r.Horizons {
		fmt.Fprintf(&b, "| %s | %d | %.3f | %.3f | %.3f | %s |\\n", m.Horizon, m.EligibleCount, m.MAESeconds, m.RMSESeconds, m.Availability, map[bool]string{true: "PASS", false: "FAIL"}[m.Passed])
	}
	fmt.Fprintf(&b, "\\nCritical integrity violations: %d\\n", r.Integrity.Critical())
	return b.String(), nil
}

// Evidence serializes accepted report bytes through #306's narrow evidence
// boundary. The caller owns atomic persistence and any rollout decision.
func (r Report) Evidence(recordedAt time.Time) (aman.ValidationEvidence, error) {
	bytes, err := r.CanonicalJSON()
	if err != nil {
		return aman.ValidationEvidence{}, err
	}
	if !utc(recordedAt) {
		return aman.ValidationEvidence{}, fmt.Errorf("%w: evidence time must be UTC", ErrInvalidReport)
	}
	return aman.ValidationEvidence{ID: r.Digest, Airport: r.Metadata.Airport, Kind: Version, Payload: bytes, RecordedAt: recordedAt}, nil
}
