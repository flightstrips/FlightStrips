// Package replay provides a deterministic, offline AMAN domain replay harness.
//
// Fixtures intentionally contain only normalized AMAN facts. They never carry
// a VATSIM/provider DTO, controller identity, or delivery/outbox state.
package replay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

const DatasetVersion = "aman-replay/v1"

var (
	ErrInvalidDataset    = errors.New("invalid AMAN replay dataset")
	ErrInvalidCheckpoint = errors.New("invalid AMAN replay checkpoint")
)

// Dataset is a versioned, portable AMAN replay fixture. Its digests pin all
// non-observation inputs so a replay cannot accidentally use live data.
type Dataset struct {
	Version  string   `json:"version"`
	Metadata Metadata `json:"metadata"`
	Records  []Record `json:"records"`
}

type Metadata struct {
	ID            string    `json:"id"`
	Airport       string    `json:"airport"`
	CodeDigest    string    `json:"code_digest"`
	ConfigDigest  string    `json:"config_digest"`
	Geometry      Manifest  `json:"geometry"`
	WeatherDigest string    `json:"weather_digest"`
	ClockOrigin   time.Time `json:"clock_origin"`
}

// Manifest names the canonical navigation input used by the fixture. The
// actual route, terminal and holding fragments remain normalized records.
type Manifest struct {
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// Record ordering is explicit: Index and At are source-ordering facts, not
// values inferred from a wall clock. Exactly one payload is present.
type Record struct {
	Index       uint64                  `json:"index"`
	At          time.Time               `json:"at"`
	Observation *aman.FlightObservation `json:"observation,omitempty"`
	RouteFact   *aman.RouteFact         `json:"route_fact,omitempty"`
	Policy      *PolicyChange           `json:"policy,omitempty"`
	Command     *Command                `json:"command,omitempty"`
	Landing     *LandingTruth           `json:"landing_truth,omitempty"`
	Retire      *Retirement             `json:"retire,omitempty"`
}

// PolicyChange is a typed replay fact; policy materialization remains owned by
// the injected AMAN domain component.
type PolicyChange struct {
	Version string `json:"version"`
}

// Command is a closed, typed command envelope. It does not model controller
// identity or transport metadata, and it cannot represent arbitrary events.
type Command struct {
	ManualFreeze  *ManualFreezeCommand  `json:"manual_freeze,omitempty"`
	ReleaseFreeze *ReleaseFreezeCommand `json:"release_freeze,omitempty"`
	AssignHolding *AssignHoldingCommand `json:"assign_holding,omitempty"`
}

type ManualFreezeCommand struct {
	Metadata aman.CommandMetadata `json:"metadata"`
	FlightID aman.FlightID        `json:"flight_id"`
}

type ReleaseFreezeCommand struct {
	Metadata aman.CommandMetadata `json:"metadata"`
	FlightID aman.FlightID        `json:"flight_id"`
}

type AssignHoldingCommand struct {
	Metadata aman.CommandMetadata `json:"metadata"`
	FlightID aman.FlightID        `json:"flight_id"`
	Holding  string               `json:"holding"`
}

// LandingTruth is the independently recorded landing fact consumed by the
// accuracy/integrity report owner. It is not a lifecycle inference.
type LandingTruth struct {
	FlightID aman.FlightID `json:"flight_id"`
	Source   string        `json:"source"`
	LandedAt time.Time     `json:"landed_at"`
}

// Retirement explicitly ends the active CID lifetime in a fixture. This lets
// a later, separate flight reuse the same CID without an alias/merge model.
type Retirement struct {
	FlightID  aman.FlightID `json:"flight_id"`
	VATSIMCID string        `json:"vatsim_cid"`
}

// Input is the provider-neutral value presented to the same domain processor
// that production composition injects. Record contains the fully bound flight
// identity after replay-only binding.
type Input struct {
	DatasetID string
	Record    Record
}

// Outcome captures domain results rather than publication delivery. Values
// are concrete AMAN contracts so golden outputs remain useful to consumers.
type Outcome struct {
	Prediction         *aman.Prediction
	Lifecycle          *LifecycleTransition
	AirportState       *aman.AirportState
	CommandOutcome     *aman.CommandOutcome
	Health             *aman.TechnicalHealth
	Audit              []aman.AuditRecord
	LandingComparisons []LandingComparison
}

type LifecycleTransition struct {
	FlightID     aman.FlightID
	From, To     aman.FlightState
	DataStatus   aman.DataStatus
	FreezeReason aman.FreezeReason
}

// LandingComparison is deliberately a report input/output fact. #329 owns
// aggregation and presentation of these comparisons.
type LandingComparison struct {
	FlightID     aman.FlightID
	PredictedAt  *time.Time
	LandedAt     time.Time
	ErrorSeconds *int64
	TruthSource  string
}

// Processor is implemented by AMAN application composition. Snapshot must
// include the processor-owned prediction history, route progress, freeze,
// commands and sequence state needed to resume deterministically.
type Processor interface {
	Apply(context.Context, Input) (Outcome, error)
	Snapshot(context.Context) ([]byte, error)
	Restore(context.Context, []byte) error
}

type Clock interface{ Set(time.Time) }

// FlightIDGenerator intentionally exists only in this fixture package. It is
// never a production UUID or alias-resolution contract.
type FlightIDGenerator interface {
	FlightID(datasetID string, firstObservationIndex uint64) aman.FlightID
}

type Dependencies struct {
	Clock     Clock
	IDs       FlightIDGenerator
	Processor Processor
}

type Runner struct{ deps Dependencies }

func NewRunner(deps Dependencies) (*Runner, error) {
	if deps.Clock == nil || deps.IDs == nil || deps.Processor == nil {
		return nil, fmt.Errorf("replay runner requires clock, ID generator, and processor")
	}
	return &Runner{deps: deps}, nil
}

type Output struct {
	Index   uint64  `json:"index"`
	Outcome Outcome `json:"outcome"`
}

type Result struct {
	DatasetDigest string     `json:"dataset_digest"`
	Outputs       []Output   `json:"outputs"`
	Checkpoint    Checkpoint `json:"checkpoint"`
	OutputDigest  string     `json:"output_digest"`
}

// Checkpoint stores the runner's replay-only CID bindings plus opaque state
// owned by the domain processor. Bindings are sorted before serialization.
type Checkpoint struct {
	DatasetDigest string       `json:"dataset_digest"`
	AfterIndex    uint64       `json:"after_index"`
	Bindings      []CIDBinding `json:"bindings"`
	Processor     []byte       `json:"processor"`
}

type CIDBinding struct {
	VATSIMCID string        `json:"vatsim_cid"`
	FlightID  aman.FlightID `json:"flight_id"`
}

func (d Dataset) Validate() error {
	if d.Version != DatasetVersion {
		return fmt.Errorf("%w: version must be %q", ErrInvalidDataset, DatasetVersion)
	}
	if strings.TrimSpace(d.Metadata.ID) == "" || strings.TrimSpace(d.Metadata.Airport) == "" ||
		strings.TrimSpace(d.Metadata.CodeDigest) == "" || strings.TrimSpace(d.Metadata.ConfigDigest) == "" ||
		strings.TrimSpace(d.Metadata.Geometry.Version) == "" || strings.TrimSpace(d.Metadata.Geometry.Digest) == "" ||
		strings.TrimSpace(d.Metadata.WeatherDigest) == "" || !utc(d.Metadata.ClockOrigin) {
		return fmt.Errorf("%w: incomplete metadata", ErrInvalidDataset)
	}
	if len(d.Records) == 0 {
		return fmt.Errorf("%w: records are required", ErrInvalidDataset)
	}
	for i, record := range d.Records {
		if record.Index != uint64(i) || !utc(record.At) {
			return fmt.Errorf("%w: record %d has invalid ordering", ErrInvalidDataset, i)
		}
		if i > 0 && record.At.Before(d.Records[i-1].At) {
			return fmt.Errorf("%w: record %d timestamp moves backward", ErrInvalidDataset, i)
		}
		if err := record.validate(); err != nil {
			return fmt.Errorf("%w: record %d: %v", ErrInvalidDataset, i, err)
		}
	}
	return nil
}

func (r Record) validate() error {
	count := 0
	if r.Observation != nil {
		count++
	}
	if r.RouteFact != nil {
		count++
	}
	if r.Policy != nil {
		count++
	}
	if r.Command != nil {
		count++
	}
	if r.Landing != nil {
		count++
	}
	if r.Retire != nil {
		count++
	}
	if count != 1 {
		return errors.New("exactly one record payload is required")
	}
	if r.Observation != nil {
		if strings.TrimSpace(r.Observation.VATSIMCID) == "" || strings.TrimSpace(r.Observation.Callsign) == "" ||
			strings.TrimSpace(r.Observation.Origin) == "" || strings.TrimSpace(r.Observation.Destination) == "" || !utc(r.Observation.ReconciledAt) {
			return errors.New("observation is incomplete")
		}
	}
	if r.RouteFact != nil && (strings.TrimSpace(r.RouteFact.ID) == "" || strings.TrimSpace(r.RouteFact.Fix) == "" || !utc(r.RouteFact.ObservedAt) || r.RouteFact.State == "" || !r.RouteFact.State.Valid()) {
		return errors.New("route fact is invalid")
	}
	if r.Policy != nil && strings.TrimSpace(r.Policy.Version) == "" {
		return errors.New("policy version is required")
	}
	if r.Command != nil {
		return r.Command.validate()
	}
	if r.Landing != nil && (strings.TrimSpace(string(r.Landing.FlightID)) == "" || strings.TrimSpace(r.Landing.Source) == "" || !utc(r.Landing.LandedAt)) {
		return errors.New("landing truth is invalid")
	}
	if r.Retire != nil && (strings.TrimSpace(string(r.Retire.FlightID)) == "" || strings.TrimSpace(r.Retire.VATSIMCID) == "") {
		return errors.New("retirement is invalid")
	}
	return nil
}

func (c Command) validate() error {
	count := 0
	if c.ManualFreeze != nil {
		count++
	}
	if c.ReleaseFreeze != nil {
		count++
	}
	if c.AssignHolding != nil {
		count++
	}
	if count != 1 {
		return errors.New("exactly one typed command is required")
	}
	metadata := aman.CommandMetadata{}
	flightID := aman.FlightID("")
	switch {
	case c.ManualFreeze != nil:
		metadata, flightID = c.ManualFreeze.Metadata, c.ManualFreeze.FlightID
	case c.ReleaseFreeze != nil:
		metadata, flightID = c.ReleaseFreeze.Metadata, c.ReleaseFreeze.FlightID
	case c.AssignHolding != nil:
		metadata, flightID = c.AssignHolding.Metadata, c.AssignHolding.FlightID
		if strings.TrimSpace(c.AssignHolding.Holding) == "" {
			return errors.New("holding is required")
		}
	}
	if strings.TrimSpace(metadata.CommandID) == "" || strings.TrimSpace(string(flightID)) == "" {
		return errors.New("command identity is incomplete")
	}
	return nil
}

func (d Dataset) CanonicalBytes() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(d)
}

// Decode parses one strict fixture document. Unknown fields and concatenated
// JSON values are rejected so fixture changes cannot silently alter a replay.
func Decode(reader io.Reader) (Dataset, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var dataset Dataset
	if err := decoder.Decode(&dataset); err != nil {
		return Dataset{}, fmt.Errorf("decode AMAN replay dataset: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Dataset{}, fmt.Errorf("decode AMAN replay dataset: trailing content")
	}
	if err := dataset.Validate(); err != nil {
		return Dataset{}, err
	}
	return dataset, nil
}

func (d Dataset) Digest() (string, error) {
	bytes, err := d.CanonicalBytes()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(bytes)
	return hex.EncodeToString(digest[:]), nil
}

func (r *Runner) Replay(ctx context.Context, dataset Dataset) (Result, error) {
	digest, err := dataset.Digest()
	if err != nil {
		return Result{}, err
	}
	return r.replay(ctx, dataset, digest, -1, len(dataset.Records)-1, nil)
}

// ReplayTo creates a checkpoint after any fixture record. It exists for
// restart testing; production code does not consume replay checkpoints.
func (r *Runner) ReplayTo(ctx context.Context, dataset Dataset, afterIndex uint64) (Result, error) {
	digest, err := dataset.Digest()
	if err != nil {
		return Result{}, err
	}
	if afterIndex >= uint64(len(dataset.Records)) {
		return Result{}, fmt.Errorf("%w: record boundary is outside dataset", ErrInvalidCheckpoint)
	}
	return r.replay(ctx, dataset, digest, -1, int(afterIndex), nil)
}

func (r *Runner) Resume(ctx context.Context, dataset Dataset, checkpoint Checkpoint) (Result, error) {
	digest, err := dataset.Digest()
	if err != nil {
		return Result{}, err
	}
	if err := checkpoint.validate(digest, len(dataset.Records)); err != nil {
		return Result{}, err
	}
	if err := r.deps.Processor.Restore(ctx, slices.Clone(checkpoint.Processor)); err != nil {
		return Result{}, fmt.Errorf("restore replay checkpoint: %w", err)
	}
	bindings := make(map[string]aman.FlightID, len(checkpoint.Bindings))
	for _, binding := range checkpoint.Bindings {
		bindings[binding.VATSIMCID] = binding.FlightID
	}
	return r.replay(ctx, dataset, digest, int(checkpoint.AfterIndex), len(dataset.Records)-1, bindings)
}

func (r *Runner) replay(ctx context.Context, dataset Dataset, digest string, after, through int, bindings map[string]aman.FlightID) (Result, error) {
	if bindings == nil {
		bindings = map[string]aman.FlightID{}
	}
	result := Result{DatasetDigest: digest, Outputs: []Output{}}
	for i := after + 1; i <= through; i++ {
		record := dataset.Records[i]
		r.deps.Clock.Set(record.At)
		bound, err := r.bind(dataset.Metadata.ID, record, bindings)
		if err != nil {
			return Result{}, fmt.Errorf("bind record %d: %w", record.Index, err)
		}
		outcome, err := r.deps.Processor.Apply(ctx, Input{DatasetID: dataset.Metadata.ID, Record: bound})
		if err != nil {
			return Result{}, fmt.Errorf("replay record %d: %w", record.Index, err)
		}
		result.Outputs = append(result.Outputs, Output{Index: record.Index, Outcome: outcome})
	}
	checkpoint, err := r.checkpoint(ctx, digest, uint64(through), bindings)
	if err != nil {
		return Result{}, err
	}
	result.Checkpoint = checkpoint
	result.OutputDigest, err = digestOutputs(result.Outputs)
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func (r *Runner) bind(datasetID string, record Record, bindings map[string]aman.FlightID) (Record, error) {
	if record.Observation != nil {
		observation := *record.Observation
		current, exists := bindings[observation.VATSIMCID]
		if observation.FlightID == "" {
			if exists {
				observation.FlightID = current
			} else {
				observation.FlightID = r.deps.IDs.FlightID(datasetID, record.Index)
			}
		} else if exists && current != observation.FlightID {
			return Record{}, errors.New("active CID cannot bind to a different flight ID")
		}
		if observation.FlightID == "" {
			return Record{}, errors.New("ID generator returned an empty flight ID")
		}
		bindings[observation.VATSIMCID] = observation.FlightID
		if err := observation.Validate(); err != nil {
			return Record{}, err
		}
		record.Observation = &observation
	}
	if record.Retire != nil {
		if bound, exists := bindings[record.Retire.VATSIMCID]; !exists || bound != record.Retire.FlightID {
			return Record{}, errors.New("retirement does not match active CID binding")
		}
		delete(bindings, record.Retire.VATSIMCID)
	}
	return record, nil
}

func (r *Runner) checkpoint(ctx context.Context, digest string, after uint64, bindings map[string]aman.FlightID) (Checkpoint, error) {
	processor, err := r.deps.Processor.Snapshot(ctx)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("snapshot replay processor: %w", err)
	}
	values := make([]CIDBinding, 0, len(bindings))
	for cid, flightID := range bindings {
		values = append(values, CIDBinding{VATSIMCID: cid, FlightID: flightID})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].VATSIMCID < values[j].VATSIMCID })
	return Checkpoint{DatasetDigest: digest, AfterIndex: after, Bindings: values, Processor: slices.Clone(processor)}, nil
}

func (c Checkpoint) validate(digest string, records int) error {
	if c.DatasetDigest != digest || records == 0 || c.AfterIndex >= uint64(records) {
		return fmt.Errorf("%w: dataset or record boundary mismatch", ErrInvalidCheckpoint)
	}
	last := ""
	for _, binding := range c.Bindings {
		if strings.TrimSpace(binding.VATSIMCID) == "" || strings.TrimSpace(string(binding.FlightID)) == "" || binding.VATSIMCID <= last {
			return fmt.Errorf("%w: bindings must be non-empty and sorted", ErrInvalidCheckpoint)
		}
		last = binding.VATSIMCID
	}
	return nil
}

func digestOutputs(outputs []Output) (string, error) {
	bytes, err := json.Marshal(outputs)
	if err != nil {
		return "", fmt.Errorf("marshal replay outputs: %w", err)
	}
	digest := sha256.Sum256(bytes)
	return hex.EncodeToString(digest[:]), nil
}

func utc(value time.Time) bool { return !value.IsZero() && value.Location() == time.UTC }
