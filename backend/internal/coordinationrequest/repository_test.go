package coordinationrequest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/pdc/testdata"
	"github.com/stretchr/testify/require"
)

func TestRepositoryRestartReplayIsDeterministicAndAdditive(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)

	later := routeRequest(t, "command-b", testTime.Add(time.Minute))
	earlier := routeRequest(t, "command-a", testTime)
	require.NoError(t, repository.Save(ctx, later))
	require.NoError(t, repository.Save(ctx, earlier))

	_, err := pool.Exec(ctx, `UPDATE aman_coordination_requests
        SET payload = payload || '{"future_field":{"enabled":true}}'::jsonb WHERE request_id = $1`, earlier.ID)
	require.NoError(t, err)

	replayed, err := NewRepository(pool).ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, []RequestID{earlier.ID, later.ID}, []RequestID{replayed[0].ID, replayed[1].ID})
	require.Equal(t, earlier, replayed[0])

	accepted, err := earlier.Decide("accept-a", "7654321", "EKCH_APP", "EKCH_APP", StateAccepted, "", testTime.Add(2*time.Minute))
	require.NoError(t, err)
	require.NoError(t, NewRepository(pool).Save(ctx, accepted))
	replayed, err = NewRepository(pool).ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, StateAccepted, replayed[0].State)

	duplicate := routeRequest(t, "command-a", testTime.Add(3*time.Minute))
	require.Equal(t, earlier.ID, duplicate.ID)
	require.NoError(t, NewRepository(pool).Save(ctx, accepted), "exact retry remains idempotent after restart")
}

func TestSubmitSupersedesSameKindButKeepsKindsIndependent(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)

	first := routeRequest(t, "route-1", testTime)
	created, err := repository.Submit(ctx, first, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(1), created.Revision)

	speed, err := New("speed-1", "EKCH", "flight-1", "EKCH_APP", "1234567", "EKCH_FMH", KindSpeed,
		Payload{Speed: &SpeedPayload{Requested: "220 KT"}}, testTime.Add(time.Minute))
	require.NoError(t, err)
	independent, err := repository.Submit(ctx, speed, 1)
	require.NoError(t, err)
	require.Nil(t, independent.SupersededRequest)

	replacement := routeRequest(t, "route-2", testTime.Add(2*time.Minute))
	replaced, err := repository.Submit(ctx, replacement, 2)
	require.NoError(t, err)
	require.Equal(t, first.ID, *replaced.Request.Supersedes)
	require.Equal(t, replacement.ID, *replaced.SupersededRequest.SupersededBy)

	replayed, err := NewRepository(pool).ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Len(t, replayed, 3)
	require.Equal(t, StateSuperseded, replayed[0].State)
	require.Equal(t, StatePending, replayed[1].State, "speed remains independent")
	require.Equal(t, StatePending, replayed[2].State)
}

func TestSubmitRejectsStaleRevisionWithoutPartialSupersede(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	first := routeRequest(t, "route-1", testTime)
	_, err := repository.Submit(ctx, first, 0)
	require.NoError(t, err)

	_, err = repository.Submit(ctx, routeRequest(t, "route-stale", testTime.Add(time.Minute)), 0)
	require.ErrorIs(t, err, ErrRevisionConflict)
	replayed, err := repository.ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, []Request{first}, replayed)
}

func TestSubmitRetryIsIdempotentAcrossRepositoryRestart(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	request := routeRequest(t, "route-retry", testTime)
	first, err := NewRepository(pool).Submit(ctx, request, 0)
	require.NoError(t, err)

	retry := request
	retry.CreatedAt, retry.UpdatedAt = testTime.Add(time.Minute), testTime.Add(time.Minute)
	duplicate, err := NewRepository(pool).Submit(ctx, retry, 0)
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	require.Equal(t, first.Request, duplicate.Request)
	require.Equal(t, uint64(1), duplicate.Revision)

	replayed, err := NewRepository(pool).ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, []Request{request}, replayed)
}

func TestDecideIsAuditedRevisionCheckedAndIdempotentAcrossRestart(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	request := routeRequest(t, "route-decision", testTime)
	_, err := repository.Submit(ctx, request, 0)
	require.NoError(t, err)

	decision := Decision{CommandID: "accept-1", Airport: "EKCH", Actor: "7654321", Role: "EKCH_APP",
		AuthoritativeRecipient: "EKCH_APP", RequestID: request.ID, RequestKind: request.Kind,
		BeforeState: StatePending, AfterState: StateAccepted, ReceivedAt: testTime.Add(time.Minute)}
	accepted, err := repository.Decide(ctx, request.ID, decision, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(2), accepted.Revision)
	require.Equal(t, StateAccepted, accepted.Request.State)
	require.Equal(t, request.Payload, accepted.Request.Payload, "acceptance must not alter route or prediction inputs")
	require.Equal(t, &decision, accepted.Request.Decision)

	retried, err := repository.Decide(ctx, request.ID, decision, 1)
	require.NoError(t, err)
	require.True(t, retried.Duplicate)
	require.Equal(t, uint64(2), retried.Revision)
	retried, err = NewRepository(pool).Decide(ctx, request.ID, decision, 1)
	require.NoError(t, err)
	require.True(t, retried.Duplicate)
	replayed, err := NewRepository(pool).ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Len(t, replayed, 1)
	require.Equal(t, &decision, replayed[0].Decision, "retry must not add another transition or audit")

	_, err = repository.Decide(ctx, request.ID, Decision{CommandID: "second-decision", Airport: "EKCH", Actor: "7654321", Role: "EKCH_APP",
		AuthoritativeRecipient: "EKCH_APP", RequestID: request.ID, RequestKind: request.Kind,
		BeforeState: StatePending, AfterState: StateRejected, Reason: "unable", ReceivedAt: testTime.Add(2 * time.Minute)}, 2)
	require.ErrorIs(t, err, ErrInvalidState)
}

func TestDecideRejectsStaleRevisionWrongRecipientAndRecordsReason(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	request := routeRequest(t, "route-reject", testTime)
	_, err := repository.Submit(ctx, request, 0)
	require.NoError(t, err)
	decision := Decision{CommandID: "reject-1", Airport: "EKCH", Actor: "7654321", Role: "EKCH_APP",
		AuthoritativeRecipient: "EKCH_APP", RequestID: request.ID, RequestKind: request.Kind,
		BeforeState: StatePending, AfterState: StateRejected, Reason: "traffic", ReceivedAt: testTime.Add(time.Minute)}

	_, err = repository.Decide(ctx, request.ID, decision, 0)
	require.ErrorIs(t, err, ErrRevisionConflict)
	wrong := decision
	wrong.CommandID, wrong.AuthoritativeRecipient = "wrong-owner", "EKCH_DEP"
	_, err = repository.Decide(ctx, request.ID, wrong, 1)
	require.ErrorIs(t, err, ErrWrongRecipient)

	rejected, err := repository.Decide(ctx, request.ID, decision, 1)
	require.NoError(t, err)
	require.Equal(t, StateRejected, rejected.Request.State)
	require.Equal(t, "traffic", rejected.Request.Decision.Reason)
}

func TestDecideRejectsSupersededAndExpiredRequestsWithoutMutation(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	superseded, err := routeRequest(t, "superseded", testTime).Supersede("replacement", testTime.Add(time.Minute))
	require.NoError(t, err)
	expired, err := routeRequest(t, "expired", testTime).Expire(Expiry{FactID: "aman/EKCH/2/flight-1/flight_completed", FactRevision: 2, Reason: ExpiryFlightCompleted, ExpiredAt: testTime.Add(time.Minute)})
	require.NoError(t, err)
	require.NoError(t, repository.Save(ctx, superseded))
	require.NoError(t, repository.Save(ctx, expired))

	for _, request := range []Request{superseded, expired} {
		decision := Decision{CommandID: "decide-" + request.CommandID, Airport: "EKCH", Actor: "7654321", Role: "EKCH_APP",
			AuthoritativeRecipient: "EKCH_APP", RequestID: request.ID, RequestKind: request.Kind,
			BeforeState: StatePending, AfterState: StateAccepted, ReceivedAt: testTime.Add(2 * time.Minute)}
		_, err = repository.Decide(ctx, request.ID, decision, 3)
		require.ErrorIs(t, err, ErrInvalidState)
	}
	replayed, err := repository.ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, []Request{expired, superseded}, replayed)
}

func TestConcurrentSubmissionsCommitAtomically(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	requests := []Request{
		routeRequest(t, "concurrent-a", testTime),
		routeRequest(t, "concurrent-b", testTime.Add(time.Second)),
	}
	start := make(chan struct{})
	errs := make([]error, len(requests))
	var wait sync.WaitGroup
	for index := range requests {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, errs[index] = repository.Submit(ctx, requests[index], 0)
		}(index)
	}
	close(start)
	wait.Wait()
	require.Equal(t, 1, boolCount(errs[0] == nil, errs[1] == nil))
	require.Equal(t, 1, boolCount(errors.Is(errs[0], ErrRevisionConflict), errors.Is(errs[1], ErrRevisionConflict)))
	replayed, err := repository.ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Len(t, replayed, 1)
	require.Equal(t, StatePending, replayed[0].State)
}

func TestTransferPendingIsAtomicTerminalSafeAndReplayIdempotent(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	route := routeRequest(t, "route-transfer", testTime)
	speed, err := New("speed-transfer", "EKCH", "flight-1", "EKCH_APP", "1234567", "EKCH_FMH", KindSpeed,
		Payload{Speed: &SpeedPayload{Requested: "220 KT"}}, testTime.Add(time.Second))
	require.NoError(t, err)
	accepted, err := routeRequest(t, "accepted", testTime).Decide("accept-terminal", "7654321", "EKCH_APP", "EKCH_APP", StateAccepted, "", testTime.Add(time.Second))
	require.NoError(t, err)
	rejected, err := routeRequest(t, "rejected", testTime).Decide("reject-terminal", "7654321", "EKCH_APP", "EKCH_APP", StateRejected, "traffic", testTime.Add(time.Second))
	require.NoError(t, err)
	superseded, err := routeRequest(t, "superseded-terminal", testTime).Supersede("replacement", testTime.Add(time.Second))
	require.NoError(t, err)
	expired, err := routeRequest(t, "expired", testTime).Expire(Expiry{FactID: "aman/EKCH/2/flight-1/flight_completed", FactRevision: 2, Reason: ExpiryFlightCompleted, ExpiredAt: testTime.Add(time.Second)})
	require.NoError(t, err)
	terminal := []Request{accepted, rejected, superseded, expired}
	for _, request := range append([]Request{route, speed}, terminal...) {
		require.NoError(t, repository.Save(ctx, request))
	}

	fact := OwnershipFact{Airport: "EKCH", FlightID: "flight-1", FactID: "es/17", Revision: 17,
		Owner: "EKCH_DEP", ObservedAt: testTime.Add(time.Minute)}
	result, err := repository.TransferPending(ctx, fact)
	require.NoError(t, err)
	require.Len(t, result.Requests, 2)
	require.Equal(t, uint64(10), result.Revision, "six requests, two decisions, one expiry, and one ownership fact")
	for _, request := range result.Requests {
		require.Equal(t, ControllerID("EKCH_DEP"), request.RecipientController)
		require.Equal(t, StatePending, request.State)
		require.Len(t, request.RecipientTransfers, 1)
	}

	retry, err := NewRepository(pool).TransferPending(ctx, fact)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.Equal(t, result.Revision, retry.Revision)
	require.Len(t, retry.Requests, 2)
	replayed, err := NewRepository(pool).ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	for _, want := range terminal {
		for _, request := range replayed {
			if request.ID == want.ID {
				require.Equal(t, want.State, request.State)
				require.Empty(t, request.RecipientTransfers, "terminal requests never transfer")
			}
		}
	}

	noOwner := OwnershipFact{Airport: "EKCH", FlightID: "flight-1", FactID: "es/18", Revision: 18,
		ObservedAt: testTime.Add(2 * time.Minute)}
	unassigned, err := repository.TransferPending(ctx, noOwner)
	require.NoError(t, err)
	require.Len(t, unassigned.Requests, 2)
	for _, request := range unassigned.Requests {
		require.Equal(t, RecipientUnassigned, request.RecipientStatus)
		require.Empty(t, request.RecipientController)
	}
}

func TestTransferPendingRollsBackEveryRequestWhenPersistenceFails(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	route := routeRequest(t, "atomic-route", testTime)
	speed, err := New("atomic-speed", "EKCH", "flight-1", "EKCH_APP", "1234567", "EKCH_FMH", KindSpeed,
		Payload{Speed: &SpeedPayload{Requested: "220 KT"}}, testTime.Add(time.Second))
	require.NoError(t, err)
	require.NoError(t, repository.Save(ctx, route))
	require.NoError(t, repository.Save(ctx, speed))
	_, err = pool.Exec(ctx, `CREATE FUNCTION fail_speed_transfer() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.request_id = 'coordination-request/atomic-speed' THEN RAISE EXCEPTION 'forced failure'; END IF; RETURN NEW; END $$;
CREATE TRIGGER fail_speed_transfer BEFORE UPDATE ON aman_coordination_requests FOR EACH ROW EXECUTE FUNCTION fail_speed_transfer()`)
	require.NoError(t, err)

	_, err = repository.TransferPending(ctx, OwnershipFact{Airport: "EKCH", FlightID: "flight-1", FactID: "es/atomic",
		Revision: 2, Owner: "EKCH_DEP", ObservedAt: testTime.Add(time.Minute)})
	require.ErrorContains(t, err, "forced failure")
	replayed, err := repository.ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, []Request{route, speed}, replayed)
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}
