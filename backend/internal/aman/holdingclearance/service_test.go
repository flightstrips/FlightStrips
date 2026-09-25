package holdingclearance

import (
	"FlightStrips/internal/aman"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServicePersistsESOnlyClearanceCreateUpdateAndCancelWithoutCID(t *testing.T) {
	now := time.Date(2026, 9, 11, 18, 0, 0, 0, time.UTC)
	repository := &memoryRepository{state: airportState(now)}
	publisher := &publisher{}
	service, err := New(Dependencies{Repository: repository, Publisher: publisher})
	require.NoError(t, err)
	altitude := int32(12000)
	fact := aman.HoldingClearanceFact{
		Callsign: "SAS123", Destination: " ekch ",
		Hold: " olpib ", HoldType: "ENROUTE", HoldEAT: " 1422 ", ClearedAltitude: &altitude, ObservedAt: now,
	}

	require.NoError(t, service.ObserveHoldingClearance(context.Background(), fact))
	require.Equal(t, 1, repository.commits)
	require.Equal(t, 1, publisher.calls)
	require.Equal(t, aman.SequenceRevision(8), repository.state.Revision)
	require.Equal(t, &aman.HoldingClearance{
		Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute, HoldEAT: "1422",
		ClearedAltitude: &altitude, ObservedAt: now,
	}, repository.state.Flights[0].HoldingClearance)
	fact.HoldEAT = "1430"
	fact.ObservedAt = now.Add(30 * time.Second)
	require.NoError(t, service.ObserveHoldingClearance(context.Background(), fact))
	require.Equal(t, "1430", repository.state.Flights[0].HoldingClearance.HoldEAT)

	// Reconstruct both repository and service from persisted JSON. Replaying the
	// same source value must not allocate another aggregate revision.
	restartedRepository := repository.restart(t)
	restarted, err := New(Dependencies{Repository: restartedRepository, Publisher: publisher})
	require.NoError(t, err)
	fact.ObservedAt = now.Add(time.Minute)
	require.NoError(t, restarted.ObserveHoldingClearance(context.Background(), fact))
	require.Zero(t, restartedRepository.commits)

	// A cancellation remains persisted as an authoritative cursor, so an older
	// replay cannot resurrect the clearance after another restart.
	fact.Hold, fact.HoldType, fact.HoldEAT, fact.ClearedAltitude = "", "tsa", "", nil
	fact.ObservedAt = now.Add(2 * time.Minute)
	require.NoError(t, restarted.ObserveHoldingClearance(context.Background(), fact))
	require.Equal(t, &aman.HoldingClearance{ObservedAt: fact.ObservedAt}, restartedRepository.state.Flights[0].HoldingClearance)
	afterCancellation := restartedRepository.restart(t)
	fact.Hold, fact.HoldType, fact.ObservedAt = "OLPIB", "enroute", now
	replayed, err := New(Dependencies{Repository: afterCancellation, Publisher: publisher})
	require.NoError(t, err)
	require.NoError(t, replayed.ObserveHoldingClearance(context.Background(), fact))
	require.Empty(t, afterCancellation.state.Flights[0].HoldingClearance.Hold)
	require.Zero(t, afterCancellation.commits)
}

func TestLegacyFlightJSONWithoutHoldingClearanceRemainsValid(t *testing.T) {
	state := airportState(time.Date(2026, 9, 11, 18, 0, 0, 0, time.UTC))
	payload, err := json.Marshal(state)
	require.NoError(t, err)
	var legacy map[string]any
	require.NoError(t, json.Unmarshal(payload, &legacy))
	flights := legacy["Flights"].([]any)
	delete(flights[0].(map[string]any), "HoldingClearance")
	payload, err = json.Marshal(legacy)
	require.NoError(t, err)

	var restored aman.AirportState
	require.NoError(t, json.Unmarshal(payload, &restored))
	require.Nil(t, restored.Flights[0].HoldingClearance)
	require.NoError(t, restored.Validate())
}

type memoryRepository struct {
	state   aman.AirportState
	commits int
	loads   int
}

func (r *memoryRepository) LoadAirportState(context.Context, string) (aman.AirportState, error) {
	r.loads++
	return cloneState(r.state), nil
}

func TestBatchLoadsCommitsAndPublishesOnceForAllFlights(t *testing.T) {
	now := time.Date(2026, 9, 14, 19, 0, 0, 0, time.UTC)
	state := airportState(now)
	base := state.Flights[0]
	state.Flights = nil
	var facts []aman.HoldingClearanceFact
	for i := 0; i < 100; i++ {
		flight := base
		flight.Callsign = fmt.Sprintf("SAS%d", i)
		state.Flights = append(state.Flights, flight)
		facts = append(facts, aman.HoldingClearanceFact{
			Callsign: flight.Callsign, Destination: "EKCH",
			Hold: "OLPIB", HoldType: "enroute", ObservedAt: now,
		})
	}
	repo := &memoryRepository{state: state}
	pub := &publisher{}
	service, err := New(Dependencies{Repository: repo, Publisher: pub})
	require.NoError(t, err)
	require.NoError(t, service.ObserveHoldingClearances(context.Background(), facts))
	require.Equal(t, 1, repo.loads)
	require.Equal(t, 1, repo.commits)
	require.Equal(t, 1, pub.calls)
	require.Equal(t, aman.SequenceRevision(8), repo.state.Revision)
	for _, flight := range repo.state.Flights {
		require.Equal(t, "OLPIB", flight.HoldingClearance.Hold)
	}
	require.NoError(t, service.ObserveHoldingClearances(context.Background(), facts))
	require.Equal(t, 2, repo.loads)
	require.Equal(t, 1, repo.commits, "an unchanged full sync must not republish")
	require.Equal(t, 1, pub.calls)
}

func TestBatchRetryPreservesNewerConcurrentClearanceAndAppliesOtherFacts(t *testing.T) {
	now := time.Date(2026, 9, 14, 19, 0, 0, 0, time.UTC)
	state := airportState(now)
	second := state.Flights[0]
	second.Callsign = "SAS456"
	state.Flights = append(state.Flights, second)
	repo := &conflictingRepository{memoryRepository: memoryRepository{state: state}, now: now}
	pub := &publisher{}
	service, err := New(Dependencies{Repository: repo, Publisher: pub})
	require.NoError(t, err)
	facts := []aman.HoldingClearanceFact{
		{Callsign: "SAS123", Destination: "EKCH", Hold: "OLPIB", HoldType: "enroute", ObservedAt: now},
		{Callsign: "SAS456", Destination: "EKCH", Hold: "OLPIB", HoldType: "enroute", ObservedAt: now},
		{Callsign: "missing", Destination: "EKCH", Hold: "OLPIB", ObservedAt: now},
	}
	require.NoError(t, service.ObserveHoldingClearances(context.Background(), facts))
	require.Equal(t, 2, repo.loads)
	require.Equal(t, 2, repo.attempts)
	require.Equal(t, 1, repo.commits)
	require.Equal(t, 1, pub.calls)
	require.Equal(t, aman.SequenceRevision(9), repo.state.Revision)
	require.Equal(t, "OLPIB", repo.state.Flights[0].HoldingClearance.Hold)
	require.Empty(t, repo.state.Flights[1].HoldingClearance.Hold, "a newer cancellation must survive the retry")
}

type conflictingRepository struct {
	memoryRepository
	now      time.Time
	attempts int
}

func (r *conflictingRepository) Commit(ctx context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	r.attempts++
	if r.attempts == 1 {
		r.state.Revision++
		r.state.Flights[1].HoldingClearance = &aman.HoldingClearance{ObservedAt: r.now.Add(time.Minute)}
		return aman.CommitResult{}, &aman.DomainError{Class: aman.ErrorRevisionConflict}
	}
	return r.memoryRepository.Commit(ctx, commit)
}

func TestBatchRejectsInvalidFactsBeforeWritingAnyAirport(t *testing.T) {
	now := time.Date(2026, 9, 14, 19, 0, 0, 0, time.UTC)
	repo := &memoryRepository{state: airportState(now)}
	pub := &publisher{}
	service, err := New(Dependencies{Repository: repo, Publisher: pub})
	require.NoError(t, err)
	err = service.ObserveHoldingClearances(context.Background(), []aman.HoldingClearanceFact{
		{Callsign: "flight-1", Destination: "EKCH", Hold: "OLPIB", ObservedAt: now},
		{Callsign: "flight-2", Destination: "ESSA"},
	})
	require.Error(t, err)
	require.Zero(t, repo.loads)
	require.Zero(t, repo.commits)
	require.Zero(t, pub.calls)
}

func (r *memoryRepository) Commit(_ context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	if commit.ExpectedRevision != r.state.Revision {
		return aman.CommitResult{}, &aman.DomainError{Class: aman.ErrorRevisionConflict, Message: "conflict"}
	}
	if err := commit.Validate(); err != nil {
		return aman.CommitResult{}, err
	}
	r.state = cloneState(commit.State)
	r.commits++
	return aman.CommitResult{State: cloneState(r.state)}, nil
}

func (r *memoryRepository) restart(t *testing.T) *memoryRepository {
	t.Helper()
	payload, err := json.Marshal(r.state)
	require.NoError(t, err)
	var state aman.AirportState
	require.NoError(t, json.Unmarshal(payload, &state))
	return &memoryRepository{state: state}
}

type publisher struct{ calls int }

func (p *publisher) PublishAMANState(context.Context, aman.AirportState) error {
	p.calls++
	return nil
}

func airportState(now time.Time) aman.AirportState {
	return aman.AirportState{
		Airport: "EKCH", Revision: 7, GeneratedAt: now.Add(-time.Minute), PolicyVersion: "test", Mode: aman.ModeShadow,
		Flights: []aman.AMANFlight{{
			Callsign: "SAS123", State: aman.StateAirborne,
			DataStatus: aman.DataFresh, FreezeReason: aman.FreezeNone, UpdatedAt: now.Add(-time.Minute),
		}},
	}
}

func cloneState(state aman.AirportState) aman.AirportState {
	payload, _ := json.Marshal(state)
	var cloned aman.AirportState
	_ = json.Unmarshal(payload, &cloned)
	return cloned
}
