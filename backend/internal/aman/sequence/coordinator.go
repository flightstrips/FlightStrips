package sequence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/aman"
)

// FullStatePublisher maps the complete committed airport state onto the
// existing frontend and EuroScope hubs. Transport DTOs and login handlers are
// owned by their respective tasks; this boundary deliberately has no outbox,
// acknowledgement, or retry contract.
type FullStatePublisher interface {
	PublishAMANState(context.Context, aman.AirportState) error
}

// AuditEntry is audit data before the coordinator binds it to the committed
// airport revision and transaction time.
type AuditEntry struct {
	Category string
	Payload  json.RawMessage
}

// CommandChange is the uncommitted result of one typed command. Apply must not
// allocate a revision or change the airport identity; Coordinator is the sole
// sequence-revision writer.
type CommandChange struct {
	State       aman.AirportState
	Changed     bool
	Outcome     json.RawMessage
	Audit       []AuditEntry
	QueueOffers *QueueOfferCalculation
}

// CommandMutation adapts a typed policy operation to the persisted aggregate.
// It receives a detached copy so a failed policy or commit cannot mutate the
// repository-owned current state in memory.
type CommandMutation func(aman.AirportState) (CommandChange, error)

type CommandResult struct {
	State     aman.AirportState
	Outcome   aman.CommandOutcome
	Changed   bool
	Duplicate bool
}

// PublicationError means the transaction committed successfully but direct
// full-state publication failed. Callers must not treat it as a rollback.
// Reconnect and command retry recover through persisted state and outcome.
type PublicationError struct{ Err error }

func (e *PublicationError) Error() string {
	return "publish committed AMAN state: " + e.Err.Error()
}

func (e *PublicationError) Unwrap() error { return e.Err }

type CoordinatorDependencies struct {
	States    aman.AirportStateReader
	Outcomes  aman.CommandOutcomeReader
	Committer aman.StateCommitter
	Publisher FullStatePublisher
	Now       func() time.Time
}

// Coordinator serializes commands per airport in process while retaining the
// repository compare-and-swap as the final authority across processes.
type Coordinator struct {
	states    aman.AirportStateReader
	outcomes  aman.CommandOutcomeReader
	committer aman.StateCommitter
	publisher FullStatePublisher
	now       func() time.Time

	locksMu sync.Mutex
	locks   map[string]*sync.Mutex
}

func NewCoordinator(deps CoordinatorDependencies) (*Coordinator, error) {
	for _, dependency := range []struct {
		name  string
		value any
	}{
		{"state reader", deps.States},
		{"command outcome reader", deps.Outcomes},
		{"state committer", deps.Committer},
		{"full-state publisher", deps.Publisher},
	} {
		if isNilDependency(dependency.value) {
			return nil, fmt.Errorf("AMAN sequence coordinator requires %s", dependency.name)
		}
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Coordinator{
		states: deps.States, outcomes: deps.Outcomes, committer: deps.Committer,
		publisher: deps.Publisher, now: deps.Now, locks: make(map[string]*sync.Mutex),
	}, nil
}

func (*Coordinator) Name() string { return "AMAN sequence coordinator" }

// CurrentState is the restart/reconnect path. It always reads the latest
// persisted replacement state and never depends on prior publication.
func (c *Coordinator) CurrentState(ctx context.Context, airport string) (aman.AirportState, error) {
	if err := validateAirport(airport); err != nil {
		return aman.AirportState{}, err
	}
	return c.states.LoadAirportState(ctx, airport)
}

// ExecuteCommand coordinates one already-authorized typed command. Duplicate
// IDs are resolved before policy, and unchanged outcomes are persisted without
// allocating or publishing a revision.
func (c *Coordinator) ExecuteCommand(ctx context.Context, airport string, metadata aman.CommandMetadata, apply CommandMutation) (CommandResult, error) {
	if err := validateCommandRequest(airport, metadata, apply); err != nil {
		return CommandResult{}, err
	}
	lock := c.airportLock(airport)
	lock.Lock()
	defer lock.Unlock()

	if duplicate, ok, err := c.loadDuplicate(ctx, airport, metadata.CommandID); err != nil || ok {
		return duplicate, err
	}

	current, err := c.states.LoadAirportState(ctx, airport)
	if err != nil {
		return CommandResult{}, err
	}
	if metadata.ExpectedRevision != current.Revision {
		return CommandResult{State: current}, revisionConflict(metadata.ExpectedRevision, current.Revision)
	}

	detached, err := cloneState(current)
	if err != nil {
		return CommandResult{}, err
	}
	change, err := apply(detached)
	if err != nil {
		return CommandResult{State: current}, err
	}
	now := c.now().UTC()
	commit, err := prepareCommit(current, metadata, change, now)
	if err != nil {
		return CommandResult{State: current}, err
	}
	committed, err := c.committer.Commit(ctx, commit)
	if err != nil {
		return CommandResult{State: current}, err
	}
	if committed.CommandOutcome == nil {
		return CommandResult{State: committed.State}, errors.New("AMAN sequence commit returned no command outcome")
	}
	result := CommandResult{
		State: committed.State, Outcome: *committed.CommandOutcome,
		Changed: change.Changed && !committed.DuplicateCommand, Duplicate: committed.DuplicateCommand,
	}
	if committed.DuplicateCommand || !change.Changed {
		return result, nil
	}

	// The request may have timed out immediately after commit. Publication is
	// still attempted once, but it is never persisted or retried here.
	if err := c.publisher.PublishAMANState(context.WithoutCancel(ctx), committed.State); err != nil {
		return result, &PublicationError{Err: err}
	}
	return result, nil
}

func (c *Coordinator) loadDuplicate(ctx context.Context, airport, commandID string) (CommandResult, bool, error) {
	outcome, err := c.outcomes.LoadCommandOutcome(ctx, commandID)
	if err != nil {
		if isNotFound(err) {
			return CommandResult{}, false, nil
		}
		return CommandResult{}, false, err
	}
	if outcome.Airport != airport {
		return CommandResult{}, false, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "command ID already belongs to another airport"}
	}
	state, err := c.states.LoadAirportState(ctx, airport)
	if err != nil {
		return CommandResult{}, false, err
	}
	return CommandResult{State: state, Outcome: outcome, Duplicate: true}, true, nil
}

func (c *Coordinator) airportLock(airport string) *sync.Mutex {
	c.locksMu.Lock()
	defer c.locksMu.Unlock()
	lock := c.locks[airport]
	if lock == nil {
		lock = &sync.Mutex{}
		c.locks[airport] = lock
	}
	return lock
}

func prepareCommit(current aman.AirportState, metadata aman.CommandMetadata, change CommandChange, recordedAt time.Time) (aman.StateCommit, error) {
	if change.State.Airport != current.Airport {
		return aman.StateCommit{}, invalidCoordinatorChange("policy changed airport identity")
	}
	if change.State.Revision != current.Revision {
		return aman.StateCommit{}, invalidCoordinatorChange("policy attempted to allocate a sequence revision")
	}
	if !json.Valid(change.Outcome) {
		return aman.StateCommit{}, invalidCoordinatorChange("command outcome must be valid JSON")
	}
	if len(change.Audit) == 0 {
		return aman.StateCommit{}, invalidCoordinatorChange("accepted command requires an audit record")
	}

	next := change.State
	if change.Changed {
		if current.Revision == aman.SequenceRevision(math.MaxUint64) {
			return aman.StateCommit{}, invalidCoordinatorChange("sequence revision is exhausted")
		}
		next.Revision = current.Revision + 1
		next.GeneratedAt = recordedAt
		for index := range next.Flights {
			if next.Flights[index].Slot != nil {
				next.Flights[index].Slot.Revision = next.Revision
			}
			// Offers cannot be carried across a changed airport revision. A
			// supplied calculation below replaces them from the same slot input.
			next.Flights[index].QueueOffers = nil
		}
		if change.QueueOffers != nil {
			projected, err := change.QueueOffers.project(next, next.Revision, recordedAt)
			if err != nil {
				return aman.StateCommit{}, err
			}
			next = projected
		}
	} else {
		if change.QueueOffers != nil {
			return aman.StateCommit{}, invalidCoordinatorChange("unchanged command cannot recompute queue offers")
		}
		equal, err := equalStates(current, next)
		if err != nil {
			return aman.StateCommit{}, err
		}
		if !equal {
			return aman.StateCommit{}, invalidCoordinatorChange("unchanged command modified airport state")
		}
		next = current
	}

	outcome := aman.CommandOutcome{
		CommandID: metadata.CommandID, Airport: current.Airport, Revision: next.Revision,
		Payload: append([]byte(nil), change.Outcome...), RecordedAt: recordedAt,
	}
	audits := make([]aman.AuditRecord, 0, len(change.Audit))
	for _, entry := range change.Audit {
		if strings.TrimSpace(entry.Category) == "" || entry.Category != strings.TrimSpace(entry.Category) || !json.Valid(entry.Payload) {
			return aman.StateCommit{}, invalidCoordinatorChange("audit category and JSON payload are required")
		}
		audits = append(audits, aman.AuditRecord{
			Airport: current.Airport, Revision: next.Revision, Category: entry.Category,
			Payload: append([]byte(nil), entry.Payload...), RecordedAt: recordedAt,
		})
	}
	return aman.StateCommit{
		ExpectedRevision: current.Revision, State: next, CommandOutcome: &outcome, AuditRecords: audits,
	}, nil
}

func validateCommandRequest(airport string, metadata aman.CommandMetadata, apply CommandMutation) error {
	if err := validateAirport(airport); err != nil {
		return err
	}
	if metadata.CommandID == "" || metadata.CommandID != strings.TrimSpace(metadata.CommandID) {
		return &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "command ID is required"}
	}
	if apply == nil {
		return &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "typed command mutation is required"}
	}
	return nil
}

func validateAirport(airport string) error {
	if airport == "" || airport != strings.TrimSpace(airport) {
		return &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "airport is required"}
	}
	return nil
}

func revisionConflict(expected, current aman.SequenceRevision) error {
	return &aman.DomainError{Class: aman.ErrorRevisionConflict, Message: fmt.Sprintf("expected revision %d does not match current revision %d", expected, current)}
}

func invalidCoordinatorChange(message string) error {
	return &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "sequence coordinator: " + message}
}

func isNotFound(err error) bool {
	var domain *aman.DomainError
	return errors.As(err, &domain) && domain.Class == aman.ErrorNotFound
}

func cloneState(state aman.AirportState) (aman.AirportState, error) {
	encoded, err := json.Marshal(state)
	if err != nil {
		return aman.AirportState{}, fmt.Errorf("clone AMAN airport state: %w", err)
	}
	var cloned aman.AirportState
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return aman.AirportState{}, fmt.Errorf("clone AMAN airport state: %w", err)
	}
	return cloned, nil
}

func equalStates(left, right aman.AirportState) (bool, error) {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false, fmt.Errorf("compare AMAN airport state: %w", err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false, fmt.Errorf("compare AMAN airport state: %w", err)
	}
	return string(leftJSON) == string(rightJSON), nil
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var _ aman.Component = (*Coordinator)(nil)
