package sequence_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestCoordinatorSerializesSameAirportAndRepositoryCASRemainsFinalAuthority(t *testing.T) {
	repository := newCoordinatorRepository(coordinatorState(4))
	first := newTestCoordinator(t, repository, &recordingPublisher{repository: repository})
	second := newTestCoordinator(t, repository, &recordingPublisher{repository: repository})
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	mutation := func(state aman.AirportState) (sequence.CommandChange, error) {
		ready <- struct{}{}
		<-release
		state.Flights[0].Order = intPointer(2)
		return commandChange(state, true), nil
	}

	errorsSeen := make(chan error, 2)
	for _, coordinator := range []*sequence.Coordinator{first, second} {
		go func(coordinator *sequence.Coordinator) {
			_, err := coordinator.ExecuteCommand(context.Background(), "EKCH", aman.CommandMetadata{CommandID: fmt.Sprintf("race-%p", coordinator), ExpectedRevision: 4}, mutation)
			errorsSeen <- err
		}(coordinator)
	}
	<-ready
	<-ready
	close(release)

	var successes, conflicts int
	for range 2 {
		err := <-errorsSeen
		if err == nil {
			successes++
			continue
		}
		requireDomainError(t, err, aman.ErrorRevisionConflict)
		conflicts++
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	require.Equal(t, aman.SequenceRevision(5), repository.current().Revision)
}

func TestCoordinatorInProcessSerializationRejectsStaleCommandBeforePolicy(t *testing.T) {
	repository := newCoordinatorRepository(coordinatorState(4))
	coordinator := newTestCoordinator(t, repository, &recordingPublisher{repository: repository})
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var stalePolicyCalls atomic.Int32

	firstResult := make(chan error, 1)
	go func() {
		_, err := coordinator.ExecuteCommand(context.Background(), "EKCH", aman.CommandMetadata{CommandID: "first", ExpectedRevision: 4}, func(state aman.AirportState) (sequence.CommandChange, error) {
			close(firstStarted)
			<-releaseFirst
			state.Flights[0].Order = intPointer(2)
			return commandChange(state, true), nil
		})
		firstResult <- err
	}()
	<-firstStarted

	staleResult := make(chan error, 1)
	go func() {
		_, err := coordinator.ExecuteCommand(context.Background(), "EKCH", aman.CommandMetadata{CommandID: "stale", ExpectedRevision: 4}, func(state aman.AirportState) (sequence.CommandChange, error) {
			stalePolicyCalls.Add(1)
			return commandChange(state, true), nil
		})
		staleResult <- err
	}()
	close(releaseFirst)
	require.NoError(t, <-firstResult)
	requireDomainError(t, <-staleResult, aman.ErrorRevisionConflict)
	require.Zero(t, stalePolicyCalls.Load())
}

func TestCoordinatorDuplicateIsIdempotentBeforeAndAfterRestart(t *testing.T) {
	repository := newCoordinatorRepository(coordinatorState(7))
	publisher := &recordingPublisher{repository: repository}
	coordinator := newTestCoordinator(t, repository, publisher)
	metadata := aman.CommandMetadata{CommandID: "move-1", ExpectedRevision: 7}
	var policyCalls atomic.Int32
	mutation := func(state aman.AirportState) (sequence.CommandChange, error) {
		policyCalls.Add(1)
		state.Flights[0].Order = intPointer(2)
		return commandChange(state, true), nil
	}

	first, err := coordinator.ExecuteCommand(context.Background(), "EKCH", metadata, mutation)
	require.NoError(t, err)
	require.True(t, first.Changed)
	require.False(t, first.Duplicate)
	require.Equal(t, aman.SequenceRevision(8), first.State.Revision)

	duplicate, err := coordinator.ExecuteCommand(context.Background(), "EKCH", metadata, mutation)
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	require.False(t, duplicate.Changed)
	require.Equal(t, first.Outcome, duplicate.Outcome)

	restarted := newTestCoordinator(t, repository, publisher)
	afterRestart, err := restarted.ExecuteCommand(context.Background(), "EKCH", metadata, mutation)
	require.NoError(t, err)
	require.True(t, afterRestart.Duplicate)
	require.Equal(t, int32(1), policyCalls.Load(), "stored duplicates must not re-run policy")
	require.Equal(t, 1, publisher.calls())
}

func TestCoordinatorPersistsNoOpOutcomeWithoutRevisionOrPublication(t *testing.T) {
	repository := newCoordinatorRepository(coordinatorState(11))
	publisher := &recordingPublisher{repository: repository}
	coordinator := newTestCoordinator(t, repository, publisher)
	metadata := aman.CommandMetadata{CommandID: "same-rate", ExpectedRevision: 11}

	result, err := coordinator.ExecuteCommand(context.Background(), "EKCH", metadata, func(state aman.AirportState) (sequence.CommandChange, error) {
		return commandChange(state, false), nil
	})
	require.NoError(t, err)
	require.False(t, result.Changed)
	require.Equal(t, aman.SequenceRevision(11), result.State.Revision)
	require.Equal(t, aman.SequenceRevision(11), result.Outcome.Revision)
	require.Equal(t, 0, publisher.calls())

	restarted := newTestCoordinator(t, repository, publisher)
	duplicate, err := restarted.ExecuteCommand(context.Background(), "EKCH", metadata, func(aman.AirportState) (sequence.CommandChange, error) {
		t.Fatal("durable no-op outcome did not survive restart")
		return sequence.CommandChange{}, nil
	})
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	require.Equal(t, aman.SequenceRevision(11), duplicate.State.Revision)
	require.Len(t, repository.auditRecords(), 1)
}

func TestCoordinatorTransactionFailureNeverPublishesOrExposesPartialState(t *testing.T) {
	repository := newCoordinatorRepository(coordinatorState(2))
	repository.commitError = errors.New("injected transaction failure")
	publisher := &recordingPublisher{repository: repository}
	coordinator := newTestCoordinator(t, repository, publisher)

	_, err := coordinator.ExecuteCommand(context.Background(), "EKCH", aman.CommandMetadata{CommandID: "failed", ExpectedRevision: 2}, changedOrder(2))
	require.ErrorContains(t, err, "injected transaction failure")
	require.Equal(t, aman.SequenceRevision(2), repository.current().Revision)
	require.Empty(t, repository.outcomeRecords())
	require.Empty(t, repository.auditRecords())
	require.Equal(t, 0, publisher.calls())
}

func TestCoordinatorPublicationFailureLeavesCommittedStateRecoverableOnReconnect(t *testing.T) {
	repository := newCoordinatorRepository(coordinatorState(3))
	publisher := &recordingPublisher{repository: repository, err: errors.New("frontend unavailable")}
	coordinator := newTestCoordinator(t, repository, publisher)
	metadata := aman.CommandMetadata{CommandID: "committed-before-send", ExpectedRevision: 3}

	result, err := coordinator.ExecuteCommand(context.Background(), "EKCH", metadata, changedOrder(2))
	var publicationError *sequence.PublicationError
	require.ErrorAs(t, err, &publicationError)
	require.True(t, result.Changed)
	require.Equal(t, aman.SequenceRevision(4), result.State.Revision)
	require.Equal(t, aman.SequenceRevision(4), repository.current().Revision)
	require.Equal(t, 1, publisher.calls())
	require.Equal(t, []string{"commit:4", "publish:4"}, repository.stepsCopy(), "publication must follow storage")

	restarted := newTestCoordinator(t, repository, &recordingPublisher{repository: repository})
	reconnected, err := restarted.CurrentState(context.Background(), "EKCH")
	require.NoError(t, err)
	require.Equal(t, result.State, reconnected)

	duplicate, err := restarted.ExecuteCommand(context.Background(), "EKCH", metadata, func(aman.AirportState) (sequence.CommandChange, error) {
		t.Fatal("retry after a post-commit timeout must use the stored outcome")
		return sequence.CommandChange{}, nil
	})
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
}

func TestCoordinatorIsTheOnlySequenceRevisionWriter(t *testing.T) {
	repository := newCoordinatorRepository(coordinatorState(5))
	coordinator := newTestCoordinator(t, repository, &recordingPublisher{repository: repository})

	_, err := coordinator.ExecuteCommand(context.Background(), "EKCH", aman.CommandMetadata{CommandID: "second-writer", ExpectedRevision: 5}, func(state aman.AirportState) (sequence.CommandChange, error) {
		state.Revision++
		return commandChange(state, true), nil
	})
	requireDomainError(t, err, aman.ErrorInvalidArgument)
	require.ErrorContains(t, err, "attempted to allocate")
	require.Equal(t, aman.SequenceRevision(5), repository.current().Revision)
	require.Empty(t, repository.outcomeRecords())
}

func TestCoordinatorRejectsUnchangedMutationThatModifiesState(t *testing.T) {
	repository := newCoordinatorRepository(coordinatorState(9))
	coordinator := newTestCoordinator(t, repository, &recordingPublisher{repository: repository})

	_, err := coordinator.ExecuteCommand(context.Background(), "EKCH", aman.CommandMetadata{CommandID: "false-noop", ExpectedRevision: 9}, func(state aman.AirportState) (sequence.CommandChange, error) {
		state.Flights[0].Order = intPointer(9)
		return commandChange(state, false), nil
	})
	requireDomainError(t, err, aman.ErrorInvalidArgument)
	require.Equal(t, aman.SequenceRevision(9), repository.current().Revision)
}

func TestCoordinatorPublishesAfterCommitEvenWhenRequestContextWasCancelled(t *testing.T) {
	repository := newCoordinatorRepository(coordinatorState(1))
	publisher := &recordingPublisher{repository: repository, requireLiveContext: true}
	coordinator := newTestCoordinator(t, repository, publisher)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := coordinator.ExecuteCommand(ctx, "EKCH", aman.CommandMetadata{CommandID: "timed-out", ExpectedRevision: 1}, changedOrder(2))
	require.NoError(t, err)
	require.Equal(t, aman.SequenceRevision(2), result.State.Revision)
	require.Equal(t, 1, publisher.calls())
}

func newTestCoordinator(t *testing.T, repository *coordinatorRepository, publisher sequence.FullStatePublisher) *sequence.Coordinator {
	t.Helper()
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{
		States: repository, Outcomes: repository, Committer: repository, Publisher: publisher,
		Now: func() time.Time { return testTime().Add(time.Hour) },
	})
	require.NoError(t, err)
	return coordinator
}

func changedOrder(order int) sequence.CommandMutation {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		state.Flights[0].Order = intPointer(order)
		return commandChange(state, true), nil
	}
}

func commandChange(state aman.AirportState, changed bool) sequence.CommandChange {
	return sequence.CommandChange{
		State: state, Changed: changed, Outcome: json.RawMessage(`{"status":"accepted"}`),
		Audit: []sequence.AuditEntry{{Category: "sequence_command_accepted", Payload: json.RawMessage(`{"status":"accepted"}`)}},
	}
}

func coordinatorState(revision aman.SequenceRevision) aman.AirportState {
	at := testTime().Add(time.Duration(revision) * time.Minute)
	return aman.AirportState{
		Airport: "EKCH", Revision: revision, GeneratedAt: at, PolicyVersion: "policy-v1", Mode: aman.ModeAuthoritative, Authoritative: true,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: "A"}},
		Flights: []aman.AMANFlight{{
			ID: "flight-1", VATSIMCID: "123", CurrentCallsign: "SAS123", State: aman.StateStable, DataStatus: aman.DataFresh,
			FreezeReason: aman.FreezeNone, Slot: &aman.Slot{Time: at.Add(10 * time.Minute), RunwayGroupID: "A", Sequence: 1, Revision: revision, Reason: "rate_wtc"},
			Order: intPointer(1), UpdatedAt: at,
		}},
	}
}

type coordinatorRepository struct {
	mu          sync.Mutex
	state       aman.AirportState
	outcomes    map[string]aman.CommandOutcome
	audits      []aman.AuditRecord
	steps       []string
	commitError error
}

func newCoordinatorRepository(state aman.AirportState) *coordinatorRepository {
	return &coordinatorRepository{state: state, outcomes: make(map[string]aman.CommandOutcome)}
}

func (r *coordinatorRepository) LoadAirportState(_ context.Context, airport string) (aman.AirportState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state.Airport != airport {
		return aman.AirportState{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "airport not found"}
	}
	return cloneCoordinatorState(r.state), nil
}

func (r *coordinatorRepository) LoadCommandOutcome(_ context.Context, commandID string) (aman.CommandOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	outcome, ok := r.outcomes[commandID]
	if !ok {
		return aman.CommandOutcome{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "outcome not found"}
	}
	outcome.Payload = append([]byte(nil), outcome.Payload...)
	return outcome, nil
}

func (r *coordinatorRepository) Commit(_ context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if commit.CommandOutcome != nil {
		if outcome, ok := r.outcomes[commit.CommandOutcome.CommandID]; ok {
			copy := outcome
			return aman.CommitResult{State: cloneCoordinatorState(r.state), CommandOutcome: &copy, DuplicateCommand: true}, nil
		}
	}
	if err := commit.Validate(); err != nil {
		return aman.CommitResult{}, err
	}
	if r.state.Revision != commit.ExpectedRevision {
		return aman.CommitResult{}, &aman.DomainError{Class: aman.ErrorRevisionConflict, Message: "CAS conflict"}
	}
	if r.commitError != nil {
		return aman.CommitResult{}, r.commitError
	}
	if commit.State.Revision == commit.ExpectedRevision+1 {
		r.state = cloneCoordinatorState(commit.State)
	}
	if commit.CommandOutcome != nil {
		outcome := *commit.CommandOutcome
		outcome.Payload = append([]byte(nil), outcome.Payload...)
		r.outcomes[outcome.CommandID] = outcome
	}
	r.audits = append(r.audits, commit.AuditRecords...)
	r.steps = append(r.steps, fmt.Sprintf("commit:%d", commit.State.Revision))
	result := aman.CommitResult{State: cloneCoordinatorState(r.state)}
	if commit.CommandOutcome != nil {
		outcome := *commit.CommandOutcome
		result.CommandOutcome = &outcome
	}
	return result, nil
}

func (r *coordinatorRepository) current() aman.AirportState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneCoordinatorState(r.state)
}

func (r *coordinatorRepository) outcomeRecords() map[string]aman.CommandOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := make(map[string]aman.CommandOutcome, len(r.outcomes))
	for key, value := range r.outcomes {
		copy[key] = value
	}
	return copy
}

func (r *coordinatorRepository) auditRecords() []aman.AuditRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]aman.AuditRecord(nil), r.audits...)
}

func (r *coordinatorRepository) stepsCopy() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.steps...)
}

type recordingPublisher struct {
	mu                 sync.Mutex
	repository         *coordinatorRepository
	states             []aman.AirportState
	err                error
	requireLiveContext bool
}

func (p *recordingPublisher) PublishAMANState(ctx context.Context, state aman.AirportState) error {
	if p.requireLiveContext && ctx.Err() != nil {
		return errors.New("publication inherited cancelled request context")
	}
	stored := p.repository.current()
	if stored.Revision != state.Revision {
		return fmt.Errorf("published revision %d before commit %d", state.Revision, stored.Revision)
	}
	p.mu.Lock()
	p.states = append(p.states, cloneCoordinatorState(state))
	p.mu.Unlock()
	p.repository.mu.Lock()
	p.repository.steps = append(p.repository.steps, fmt.Sprintf("publish:%d", state.Revision))
	p.repository.mu.Unlock()
	return p.err
}

func (p *recordingPublisher) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.states)
}

func cloneCoordinatorState(state aman.AirportState) aman.AirportState {
	encoded, err := json.Marshal(state)
	if err != nil {
		panic(err)
	}
	var cloned aman.AirportState
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		panic(err)
	}
	return cloned
}

func intPointer(value int) *int { return &value }

func requireDomainError(t *testing.T, err error, class aman.ErrorClass) {
	t.Helper()
	var domain *aman.DomainError
	require.ErrorAs(t, err, &domain)
	require.Equal(t, class, domain.Class)
}
