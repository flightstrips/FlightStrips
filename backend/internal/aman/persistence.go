package aman

import (
	"context"
	"strings"
	"time"
)

// CommandOutcome is the durable result of one controller command. Payload is
// deliberately opaque to this repository: the command owner defines its
// versioned result schema.
type CommandOutcome struct {
	CommandID  string
	Airport    string
	Revision   SequenceRevision
	Payload    []byte
	RecordedAt time.Time
}

// AuditRecord is a structured, append-only record of a committed AMAN change.
// Category and Payload are owned by the component that made the change.
type AuditRecord struct {
	Airport    string
	Revision   SequenceRevision
	Category   string
	Payload    []byte
	RecordedAt time.Time
}

// ValidationEvidence is accepted evidence used by AMAN rollout gates. Its
// payload is versioned by the validation component rather than this store.
type ValidationEvidence struct {
	ID         string
	Airport    string
	Kind       string
	Payload    []byte
	RecordedAt time.Time
}

// StateCommit is the single atomic AMAN persistence operation. A successful
// commit advances exactly one airport revision, optionally records a command
// result, and appends its audit and validation evidence.
type StateCommit struct {
	ExpectedRevision   SequenceRevision
	State              AirportState
	CommandOutcome     *CommandOutcome
	AuditRecords       []AuditRecord
	ValidationEvidence []ValidationEvidence
}

// CommitResult is safe for an owner to publish only after Commit returns nil.
// DuplicateCommand means the stored command outcome was returned and no state
// transition or revision allocation was performed.
type CommitResult struct {
	State            AirportState
	CommandOutcome   *CommandOutcome
	DuplicateCommand bool
}

// AirportStateReader is a consumer-owned read boundary for restart and
// reconnect snapshots. It is intentionally separate from command persistence.
type AirportStateReader interface {
	LoadAirportState(context.Context, string) (AirportState, error)
}

// CommandOutcomeReader is the idempotency read boundary for command owners.
type CommandOutcomeReader interface {
	LoadCommandOutcome(context.Context, string) (CommandOutcome, error)
}

// StateCommitter is the mutation boundary for state and command owners. It is
// not a general Store or TxStore interface; later consumers should depend only
// on this capability when they need the atomic AMAN transition.
type StateCommitter interface {
	Commit(context.Context, StateCommit) (CommitResult, error)
}

// AuditReader and ValidationEvidenceReader keep rollout and audit consumers
// from depending on command or state mutation capabilities.
type AuditReader interface {
	ListAuditRecords(context.Context, string) ([]AuditRecord, error)
}

type ValidationEvidenceReader interface {
	ListValidationEvidence(context.Context, string) ([]ValidationEvidence, error)
}

// ObservationSink receives normalized provider-neutral facts before prediction
// or sequencing. Source adapters own their delivery scheduling; AMAN owners
// decide how an accepted observation changes the aggregate.
type ObservationSink interface {
	Observe(context.Context, FlightObservation) error
}

// VATSIMFlightIdentityBinder is the narrow durable identity capability needed
// by a VATSIM observation adapter. It finds the active binding for a stable
// VATSIM CID, so a corrected callsign never rekeys an active AMAN flight.
//
// It deliberately does not offer aliases, merges, tombstones, or a general
// identity lookup API.
type VATSIMFlightIdentityBinder interface {
	BindVATSIMFlight(context.Context, VATSIMFlightIdentity) (FlightID, error)
}

// VATSIMFlightIdentityRetirer releases an active CID binding once the AMAN
// lifecycle removes that flight. Lifecycle policy owns when to invoke this;
// this narrow repository seam only makes a later flight from the same CID able
// to receive a new generated FlightID.
type VATSIMFlightIdentityRetirer interface {
	RetireVATSIMFlight(context.Context, FlightID) error
}

type VATSIMFlightIdentity struct {
	VATSIMCID       string
	CurrentCallsign string
}

func (i VATSIMFlightIdentity) Validate() error {
	if !isTrimmedNonEmpty(i.VATSIMCID) || !isTrimmedNonEmpty(i.CurrentCallsign) {
		return invalid("VATSIM flight identity is incomplete")
	}
	return nil
}

func (c StateCommit) Validate() error {
	if err := c.State.Validate(); err != nil {
		return err
	}
	if c.State.Revision != c.ExpectedRevision+1 {
		return invalid("state revision must be exactly one greater than expected revision")
	}
	if c.CommandOutcome != nil {
		if err := c.CommandOutcome.validateFor(c.State.Airport, c.State.Revision); err != nil {
			return err
		}
	}
	for _, record := range c.AuditRecords {
		if err := record.validateFor(c.State.Airport, c.State.Revision); err != nil {
			return err
		}
	}
	for _, evidence := range c.ValidationEvidence {
		if err := evidence.validateFor(c.State.Airport); err != nil {
			return err
		}
	}
	return nil
}

func (o CommandOutcome) validateFor(airport string, revision SequenceRevision) error {
	if strings.TrimSpace(o.CommandID) == "" || o.CommandID != strings.TrimSpace(o.CommandID) {
		return invalid("command ID is required")
	}
	if o.Airport != airport || o.Revision != revision {
		return invalid("command outcome must belong to the committed airport revision")
	}
	return requireUTCTime("command outcome recorded at", o.RecordedAt)
}

func (r AuditRecord) validateFor(airport string, revision SequenceRevision) error {
	if r.Airport != airport || r.Revision != revision || !isTrimmedNonEmpty(r.Category) {
		return invalid("audit record must belong to the committed airport revision")
	}
	return requireUTCTime("audit record recorded at", r.RecordedAt)
}

func (e ValidationEvidence) validateFor(airport string) error {
	if !isTrimmedNonEmpty(e.ID) || e.Airport != airport || !isTrimmedNonEmpty(e.Kind) {
		return invalid("validation evidence is incomplete")
	}
	return requireUTCTime("validation evidence recorded at", e.RecordedAt)
}
